package restore

import (
	"dackup/internal/backend"
	"dackup/internal/shared"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fakeBinaryBackend implements both backend.Backend and the optional
// backend.BinaryChecker interface, so tests can exercise checkBackendBinary
// without needing a real borg/kopia config.
type fakeBinaryBackend struct {
	binaryName string
}

func (fakeBinaryBackend) Name() string                    { return "fake" }
func (fakeBinaryBackend) Backup(stagingDir string) error  { return nil }
func (fakeBinaryBackend) Restore(stagingDir string) error { return nil }
func (fb fakeBinaryBackend) BinaryName() string           { return fb.binaryName }

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
	running      map[string]bool
	stopErrs     map[string]error
	lookPathErrs map[string]error

	stopped  []string
	started  []string
	ranRsync bool
	ranChown bool
}

func (runner *fakeOrchestrationRunner) LookPath(file string) (string, error) {
	if err, ok := runner.lookPathErrs[file]; ok {
		return "", err
	}
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

func containerFromFilterArg(args []string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, "name=^/") && strings.HasSuffix(arg, "$") {
			return strings.TrimSuffix(strings.TrimPrefix(arg, "name=^/"), "$")
		}
	}

	return ""
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

// testOrchestrationFixture bundles the temp dirs and fakes a
// runRestoreWithService test needs, keeping individual tests focused on the
// scenario being verified.
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
			Direction: shared.TransferRestore,
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

func withRestoreDirs(t *testing.T, srcDir string, dstDir string) {
	t.Helper()

	originalSrcDir := restoreSrcDir
	originalDstDir := restoreDstDir
	restoreSrcDir = srcDir
	restoreDstDir = dstDir

	t.Cleanup(func() {
		restoreSrcDir = originalSrcDir
		restoreDstDir = originalDstDir
	})
}

func TestNewCommandService_BuildsServiceWithDependencies(t *testing.T) {
	service := newCommandService()

	if service.fs == nil {
		t.Fatal("expected fs to be set")
	}

	if service.runner == nil {
		t.Fatal("expected runner to be set")
	}

	if service.logger == nil {
		t.Fatal("expected logger to be set")
	}

	if service.transfer.Direction != shared.TransferRestore {
		t.Fatalf("expected transfer direction %v, got %v", shared.TransferRestore, service.transfer.Direction)
	}
}

func TestRunRestore_ReturnsErrorForMissingConfigFile(t *testing.T) {
	original := restoreConfigFile
	defer func() { restoreConfigFile = original }()
	restoreConfigFile = filepath.Join(t.TempDir(), "missing.json")

	if err := runRestore(nil, false, false); err == nil {
		t.Fatal("expected an error for a missing config file")
	}
}

func TestRunRestore_DelegatesToRunRestoreWithService(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	config := shared.DackupConfig{User: "owner", Group: "group"}
	if err := shared.WriteDackupConfig(configPath, config, nil); err != nil {
		t.Fatalf("failed to seed config: %v", err)
	}

	originalConfigPath := restoreConfigFile
	defer func() { restoreConfigFile = originalConfigPath }()
	restoreConfigFile = configPath

	// Point at nonexistent src/dst dirs so PreflightChecks fails fast and
	// deterministically, without ever touching docker/rsync/chown.
	withRestoreDirs(t, filepath.Join(dir, "does-not-exist-src"), filepath.Join(dir, "does-not-exist-dst"))

	if err := runRestore(nil, true, true); err == nil {
		t.Fatal("expected an error from the underlying preflight check")
	}
}

