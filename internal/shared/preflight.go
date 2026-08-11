package shared

import (
	"fmt"
	"strings"
)

func PreflightChecks(
	action string,
	effectiveConfigPath string,
	config DackupConfig,
	configs []ContainerConfig,
	sourceRoot string,
	destinationRoot string,
	resolver PathResolver,
	fs FileSystem,
	runner CommandRunner,
) error {
	if fs == nil {
		fs = OSFileSystem{}
	}

	if runner == nil {
		runner = OSCommandRunner{}
	}

	if _, err := fs.Stat(effectiveConfigPath); err != nil {
		return fmt.Errorf("config file not found: %s", effectiveConfigPath)
	}

	if strings.TrimSpace(config.User) == "" {
		return fmt.Errorf("config field %q is required", "user")
	}

	if strings.TrimSpace(config.Group) == "" {
		return fmt.Errorf("config field %q is required", "group")
	}

	srcInfo, err := fs.Stat(sourceRoot)
	if err != nil || !srcInfo.IsDir() {
		return fmt.Errorf("%s source directory not found: %s", action, sourceRoot)
	}

	dstInfo, err := fs.Stat(destinationRoot)
	if err != nil || !dstInfo.IsDir() {
		return fmt.Errorf("%s destination directory not found: %s", action, destinationRoot)
	}

	if strings.TrimSpace(config.BackendDir) != "" {
		backendInfo, err := fs.Stat(config.BackendDir)
		if err != nil || !backendInfo.IsDir() {
			return fmt.Errorf("%s backend directory not found: %s", action, config.BackendDir)
		}
	}

	if _, err := runner.LookPath("docker"); err != nil {
		return fmt.Errorf("docker CLI not found; please install Docker")
	}

	if _, err := runner.LookPath("rsync"); err != nil {
		return fmt.Errorf("rsync not found; please install rsync")
	}

	for _, containerConfig := range configs {
		for _, path := range containerConfig.Paths {
			srcPath := resolver.SourcePath(path)

			info, err := fs.Stat(srcPath)
			if err != nil {
				return fmt.Errorf("configured %s path does not exist for container %s: %s", action, containerConfig.Container, srcPath)
			}

			if !info.IsDir() {
				return fmt.Errorf("configured %s path is not a directory for container %s: %s", action, containerConfig.Container, srcPath)
			}
		}
	}

	return nil
}
