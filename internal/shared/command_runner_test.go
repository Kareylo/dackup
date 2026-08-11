package shared

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOSCommandRunner_RunInDirWithEnvSetsWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "pwd-output.txt")

	err := OSCommandRunner{}.RunInDirWithEnv(dir, nil, "sh", "-c", "pwd > "+outputPath)
	if err != nil {
		t.Fatalf("RunInDirWithEnv returned error: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read command output: %v", err)
	}

	got := strings.TrimSpace(string(data))
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("failed to resolve temp dir: %v", err)
	}

	if got != want {
		t.Fatalf("expected command to run in %q, got %q", want, got)
	}
}

func TestOSCommandRunner_RunInDirWithEnvEmptyDirLeavesWorkingDirectoryUnchanged(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	outputPath := filepath.Join(t.TempDir(), "pwd-output.txt")

	runner := OSCommandRunner{}
	if err := runner.RunInDirWithEnv("", nil, "sh", "-c", "pwd > "+outputPath); err != nil {
		t.Fatalf("RunInDirWithEnv returned error: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read command output: %v", err)
	}

	got := strings.TrimSpace(string(data))
	want, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		t.Fatalf("failed to resolve cwd: %v", err)
	}

	if got != want {
		t.Fatalf("expected command to run in %q, got %q", want, got)
	}
}

func TestOSCommandRunner_OutputWithEnvSetsExtraEnvVars(t *testing.T) {
	output, err := OSCommandRunner{}.OutputWithEnv([]string{"DACKUP_TEST_VAR=hello"}, "sh", "-c", "echo $DACKUP_TEST_VAR")
	if err != nil {
		t.Fatalf("OutputWithEnv returned error: %v", err)
	}

	if got := strings.TrimSpace(string(output)); got != "hello" {
		t.Fatalf("expected output %q, got %q", "hello", got)
	}
}

func TestLoggedCommandRunner_RunInDirWithEnvWritesToLogFileAndRunsInDir(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "run.log")
	workDir := t.TempDir()

	runner := LoggedCommandRunner{LogFile: logFile}

	if err := runner.RunInDirWithEnv(workDir, []string{"DACKUP_TEST_VAR=hi"}, "sh", "-c", "pwd; echo $DACKUP_TEST_VAR"); err != nil {
		t.Fatalf("RunInDirWithEnv returned error: %v", err)
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	resolvedWorkDir, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		t.Fatalf("failed to resolve work dir: %v", err)
	}

	logged := string(data)
	if !strings.Contains(logged, resolvedWorkDir) {
		t.Fatalf("expected log file to contain working directory %q, got %q", resolvedWorkDir, logged)
	}

	if !strings.Contains(logged, "hi") {
		t.Fatalf("expected log file to contain env var value %q, got %q", "hi", logged)
	}
}

func TestLoggedCommandRunner_OutputWithEnvDelegatesToWrappedRunner(t *testing.T) {
	runner := LoggedCommandRunner{LogFile: filepath.Join(t.TempDir(), "run.log")}

	output, err := runner.OutputWithEnv([]string{"DACKUP_TEST_VAR=delegated"}, "sh", "-c", "echo $DACKUP_TEST_VAR")
	if err != nil {
		t.Fatalf("OutputWithEnv returned error: %v", err)
	}

	if got := strings.TrimSpace(string(output)); got != "delegated" {
		t.Fatalf("expected output %q, got %q", "delegated", got)
	}
}

type fakeCommandRunnerWithoutEnvSupport struct{}

func (fakeCommandRunnerWithoutEnvSupport) Run(name string, args ...string) error { return nil }
func (fakeCommandRunnerWithoutEnvSupport) Output(name string, args ...string) ([]byte, error) {
	return nil, nil
}
func (fakeCommandRunnerWithoutEnvSupport) LookPath(file string) (string, error) { return file, nil }

func TestLoggedCommandRunner_OutputWithEnvErrorsWhenWrappedRunnerLacksSupport(t *testing.T) {
	runner := LoggedCommandRunner{
		Runner:  fakeCommandRunnerWithoutEnvSupport{},
		LogFile: filepath.Join(t.TempDir(), "run.log"),
	}

	if _, err := runner.OutputWithEnv(nil, "echo", "hi"); err == nil {
		t.Fatal("expected an error when the wrapped runner does not implement EnvCommandRunner")
	}
}
