package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type containerConfig struct {
	Container string   `json:"container"`
	ToStop    bool     `json:"to_stop"`
	Paths     []string `json:"paths,omitempty"`
	Contains  []string `json:"contains,omitempty"`
}

var (
	backupJSONFile string
	backupSrcDir   = "/opt/apps_docker"
	backupDstDir   = "/backups/in"
	backupLogFile  = "/var/log/docker-backup.log"
)

var backupCmd = &cobra.Command{
	Use:   "backup [container] [container2] ...",
	Short: "Back up Docker application data with rsync",
	Long: `Stop selected Docker containers, back up configured Docker application paths,
fix backup ownership, and restart only the containers that were actually stopped.

When no container is specified, all configured containers are backed up.

Examples:
  sudo dackup backup
  sudo dackup backup paperless
  sudo dackup backup paperless adguard`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBackup(
			args,
			cmd.Flags().Changed("src-dir"),
			cmd.Flags().Changed("dst-dir"),
		)
	},
}

func init() {
	var err error
	backupJSONFile, err = defaultDackupConfigPath()
	if err != nil {
		backupJSONFile = "config.json"
	}

	rootCmd.AddCommand(backupCmd)

	backupCmd.Flags().StringVar(&backupJSONFile, "config-file", backupJSONFile, "main dackup config file")
	backupCmd.Flags().StringVar(&backupSrcDir, "src-dir", backupSrcDir, "source root directory")
	backupCmd.Flags().StringVar(&backupDstDir, "dst-dir", backupDstDir, "destination backup root directory")
	backupCmd.Flags().StringVar(&backupLogFile, "log-file", backupLogFile, "log file path")
}

func runBackup(requestedContainers []string, srcDirFlagChanged bool, dstDirFlagChanged bool) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("this command requires root privileges; run it with sudo")
	}

	config, effectiveConfigPath, err := effectiveDackupConfig(backupJSONFile)
	if err != nil {
		return err
	}

	applyBackupDirectoryConfig(config, srcDirFlagChanged, dstDirFlagChanged)

	configs, err := filterConfigsForBackup(config.Containers, requestedContainers)
	if err != nil {
		return err
	}

	if err := preflightChecks(effectiveConfigPath, config, configs); err != nil {
		return err
	}

	containersToStop := containersToStopFromConfig(configs)

	stoppedContainers, err := stopRunningContainers(containersToStop)
	if err != nil {
		return err
	}

	if config.User == "" {
		config.User = "root"
	}
	if config.Group == "" {
		config.Group = "root"
	}

	if err := runConfiguredBackups(configs); err != nil {
		return err
	}

	if err := fixBackupOwnership(config.User, config.Group); err != nil {
		return err
	}

	if err := restartStoppedContainers(stoppedContainers); err != nil {
		return err
	}

	logMessage("INFO", "Backup command finished successfully")
	return nil
}

func applyBackupDirectoryConfig(config dackupConfig, srcDirFlagChanged bool, dstDirFlagChanged bool) {
	if !srcDirFlagChanged && strings.TrimSpace(config.BackupSrcDir) != "" {
		backupSrcDir = config.BackupSrcDir
	}

	if !dstDirFlagChanged && strings.TrimSpace(config.BackupDstDir) != "" {
		backupDstDir = config.BackupDstDir
	}
}

func filterConfigsForBackup(configs []containerConfig, requestedContainers []string) ([]containerConfig, error) {
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

		selectContainerAndContainedForBackup(requestedContainer, configByContainer, selected)
	}

	var filteredConfigs []containerConfig
	for _, config := range configs {
		if selected[config.Container] {
			filteredConfigs = append(filteredConfigs, config)
		}
	}

	if len(filteredConfigs) == 0 {
		return nil, fmt.Errorf("no containers selected for backup")
	}

	return filteredConfigs, nil
}

func selectContainerAndContainedForBackup(
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

		selectContainerAndContainedForBackup(containedContainer, configByContainer, selected)
	}
}

