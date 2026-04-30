package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	restoreConfigFile string
	restoreSrcDir     = "/backups/in"
	restoreDstDir     = "/opt/apps_docker"
	restoreLogFile    = "/var/log/docker-restore.log"
)

var restoreCmd = &cobra.Command{
	Use:   "restore [container] [container2] ...",
	Short: "Restore Docker application data with rsync",
	Long: `Stop selected Docker containers, restore configured Docker application paths,
fix restored ownership, and restart only the containers that were actually stopped.

When no container is specified, all configured containers are restored.

Examples:
  sudo dackup restore
  sudo dackup restore paperless
  sudo dackup restore paperless adguard`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRestore(args)
	},
}

func init() {
	var err error
	restoreConfigFile, err = defaultDackupConfigPath()
	if err != nil {
		restoreConfigFile = "config.json"
	}

	rootCmd.AddCommand(restoreCmd)

	restoreCmd.Flags().StringVar(&restoreConfigFile, "config-file", restoreConfigFile, "main dackup config file")
	restoreCmd.Flags().StringVar(&restoreSrcDir, "src-dir", restoreSrcDir, "restore source root directory")
	restoreCmd.Flags().StringVar(&restoreDstDir, "dst-dir", restoreDstDir, "restore destination root directory")
	restoreCmd.Flags().StringVar(&restoreLogFile, "log-file", restoreLogFile, "restore log file path")
}

func runRestore(requestedContainers []string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("this command requires root privileges; run it with sudo")
	}

	config, effectiveConfigPath, err := effectiveDackupConfig(restoreConfigFile)
	if err != nil {
		return err
	}

	configs, err := filterConfigsForRestore(config.Containers, requestedContainers)
	if err != nil {
		return err
	}

	if err := restorePreflightChecks(effectiveConfigPath, config, configs); err != nil {
		return err
	}

	containersToStop := restoreContainersToStopFromConfig(configs)

	stoppedContainers, err := restoreStopRunningContainers(containersToStop)
	if err != nil {
		return err
	}

	if err := runConfiguredRestores(configs); err != nil {
		return err
	}

	if err := fixRestoreOwnership(configs, config.User, config.Group); err != nil {
		return err
	}

	if err := restoreStartStoppedContainers(stoppedContainers); err != nil {
		return err
	}

	restoreLogMessage("INFO", "Restore command finished successfully")
	return nil
}

func restorePreflightChecks(effectiveConfigPath string, config dackupConfig, configs []containerConfig) error {
	if _, err := os.Stat(effectiveConfigPath); err != nil {
		return fmt.Errorf("config file not found: %s", effectiveConfigPath)
	}

	if strings.TrimSpace(config.User) == "" {
		return fmt.Errorf("config field %q is required", "user")
	}

	if strings.TrimSpace(config.Group) == "" {
		return fmt.Errorf("config field %q is required", "group")
	}

	srcInfo, err := os.Stat(restoreSrcDir)
	if err != nil || !srcInfo.IsDir() {
		return fmt.Errorf("restore source directory not found: %s", restoreSrcDir)
	}

	dstInfo, err := os.Stat(restoreDstDir)
	if err != nil || !dstInfo.IsDir() {
		return fmt.Errorf("restore destination directory not found: %s", restoreDstDir)
	}

	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker CLI not found; please install Docker")
	}

	if _, err := exec.LookPath("rsync"); err != nil {
		return fmt.Errorf("rsync not found; please install rsync")
	}

	for _, containerConfig := range configs {
		for _, path := range containerConfig.Paths {
			srcPath := restoreSourcePath(path)

			info, err := os.Stat(srcPath)
			if err != nil {
				return fmt.Errorf("configured restore path does not exist for container %s: %s", containerConfig.Container, srcPath)
			}

			if !info.IsDir() {
				return fmt.Errorf("configured restore path is not a directory for container %s: %s", containerConfig.Container, srcPath)
			}
		}
	}

	return nil
}

