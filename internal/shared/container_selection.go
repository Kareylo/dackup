package shared

import (
	"fmt"
	"strings"
)

func SelectContainerAndContained(
	containerName string,
	configByContainer map[string]ContainerConfig,
	selected map[string]bool,
) {
	containerName = strings.TrimSpace(containerName)
	if containerName == "" || selected[containerName] {
		return
	}

	config, exists := configByContainer[containerName]
	if !exists {
		return
	}

	selected[containerName] = true

	for _, containedContainer := range config.Contains {
		SelectContainerAndContained(containedContainer, configByContainer, selected)
	}
}

func FilterContainerConfigs(
	configs []ContainerConfig,
	requestedContainers []string,
	action string,
) ([]ContainerConfig, error) {
	if len(requestedContainers) == 0 {
		return configs, nil
	}

	configByContainer := make(map[string]ContainerConfig)
	for _, config := range configs {
		configByContainer[config.Container] = config
	}

	selected := make(map[string]bool)

	for _, requestedContainer := range requestedContainers {
		requestedContainer = strings.TrimSpace(requestedContainer)
		if requestedContainer == "" {
			continue
		}

		if _, exists := configByContainer[requestedContainer]; !exists {
			return nil, fmt.Errorf("container %q was not found in the configuration", requestedContainer)
		}

		SelectContainerAndContained(requestedContainer, configByContainer, selected)
	}

	var filteredConfigs []ContainerConfig
	for _, config := range configs {
		if selected[config.Container] {
			filteredConfigs = append(filteredConfigs, config)
		}
	}

	if len(filteredConfigs) == 0 {
		return nil, fmt.Errorf("no containers selected for %s", action)
	}

	return filteredConfigs, nil
}

func ContainersToStopFromConfig(configs []ContainerConfig) []string {
	seen := make(map[string]bool)
	var containers []string

	for _, config := range configs {
		if !config.ToStop {
			continue
		}

		AddUniqueContainer(config.Container, seen, &containers)

		for _, containedContainer := range config.Contains {
			AddUniqueContainer(containedContainer, seen, &containers)
		}
	}

	return containers
}

func AddUniqueContainer(container string, seen map[string]bool, containers *[]string) {
	container = strings.TrimSpace(container)
	if container == "" || seen[container] {
		return
	}

	seen[container] = true
	*containers = append(*containers, container)
}
