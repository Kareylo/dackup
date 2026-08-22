package config

import (
	"bufio"
	"dackup/internal/shared"
	"fmt"
)

func (service commandService) askContainers() ([]shared.ContainerConfig, error) {
	var configs []shared.ContainerConfig

	fmt.Println("Creating dackup containers configuration.")
	fmt.Println("You will now be asked to add containers.")
	fmt.Println()

	for {
		config, err := service.askContainerConfig()
		if err != nil {
			return nil, err
		}

		configs = append(configs, config)

		addAnother, err := service.prompt.Bool("Add another container?", true)
		if err != nil {
			return nil, err
		}

		if !addAnother {
			break
		}

		fmt.Println()
	}

	return configs, nil
}

func (service commandService) askContainerConfig() (shared.ContainerConfig, error) {
	container, err := service.prompt.RequiredString("Container name")
	if err != nil {
		return shared.ContainerConfig{}, err
	}

	toStop, err := service.prompt.Bool("Stop this container before backup?", false)
	if err != nil {
		return shared.ContainerConfig{}, err
	}

	paths, err := service.prompt.StringList("Backup paths, separated by commas. Leave empty if none")
	if err != nil {
		return shared.ContainerConfig{}, err
	}

	contains, err := service.prompt.StringList("Contained/dependent containers, separated by commas. Leave empty if none")
	if err != nil {
		return shared.ContainerConfig{}, err
	}

	config := shared.ContainerConfig{
		Container: container,
		ToStop:    toStop,
	}

	if len(paths) > 0 {
		config.Paths = paths
	}

	if len(contains) > 0 {
		config.Contains = contains
	}

	return config, nil
}

func (service commandService) askUpdatedContainerConfig(
	currentConfig shared.ContainerConfig,
) (shared.ContainerConfig, error) {
	fmt.Printf("Updating container %q. Press Enter to keep the current value.\n", currentConfig.Container)
	fmt.Println()

	container, err := service.prompt.StringWithDefault("Container name", currentConfig.Container)
	if err != nil {
		return shared.ContainerConfig{}, err
	}

	toStop, err := service.prompt.Bool(
		fmt.Sprintf("Stop this container before backup? Current value: %t", currentConfig.ToStop),
		currentConfig.ToStop,
	)
	if err != nil {
		return shared.ContainerConfig{}, err
	}

	paths, err := service.prompt.StringListWithDefault("Backup paths, separated by commas", currentConfig.Paths)
	if err != nil {
		return shared.ContainerConfig{}, err
	}

	contains, err := service.prompt.StringListWithDefault("Contained/dependent containers, separated by commas", currentConfig.Contains)
	if err != nil {
		return shared.ContainerConfig{}, err
	}

	updatedConfig := shared.ContainerConfig{
		Container: container,
		ToStop:    toStop,
	}

	if len(paths) > 0 {
		updatedConfig.Paths = paths
	}

	if len(contains) > 0 {
		updatedConfig.Contains = contains
	}

	return updatedConfig, nil
}

func askStringWithDefault(reader *bufio.Reader, label string, defaultValue string) (string, error) {
	return shared.NewPromptService(reader).StringWithDefault(label, defaultValue)
}

func askStringListWithDefault(reader *bufio.Reader, label string, defaultValues []string) ([]string, error) {
	return shared.NewPromptService(reader).StringListWithDefault(label, defaultValues)
}

func parseStringList(value string) []string {
	return shared.ParseStringList(value)
}

func (service commandService) readExistingContainerConfigs(path string) ([]shared.ContainerConfig, error) {
	if !shared.FileExists(path) {
		create, err := service.askCreateMissingConfig(path)
		if err != nil {
			return nil, err
		}

		if !create {
			return nil, fmt.Errorf("configuration file does not exist: %s", path)
		}

		if err := shared.WriteContainerConfigsToPath(path, []shared.ContainerConfig{}, options); err != nil {
			return nil, err
		}

		return []shared.ContainerConfig{}, nil
	}

	return shared.ReadContainerConfigsFromPath(path)
}

func (service commandService) askCreateMissingConfig(path string) (bool, error) {
	return service.prompt.Bool(fmt.Sprintf("Configuration file does not exist at %s. Create it?", path), true)
}
