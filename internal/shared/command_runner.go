package shared

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// CommandRunner abstracts running an external command, so callers are
// testable without invoking a real subprocess.
type CommandRunner interface {
	Run(name string, args ...string) error
	Output(name string, args ...string) ([]byte, error)
	LookPath(file string) (string, error)
}

// EnvCommandRunner is an optional capability a CommandRunner can also
// implement to run a command with a specific working directory and/or
// extra environment variables set — needed by backends like borg that
// require both (BORG_PASSPHRASE for authentication, and a repo-relative
// cwd so archived paths stay relative rather than absolute). Not every
// caller needs this, so it's kept separate from the core CommandRunner
// interface; callers that need it type-assert for it.
type EnvCommandRunner interface {
	// RunInDirWithEnv runs name with args. dir overrides the process's
	// working directory unless empty; env is appended to the current
	// environment.
	RunInDirWithEnv(dir string, env []string, name string, args ...string) error

	// OutputWithEnv runs name with args, with env appended to the current
	// environment, and returns its standard output.
	OutputWithEnv(env []string, name string, args ...string) ([]byte, error)
}

// OSCommandRunner is the real CommandRunner (and EnvCommandRunner)
// implementation, running commands via os/exec.
type OSCommandRunner struct{}

func (OSCommandRunner) Run(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

func (OSCommandRunner) Output(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

func (OSCommandRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (OSCommandRunner) RunInDirWithEnv(dir string, env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), env...)

	return cmd.Run()
}

func (OSCommandRunner) OutputWithEnv(env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), env...)

	return cmd.Output()
}

// LoggedCommandRunner wraps a CommandRunner, redirecting each command's
// stdout/stderr to LogFile (and echoing the command line when
// Options.Verbose is set) instead of the calling process's own streams.
type LoggedCommandRunner struct {
	Runner  CommandRunner
	FS      FileSystem
	LogFile string
	Options *Options
}

// Run always executes name as a real subprocess, redirecting its
// stdout/stderr to LogFile — unlike Output/LookPath, it never delegates to
// an injected Runner, because CommandRunner's Run signature has no way to
// express a custom stdout/stderr writer. A test faking stop/start/rsync/
// chown calls must construct a bare CommandRunner directly rather than
// wrapping it in a LoggedCommandRunner.
func (runner LoggedCommandRunner) Run(name string, args ...string) error {
	fs := runner.FS
	if fs == nil {
		fs = OSFileSystem{}
	}

	logFile, err := fs.OpenFile(runner.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	if runner.Options != nil && runner.Options.Verbose {
		fmt.Printf("Running: %s %s\n", name, strings.Join(args, " "))
	}

	cmd := exec.Command(name, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	return cmd.Run()
}

func (runner LoggedCommandRunner) Output(name string, args ...string) ([]byte, error) {
	commandRunner := runner.Runner
	if commandRunner == nil {
		commandRunner = OSCommandRunner{}
	}

	return commandRunner.Output(name, args...)
}

func (runner LoggedCommandRunner) LookPath(file string) (string, error) {
	commandRunner := runner.Runner
	if commandRunner == nil {
		commandRunner = OSCommandRunner{}
	}

	return commandRunner.LookPath(file)
}

func (runner LoggedCommandRunner) RunInDirWithEnv(dir string, env []string, name string, args ...string) error {
	fs := runner.FS
	if fs == nil {
		fs = OSFileSystem{}
	}

	logFile, err := fs.OpenFile(runner.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	if runner.Options != nil && runner.Options.Verbose {
		fmt.Printf("Running: %s %s\n", name, strings.Join(args, " "))
	}

	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	return cmd.Run()
}

func (runner LoggedCommandRunner) OutputWithEnv(env []string, name string, args ...string) ([]byte, error) {
	commandRunner := runner.Runner
	if commandRunner == nil {
		commandRunner = OSCommandRunner{}
	}

	envRunner, ok := commandRunner.(EnvCommandRunner)
	if !ok {
		return nil, fmt.Errorf("command runner does not support environment variables")
	}

	return envRunner.OutputWithEnv(env, name, args...)
}
