package backup

import (
	"dackup/internal/shared"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type fakeOrchestrationLogger struct {
	lines []string
}

func (logger *fakeOrchestrationLogger) Log(level string, message string) {
	logger.lines = append(logger.lines, fmt.Sprintf("[%s] %s", level, message))
}

// fakeOrchestrationRunner stands in for docker/rsync/chown so tests never
// spawn real subprocesses. Output() answers docker ps queries (container
// state); Run() answers docker stop/start plus rsync/chown.
type fakeOrchestrationRunner struct {
	running  map[string]bool
	stopErrs map[string]error

	stopped  []string
	started  []string
	ranRsync bool
	ranChown bool
}

func (runner *fakeOrchestrationRunner) LookPath(file string) (string, error) {
	return file, nil
}

func (runner *fakeOrchestrationRunner) Output(name string, args ...string) ([]byte, error) {
	container := containerFromFilterArg(args)
	if runner.running[container] {
		return []byte("abc123\n"), nil
	}
	return nil, nil
}

func (runner *fakeOrchestrationRunner) Run(name string, args ...string) error {
	switch name {
	case "docker":
		container := args[1]
		switch args[0] {
		case "stop":
			runner.stopped = append(runner.stopped, container)
			return runner.stopErrs[container]
		case "start":
			runner.started = append(runner.started, container)
			return nil
		}
	case "rsync":
		runner.ranRsync = true
		return nil
	case "chown":
		runner.ranChown = true
		return nil
	}

	return fmt.Errorf("unexpected command: %s %v", name, args)
}

func touchFile(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}

	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
}

func containerFromFilterArg(args []string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, "name=^/") && strings.HasSuffix(arg, "$") {
			return strings.TrimSuffix(strings.TrimPrefix(arg, "name=^/"), "$")
		}
	}

	return ""
}

// testOrchestrationFixture bundles the temp dirs and fakes a runBackupWithService
// test needs, keeping individual tests focused on the scenario being verified.
type testOrchestrationFixture struct {
	srcDir  string
	dstDir  string
	runner  *fakeOrchestrationRunner
	logger  *fakeOrchestrationLogger
	service commandService
}

func newTestOrchestrationFixture(t *testing.T, running map[string]bool, stopErrs map[string]error) testOrchestrationFixture {
	t.Helper()

	srcDir := t.TempDir()
	dstDir := t.TempDir()

	runner := &fakeOrchestrationRunner{running: running, stopErrs: stopErrs}
	logger := &fakeOrchestrationLogger{}

	fs := shared.OSFileSystem{}
	paths := shared.PathResolver{SourceRoot: srcDir, DestinationRoot: dstDir}

	service := commandService{
		fs:     fs,
		runner: runner,
		logger: logger,
		paths:  paths,
		transfer: shared.TransferService{
			Direction: shared.TransferBackup,
			SourceDir: srcDir,
			DestDir:   dstDir,
			FS:        fs,
			Runner:    runner,
			Logger:    logger,
			Paths:     paths,
		},
	}

	return testOrchestrationFixture{srcDir: srcDir, dstDir: dstDir, runner: runner, logger: logger, service: service}
}

func withBackupDirs(t *testing.T, srcDir string, dstDir string) {
	t.Helper()

	originalSrcDir := backupSrcDir
	originalDstDir := backupDstDir
	backupSrcDir = srcDir
	backupDstDir = dstDir

	t.Cleanup(func() {
		backupSrcDir = originalSrcDir
		backupDstDir = originalDstDir
	})
}

func TestRunBackupWithService_HappyPath(t *testing.T) {
	fixture := newTestOrchestrationFixture(t, map[string]bool{"a": true}, nil)
	withBackupDirs(t, fixture.srcDir, fixture.dstDir)

	if err := os.MkdirAll(filepath.Join(fixture.srcDir, "data"), 0o755); err != nil {
		t.Fatalf("failed to create configured path: %v", err)
	}

	effectiveConfigPath := filepath.Join(fixture.dstDir, "config.json")
	touchFile(t, effectiveConfigPath)

	config := shared.DackupConfig{
		User:  "backupuser",
		Group: "backupgroup",
		Containers: []shared.ContainerConfig{
			{Container: "a", ToStop: true, Paths: []string{"data"}},
		},
	}

	err := runBackupWithService(fixture.service, config, effectiveConfigPath, nil)
	if err != nil {
		t.Fatalf("runBackupWithService returned error: %v", err)
	}

	if !reflect.DeepEqual(fixture.runner.stopped, []string{"a"}) {
		t.Fatalf("expected container a to be stopped, got %#v", fixture.runner.stopped)
	}

	if !reflect.DeepEqual(fixture.runner.started, []string{"a"}) {
		t.Fatalf("expected container a to be restarted, got %#v", fixture.runner.started)
	}

	if !fixture.runner.ranRsync {
		t.Fatal("expected rsync to have been invoked")
	}

	if !fixture.runner.ranChown {
		t.Fatal("expected chown to have been invoked")
	}
}

func TestRunBackupWithService_UnknownBackendFailsBeforeTouchingContainers(t *testing.T) {
	fixture := newTestOrchestrationFixture(t, map[string]bool{"a": true}, nil)
	withBackupDirs(t, fixture.srcDir, fixture.dstDir)

	effectiveConfigPath := filepath.Join(fixture.dstDir, "config.json")
	touchFile(t, effectiveConfigPath)

	config := shared.DackupConfig{
		User:    "backupuser",
		Group:   "backupgroup",
		Backend: "unknown-backend",
		Containers: []shared.ContainerConfig{
			{Container: "a", ToStop: true, Paths: []string{"data"}},
		},
	}

	err := runBackupWithService(fixture.service, config, effectiveConfigPath, nil)
	if err == nil {
		t.Fatal("expected an error for an unknown backend name")
	}

	if len(fixture.runner.stopped) != 0 {
		t.Fatalf("expected no containers to be stopped when backend resolution fails, got %#v", fixture.runner.stopped)
	}
}

func TestRunBackupWithService_StopFailureAbortsAndRestartsAlreadyStoppedContainers(t *testing.T) {
	fixture := newTestOrchestrationFixture(t,
		map[string]bool{"a": true, "b": true},
		map[string]error{"b": fmt.Errorf("docker daemon unreachable")},
	)
	withBackupDirs(t, fixture.srcDir, fixture.dstDir)

	if err := os.MkdirAll(filepath.Join(fixture.srcDir, "data"), 0o755); err != nil {
		t.Fatalf("failed to create configured path: %v", err)
	}

	effectiveConfigPath := filepath.Join(fixture.dstDir, "config.json")
	touchFile(t, effectiveConfigPath)

	config := shared.DackupConfig{
		User:  "backupuser",
		Group: "backupgroup",
		Containers: []shared.ContainerConfig{
			{Container: "a", ToStop: true, Paths: []string{"data"}},
			{Container: "b", ToStop: true, Paths: []string{"data"}},
		},
	}

	err := runBackupWithService(fixture.service, config, effectiveConfigPath, nil)
	if err == nil {
		t.Fatal("expected an error when a container fails to stop")
	}

	if !strings.Contains(err.Error(), "b") {
		t.Fatalf("expected error to mention failed container %q, got %q", "b", err.Error())
	}

	if fixture.runner.ranRsync {
		t.Fatal("expected staging to be skipped when a container fails to stop")
	}

	if !reflect.DeepEqual(fixture.runner.started, []string{"a"}) {
		t.Fatalf("expected already-stopped container a to be restarted despite the abort, got %#v", fixture.runner.started)
	}
}