func filterConfigsForRestore(configs []containerConfig, requestedContainers []string) ([]containerConfig, error) {
	if len(requestedContainers) == 0 {
		return configs, nil
	}

	configByContainer := make(map[string]containerConfig)
	for _, config := range configs {
		configByContainer[config.Container] = config
	}

	selected := make(map[string]bool)

	for _, requestedContainer := range requestedContainers {
		requestedContainer = strings.TrimSpace(requestedContainer)
		if requestedContainer == "" {
			continue
		}

		if _, exists := configByContainer[requestedContainer]; !exists {
			return nil, fmt.Errorf("container %q was not found in the configuration", requestedContainer)
		}

		selectContainerAndContainedForRestore(requestedContainer, configByContainer, selected)
	}

	var filteredConfigs []containerConfig
	for _, config := range configs {
		if selected[config.Container] {
			filteredConfigs = append(filteredConfigs, config)
		}
	}

	if len(filteredConfigs) == 0 {
		return nil, fmt.Errorf("no containers selected for restore")
	}

	return filteredConfigs, nil
}

func selectContainerAndContainedForRestore(
	containerName string,
	configByContainer map[string]containerConfig,
	selected map[string]bool,
) {
	containerName = strings.TrimSpace(containerName)
	if containerName == "" || selected[containerName] {
		return
	}

	config, exists := configByContainer[containerName]
	if !exists {
		return
	}

	selected[containerName] = true

	for _, containedContainer := range config.Contains {
		containedContainer = strings.TrimSpace(containedContainer)
		if containedContainer == "" {
			continue
		}

		selectContainerAndContainedForRestore(containedContainer, configByContainer, selected)
	}
}

func restoreContainersToStopFromConfig(configs []containerConfig) []string {
	seen := make(map[string]bool)
	var containers []string

	for _, config := range configs {
		if !config.ToStop {
			continue
		}

		restoreAddContainer(config.Container, seen, &containers)

		for _, containedContainer := range config.Contains {
			restoreAddContainer(containedContainer, seen, &containers)
		}
	}

	return containers
}

func restoreAddContainer(container string, seen map[string]bool, containers *[]string) {
	container = strings.TrimSpace(container)
	if container == "" || seen[container] {
		return
	}

	seen[container] = true
	*containers = append(*containers, container)
}

func restoreStopRunningContainers(containers []string) ([]string, error) {
	restoreLogMessage("INFO", "Stopping containers before restore ...")

	if len(containers) == 0 {
		restoreLogMessage("WARN", `No containers marked with "to_stop": true; skipping stop step`)
		return nil, nil
	}

	var stoppedContainers []string

	for _, container := range containers {
		running, err := dockerContainerRunning(container)
		if err != nil {
			restoreLogMessage("ERROR", fmt.Sprintf("Failed to inspect container %s: %v", container, err))
			continue
		}

		if !running {
			restoreLogMessage("INFO", fmt.Sprintf("Container %s is not running; it will not be stopped or restarted", container))
			continue
		}

		restoreLogMessage("INFO", fmt.Sprintf("Stopping container: %s", container))

		if dryRun {
			restoreLogMessage("INFO", fmt.Sprintf("[dry-run] Would stop container %s", container))
			stoppedContainers = append(stoppedContainers, container)
			continue
		}

		if err := restoreRunLoggedCommand("docker", "stop", container); err != nil {
			restoreLogMessage("ERROR", fmt.Sprintf("Failed to stop container %s; continuing", container))
			continue
		}

		restoreLogMessage("INFO", fmt.Sprintf("Container %s stopped", container))
		stoppedContainers = append(stoppedContainers, container)
	}

	return stoppedContainers, nil
}

func runConfiguredRestores(configs []containerConfig) error {
	restoreLogMessage("INFO", fmt.Sprintf("Starting configured restores from %s to %s ...", restoreSrcDir, restoreDstDir))

	restoredPaths := make(map[string]bool)

	for _, config := range configs {
		if len(config.Paths) == 0 {
			restoreLogMessage("INFO", fmt.Sprintf("No paths configured for container %s; skipping restore for this entry", config.Container))
			continue
		}

		for _, path := range config.Paths {
			cleanPath := restoreCleanConfiguredPath(path)
			if cleanPath == "" {
				restoreLogMessage("WARN", fmt.Sprintf("Empty path configured for container %s; skipping", config.Container))
				continue
			}

			if restoredPaths[cleanPath] {
				restoreLogMessage("INFO", fmt.Sprintf("Path %s already restored; skipping duplicate", cleanPath))
				continue
			}

			srcPath := restoreSourcePath(cleanPath)
			dstPath := restoreDestinationPath(cleanPath)

			if err := restoreSinglePath(config.Container, srcPath, dstPath); err != nil {
				return err
			}

			restoredPaths[cleanPath] = true
		}
	}

	restoreLogMessage("INFO", "Configured restores completed successfully")
	return nil
}

