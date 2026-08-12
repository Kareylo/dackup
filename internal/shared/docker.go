package shared

import "fmt"

// DockerService checks container state via the docker CLI.
type DockerService struct {
	Runner CommandRunner
}

// ContainerRunning reports whether container is currently running.
func (service DockerService) ContainerRunning(container string) (bool, error) {
	runner := service.Runner
	if runner == nil {
		runner = OSCommandRunner{}
	}

	output, err := runner.Output("docker", "ps", "-q", "-f", fmt.Sprintf("name=^/%s$", container))
	if err != nil {
		return false, err
	}

	return len(output) > 0, nil
}

// ContainerExists reports whether container exists, running or not.
func (service DockerService) ContainerExists(container string) (bool, error) {
	runner := service.Runner
	if runner == nil {
		runner = OSCommandRunner{}
	}

	output, err := runner.Output("docker", "ps", "-a", "-q", "-f", fmt.Sprintf("name=^/%s$", container))
	if err != nil {
		return false, err
	}

	return len(output) > 0, nil
}
