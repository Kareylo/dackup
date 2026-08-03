package shared

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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

type FileSystem interface {
	Stat(name string) (os.FileInfo, error)
	MkdirAll(path string, perm os.FileMode) error
	OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)
}

type OSFileSystem struct{}

func (OSFileSystem) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (OSFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (OSFileSystem) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag, perm)
}

type Logger interface {
	Log(level string, message string)
}

type FileLogger struct {
	LogFile string
	FS      FileSystem
}

func (logger FileLogger) Log(level string, message string) {
	fs := logger.FS
	if fs == nil {
		fs = OSFileSystem{}
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("[%s] [%s] %s", timestamp, level, message)

	fmt.Println(line)

	logFile, err := fs.OpenFile(logger.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write log file: %v\n", err)
		return
	}
	defer logFile.Close()

	writer := bufio.NewWriter(logFile)
	_, _ = writer.WriteString(line + "\n")
	_ = writer.Flush()
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

type DockerService struct {
	Runner CommandRunner
}

func (service DockerService) ContainerRunning(container string) (bool, error) {
	runner := service.Runner
	if runner == nil {
		runner = OSCommandRunner{}
	}

	output, err := runner.Output("docker", "ps", "-q", "-f", fmt.Sprintf("name=^/%s$", container))
	if err != nil {
		return false, err
	}

	return len(output) > 0, nil
}

func (service DockerService) ContainerExists(container string) (bool, error) {
	runner := service.Runner
	if runner == nil {
		runner = OSCommandRunner{}
	}

	output, err := runner.Output("docker", "ps", "-a", "-q", "-f", fmt.Sprintf("name=^/%s$", container))
	if err != nil {
		return false, err
	}

	return len(output) > 0, nil
}

type PathResolver struct {
	SourceRoot      string
	DestinationRoot string
}

func (resolver PathResolver) SourcePath(configuredPath string) string {
	return filepath.Join(resolver.SourceRoot, CleanConfiguredPath(configuredPath))
}

func (resolver PathResolver) DestinationPath(configuredPath string) string {
	return filepath.Join(resolver.DestinationRoot, CleanConfiguredPath(configuredPath))
}

func CleanConfiguredPath(configuredPath string) string {
	return strings.TrimPrefix(filepath.Clean(configuredPath), string(os.PathSeparator))
}

func SelectContainerAndContained(
	containerName string,
	configByContainer map[string]ContainerConfig,
	selected map[string]bool,
) {
	containerName = strings.TrimSpace(containerName)
	if containerName == "" || selected[containerName] {
		return
	}

	config, exists := configByContainer[containerName]
	if !exists {
		return
	}

	selected[containerName] = true

	for _, containedContainer := range config.Contains {
		SelectContainerAndContained(containedContainer, configByContainer, selected)
	}
}

func FilterContainerConfigs(
	configs []ContainerConfig,
	requestedContainers []string,
	action string,
) ([]ContainerConfig, error) {
	if len(requestedContainers) == 0 {
		return configs, nil
	}

	configByContainer := make(map[string]ContainerConfig)
	for _, config := range configs {
		configByContainer[config.Container] = config
	}

	selected := make(map[string]bool)

	for _, requestedContainer := range requestedContainers {
		requestedContainer = strings.TrimSpace(requestedContainer)
		if requestedContainer == "" {
			continue
		}

		if _, exists := configByContainer[requestedContainer]; !exists {
			return nil, fmt.Errorf("container %q was not found in the configuration", requestedContainer)
		}

		SelectContainerAndContained(requestedContainer, configByContainer, selected)
	}

	var filteredConfigs []ContainerConfig
	for _, config := range configs {
		if selected[config.Container] {
			filteredConfigs = append(filteredConfigs, config)
		}
	}

	if len(filteredConfigs) == 0 {
		return nil, fmt.Errorf("no containers selected for %s", action)
	}

	return filteredConfigs, nil
}

func ContainersToStopFromConfig(configs []ContainerConfig) []string {
	seen := make(map[string]bool)
	var containers []string

	for _, config := range configs {
		if !config.ToStop {
			continue
		}

		AddUniqueContainer(config.Container, seen, &containers)

		for _, containedContainer := range config.Contains {
			AddUniqueContainer(containedContainer, seen, &containers)
		}
	}

	return containers
}

func AddUniqueContainer(container string, seen map[string]bool, containers *[]string) {
	container = strings.TrimSpace(container)
	if container == "" || seen[container] {
		return
	}

	seen[container] = true
	*containers = append(*containers, container)
}

type ContainerLifecycleService struct {
	Docker  DockerService
	Runner  CommandRunner
	Logger  Logger
	Options *Options
}

func (service ContainerLifecycleService) StopRunningContainers(containers []string, action string) ([]string, error) {
	service.Logger.Log("INFO", fmt.Sprintf("Stopping containers before %s ...", action))

	if len(containers) == 0 {
		service.Logger.Log("WARN", `No containers marked with "to_stop": true; skipping stop step`)
		return nil, nil
	}

	var stoppedContainers []string

	for _, container := range containers {
		running, err := service.Docker.ContainerRunning(container)
		if err != nil {
			service.Logger.Log("ERROR", fmt.Sprintf("Failed to inspect container %s: %v", container, err))
			continue
		}

		if !running {
			service.Logger.Log("INFO", fmt.Sprintf("Container %s is not running; nothing to stop", container))
			continue
		}

		service.Logger.Log("INFO", fmt.Sprintf("Stopping container: %s", container))

		if service.Options != nil && service.Options.DryRun {
			service.Logger.Log("INFO", fmt.Sprintf("[dry-run] Would stop container %s", container))
			stoppedContainers = append(stoppedContainers, container)
			continue
		}

		if err := service.Runner.Run("docker", "stop", container); err != nil {
			service.Logger.Log("ERROR", fmt.Sprintf("Failed to stop container %s; continuing", container))
			continue
		}

		service.Logger.Log("INFO", fmt.Sprintf("Container %s stopped", container))
		stoppedContainers = append(stoppedContainers, container)
	}

	return stoppedContainers, nil
}

func (service ContainerLifecycleService) StartStoppedContainers(stoppedContainers []string, action string) error {
	service.Logger.Log("INFO", "Starting previously stopped containers ...")

	if len(stoppedContainers) == 0 {
		service.Logger.Log("INFO", fmt.Sprintf("No containers were stopped by %s; nothing to restart", action))
		return nil
	}

	for _, container := range stoppedContainers {
		exists, err := service.Docker.ContainerExists(container)
		if err != nil {
			service.Logger.Log("ERROR", fmt.Sprintf("Failed to inspect container %s: %v", container, err))
			continue
		}

		if !exists {
			service.Logger.Log("WARN", fmt.Sprintf("Container %s does not exist on this host; skipping", container))
			continue
		}

		service.Logger.Log("INFO", fmt.Sprintf("Starting container: %s", container))

		if service.Options != nil && service.Options.DryRun {
			service.Logger.Log("INFO", fmt.Sprintf("[dry-run] Would start container %s", container))
			continue
		}

		if err := service.Runner.Run("docker", "start", container); err != nil {
			service.Logger.Log("ERROR", fmt.Sprintf("Failed to start container %s; check manually", container))
			continue
		}

		service.Logger.Log("INFO", fmt.Sprintf("Container %s started", container))
	}

	return nil
}

func PreflightChecks(
	action string,
	effectiveConfigPath string,
	config DackupConfig,
	configs []ContainerConfig,
	sourceRoot string,
	destinationRoot string,
	resolver PathResolver,
	fs FileSystem,
	runner CommandRunner,
) error {
	if fs == nil {
		fs = OSFileSystem{}
	}

	if runner == nil {
		runner = OSCommandRunner{}
	}

	if _, err := fs.Stat(effectiveConfigPath); err != nil {
		return fmt.Errorf("config file not found: %s", effectiveConfigPath)
	}

	if strings.TrimSpace(config.User) == "" {
		return fmt.Errorf("config field %q is required", "user")
	}

	if strings.TrimSpace(config.Group) == "" {
		return fmt.Errorf("config field %q is required", "group")
	}

	srcInfo, err := fs.Stat(sourceRoot)
	if err != nil || !srcInfo.IsDir() {
		return fmt.Errorf("%s source directory not found: %s", action, sourceRoot)
	}

	dstInfo, err := fs.Stat(destinationRoot)
	if err != nil || !dstInfo.IsDir() {
		return fmt.Errorf("%s destination directory not found: %s", action, destinationRoot)
	}

	if _, err := runner.LookPath("docker"); err != nil {
		return fmt.Errorf("docker CLI not found; please install Docker")
	}

	if _, err := runner.LookPath("rsync"); err != nil {
		return fmt.Errorf("rsync not found; please install rsync")
	}

	for _, containerConfig := range configs {
		for _, path := range containerConfig.Paths {
			srcPath := resolver.SourcePath(path)

			info, err := fs.Stat(srcPath)
			if err != nil {
				return fmt.Errorf("configured %s path does not exist for container %s: %s", action, containerConfig.Container, srcPath)
			}

			if !info.IsDir() {
				return fmt.Errorf("configured %s path is not a directory for container %s: %s", action, containerConfig.Container, srcPath)
			}
		}
	}

	return nil
}
