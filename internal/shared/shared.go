package shared

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultConfigRelativePath is DefaultDackupConfigPath's path, relative to
// the user's home directory.
const DefaultConfigRelativePath = ".config/dackup/config.json"

// Options carries the global --verbose/--dry-run flags into every service.
type Options struct {
	Verbose bool
	DryRun  bool
}

// ContainerConfig is one configured Docker container: whether to stop it,
// which paths to back up/restore, and which other containers it Contains
// (selected automatically alongside it).
type ContainerConfig struct {
	Container string   `json:"container"`
	ToStop    bool     `json:"to_stop"`
	Paths     []string `json:"paths,omitempty"`
	Contains  []string `json:"contains,omitempty"`
}

// DackupConfig is the main dackup configuration file's contents: user/group
// for ownership fixes, the source/staging/backend directory roots, the
// configured backup backend (if any), and either an inline Containers list
// or a ConfigFile pointer to a separate file holding it — see
// EffectiveDackupConfig.
type DackupConfig struct {
	User            string            `json:"user,omitempty"`
	Group           string            `json:"group,omitempty"`
	ConfigFile      string            `json:"config_file,omitempty"`
	DataDir         string            `json:"data_dir,omitempty"`
	StagingDir      string            `json:"staging_dir,omitempty"`
	BackendDir      string            `json:"backend_dir,omitempty"`
	Backend         string            `json:"backend,omitempty"`
	BackendSettings json.RawMessage   `json:"backend_settings,omitempty"`
	Containers      []ContainerConfig `json:"containers,omitempty"`
}

// DefaultDackupConfigPath returns ~/.config/dackup/config.json.
func DefaultDackupConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to find user home directory: %w", err)
	}

	return filepath.Join(homeDir, DefaultConfigRelativePath), nil
}

// ReadDackupConfig reads and parses the DackupConfig JSON file at path.
func ReadDackupConfig(path string) (DackupConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return DackupConfig{}, fmt.Errorf("failed to open config file %s: %w", path, err)
	}
	defer file.Close()

	var config DackupConfig
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return DackupConfig{}, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	return config, nil
}

// WriteDackupConfig writes config as indented JSON to path, creating its
// parent directory if needed. With options.DryRun set, it prints what
// would be written instead of touching disk.
func WriteDackupConfig(path string, config DackupConfig, options *Options) error {
	if options != nil && options.DryRun {
		content, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to encode config: %w", err)
		}

		fmt.Println("[dry-run] Would write config:")
		fmt.Println(string(content))
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	content, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}

	content = append(content, '\n')

	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", path, err)
	}

	return nil
}

// EffectiveDackupConfig reads the main config at mainConfigPath and, if it
// points at a separate ConfigFile, overlays that file's Containers onto it
// — so callers always see the right Containers regardless of which file
// they live in. It returns the effective config and the path Containers
// actually came from.
func EffectiveDackupConfig(mainConfigPath string) (DackupConfig, string, error) {
	mainConfig, err := ReadDackupConfig(mainConfigPath)
	if err != nil {
		return DackupConfig{}, "", err
	}

	effectiveConfigPath := mainConfigPath
	effectiveConfig := mainConfig

	if mainConfig.ConfigFile != "" {
		effectiveConfigPath = mainConfig.ConfigFile

		containersConfig, err := ReadDackupConfig(mainConfig.ConfigFile)
		if err != nil {
			return DackupConfig{}, "", err
		}

		effectiveConfig.Containers = containersConfig.Containers
	}

	return effectiveConfig, effectiveConfigPath, nil
}

// EffectiveContainersConfigPath returns the path that actually holds
// Containers: mainConfigPath's own ConfigFile if set, otherwise
// mainConfigPath itself. Returns mainConfigPath unchanged if it doesn't
// exist yet (e.g. before "dackup config init").
func EffectiveContainersConfigPath(mainConfigPath string) (string, error) {
	if !FileExists(mainConfigPath) {
		return mainConfigPath, nil
	}

	config, err := ReadDackupConfig(mainConfigPath)
	if err != nil {
		return "", err
	}

	if config.ConfigFile != "" {
		return config.ConfigFile, nil
	}

	return mainConfigPath, nil
}

// ReadContainerConfigsFromPath reads the Containers list from the
// DackupConfig JSON file at path.
func ReadContainerConfigsFromPath(path string) ([]ContainerConfig, error) {
	config, err := ReadDackupConfig(path)
	if err != nil {
		return nil, err
	}

	return config.Containers, nil
}

// WriteContainerConfigsToPath writes containers into the DackupConfig JSON
// file at path, preserving that file's other existing fields (reading it
// first if it already exists).
func WriteContainerConfigsToPath(path string, containers []ContainerConfig, options *Options) error {
	existingConfig := DackupConfig{}

	if FileExists(path) {
		config, err := ReadDackupConfig(path)
		if err != nil {
			return err
		}

		existingConfig = config
	}

	existingConfig.Containers = containers

	return WriteDackupConfig(path, existingConfig, options)
}

// FileExists reports whether path exists on disk.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
