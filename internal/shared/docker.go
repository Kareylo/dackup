package shared

import "fmt"

type DockerService struct {
	Runner CommandRunner
}

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
