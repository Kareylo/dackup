package config

import (
	"dackup/internal/shared"
	"fmt"
	"strings"
)

func printContainers(configs []shared.ContainerConfig) {
	fmt.Println("Existing containers:")
	fmt.Println()

	for index, config := range configs {
		fmt.Printf("%d. %s\n", index+1, config.Container)
		fmt.Printf("   Stop before backup: %t\n", config.ToStop)

		if len(config.Paths) > 0 {
			fmt.Printf("   Paths: %s\n", strings.Join(config.Paths, ", "))
		} else {
			fmt.Println("   Paths: none")
		}

		if len(config.Contains) > 0 {
			fmt.Printf("   Contains: %s\n", strings.Join(config.Contains, ", "))
		} else {
			fmt.Println("   Contains: none")
		}

		fmt.Println()
	}
}

func findContainerIndex(configs []shared.ContainerConfig, containerName string) int {
	containerName = strings.TrimSpace(containerName)

	for index, config := range configs {
		if config.Container == containerName {
			return index
		}
	}

	return -1
}
