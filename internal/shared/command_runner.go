package shared

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type CommandRunner interface {
	Run(name string, args ...string) error
	Output(name string, args ...string) ([]byte, error)
	LookPath(file string) (string, error)
}

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

type LoggedCommandRunner struct {
	Runner  CommandRunner
	FS      FileSystem
	LogFile string
	Options *Options
}

func (runner LoggedCommandRunner) Run(name string, args ...string) error {
	commandRunner := runner.Runner
	if commandRunner == nil {
		commandRunner = OSCommandRunner{}
	}

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
