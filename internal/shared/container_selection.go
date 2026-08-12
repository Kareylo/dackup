package shared

import (
	"fmt"
	"strings"
)

// SelectContainerAndContained marks containerName selected in selected,
// then recursively does the same for every container it Contains. Already
// visited containers are skipped, so a cycle in Contains terminates safely.
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

// FilterContainerConfigs narrows configs down to requestedContainers plus
// each one's Contains dependents (recursively, via
// SelectContainerAndContained), preserving configs's original order. An
// empty requestedContainers returns configs unchanged (meaning "all
// containers"); requesting an unconfigured container is an error. action
// names the operation for the "no containers selected" error message.
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

// ContainersToStopFromConfig lists, in encounter order and without
// duplicates, every container that should be stopped: each config with
// ToStop true, plus everything it Contains.
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

// AddUniqueContainer appends container to *containers, unless it's empty
// or already present in seen (which is updated in place).
func AddUniqueContainer(container string, seen map[string]bool, containers *[]string) {
	container = strings.TrimSpace(container)
	if container == "" || seen[container] {
		return
	}

	seen[container] = true
	*containers = append(*containers, container)
}
