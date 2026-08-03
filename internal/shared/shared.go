package shared

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const DefaultConfigRelativePath = ".config/dackup/config.json"

type Options struct {
	Verbose bool
	DryRun  bool
}

type ContainerConfig struct {
	Container string   `json:"container"`
	ToStop    bool     `json:"to_stop"`
	Paths     []string `json:"paths,omitempty"`
	Contains  []string `json:"contains,omitempty"`
}

type DackupConfig struct {
	User         string            `json:"user,omitempty"`
	Group        string            `json:"group,omitempty"`
	ConfigFile   string            `json:"config_file,omitempty"`
	BackupSrcDir string            `json:"backup_src_dir,omitempty"`
	BackupDstDir string            `json:"backup_dst_dir,omitempty"`
	Containers   []ContainerConfig `json:"containers,omitempty"`
}

func DefaultDackupConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to find user home directory: %w", err)
	}

	return filepath.Join(homeDir, DefaultConfigRelativePath), nil
}

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

	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", path, err)
	}

	return nil
}

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

func ReadContainerConfigsFromPath(path string) ([]ContainerConfig, error) {
	config, err := ReadDackupConfig(path)
	if err != nil {
		return nil, err
	}

	return config.Containers, nil
}

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

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