func preflightChecks(effectiveConfigPath string, config dackupConfig, configs []containerConfig) error {
	if _, err := os.Stat(effectiveConfigPath); err != nil {
		return fmt.Errorf("config file not found: %s", effectiveConfigPath)
	}

	if strings.TrimSpace(config.User) == "" {
		return fmt.Errorf("config field %q is required", "user")
	}

	if strings.TrimSpace(config.Group) == "" {
		return fmt.Errorf("config field %q is required", "group")
	}

	srcInfo, err := os.Stat(backupSrcDir)
	if err != nil || !srcInfo.IsDir() {
		return fmt.Errorf("source directory not found: %s", backupSrcDir)
	}

	dstInfo, err := os.Stat(backupDstDir)
	if err != nil || !dstInfo.IsDir() {
		return fmt.Errorf("destination directory not found: %s", backupDstDir)
	}

	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker CLI not found; please install Docker")
	}

	if _, err := exec.LookPath("rsync"); err != nil {
		return fmt.Errorf("rsync not found; please install rsync")
	}

	for _, containerConfig := range configs {
		for _, path := range containerConfig.Paths {
			srcPath := sourcePath(path)

			info, err := os.Stat(srcPath)
			if err != nil {
				return fmt.Errorf("configured path does not exist for container %s: %s", containerConfig.Container, srcPath)
			}

			if !info.IsDir() {
				return fmt.Errorf("configured path is not a directory for container %s: %s", containerConfig.Container, srcPath)
			}
		}
	}

	return nil
}

func containersToStopFromConfig(configs []containerConfig) []string {
	seen := make(map[string]bool)
	var containers []string

	for _, config := range configs {
		if !config.ToStop {
			continue
		}

		addContainer(config.Container, seen, &containers)

		for _, containedContainer := range config.Contains {
			addContainer(containedContainer, seen, &containers)
		}
	}

	return containers
}

func addContainer(container string, seen map[string]bool, containers *[]string) {
	container = strings.TrimSpace(container)
	if container == "" || seen[container] {
		return
	}

	seen[container] = true
	*containers = append(*containers, container)
}

func stopRunningContainers(containers []string) ([]string, error) {
	logMessage("INFO", fmt.Sprintf("Stopping containers listed in %s ...", backupJSONFile))

	if len(containers) == 0 {
		logMessage("WARN", `No containers marked with "to_stop": true; skipping stop step`)
		return nil, nil
	}

	var stoppedContainers []string

	for _, container := range containers {
		running, err := dockerContainerRunning(container)
		if err != nil {
			logMessage("ERROR", fmt.Sprintf("Failed to inspect container %s: %v", container, err))
			continue
		}

		if !running {
			logMessage("INFO", fmt.Sprintf("Container %s is not running; nothing to stop", container))
			continue
		}

		logMessage("INFO", fmt.Sprintf("Stopping container: %s", container))

		if dryRun {
			logMessage("INFO", fmt.Sprintf("[dry-run] Would stop container %s", container))
			stoppedContainers = append(stoppedContainers, container)
			continue
		}

		if err := runLoggedCommand("docker", "stop", container); err != nil {
			logMessage("ERROR", fmt.Sprintf("Failed to stop container %s; continuing", container))
			continue
		}

		logMessage("INFO", fmt.Sprintf("Container %s stopped", container))
		stoppedContainers = append(stoppedContainers, container)
	}

	return stoppedContainers, nil
}

func runConfiguredBackups(configs []containerConfig) error {
	logMessage("INFO", fmt.Sprintf("Starting configured backups from %s to %s ...", backupSrcDir, backupDstDir))

	backedUpPaths := make(map[string]bool)

	for _, config := range configs {
		if len(config.Paths) == 0 {
			logMessage("INFO", fmt.Sprintf("No paths configured for container %s; skipping backup for this entry", config.Container))
			continue
		}

		for _, path := range config.Paths {
			cleanPath := cleanConfiguredPath(path)
			if cleanPath == "" {
				logMessage("WARN", fmt.Sprintf("Empty path configured for container %s; skipping", config.Container))
				continue
			}

			if backedUpPaths[cleanPath] {
				logMessage("INFO", fmt.Sprintf("Path %s already backed up; skipping duplicate", cleanPath))
				continue
			}

			srcPath := sourcePath(cleanPath)
			dstPath := destinationPath(cleanPath)

			if err := backupSinglePath(config.Container, srcPath, dstPath); err != nil {
				return err
			}

			backedUpPaths[cleanPath] = true
		}
	}

	logMessage("INFO", "Configured backups completed successfully")
	return nil
}

