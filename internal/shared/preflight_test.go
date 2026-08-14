package shared

import (
	"os"
	"path/filepath"
	"testing"
)

type fakePreflightRunner struct {
	lookPathErrs map[string]error
}

func (runner fakePreflightRunner) Run(name string, args ...string) error { return nil }

func (runner fakePreflightRunner) Output(name string, args ...string) ([]byte, error) {
	return nil, nil
}

func (runner fakePreflightRunner) LookPath(file string) (string, error) {
	if err, ok := runner.lookPathErrs[file]; ok {
		return "", err
	}
	return file, nil
}

// preflightFixture bundles a valid, all-checks-pass PreflightChecks call so
// individual tests only need to break the one thing they're testing.
type preflightFixture struct {
	configPath string
	sourceRoot string
	destRoot   string
	config     DackupConfig
	configs    []ContainerConfig
	resolver   PathResolver
	runner     fakePreflightRunner
}

func newPreflightFixture(t *testing.T) preflightFixture {
	t.Helper()

	dir := t.TempDir()
	sourceRoot := filepath.Join(dir, "src")
	destRoot := filepath.Join(dir, "dst")
	configPath := filepath.Join(dir, "config.json")

	for _, path := range []string{sourceRoot, destRoot} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("failed to create %s: %v", path, err)
		}
	}

	if err := os.WriteFile(configPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	containerPath := filepath.Join(sourceRoot, "app")
	if err := os.MkdirAll(containerPath, 0o755); err != nil {
		t.Fatalf("failed to create configured path: %v", err)
	}

	return preflightFixture{
		configPath: configPath,
		sourceRoot: sourceRoot,
		destRoot:   destRoot,
		config:     DackupConfig{User: "user", Group: "group"},
		configs:    []ContainerConfig{{Container: "app", Paths: []string{"/app"}}},
		resolver:   PathResolver{SourceRoot: sourceRoot, DestinationRoot: destRoot},
		runner:     fakePreflightRunner{},
	}
}

func (fixture preflightFixture) run() error {
	return PreflightChecks(
		"backup",
		fixture.configPath,
		fixture.config,
		fixture.configs,
		fixture.sourceRoot,
		fixture.destRoot,
		fixture.resolver,
		OSFileSystem{},
		fixture.runner,
	)
}

func TestPreflightChecks_PassesWithValidFixture(t *testing.T) {
	fixture := newPreflightFixture(t)

	if err := fixture.run(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestPreflightChecks_MissingConfigFileReturnsError(t *testing.T) {
	fixture := newPreflightFixture(t)
	fixture.configPath = filepath.Join(t.TempDir(), "missing.json")

	if err := fixture.run(); err == nil {
		t.Fatal("expected an error for a missing config file")
	}
}

func TestPreflightChecks_MissingUserReturnsError(t *testing.T) {
	fixture := newPreflightFixture(t)
	fixture.config.User = ""

	if err := fixture.run(); err == nil {
		t.Fatal("expected an error for a missing user")
	}
}

func TestPreflightChecks_MissingGroupReturnsError(t *testing.T) {
	fixture := newPreflightFixture(t)
	fixture.config.Group = ""

	if err := fixture.run(); err == nil {
		t.Fatal("expected an error for a missing group")
	}
}

func TestPreflightChecks_MissingSourceDirReturnsError(t *testing.T) {
	fixture := newPreflightFixture(t)
	fixture.sourceRoot = filepath.Join(t.TempDir(), "does-not-exist")

	if err := fixture.run(); err == nil {
		t.Fatal("expected an error for a missing source directory")
	}
}

func TestPreflightChecks_SourceDirIsAFileReturnsError(t *testing.T) {
	fixture := newPreflightFixture(t)
	filePath := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	fixture.sourceRoot = filePath

	if err := fixture.run(); err == nil {
		t.Fatal("expected an error when the source root is a file, not a directory")
	}
}

func TestPreflightChecks_MissingDestinationDirReturnsError(t *testing.T) {
	fixture := newPreflightFixture(t)
	fixture.destRoot = filepath.Join(t.TempDir(), "does-not-exist")

	if err := fixture.run(); err == nil {
		t.Fatal("expected an error for a missing destination directory")
	}
}

func TestPreflightChecks_MissingBackendDirReturnsError(t *testing.T) {
	fixture := newPreflightFixture(t)
	fixture.config.BackendDir = filepath.Join(t.TempDir(), "does-not-exist")

	if err := fixture.run(); err == nil {
		t.Fatal("expected an error for a configured but missing backend directory")
	}
}

func TestPreflightChecks_EmptyBackendDirIsSkipped(t *testing.T) {
	fixture := newPreflightFixture(t)
	fixture.config.BackendDir = "   "

	if err := fixture.run(); err != nil {
		t.Fatalf("expected an all-whitespace backend dir to be treated as unset, got %v", err)
	}
}

func TestPreflightChecks_MissingDockerReturnsError(t *testing.T) {
	fixture := newPreflightFixture(t)
	fixture.runner = fakePreflightRunner{lookPathErrs: map[string]error{"docker": os.ErrNotExist}}

	err := fixture.run()
	if err == nil {
		t.Fatal("expected an error when docker is not on PATH")
	}
}

func TestPreflightChecks_MissingRsyncReturnsError(t *testing.T) {
	fixture := newPreflightFixture(t)
	fixture.runner = fakePreflightRunner{lookPathErrs: map[string]error{"rsync": os.ErrNotExist}}

	err := fixture.run()
	if err == nil {
		t.Fatal("expected an error when rsync is not on PATH")
	}
}

func TestPreflightChecks_MissingConfiguredPathReturnsError(t *testing.T) {
	fixture := newPreflightFixture(t)
	fixture.configs = []ContainerConfig{{Container: "app", Paths: []string{"/does-not-exist"}}}

	if err := fixture.run(); err == nil {
		t.Fatal("expected an error for a configured path that does not exist under the source root")
	}
}

func TestPreflightChecks_ConfiguredPathIsAFileReturnsError(t *testing.T) {
	fixture := newPreflightFixture(t)
	filePath := filepath.Join(fixture.sourceRoot, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	fixture.configs = []ContainerConfig{{Container: "app", Paths: []string{"/not-a-dir"}}}

	if err := fixture.run(); err == nil {
		t.Fatal("expected an error when a configured path resolves to a file, not a directory")
	}
}
