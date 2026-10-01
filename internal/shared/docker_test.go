package shared

import (
	"fmt"
	"testing"
)

// fakeDockerRunner backs DockerService's "docker ps"/"docker ps -a" calls,
// reporting a container as found only for the ps mode(s) listed in modes.
type fakeDockerRunner struct {
	// found maps container name to which "docker ps" modes should report it
	// as found: "running" for a plain "docker ps" call, "all" for "docker ps
	// -a". A container present under "all" but not "running" exists but is
	// stopped.
	found map[string]map[string]bool
	err   error
}

func (runner fakeDockerRunner) Run(name string, args ...string) error {
	return fmt.Errorf("unexpected Run call: %s %v", name, args)
}

func (runner fakeDockerRunner) Output(name string, args ...string) ([]byte, error) {
	if runner.err != nil {
		return nil, runner.err
	}

	mode := "running"
	if len(args) > 1 && args[1] == "-a" {
		mode = "all"
	}

	container := containerFromFilterArg(args)
	if runner.found[container][mode] {
		return []byte("abc123\n"), nil
	}

	return nil, nil
}

func (runner fakeDockerRunner) LookPath(file string) (string, error) {
	return file, nil
}

func TestDockerService_ContainerRunning_ReportsRunningContainer(t *testing.T) {
	service := DockerService{Runner: fakeDockerRunner{
		found: map[string]map[string]bool{"app": {"running": true}},
	}}

	running, err := service.ContainerRunning("app")
	if err != nil {
		t.Fatalf("ContainerRunning returned error: %v", err)
	}

	if !running {
		t.Fatal("expected app to be reported as running")
	}
}

func TestDockerService_ContainerRunning_ReportsNotRunning(t *testing.T) {
	service := DockerService{Runner: fakeDockerRunner{found: map[string]map[string]bool{}}}

	running, err := service.ContainerRunning("app")
	if err != nil {
		t.Fatalf("ContainerRunning returned error: %v", err)
	}

	if running {
		t.Fatal("expected app to be reported as not running")
	}
}

func TestDockerService_ContainerRunning_PropagatesRunnerError(t *testing.T) {
	service := DockerService{Runner: fakeDockerRunner{err: fmt.Errorf("docker daemon unreachable")}}

	if _, err := service.ContainerRunning("app"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDockerService_ContainerExists_ReportsExistingStoppedContainer(t *testing.T) {
	service := DockerService{Runner: fakeDockerRunner{
		found: map[string]map[string]bool{"app": {"all": true}},
	}}

	exists, err := service.ContainerExists("app")
	if err != nil {
		t.Fatalf("ContainerExists returned error: %v", err)
	}

	if !exists {
		t.Fatal("expected app to be reported as existing")
	}
}

func TestDockerService_ContainerExists_ReportsMissingContainer(t *testing.T) {
	service := DockerService{Runner: fakeDockerRunner{found: map[string]map[string]bool{}}}

	exists, err := service.ContainerExists("app")
	if err != nil {
		t.Fatalf("ContainerExists returned error: %v", err)
	}

	if exists {
		t.Fatal("expected app to be reported as not existing")
	}
}

func TestDockerService_ContainerExists_PropagatesRunnerError(t *testing.T) {
	service := DockerService{Runner: fakeDockerRunner{err: fmt.Errorf("docker daemon unreachable")}}

	if _, err := service.ContainerExists("app"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDockerService_ContainerRunning_UsesOSCommandRunnerWhenRunnerIsUnset(t *testing.T) {
	if _, err := (OSCommandRunner{}).LookPath("docker"); err != nil {
		t.Skip("docker binary not found on PATH; skipping nil-Runner default check")
	}

	service := DockerService{}

	// Exercises the nil-Runner default-to-OSCommandRunner branch in
	// queryContainer; whether the docker daemon is actually reachable here
	// isn't the point, so the result itself isn't asserted on.
	if _, err := service.ContainerRunning("dackup-nonexistent-container-xyz"); err != nil {
		t.Logf("ContainerRunning returned error (expected if no docker daemon is running): %v", err)
	}
}

func TestValidateContainerName_AcceptsDockerNames(t *testing.T) {
	for _, name := range []string{"a", "app", "A1", "my_app.v2-1", "paperless-ngx", "db.1"} {
		if err := ValidateContainerName(name); err != nil {
			t.Errorf("ValidateContainerName(%q) returned error %v, want nil", name, err)
		}
	}
}

func TestValidateContainerName_RejectsUnsafeNames(t *testing.T) {
	for _, name := range []string{"", "..", "../x", "x/y", "/app", ".hidden", "-rm", "_x", ".*", "a b", "a$", "app\n", "a|b"} {
		if err := ValidateContainerName(name); err == nil {
			t.Errorf("ValidateContainerName(%q) returned nil, want an error", name)
		}
	}
}

// recordingDockerRunner records the args of every Output call.
type recordingDockerRunner struct {
	calls *[][]string
}

func (runner recordingDockerRunner) Run(name string, args ...string) error { return nil }

func (runner recordingDockerRunner) Output(name string, args ...string) ([]byte, error) {
	*runner.calls = append(*runner.calls, args)
	return nil, nil
}

func (runner recordingDockerRunner) LookPath(file string) (string, error) { return file, nil }

func TestDockerService_QuotesContainerNameInFilter(t *testing.T) {
	var calls [][]string
	service := DockerService{Runner: recordingDockerRunner{calls: &calls}}

	if _, err := service.ContainerRunning("db.1"); err != nil {
		t.Fatalf("ContainerRunning returned error: %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("expected 1 docker call, got %d", len(calls))
	}

	want := `name=^/db\.1$`
	found := false
	for _, arg := range calls[0] {
		if arg == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected filter arg %q in %v", want, calls[0])
	}
}
