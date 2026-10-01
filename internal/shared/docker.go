package shared

import (
	"fmt"
	"regexp"
)

// containerNamePattern is Docker's own container name rule
// ([a-zA-Z0-9][a-zA-Z0-9_.-]+, from moby's daemon/names), relaxed to also
// allow one-character names. It rules out path separators, a leading "."
// or "-", and regex/shell metacharacters.
var containerNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// ValidateContainerName rejects a container name Docker itself wouldn't
// accept. Container names end up as backend repository directory names and
// as docker CLI arguments, so an unvalidated one (e.g. "../x" or ".*")
// could escape backend_dir or match the wrong containers.
func ValidateContainerName(name string) error {
	if !containerNamePattern.MatchString(name) {
		return fmt.Errorf("invalid container name %q: only [a-zA-Z0-9][a-zA-Z0-9_.-]* is allowed", name)
	}

	return nil
}

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

	args = append(args, "-q", "-f", fmt.Sprintf("name=^/%s$", regexp.QuoteMeta(container)))

	output, err := runner.Output("docker", args...)
	if err != nil {
		return false, err
	}

	return len(output) > 0, nil
}
