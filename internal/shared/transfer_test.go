package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type fakeTransferLogger struct {
	lines []string
}

func (logger *fakeTransferLogger) Log(level string, message string) {
	logger.lines = append(logger.lines, fmt.Sprintf("[%s] %s", level, message))
}

type fakeTransferRunner struct {
	runErr      error
	ranCommands [][]string
}

func (runner *fakeTransferRunner) Run(name string, args ...string) error {
	runner.ranCommands = append(runner.ranCommands, append([]string{name}, args...))
	return runner.runErr
}

func (runner *fakeTransferRunner) Output(name string, args ...string) ([]byte, error) {
	return nil, nil
}

func (runner *fakeTransferRunner) LookPath(file string) (string, error) {
	return file, nil
}

type fakeTransferFS struct {
	mkdirAllErr error
}

func (fs fakeTransferFS) Stat(name string) (os.FileInfo, error) {
	return OSFileSystem{}.Stat(name)
}

func (fs fakeTransferFS) MkdirAll(path string, perm os.FileMode) error {
	if fs.mkdirAllErr != nil {
		return fs.mkdirAllErr
	}
	return OSFileSystem{}.MkdirAll(path, perm)
}

func (fs fakeTransferFS) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return OSFileSystem{}.OpenFile(name, flag, perm)
}

func TestTransferService_SinglePath_DryRunSkipsMkdirAndRsync(t *testing.T) {
	runner := &fakeTransferRunner{}
	service := TransferService{
		Direction: TransferBackup,
		Options:   &Options{DryRun: true},
		FS:        fakeTransferFS{},
		Runner:    runner,
		Logger:    &fakeTransferLogger{},
	}

	dstPath := filepath.Join(t.TempDir(), "does-not-exist-yet")

	if err := service.SinglePath("app", t.TempDir(), dstPath); err != nil {
		t.Fatalf("SinglePath returned error: %v", err)
	}

	if len(runner.ranCommands) != 0 {
		t.Fatalf("expected no commands to run in dry-run mode, got %#v", runner.ranCommands)
	}

	if _, err := os.Stat(dstPath); err == nil {
		t.Fatal("expected dry-run to not create the destination directory")
	}
}

func TestTransferService_SinglePath_RunsRsyncAndCreatesDestination(t *testing.T) {
	runner := &fakeTransferRunner{}
	service := TransferService{
		Direction: TransferBackup,
		FS:        fakeTransferFS{},
		Runner:    runner,
		Logger:    &fakeTransferLogger{},
	}

	srcPath := t.TempDir()
	dstPath := filepath.Join(t.TempDir(), "created")

	if err := service.SinglePath("app", srcPath, dstPath); err != nil {
		t.Fatalf("SinglePath returned error: %v", err)
	}

	if info, err := os.Stat(dstPath); err != nil || !info.IsDir() {
		t.Fatalf("expected destination directory to be created, stat error: %v", err)
	}

	if len(runner.ranCommands) != 1 || runner.ranCommands[0][0] != "rsync" {
		t.Fatalf("expected exactly one rsync invocation, got %#v", runner.ranCommands)
	}
}

func TestTransferService_SinglePath_MkdirFailureReturnsError(t *testing.T) {
	runner := &fakeTransferRunner{}
	service := TransferService{
		Direction: TransferBackup,
		FS:        fakeTransferFS{mkdirAllErr: fmt.Errorf("permission denied")},
		Runner:    runner,
		Logger:    &fakeTransferLogger{},
	}

	if err := service.SinglePath("app", t.TempDir(), filepath.Join(t.TempDir(), "dst")); err == nil {
		t.Fatal("expected an error when creating the destination directory fails")
	}

	if len(runner.ranCommands) != 0 {
		t.Fatalf("expected rsync to never run when mkdir fails, got %#v", runner.ranCommands)
	}
}

func TestTransferService_SinglePath_RsyncFailureReturnsError(t *testing.T) {
	runner := &fakeTransferRunner{runErr: fmt.Errorf("rsync exited 23")}
	service := TransferService{
		Direction: TransferBackup,
		LogFile:   "/var/log/dackup-test.log",
		FS:        fakeTransferFS{},
		Runner:    runner,
		Logger:    &fakeTransferLogger{},
	}

	err := service.SinglePath("app", t.TempDir(), filepath.Join(t.TempDir(), "dst"))
	if err == nil {
		t.Fatal("expected an error when rsync fails")
	}
}

