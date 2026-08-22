package shared

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type fakeLifecycleLogger struct {
	lines []string
}

func (logger *fakeLifecycleLogger) Log(level string, message string) {
	logger.lines = append(logger.lines, fmt.Sprintf("[%s] %s", level, message))
}

// fakeLifecycleRunner backs both DockerService's inspect calls (docker ps /
// docker ps -a) and ContainerLifecycleService's own stop/start calls, so
// tests can control which containers are "running", which merely "exist"
// (stopped but present), and which stop/start/inspect calls fail. A
// container absent from both running and existing is treated as not
// present on the host at all.
type fakeLifecycleRunner struct {
	running     map[string]bool
	existing    map[string]bool
	inspectErrs map[string]error
	stopErrs    map[string]error
	startErrs   map[string]error

	stopped []string
	started []string
}

func (runner *fakeLifecycleRunner) Run(name string, args ...string) error {
	if name != "docker" || len(args) < 2 {
		return fmt.Errorf("unexpected command: %s %v", name, args)
	}

	container := args[1]

	switch args[0] {
	case "stop":
		runner.stopped = append(runner.stopped, container)
		return runner.stopErrs[container]
	case "start":
		runner.started = append(runner.started, container)
		return runner.startErrs[container]
	default:
		return fmt.Errorf("unexpected docker subcommand: %s", args[0])
	}
}

func (runner *fakeLifecycleRunner) Output(name string, args ...string) ([]byte, error) {
	container := containerFromFilterArg(args)

	if err, ok := runner.inspectErrs[container]; ok {
		return nil, err
	}

	all := len(args) > 1 && args[1] == "-a"

	if runner.running[container] || (all && runner.existing[container]) {
		return []byte("abc123\n"), nil
	}

	return nil, nil
}

func (runner *fakeLifecycleRunner) LookPath(file string) (string, error) {
	return file, nil
}

// containerFromFilterArg extracts the container name back out of the
// "name=^/<container>$" docker ps filter built by DockerService.
func containerFromFilterArg(args []string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, "name=^/") && strings.HasSuffix(arg, "$") {
			return strings.TrimSuffix(strings.TrimPrefix(arg, "name=^/"), "$")
		}
	}

	return ""
}

func TestStopRunningContainers_StopsOnlyRunningContainers(t *testing.T) {
	runner := &fakeLifecycleRunner{running: map[string]bool{"a": true, "b": false}}
	service := ContainerLifecycleService{
		Docker: DockerService{Runner: runner},
		Runner: runner,
		Logger: &fakeLifecycleLogger{},
	}

	stopped, err := service.StopRunningContainers([]string{"a", "b"}, "backup")
	if err != nil {
		t.Fatalf("StopRunningContainers returned error: %v", err)
	}

	if !reflect.DeepEqual(stopped, []string{"a"}) {
		t.Fatalf("expected [a] to be stopped, got %#v", stopped)
	}

	if !reflect.DeepEqual(runner.stopped, []string{"a"}) {
		t.Fatalf("expected docker stop to be called only for [a], got %#v", runner.stopped)
	}
}

func TestStopRunningContainers_DockerStopFailureIsReportedNotSwallowed(t *testing.T) {
	runner := &fakeLifecycleRunner{
		running:  map[string]bool{"a": true, "b": true},
		stopErrs: map[string]error{"a": fmt.Errorf("docker daemon unreachable")},
	}
	service := ContainerLifecycleService{
		Docker: DockerService{Runner: runner},
		Runner: runner,
		Logger: &fakeLifecycleLogger{},
	}

	stopped, err := service.StopRunningContainers([]string{"a", "b"}, "backup")
	if err == nil {
		t.Fatal("expected StopRunningContainers to return an error when docker stop fails")
	}

	if !strings.Contains(err.Error(), "a") {
		t.Fatalf("expected error to mention failed container %q, got %q", "a", err.Error())
	}

	if !reflect.DeepEqual(stopped, []string{"b"}) {
		t.Fatalf("expected only successfully-stopped [b] to be returned, got %#v", stopped)
	}
}

func TestStopRunningContainers_InspectFailureIsReportedNotSwallowed(t *testing.T) {
	runner := &fakeLifecycleRunner{
		running:     map[string]bool{"b": true},
		inspectErrs: map[string]error{"a": fmt.Errorf("docker ps failed")},
	}
	service := ContainerLifecycleService{
		Docker: DockerService{Runner: runner},
		Runner: runner,
		Logger: &fakeLifecycleLogger{},
	}

	stopped, err := service.StopRunningContainers([]string{"a", "b"}, "backup")
	if err == nil {
		t.Fatal("expected StopRunningContainers to return an error when inspecting a container fails")
	}

	if !strings.Contains(err.Error(), "a") {
		t.Fatalf("expected error to mention container %q whose state is unknown, got %q", "a", err.Error())
	}

	if !reflect.DeepEqual(stopped, []string{"b"}) {
		t.Fatalf("expected only successfully-stopped [b] to be returned, got %#v", stopped)
	}
}

