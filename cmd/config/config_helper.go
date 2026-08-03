package config

import "dackup/internal/shared"

type dackupConfig = shared.DackupConfig

func defaultDackupConfigPath() (string, error) {
	return shared.DefaultDackupConfigPath()
}

func readDackupConfig(path string) (dackupConfig, error) {
	return shared.ReadDackupConfig(path)
}

func writeDackupConfig(path string, config dackupConfig) error {
	return shared.WriteDackupConfig(path, config, options)
}

func effectiveDackupConfig(mainConfigPath string) (dackupConfig, string, error) {
	return shared.EffectiveDackupConfig(mainConfigPath)
}

func effectiveContainersConfigPath(mainConfigPath string) (string, error) {
	return shared.EffectiveContainersConfigPath(mainConfigPath)
}

func readContainerConfigsFromPath(path string) ([]shared.ContainerConfig, error) {
	return shared.ReadContainerConfigsFromPath(path)
}

func writeContainerConfigsToPath(path string, containers []shared.ContainerConfig) error {
	return shared.WriteContainerConfigsToPath(path, containers, options)
}

func fileExists(path string) bool {
	return shared.FileExists(path)
}
