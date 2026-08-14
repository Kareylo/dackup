package shared

import "fmt"

// DockerService checks container state via the docker CLI.
type DockerService struct {
	Runner CommandRunner
}

// ContainerRunning reports whether container is currently running.
func (service DockerService) ContainerRunning(container string) (bool, error) {
	return service.queryContainer(container, "ps")
}

// ContainerExists reports whether container exists, running or not.
func (service DockerService) ContainerExists(container string) (bool, error) {
	return service.queryContainer(container, "ps", "-a")
}

func (service DockerService) queryContainer(container string, args ...string) (bool, error) {
	runner := service.Runner
	if runner == nil {
		runner = OSCommandRunner{}
	}

	args = append(args, "-q", "-f", fmt.Sprintf("name=^/%s$", container))

	output, err := runner.Output("docker", args...)
	if err != nil {
		return false, err
	}

	return len(output) > 0, nil
}
