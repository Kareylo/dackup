package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const defaultConfigRelativePath = ".config/dackup/config.json"

type dackupConfig struct {
	User       string            `json:"user,omitempty"`
	Group      string            `json:"group,omitempty"`
	ConfigFile string            `json:"config_file,omitempty"`
	Containers []containerConfig `json:"containers,omitempty"`
}

func defaultDackupConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to find user home directory: %w", err)
	}

	return filepath.Join(homeDir, defaultConfigRelativePath), nil
}

func readDackupConfig(path string) (dackupConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return dackupConfig{}, fmt.Errorf("failed to open config file %s: %w", path, err)
	}
	defer file.Close()

	var config dackupConfig
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return dackupConfig{}, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	return config, nil
}

func writeDackupConfig(path string, config dackupConfig) error {
	if dryRun {
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

func effectiveDackupConfig(mainConfigPath string) (dackupConfig, string, error) {
	mainConfig, err := readDackupConfig(mainConfigPath)
	if err != nil {
		return dackupConfig{}, "", err
	}

	effectiveConfigPath := mainConfigPath
	effectiveConfig := mainConfig

	if mainConfig.ConfigFile != "" {
		effectiveConfigPath = mainConfig.ConfigFile

		containersConfig, err := readDackupConfig(mainConfig.ConfigFile)
		if err != nil {
			return dackupConfig{}, "", err
		}

		effectiveConfig.Containers = containersConfig.Containers
	}

	return effectiveConfig, effectiveConfigPath, nil
}

func effectiveContainersConfigPath(mainConfigPath string) (string, error) {
	if !fileExists(mainConfigPath) {
		return mainConfigPath, nil
	}

	config, err := readDackupConfig(mainConfigPath)
	if err != nil {
		return "", err
	}

	if config.ConfigFile != "" {
		return config.ConfigFile, nil
	}

	return mainConfigPath, nil
}

func readContainerConfigsFromPath(path string) ([]containerConfig, error) {
	config, err := readDackupConfig(path)
	if err != nil {
		return nil, err
	}

	return config.Containers, nil
}

func writeContainerConfigsToPath(path string, containers []containerConfig) error {
	existingConfig := dackupConfig{}

	if fileExists(path) {
		config, err := readDackupConfig(path)
		if err != nil {
			return err
		}

		existingConfig = config
	}

	existingConfig.Containers = containers

	return writeDackupConfig(path, existingConfig)
}
