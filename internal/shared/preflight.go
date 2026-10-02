package shared

import (
	"fmt"
	"regexp"
	"strings"
)

// ownerNamePattern accepts a user or group name, or a numeric ID, that chown
// can only read as an operand: it never starts with '-' or '.', and holds no
// ':', '/', '=' or whitespace. A trailing '$' is allowed for machine
// accounts.
var ownerNamePattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*\$?$`)

// ValidateOwnerName rejects a user or group name that chown could parse as
// an option or that would change the owner:group operand. field names the
// config field in the error.
func ValidateOwnerName(field string, name string) error {
	if !ownerNamePattern.MatchString(name) {
		return fmt.Errorf("config field %q has invalid value %q: only [A-Za-z0-9_][A-Za-z0-9_.-]* (optionally ending in $) is allowed", field, name)
	}

	return nil
}

// PreflightChecks validates that action (backup or restore) can proceed:
// the effective config file exists, config.User/Group are set and valid
// (see ValidateOwnerName), the source/
// destination/backend roots exist as directories, docker and rsync are on
// PATH, every container name and contains entry in configs is a valid
// Docker name (see ValidateContainerName), and every configured path in
// configs stays inside its root (see
// ValidateConfiguredPath) and resolves (via resolver) to an existing source
// directory. fs/runner default to their real
// implementations when nil.
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

	if err := ValidateOwnerName("user", config.User); err != nil {
		return err
	}

	if err := ValidateOwnerName("group", config.Group); err != nil {
		return err
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
		if err := ValidateContainerName(containerConfig.Container); err != nil {
			return err
		}

		for _, contained := range containerConfig.Contains {
			if err := ValidateContainerName(contained); err != nil {
				return fmt.Errorf("invalid contains entry for container %s: %w", containerConfig.Container, err)
			}
		}
	}

	for _, containerConfig := range configs {
		for _, path := range containerConfig.Paths {
			if err := ValidateConfiguredPath(path); err != nil {
				return fmt.Errorf("invalid %s path for container %s: %w", action, containerConfig.Container, err)
			}

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