func restoreSinglePath(container string, srcPath string, dstPath string) error {
	restoreLogMessage("INFO", fmt.Sprintf("Restoring %s for container %s to %s ...", srcPath, container, dstPath))

	if dryRun {
		restoreLogMessage("INFO", fmt.Sprintf("[dry-run] Would create destination directory %s", dstPath))
		restoreLogMessage("INFO", fmt.Sprintf("[dry-run] Would run rsync -a --delete %s/ %s/", srcPath, dstPath))
		return nil
	}

	if err := os.MkdirAll(dstPath, 0o755); err != nil {
		return fmt.Errorf("failed to create restore destination directory %s: %w", dstPath, err)
	}

	src := filepath.Clean(srcPath) + string(os.PathSeparator)
	dst := filepath.Clean(dstPath) + string(os.PathSeparator)

	if err := restoreRunLoggedCommand("rsync", "-a", "--delete", src, dst); err != nil {
		return fmt.Errorf("restore rsync failed for %s; see %s for details: %w", srcPath, restoreLogFile, err)
	}

	restoreLogMessage("INFO", fmt.Sprintf("Restore completed for %s", srcPath))
	return nil
}

func fixRestoreOwnership(configs []containerConfig, owner string, group string) error {
	restoreLogMessage("INFO", fmt.Sprintf("Setting ownership of restored paths to %s:%s ...", owner, group))

	changedPaths := make(map[string]bool)

	for _, config := range configs {
		for _, path := range config.Paths {
			cleanPath := restoreCleanConfiguredPath(path)
			if cleanPath == "" || changedPaths[cleanPath] {
				continue
			}

			dstPath := restoreDestinationPath(cleanPath)

			if dryRun {
				restoreLogMessage("INFO", fmt.Sprintf("[dry-run] Would run chown -R %s:%s %s", owner, group, dstPath))
				changedPaths[cleanPath] = true
				continue
			}

			if err := restoreRunLoggedCommand("chown", "-R", fmt.Sprintf("%s:%s", owner, group), dstPath); err != nil {
				return fmt.Errorf("chown failed for %s; see %s for details: %w", dstPath, restoreLogFile, err)
			}

			changedPaths[cleanPath] = true
		}
	}

	restoreLogMessage("INFO", "Restore ownership set correctly")
	return nil
}

func restoreStartStoppedContainers(stoppedContainers []string) error {
	restoreLogMessage("INFO", "Starting previously stopped containers ...")

	if len(stoppedContainers) == 0 {
		restoreLogMessage("INFO", "No containers were stopped by restore; nothing to restart")
		return nil
	}

	for _, container := range stoppedContainers {
		exists, err := dockerContainerExists(container)
		if err != nil {
			restoreLogMessage("ERROR", fmt.Sprintf("Failed to inspect container %s: %v", container, err))
			continue
		}

		if !exists {
			restoreLogMessage("WARN", fmt.Sprintf("Container %s does not exist on this host; skipping", container))
			continue
		}

		restoreLogMessage("INFO", fmt.Sprintf("Starting container: %s", container))

		if dryRun {
			restoreLogMessage("INFO", fmt.Sprintf("[dry-run] Would start container %s", container))
			continue
		}

		if err := restoreRunLoggedCommand("docker", "start", container); err != nil {
			restoreLogMessage("ERROR", fmt.Sprintf("Failed to start container %s; check manually", container))
			continue
		}

		restoreLogMessage("INFO", fmt.Sprintf("Container %s started", container))
	}

	return nil
}

func restoreSourcePath(configuredPath string) string {
	cleanPath := restoreCleanConfiguredPath(configuredPath)
	return filepath.Join(restoreSrcDir, cleanPath)
}

func restoreDestinationPath(configuredPath string) string {
	cleanPath := restoreCleanConfiguredPath(configuredPath)
	return filepath.Join(restoreDstDir, cleanPath)
}

func restoreCleanConfiguredPath(configuredPath string) string {
	return strings.TrimPrefix(filepath.Clean(configuredPath), string(os.PathSeparator))
}

func restoreRunLoggedCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)

	logFile, err := os.OpenFile(restoreLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open restore log file: %w", err)
	}
	defer logFile.Close()

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if verbose {
		fmt.Printf("Running: %s %s\n", name, strings.Join(args, " "))
	}

	return cmd.Run()
}

func restoreLogMessage(level string, message string) {
	previousBackupLogFile := backupLogFile
	backupLogFile = restoreLogFile
	logMessage(level, message)
	backupLogFile = previousBackupLogFile
}