func TestRunRestoreWithService_HappyPath(t *testing.T) {
	fixture := newTestOrchestrationFixture(t, map[string]bool{"a": true}, nil)
	withRestoreDirs(t, fixture.srcDir, fixture.dstDir)

	if err := os.MkdirAll(filepath.Join(fixture.srcDir, "data"), 0o755); err != nil {
		t.Fatalf("failed to create configured path: %v", err)
	}

	effectiveConfigPath := filepath.Join(fixture.dstDir, "config.json")
	touchFile(t, effectiveConfigPath)

	config := shared.DackupConfig{
		User:  "restoreuser",
		Group: "restoregroup",
		Containers: []shared.ContainerConfig{
			{Container: "a", ToStop: true, Paths: []string{"data"}},
		},
	}

	err := runRestoreWithService(fixture.service, config, effectiveConfigPath, nil)
	if err != nil {
		t.Fatalf("runRestoreWithService returned error: %v", err)
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

func TestRunRestoreWithService_UnknownBackendFailsBeforeTouchingContainers(t *testing.T) {
	fixture := newTestOrchestrationFixture(t, map[string]bool{"a": true}, nil)
	withRestoreDirs(t, fixture.srcDir, fixture.dstDir)

	effectiveConfigPath := filepath.Join(fixture.dstDir, "config.json")
	touchFile(t, effectiveConfigPath)

	config := shared.DackupConfig{
		User:    "restoreuser",
		Group:   "restoregroup",
		Backend: "unknown-backend",
		Containers: []shared.ContainerConfig{
			{Container: "a", ToStop: true, Paths: []string{"data"}},
		},
	}

	err := runRestoreWithService(fixture.service, config, effectiveConfigPath, nil)
	if err == nil {
		t.Fatal("expected an error for an unknown backend name")
	}

	if len(fixture.runner.stopped) != 0 {
		t.Fatalf("expected no containers to be stopped when backend resolution fails, got %#v", fixture.runner.stopped)
	}
}

func TestRunRestoreWithService_StopFailureAbortsAndRestartsAlreadyStoppedContainers(t *testing.T) {
	fixture := newTestOrchestrationFixture(t,
		map[string]bool{"a": true, "b": true},
		map[string]error{"b": fmt.Errorf("docker daemon unreachable")},
	)
	withRestoreDirs(t, fixture.srcDir, fixture.dstDir)

	if err := os.MkdirAll(filepath.Join(fixture.srcDir, "data"), 0o755); err != nil {
		t.Fatalf("failed to create configured path: %v", err)
	}

	effectiveConfigPath := filepath.Join(fixture.dstDir, "config.json")
	touchFile(t, effectiveConfigPath)

	config := shared.DackupConfig{
		User:  "restoreuser",
		Group: "restoregroup",
		Containers: []shared.ContainerConfig{
			{Container: "a", ToStop: true, Paths: []string{"data"}},
			{Container: "b", ToStop: true, Paths: []string{"data"}},
		},
	}

	err := runRestoreWithService(fixture.service, config, effectiveConfigPath, nil)
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

func TestCheckBackendBinary_MissingBinaryReturnsError(t *testing.T) {
	runner := &fakeOrchestrationRunner{lookPathErrs: map[string]error{"borg": fmt.Errorf("not found")}}

	err := checkBackendBinary(fakeBinaryBackend{binaryName: "borg"}, runner)
	if err == nil {
		t.Fatal("expected an error when the backend binary is not on PATH")
	}
}

func TestCheckBackendBinary_BackendWithoutBinaryCheckerIsSkipped(t *testing.T) {
	runner := &fakeOrchestrationRunner{}

	resolvedBackend, err := (backend.Factory{}).GetBackend("", nil)
	if err != nil {
		t.Fatalf("failed to resolve default backend: %v", err)
	}

	if err := checkBackendBinary(resolvedBackend, runner); err != nil {
		t.Fatalf("expected no error for a backend without BinaryChecker, got %v", err)
	}
}

func TestRunRestoreWithService_MissingBackendBinaryFailsBeforeTouchingContainers(t *testing.T) {
	fixture := newTestOrchestrationFixture(t, map[string]bool{"a": true}, nil)
	withRestoreDirs(t, fixture.srcDir, fixture.dstDir)
	fixture.runner.lookPathErrs = map[string]error{"borg": fmt.Errorf("not found")}

	if err := os.MkdirAll(filepath.Join(fixture.srcDir, "data"), 0o755); err != nil {
		t.Fatalf("failed to create configured path: %v", err)
	}

	effectiveConfigPath := filepath.Join(fixture.dstDir, "config.json")
	touchFile(t, effectiveConfigPath)

	backendDir := t.TempDir()
	config := shared.DackupConfig{
		User:            "restoreuser",
		Group:           "restoregroup",
		Backend:         "borg",
		BackendDir:      backendDir,
		BackendSettings: []byte(`{"encryption":"none"}`),
		Containers: []shared.ContainerConfig{
			{Container: "a", ToStop: true, Paths: []string{"data"}},
		},
	}

	err := runRestoreWithService(fixture.service, config, effectiveConfigPath, nil)
	if err == nil {
		t.Fatal("expected an error when the configured backend's binary is missing")
	}

	if !strings.Contains(err.Error(), "borg") {
		t.Fatalf("expected error to mention the missing borg binary, got %q", err.Error())
	}

	if len(fixture.runner.stopped) != 0 {
		t.Fatalf("expected no containers to be stopped when the backend binary is missing, got %#v", fixture.runner.stopped)
	}
}