func TestTransferService_Run_SkipsDuplicatePathsAndEmptyConfigs(t *testing.T) {
	runner := &fakeTransferRunner{}
	srcRoot := t.TempDir()
	dstRoot := t.TempDir()

	if err := os.MkdirAll(filepath.Join(srcRoot, "shared"), 0o755); err != nil {
		t.Fatalf("failed to create source path: %v", err)
	}

	service := TransferService{
		Direction: TransferBackup,
		SourceDir: srcRoot,
		DestDir:   dstRoot,
		FS:        fakeTransferFS{},
		Runner:    runner,
		Logger:    &fakeTransferLogger{},
		Paths:     PathResolver{SourceRoot: srcRoot, DestinationRoot: dstRoot},
	}

	configs := []ContainerConfig{
		{Container: "a", Paths: []string{"/shared"}},
		{Container: "b", Paths: []string{"/shared"}},
		{Container: "c"},
	}

	if err := service.Run(configs); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(runner.ranCommands) != 1 {
		t.Fatalf("expected the duplicate shared path to only be transferred once, got %d commands: %#v", len(runner.ranCommands), runner.ranCommands)
	}
}

func TestTransferService_Run_SkipsEmptyConfiguredPath(t *testing.T) {
	runner := &fakeTransferRunner{}
	srcRoot := t.TempDir()
	dstRoot := t.TempDir()

	service := TransferService{
		Direction: TransferBackup,
		SourceDir: srcRoot,
		DestDir:   dstRoot,
		FS:        fakeTransferFS{},
		Runner:    runner,
		Logger:    &fakeTransferLogger{},
		Paths:     PathResolver{SourceRoot: srcRoot, DestinationRoot: dstRoot},
	}

	configs := []ContainerConfig{{Container: "a", Paths: []string{"/"}}}

	if err := service.Run(configs); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(runner.ranCommands) != 0 {
		t.Fatalf("expected an empty configured path to be skipped, got %#v", runner.ranCommands)
	}
}

func TestTransferService_Run_PropagatesSinglePathError(t *testing.T) {
	srcRoot := t.TempDir()
	dstRoot := t.TempDir()

	service := TransferService{
		Direction: TransferBackup,
		SourceDir: srcRoot,
		DestDir:   dstRoot,
		FS:        fakeTransferFS{mkdirAllErr: fmt.Errorf("permission denied")},
		Runner:    &fakeTransferRunner{},
		Logger:    &fakeTransferLogger{},
		Paths:     PathResolver{SourceRoot: srcRoot, DestinationRoot: dstRoot},
	}

	configs := []ContainerConfig{{Container: "a", Paths: []string{"/app"}}}

	if err := service.Run(configs); err == nil {
		t.Fatal("expected Run to propagate a failing path's error")
	}
}

func TestTransferService_FixBackupOwnership_DryRunSkipsChown(t *testing.T) {
	runner := &fakeTransferRunner{}
	service := TransferService{
		DestDir: "/data",
		Options: &Options{DryRun: true},
		Runner:  runner,
		Logger:  &fakeTransferLogger{},
	}

	if err := service.FixBackupOwnership("user", "group"); err != nil {
		t.Fatalf("FixBackupOwnership returned error: %v", err)
	}

	if len(runner.ranCommands) != 0 {
		t.Fatalf("expected no chown call in dry-run mode, got %#v", runner.ranCommands)
	}
}

func TestTransferService_FixBackupOwnership_RunsChown(t *testing.T) {
	runner := &fakeTransferRunner{}
	service := TransferService{
		DestDir: "/data",
		Runner:  runner,
		Logger:  &fakeTransferLogger{},
	}

	if err := service.FixBackupOwnership("user", "group"); err != nil {
		t.Fatalf("FixBackupOwnership returned error: %v", err)
	}

	if len(runner.ranCommands) != 1 || runner.ranCommands[0][0] != "chown" {
		t.Fatalf("expected exactly one chown invocation, got %#v", runner.ranCommands)
	}
}

func TestTransferService_FixBackupOwnership_FailureReturnsError(t *testing.T) {
	runner := &fakeTransferRunner{runErr: fmt.Errorf("chown: operation not permitted")}
	service := TransferService{
		DestDir: "/data",
		Runner:  runner,
		Logger:  &fakeTransferLogger{},
	}

	if err := service.FixBackupOwnership("user", "group"); err == nil {
		t.Fatal("expected an error when chown fails")
	}
}

func TestTransferService_FixRestoreOwnership_DryRunSkipsChown(t *testing.T) {
	runner := &fakeTransferRunner{}
	service := TransferService{
		DestDir: "/data",
		Options: &Options{DryRun: true},
		Runner:  runner,
		Logger:  &fakeTransferLogger{},
		Paths:   PathResolver{SourceRoot: "/src", DestinationRoot: "/data"},
	}

	configs := []ContainerConfig{{Container: "app", Paths: []string{"/app"}}}

	if err := service.FixRestoreOwnership(configs, "user", "group"); err != nil {
		t.Fatalf("FixRestoreOwnership returned error: %v", err)
	}

	if len(runner.ranCommands) != 0 {
		t.Fatalf("expected no chown calls in dry-run mode, got %#v", runner.ranCommands)
	}
}