func backupSinglePath(container string, srcPath string, dstPath string) error {
	logMessage("INFO", fmt.Sprintf("Backing up %s for container %s to %s ...", srcPath, container, dstPath))

	if dryRun {
		logMessage("INFO", fmt.Sprintf("[dry-run] Would create destination directory %s", dstPath))
		logMessage("INFO", fmt.Sprintf("[dry-run] Would run rsync -a --delete %s/ %s/", srcPath, dstPath))
		return nil
	}

	if err := os.MkdirAll(dstPath, 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory %s: %w", dstPath, err)
	}

	src := filepath.Clean(srcPath) + string(os.PathSeparator)
	dst := filepath.Clean(dstPath) + string(os.PathSeparator)

	if err := runLoggedCommand("rsync", "-a", "--delete", src, dst); err != nil {
		return fmt.Errorf("rsync failed for %s; see %s for details: %w", srcPath, backupLogFile, err)
	}

	logMessage("INFO", fmt.Sprintf("Backup completed for %s", srcPath))
	return nil
}

func fixBackupOwnership(owner string, group string) error {
	logMessage("INFO", fmt.Sprintf("Setting ownership of %s to %s:%s ...", backupDstDir, owner, group))

	if dryRun {
		logMessage("INFO", fmt.Sprintf("[dry-run] Would run chown -R %s:%s %s", owner, group, backupDstDir))
		return nil
	}

	if err := runLoggedCommand("chown", "-R", fmt.Sprintf("%s:%s", owner, group), backupDstDir); err != nil {
		return fmt.Errorf("chown failed; see %s for details: %w", backupLogFile, err)
	}

	logMessage("INFO", "Ownership set correctly")
	return nil
}

func restartStoppedContainers(stoppedContainers []string) error {
	logMessage("INFO", "Starting previously stopped containers ...")

	if len(stoppedContainers) == 0 {
		logMessage("INFO", "No containers were stopped; nothing to restart")
		return nil
	}

	for _, container := range stoppedContainers {
		exists, err := dockerContainerExists(container)
		if err != nil {
			logMessage("ERROR", fmt.Sprintf("Failed to inspect container %s: %v", container, err))
			continue
		}

		if !exists {
			logMessage("WARN", fmt.Sprintf("Container %s does not exist on this host; skipping", container))
			continue
		}

		logMessage("INFO", fmt.Sprintf("Starting container: %s", container))

		if dryRun {
			logMessage("INFO", fmt.Sprintf("[dry-run] Would start container %s", container))
			continue
		}

		if err := runLoggedCommand("docker", "start", container); err != nil {
			logMessage("ERROR", fmt.Sprintf("Failed to start container %s; check manually", container))
			continue
		}

		logMessage("INFO", fmt.Sprintf("Container %s started", container))
	}

	return nil
}

func sourcePath(configuredPath string) string {
	cleanPath := cleanConfiguredPath(configuredPath)
	return filepath.Join(backupSrcDir, cleanPath)
}

func destinationPath(configuredPath string) string {
	cleanPath := cleanConfiguredPath(configuredPath)
	return filepath.Join(backupDstDir, cleanPath)
}

func cleanConfiguredPath(configuredPath string) string {
	return strings.TrimPrefix(filepath.Clean(configuredPath), string(os.PathSeparator))
}

func dockerContainerRunning(container string) (bool, error) {
	output, err := exec.Command("docker", "ps", "-q", "-f", fmt.Sprintf("name=^/%s$", container)).Output()
	if err != nil {
		return false, err
	}

	return len(output) > 0, nil
}

func dockerContainerExists(container string) (bool, error) {
	output, err := exec.Command("docker", "ps", "-a", "-q", "-f", fmt.Sprintf("name=^/%s$", container)).Output()
	if err != nil {
		return false, err
	}

	return len(output) > 0, nil
}

func runLoggedCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)

	logFile, err := os.OpenFile(backupLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if verbose {
		fmt.Printf("Running: %s %s\n", name, strings.Join(args, " "))
	}

	return cmd.Run()
}

func logMessage(level string, message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("[%s] [%s] %s", timestamp, level, message)

	fmt.Println(line)

	logFile, err := os.OpenFile(backupLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write log file: %v\n", err)
		return
	}
	defer logFile.Close()

	writer := bufio.NewWriter(logFile)
	_, _ = writer.WriteString(line + "\n")
	_ = writer.Flush()
}
