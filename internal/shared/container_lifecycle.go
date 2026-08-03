package shared

import "fmt"

type ContainerLifecycleService struct {
	Docker  DockerService
	Runner  CommandRunner
	Logger  Logger
	Options *Options
}

func (service ContainerLifecycleService) StopRunningContainers(containers []string, action string) ([]string, error) {
	service.Logger.Log("INFO", fmt.Sprintf("Stopping containers before %s ...", action))

	if len(containers) == 0 {
		service.Logger.Log("WARN", `No containers marked with "to_stop": true; skipping stop step`)
		return nil, nil
	}

	var stoppedContainers []string

	for _, container := range containers {
		running, err := service.Docker.ContainerRunning(container)
		if err != nil {
			service.Logger.Log("ERROR", fmt.Sprintf("Failed to inspect container %s: %v", container, err))
			continue
		}

		if !running {
			service.Logger.Log("INFO", fmt.Sprintf("Container %s is not running; nothing to stop", container))
			continue
		}

		service.Logger.Log("INFO", fmt.Sprintf("Stopping container: %s", container))

		if service.Options != nil && service.Options.DryRun {
			service.Logger.Log("INFO", fmt.Sprintf("[dry-run] Would stop container %s", container))
			stoppedContainers = append(stoppedContainers, container)
			continue
		}

		if err := service.Runner.Run("docker", "stop", container); err != nil {
			service.Logger.Log("ERROR", fmt.Sprintf("Failed to stop container %s; continuing", container))
			continue
		}

		service.Logger.Log("INFO", fmt.Sprintf("Container %s stopped", container))
		stoppedContainers = append(stoppedContainers, container)
	}

	return stoppedContainers, nil
}

func (service ContainerLifecycleService) StartStoppedContainers(stoppedContainers []string, action string) error {
	service.Logger.Log("INFO", "Starting previously stopped containers ...")

	if len(stoppedContainers) == 0 {
		service.Logger.Log("INFO", fmt.Sprintf("No containers were stopped by %s; nothing to restart", action))
		return nil
	}

	for _, container := range stoppedContainers {
		exists, err := service.Docker.ContainerExists(container)
		if err != nil {
			service.Logger.Log("ERROR", fmt.Sprintf("Failed to inspect container %s: %v", container, err))
			continue
		}

		if !exists {
			service.Logger.Log("WARN", fmt.Sprintf("Container %s does not exist on this host; skipping", container))
			continue
		}

		service.Logger.Log("INFO", fmt.Sprintf("Starting container: %s", container))

		if service.Options != nil && service.Options.DryRun {
			service.Logger.Log("INFO", fmt.Sprintf("[dry-run] Would start container %s", container))
			continue
		}

		if err := service.Runner.Run("docker", "start", container); err != nil {
			service.Logger.Log("ERROR", fmt.Sprintf("Failed to start container %s; check manually", container))
			continue
		}

		service.Logger.Log("INFO", fmt.Sprintf("Container %s started", container))
	}

	return nil
}
