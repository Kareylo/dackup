package backup

import (
	"dackup/internal/backend"
	"dackup/internal/shared"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	backupJSONFile string
	backupSrcDir   = "/opt/apps_docker"
	backupDstDir   = "/backups/in"
	backupLogFile  = "/var/log/docker-backup.log"
	options        *shared.Options
)

type commandService struct {
	options  *shared.Options
	fs       shared.FileSystem
	runner   shared.CommandRunner
	logger   shared.Logger
	paths    shared.PathResolver
	transfer shared.TransferService
}

// NewCommand builds the "backup" command.
func NewCommand(sharedOptions *shared.Options) *cobra.Command {
	options = sharedOptions

	var err error
	backupJSONFile, err = shared.DefaultDackupConfigPath()
	if err != nil {
		backupJSONFile = "config.json"
	}

	backupCmd := &cobra.Command{
		Use:   "backup [container] [container2] ...",
		Short: "Back up Docker application data with rsync",
		Long: `Stop selected Docker containers, back up configured Docker application paths,
fix backup ownership, and restart only the containers that were actually stopped.

When no container is specified, all configured containers are backed up.

Examples:
  dackup backup
  dackup backup paperless
  dackup backup paperless adguard`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackup(
				args,
				cmd.Flags().Changed("src-dir"),
				cmd.Flags().Changed("dst-dir"),
			)
		},
	}

	backupCmd.Flags().StringVar(&backupJSONFile, "config-file", backupJSONFile, "main dackup config file")
	backupCmd.Flags().StringVar(&backupSrcDir, "src-dir", backupSrcDir, "source root directory")
	backupCmd.Flags().StringVar(&backupDstDir, "dst-dir", backupDstDir, "destination backup root directory")
	backupCmd.Flags().StringVar(&backupLogFile, "log-file", backupLogFile, "log file path")

	return backupCmd
}

func newCommandService() commandService {
	fs := shared.OSFileSystem{}
	runner := shared.LoggedCommandRunner{
		Runner:  shared.OSCommandRunner{},
		FS:      fs,
		LogFile: backupLogFile,
		Options: options,
	}
	logger := shared.FileLogger{
		LogFile: backupLogFile,
		FS:      fs,
	}
	paths := shared.PathResolver{
		SourceRoot:      backupSrcDir,
		DestinationRoot: backupDstDir,
	}
	transfer := shared.TransferService{
		Direction: shared.TransferBackup,
		SourceDir: backupSrcDir,
		DestDir:   backupDstDir,
		LogFile:   backupLogFile,
		Options:   options,
		FS:        fs,
		Runner:    runner,
		Logger:    logger,
		Paths:     paths,
	}

	return commandService{
		options:  options,
		fs:       fs,
		runner:   runner,
		logger:   logger,
		paths:    paths,
		transfer: transfer,
	}
}

func runBackup(requestedContainers []string, srcDirFlagChanged bool, dstDirFlagChanged bool) error {
	config, effectiveConfigPath, err := shared.EffectiveDackupConfig(backupJSONFile)
	if err != nil {
		return err
	}

	applyBackupDirectoryConfig(config, srcDirFlagChanged, dstDirFlagChanged)

	service := newCommandService()

	backupBackend, err := resolveBackend(service, config)
	if err != nil {
		return err
	}

	configs, err := filterConfigsForBackup(config.Containers, requestedContainers)
	if err != nil {
		return err
	}

	if err := preflightChecks(effectiveConfigPath, config, configs); err != nil {
		return err
	}

	containersToStop := containersToStopFromConfig(configs)

	lifecycleService := shared.ContainerLifecycleService{
		Docker: shared.DockerService{
			Runner: service.runner,
		},
		Runner:  service.runner,
		Logger:  service.logger,
		Options: service.options,
	}

	stoppedContainers, err := lifecycleService.StopRunningContainers(containersToStop, "backup")
	if err != nil {
		return err
	}

	if config.User == "" {
		config.User = "root"
	}
	if config.Group == "" {
		config.Group = "root"
	}

	if err := service.transfer.Run(configs); err != nil {
		return err
	}

	if err := service.transfer.FixBackupOwnership(config.User, config.Group); err != nil {
		return err
	}

	if err := lifecycleService.StartStoppedContainers(stoppedContainers, "backup"); err != nil {
		return err
	}

	if groupedBackend, ok := backupBackend.(backend.GroupedBackend); ok {
		groups := shared.BackendGroupsFromContainerGroups(shared.ContainerGroups(configs))
		if err := groupedBackend.BackupGroups(backupDstDir, groups); err != nil {
			return err
		}
	} else if err := backupBackend.Backup(backupDstDir); err != nil {
		return err
	}

	service.logger.Log("INFO", "Backup command finished successfully")
	return nil
}

// resolveBackend constructs the configured Backend (or the default no-op backend if
// config.Backend is unset) via internal/backend.Factory, using the same
// dependencies commandService already builds for TransferService.
func resolveBackend(service commandService, config shared.DackupConfig) (backend.Backend, error) {
	factory := backend.Factory{
		Runner:     service.runner,
		Logger:     service.logger,
		Options:    service.options,
		BackendDir: config.BackendDir,
		Secrets:    shared.AESFileSecretStore{},
	}

	return factory.GetBackend(config.Backend, config.BackendSettings)
}

func applyBackupDirectoryConfig(config shared.DackupConfig, srcDirFlagChanged bool, dstDirFlagChanged bool) {
	if !srcDirFlagChanged && strings.TrimSpace(config.DataDir) != "" {
		backupSrcDir = config.DataDir
	}

	if !dstDirFlagChanged && strings.TrimSpace(config.StagingDir) != "" {
		backupDstDir = config.StagingDir
	}
}

func filterConfigsForBackup(configs []shared.ContainerConfig, requestedContainers []string) ([]shared.ContainerConfig, error) {
	return shared.FilterContainerConfigs(configs, requestedContainers, "backup")
}

func selectContainerAndContainedForBackup(
	containerName string,
	configByContainer map[string]shared.ContainerConfig,
	selected map[string]bool,
) {
	shared.SelectContainerAndContained(containerName, configByContainer, selected)
}

func preflightChecks(effectiveConfigPath string, config shared.DackupConfig, configs []shared.ContainerConfig) error {
	service := newCommandService()
	return shared.PreflightChecks(
		"backup",
		effectiveConfigPath,
		config,
		configs,
		backupSrcDir,
		backupDstDir,
		service.paths,
		service.fs,
		service.runner,
	)
}

func containersToStopFromConfig(configs []shared.ContainerConfig) []string {
	return shared.ContainersToStopFromConfig(configs)
}

func addContainer(container string, seen map[string]bool, containers *[]string) {
	shared.AddUniqueContainer(container, seen, containers)
}

func runConfiguredBackups(configs []shared.ContainerConfig) error {
	return newCommandService().transfer.Run(configs)
}

func backupSinglePath(container string, srcPath string, dstPath string) error {
	return newCommandService().transfer.SinglePath(container, srcPath, dstPath)
}

func fixBackupOwnership(owner string, group string) error {
	return newCommandService().transfer.FixBackupOwnership(owner, group)
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
	return shared.CleanConfiguredPath(configuredPath)
}
