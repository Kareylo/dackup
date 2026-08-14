package restore

import (
	"dackup/internal/backend"
	"dackup/internal/shared"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	restoreConfigFile string
	restoreSrcDir     = "/backups/in"
	restoreDstDir     = "/opt/apps_docker"
	restoreLogFile    = "/var/log/docker-restore.log"
	options           *shared.Options
)

type commandService struct {
	options  *shared.Options
	fs       shared.FileSystem
	runner   shared.CommandRunner
	logger   shared.Logger
	paths    shared.PathResolver
	transfer shared.TransferService
}

// NewCommand builds the "restore" command.
func NewCommand(sharedOptions *shared.Options) *cobra.Command {
	options = sharedOptions

	var err error
	restoreConfigFile, err = shared.DefaultDackupConfigPath()
	if err != nil {
		restoreConfigFile = "config.json"
	}

	restoreCmd := &cobra.Command{
		Use:   "restore [container] [container2] ...",
		Short: "Restore Docker application data with rsync",
		Long: `Stop selected Docker containers, restore configured Docker application paths,
fix restored ownership, and restart only the containers that were actually stopped.

When no container is specified, all configured containers are restored.

Examples:
  dackup restore
  dackup restore paperless
  dackup restore paperless adguard`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRestore(
				args,
				cmd.Flags().Changed("src-dir"),
				cmd.Flags().Changed("dst-dir"),
			)
		},
	}

	restoreCmd.Flags().StringVar(&restoreConfigFile, "config-file", restoreConfigFile, "main dackup config file")
	restoreCmd.Flags().StringVar(&restoreSrcDir, "src-dir", restoreSrcDir, "restore source root directory")
	restoreCmd.Flags().StringVar(&restoreDstDir, "dst-dir", restoreDstDir, "restore destination root directory")
	restoreCmd.Flags().StringVar(&restoreLogFile, "log-file", restoreLogFile, "restore log file path")

	return restoreCmd
}

func newCommandService() commandService {
	fs := shared.OSFileSystem{}
	runner := shared.LoggedCommandRunner{
		Runner:  shared.OSCommandRunner{},
		FS:      fs,
		LogFile: restoreLogFile,
		Options: options,
	}
	logger := shared.FileLogger{
		LogFile: restoreLogFile,
		FS:      fs,
	}
	paths := shared.PathResolver{
		SourceRoot:      restoreSrcDir,
		DestinationRoot: restoreDstDir,
	}
	transfer := shared.TransferService{
		Direction: shared.TransferRestore,
		SourceDir: restoreSrcDir,
		DestDir:   restoreDstDir,
		LogFile:   restoreLogFile,
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

func runRestore(requestedContainers []string, srcDirFlagChanged bool, dstDirFlagChanged bool) error {
	config, effectiveConfigPath, err := shared.EffectiveDackupConfig(restoreConfigFile)
	if err != nil {
		return err
	}

	applyRestoreDirectoryConfig(config, srcDirFlagChanged, dstDirFlagChanged)

	return runRestoreWithService(newCommandService(), config, effectiveConfigPath, requestedContainers)
}

// runRestoreWithService is runRestore's testable core: it takes an
// already-built commandService instead of constructing one via
// newCommandService(), so tests can inject fakes for fs/runner/logger
// instead of hitting the OS.
func runRestoreWithService(
	service commandService,
	config shared.DackupConfig,
	effectiveConfigPath string,
	requestedContainers []string,
) error {
	restoreBackend, err := resolveBackend(service, config)
	if err != nil {
		return err
	}

	configs, err := filterConfigsForRestore(config.Containers, requestedContainers)
	if err != nil {
		return err
	}

	if err := shared.PreflightChecks(
		"restore",
		effectiveConfigPath,
		config,
		configs,
		restoreSrcDir,
		restoreDstDir,
		service.paths,
		service.fs,
		service.runner,
	); err != nil {
		return err
	}

	if groupedBackend, ok := restoreBackend.(backend.GroupedBackend); ok {
		groups := shared.BackendGroupsFromContainerGroups(shared.ContainerGroups(configs))
		if err := groupedBackend.RestoreGroups(restoreSrcDir, groups); err != nil {
			return err
		}
	} else if err := restoreBackend.Restore(restoreSrcDir); err != nil {
		return err
	}

	containersToStop := restoreContainersToStopFromConfig(configs)

	lifecycleService := shared.ContainerLifecycleService{
		Docker: shared.DockerService{
			Runner: service.runner,
		},
		Runner:  service.runner,
		Logger:  service.logger,
		Options: service.options,
	}

	stoppedContainers, err := lifecycleService.StopRunningContainers(containersToStop, "restore")
	if err != nil {
		if restartErr := lifecycleService.StartStoppedContainers(stoppedContainers, "restore"); restartErr != nil {
			return fmt.Errorf("%w (additionally failed to restart already-stopped containers: %v)", err, restartErr)
		}
		return err
	}

	if err := service.transfer.Run(configs); err != nil {
		return err
	}

	if err := service.transfer.FixRestoreOwnership(configs, config.User, config.Group); err != nil {
		return err
	}

	if err := lifecycleService.StartStoppedContainers(stoppedContainers, "restore"); err != nil {
		return err
	}

	service.logger.Log("INFO", "Restore command finished successfully")
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

func applyRestoreDirectoryConfig(config shared.DackupConfig, srcDirFlagChanged bool, dstDirFlagChanged bool) {
	if !srcDirFlagChanged && strings.TrimSpace(config.StagingDir) != "" {
		restoreSrcDir = config.StagingDir
	}

	if !dstDirFlagChanged && strings.TrimSpace(config.DataDir) != "" {
		restoreDstDir = config.DataDir
	}
}

func filterConfigsForRestore(
	configs []shared.ContainerConfig,
	requestedContainers []string,
) ([]shared.ContainerConfig, error) {
	return shared.FilterContainerConfigs(configs, requestedContainers, "restore")
}

func selectContainerAndContainedForRestore(
	containerName string,
	configByContainer map[string]shared.ContainerConfig,
	selected map[string]bool,
) {
	shared.SelectContainerAndContained(containerName, configByContainer, selected)
}

func restoreContainersToStopFromConfig(configs []shared.ContainerConfig) []string {
	return shared.ContainersToStopFromConfig(configs)
}

func restoreAddContainer(container string, seen map[string]bool, containers *[]string) {
	shared.AddUniqueContainer(container, seen, containers)
}

func restoreCleanConfiguredPath(configuredPath string) string {
	return shared.CleanConfiguredPath(configuredPath)
}
