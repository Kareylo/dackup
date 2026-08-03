package shared

import (
	"fmt"
	"os"
	"path/filepath"
)

type TransferDirection string

const (
	TransferBackup  TransferDirection = "backup"
	TransferRestore TransferDirection = "restore"
)

type TransferService struct {
	Direction TransferDirection
	SourceDir string
	DestDir   string
	LogFile   string
	Options   *Options
	FS        FileSystem
	Runner    CommandRunner
	Logger    Logger
	Paths     PathResolver
}

func (service TransferService) Run(configs []ContainerConfig) error {
	logger := service.logger()
	paths := service.pathResolver()

	actionName := string(service.Direction)
	pastTense := service.pastTenseAction()

	logger.Log("INFO", fmt.Sprintf("Starting configured %ss from %s to %s ...", actionName, service.SourceDir, service.DestDir))

	transferredPaths := make(map[string]bool)

	for _, config := range configs {
		if len(config.Paths) == 0 {
			logger.Log("INFO", fmt.Sprintf("No paths configured for container %s; skipping %s for this entry", config.Container, actionName))
			continue
		}

		for _, path := range config.Paths {
			cleanPath := CleanConfiguredPath(path)
			if cleanPath == "" {
				logger.Log("WARN", fmt.Sprintf("Empty path configured for container %s; skipping", config.Container))
				continue
			}

			if transferredPaths[cleanPath] {
				logger.Log("INFO", fmt.Sprintf("Path %s already %s; skipping duplicate", cleanPath, pastTense))
				continue
			}

			srcPath := paths.SourcePath(cleanPath)
			dstPath := paths.DestinationPath(cleanPath)

			if err := service.SinglePath(config.Container, srcPath, dstPath); err != nil {
				return err
			}

			transferredPaths[cleanPath] = true
		}
	}

	logger.Log("INFO", fmt.Sprintf("Configured %ss completed successfully", actionName))
	return nil
}

func (service TransferService) SinglePath(container string, srcPath string, dstPath string) error {
	logger := service.logger()
	fs := service.fileSystem()
	runner := service.commandRunner()

	actionName := string(service.Direction)
	progressVerb := service.progressVerb()

	logger.Log("INFO", fmt.Sprintf("%s %s for container %s to %s ...", progressVerb, srcPath, container, dstPath))

	if service.Options != nil && service.Options.DryRun {
		logger.Log("INFO", fmt.Sprintf("[dry-run] Would create destination directory %s", dstPath))
		logger.Log("INFO", fmt.Sprintf("[dry-run] Would run rsync -a --delete %s/ %s/", srcPath, dstPath))
		return nil
	}

	if err := fs.MkdirAll(dstPath, 0o755); err != nil {
		return fmt.Errorf("failed to create %s destination directory %s: %w", actionName, dstPath, err)
	}

	src := filepath.Clean(srcPath) + string(os.PathSeparator)
	dst := filepath.Clean(dstPath) + string(os.PathSeparator)

	if err := runner.Run("rsync", "-a", "--delete", src, dst); err != nil {
		return fmt.Errorf("%s rsync failed for %s; see %s for details: %w", actionName, srcPath, service.LogFile, err)
	}

	logger.Log("INFO", fmt.Sprintf("%s completed for %s", service.titleAction(), srcPath))
	return nil
}

func (service TransferService) FixBackupOwnership(owner string, group string) error {
	logger := service.logger()
	runner := service.commandRunner()

	logger.Log("INFO", fmt.Sprintf("Setting ownership of %s to %s:%s ...", service.DestDir, owner, group))

	if service.Options != nil && service.Options.DryRun {
		logger.Log("INFO", fmt.Sprintf("[dry-run] Would run chown -R %s:%s %s", owner, group, service.DestDir))
		return nil
	}

	if err := runner.Run("chown", "-R", fmt.Sprintf("%s:%s", owner, group), service.DestDir); err != nil {
		return fmt.Errorf("chown failed; see %s for details: %w", service.LogFile, err)
	}

	logger.Log("INFO", "Ownership set correctly")
	return nil
}

func (service TransferService) FixRestoreOwnership(configs []ContainerConfig, owner string, group string) error {
	logger := service.logger()
	runner := service.commandRunner()
	paths := service.pathResolver()

	logger.Log("INFO", fmt.Sprintf("Setting ownership of restored paths to %s:%s ...", owner, group))

	changedPaths := make(map[string]bool)

	for _, config := range configs {
		for _, path := range config.Paths {
			cleanPath := CleanConfiguredPath(path)
			if cleanPath == "" || changedPaths[cleanPath] {
				continue
			}

			dstPath := paths.DestinationPath(cleanPath)

			if service.Options != nil && service.Options.DryRun {
				logger.Log("INFO", fmt.Sprintf("[dry-run] Would run chown -R %s:%s %s", owner, group, dstPath))
				changedPaths[cleanPath] = true
				continue
			}

			if err := runner.Run("chown", "-R", fmt.Sprintf("%s:%s", owner, group), dstPath); err != nil {
				return fmt.Errorf("chown failed for %s; see %s for details: %w", dstPath, service.LogFile, err)
			}

			changedPaths[cleanPath] = true
		}
	}

	logger.Log("INFO", "Restore ownership set correctly")
	return nil
}

func (service TransferService) fileSystem() FileSystem {
	if service.FS != nil {
		return service.FS
	}

	return OSFileSystem{}
}

func (service TransferService) commandRunner() CommandRunner {
	if service.Runner != nil {
		return service.Runner
	}

	return OSCommandRunner{}
}

func (service TransferService) logger() Logger {
	if service.Logger != nil {
		return service.Logger
	}

	return FileLogger{
		LogFile: service.LogFile,
		FS:      service.fileSystem(),
	}
}

func (service TransferService) pathResolver() PathResolver {
	if service.Paths.SourceRoot != "" || service.Paths.DestinationRoot != "" {
		return service.Paths
	}

	return PathResolver{
		SourceRoot:      service.SourceDir,
		DestinationRoot: service.DestDir,
	}
}

func (service TransferService) progressVerb() string {
	switch service.Direction {
	case TransferRestore:
		return "Restoring"
	default:
		return "Backing up"
	}
}

func (service TransferService) pastTenseAction() string {
	switch service.Direction {
	case TransferRestore:
		return "restored"
	default:
		return "backed up"
	}
}

func (service TransferService) titleAction() string {
	switch service.Direction {
	case TransferRestore:
		return "Restore"
	default:
		return "Backup"
	}
}