func TestStopRunningContainers_NoFailuresReturnsNilError(t *testing.T) {
	runner := &fakeLifecycleRunner{running: map[string]bool{"a": true}}
	service := ContainerLifecycleService{
		Docker: DockerService{Runner: runner},
		Runner: runner,
		Logger: &fakeLifecycleLogger{},
	}

	_, err := service.StopRunningContainers([]string{"a"}, "backup")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestStopRunningContainers_EmptyContainersListIsNoOp(t *testing.T) {
	runner := &fakeLifecycleRunner{}
	service := ContainerLifecycleService{
		Docker: DockerService{Runner: runner},
		Runner: runner,
		Logger: &fakeLifecycleLogger{},
	}

	stopped, err := service.StopRunningContainers(nil, "backup")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if stopped != nil {
		t.Fatalf("expected nil stopped containers, got %#v", stopped)
	}

	if len(runner.stopped) != 0 {
		t.Fatalf("expected docker stop to never be called, got %#v", runner.stopped)
	}
}

func TestStopRunningContainers_DryRunDoesNotCallDockerStop(t *testing.T) {
	runner := &fakeLifecycleRunner{running: map[string]bool{"a": true}}
	service := ContainerLifecycleService{
		Docker:  DockerService{Runner: runner},
		Runner:  runner,
		Logger:  &fakeLifecycleLogger{},
		Options: &Options{DryRun: true},
	}

	stopped, err := service.StopRunningContainers([]string{"a"}, "backup")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if !reflect.DeepEqual(stopped, []string{"a"}) {
		t.Fatalf("expected [a] to be reported as stopped, got %#v", stopped)
	}

	if len(runner.stopped) != 0 {
		t.Fatalf("expected docker stop to never actually be called in dry-run, got %#v", runner.stopped)
	}
}

func TestStartStoppedContainers_EmptyListIsNoOp(t *testing.T) {
	runner := &fakeLifecycleRunner{}
	service := ContainerLifecycleService{
		Docker: DockerService{Runner: runner},
		Runner: runner,
		Logger: &fakeLifecycleLogger{},
	}

	if err := service.StartStoppedContainers(nil, "backup"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(runner.started) != 0 {
		t.Fatalf("expected docker start to never be called, got %#v", runner.started)
	}
}

func TestStartStoppedContainers_StartsExistingContainers(t *testing.T) {
	runner := &fakeLifecycleRunner{existing: map[string]bool{"a": true}}
	service := ContainerLifecycleService{
		Docker: DockerService{Runner: runner},
		Runner: runner,
		Logger: &fakeLifecycleLogger{},
	}

	if err := service.StartStoppedContainers([]string{"a"}, "backup"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if !reflect.DeepEqual(runner.started, []string{"a"}) {
		t.Fatalf("expected docker start to be called for [a], got %#v", runner.started)
	}
}

func TestStartStoppedContainers_SkipsContainerThatNoLongerExists(t *testing.T) {
	runner := &fakeLifecycleRunner{}
	service := ContainerLifecycleService{
		Docker: DockerService{Runner: runner},
		Runner: runner,
		Logger: &fakeLifecycleLogger{},
	}

	if err := service.StartStoppedContainers([]string{"a"}, "backup"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(runner.started) != 0 {
		t.Fatalf("expected docker start to never be called for a missing container, got %#v", runner.started)
	}
}

func TestStartStoppedContainers_InspectFailureSkipsButDoesNotAbort(t *testing.T) {
	runner := &fakeLifecycleRunner{
		existing:    map[string]bool{"b": true},
		inspectErrs: map[string]error{"a": fmt.Errorf("docker ps failed")},
	}
	service := ContainerLifecycleService{
		Docker: DockerService{Runner: runner},
		Runner: runner,
		Logger: &fakeLifecycleLogger{},
	}

	if err := service.StartStoppedContainers([]string{"a", "b"}, "backup"); err != nil {
		t.Fatalf("expected nil error even when a container's state can't be inspected, got %v", err)
	}

	if !reflect.DeepEqual(runner.started, []string{"b"}) {
		t.Fatalf("expected only [b] to be started, got %#v", runner.started)
	}
}

func TestStartStoppedContainers_StartFailureIsLoggedButDoesNotAbort(t *testing.T) {
	runner := &fakeLifecycleRunner{
		existing:  map[string]bool{"a": true, "b": true},
		startErrs: map[string]error{"a": fmt.Errorf("docker daemon unreachable")},
	}
	service := ContainerLifecycleService{
		Docker: DockerService{Runner: runner},
		Runner: runner,
		Logger: &fakeLifecycleLogger{},
	}

	if err := service.StartStoppedContainers([]string{"a", "b"}, "backup"); err != nil {
		t.Fatalf("expected nil error even when docker start fails, got %v", err)
	}

	if !reflect.DeepEqual(runner.started, []string{"a", "b"}) {
		t.Fatalf("expected docker start to be attempted for both [a b], got %#v", runner.started)
	}
}

func TestStartStoppedContainers_DryRunDoesNotCallDockerStart(t *testing.T) {
	runner := &fakeLifecycleRunner{existing: map[string]bool{"a": true}}
	service := ContainerLifecycleService{
		Docker:  DockerService{Runner: runner},
		Runner:  runner,
		Logger:  &fakeLifecycleLogger{},
		Options: &Options{DryRun: true},
	}

	if err := service.StartStoppedContainers([]string{"a"}, "backup"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(runner.started) != 0 {
		t.Fatalf("expected docker start to never actually be called in dry-run, got %#v", runner.started)
	}
}