func TestTransferService_FixRestoreOwnership_DedupesPathsAcrossContainers(t *testing.T) {
	runner := &fakeTransferRunner{}
	service := TransferService{
		DestDir: "/data",
		Runner:  runner,
		Logger:  &fakeTransferLogger{},
		Paths:   PathResolver{SourceRoot: "/src", DestinationRoot: "/data"},
	}

	configs := []ContainerConfig{
		{Container: "a", Paths: []string{"/shared"}},
		{Container: "b", Paths: []string{"/shared"}},
	}

	if err := service.FixRestoreOwnership(configs, "user", "group"); err != nil {
		t.Fatalf("FixRestoreOwnership returned error: %v", err)
	}

	if len(runner.ranCommands) != 1 {
		t.Fatalf("expected the duplicate shared path to only be chowned once, got %d commands: %#v", len(runner.ranCommands), runner.ranCommands)
	}
}

func TestTransferService_FixRestoreOwnership_FailureReturnsError(t *testing.T) {
	runner := &fakeTransferRunner{runErr: fmt.Errorf("chown: operation not permitted")}
	service := TransferService{
		DestDir: "/data",
		Runner:  runner,
		Logger:  &fakeTransferLogger{},
		Paths:   PathResolver{SourceRoot: "/src", DestinationRoot: "/data"},
	}

	configs := []ContainerConfig{{Container: "app", Paths: []string{"/app"}}}

	if err := service.FixRestoreOwnership(configs, "user", "group"); err == nil {
		t.Fatal("expected an error when chown fails")
	}
}

func TestTransferService_fileSystem_DefaultsToOSFileSystem(t *testing.T) {
	service := TransferService{}

	if _, ok := service.fileSystem().(OSFileSystem); !ok {
		t.Fatalf("expected default file system to be OSFileSystem, got %T", service.fileSystem())
	}
}

func TestTransferService_fileSystem_UsesConfiguredFS(t *testing.T) {
	fs := fakeTransferFS{}
	service := TransferService{FS: fs}

	if service.fileSystem() != FileSystem(fs) {
		t.Fatal("expected the configured FS to be returned unchanged")
	}
}

func TestTransferService_commandRunner_DefaultsToOSCommandRunner(t *testing.T) {
	service := TransferService{}

	if _, ok := service.commandRunner().(OSCommandRunner); !ok {
		t.Fatalf("expected default command runner to be OSCommandRunner, got %T", service.commandRunner())
	}
}

func TestTransferService_commandRunner_UsesConfiguredRunner(t *testing.T) {
	runner := &fakeTransferRunner{}
	service := TransferService{Runner: runner}

	if service.commandRunner() != CommandRunner(runner) {
		t.Fatal("expected the configured runner to be returned unchanged")
	}
}

func TestTransferService_logger_DefaultsToFileLogger(t *testing.T) {
	service := TransferService{LogFile: filepath.Join(t.TempDir(), "transfer.log")}

	if _, ok := service.logger().(FileLogger); !ok {
		t.Fatalf("expected default logger to be FileLogger, got %T", service.logger())
	}
}

func TestTransferService_logger_UsesConfiguredLogger(t *testing.T) {
	logger := &fakeTransferLogger{}
	service := TransferService{Logger: logger}

	if service.logger() != Logger(logger) {
		t.Fatal("expected the configured logger to be returned unchanged")
	}
}

func TestTransferService_pathResolver_DefaultsToSourceAndDestDir(t *testing.T) {
	service := TransferService{SourceDir: "/data", DestDir: "/staging"}

	want := PathResolver{SourceRoot: "/data", DestinationRoot: "/staging"}
	if got := service.pathResolver(); got != want {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestTransferService_pathResolver_UsesConfiguredPaths(t *testing.T) {
	configured := PathResolver{SourceRoot: "/configured-src", DestinationRoot: "/configured-dst"}
	service := TransferService{SourceDir: "/data", DestDir: "/staging", Paths: configured}

	if got := service.pathResolver(); got != configured {
		t.Fatalf("expected %#v, got %#v", configured, got)
	}
}

func TestTransferService_DirectionWording_Restore(t *testing.T) {
	service := TransferService{Direction: TransferRestore}

	if got := service.progressVerb(); got != "Restoring" {
		t.Fatalf("expected progressVerb %q, got %q", "Restoring", got)
	}

	if got := service.pastTenseAction(); got != "restored" {
		t.Fatalf("expected pastTenseAction %q, got %q", "restored", got)
	}

	if got := service.titleAction(); got != "Restore" {
		t.Fatalf("expected titleAction %q, got %q", "Restore", got)
	}
}

func TestTransferService_DirectionWording_Backup(t *testing.T) {
	service := TransferService{Direction: TransferBackup}

	if got := service.progressVerb(); got != "Backing up" {
		t.Fatalf("expected progressVerb %q, got %q", "Backing up", got)
	}

	if got := service.pastTenseAction(); got != "backed up" {
		t.Fatalf("expected pastTenseAction %q, got %q", "backed up", got)
	}

	if got := service.titleAction(); got != "Backup" {
		t.Fatalf("expected titleAction %q, got %q", "Backup", got)
	}
}
