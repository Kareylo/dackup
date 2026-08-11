package config

import (
	"bufio"
	"dackup/internal/shared"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const (
	defaultDataDir    = "/opt/apps_docker"
	defaultStagingDir = "/backups/in"
)

var (
	configFilePath string
	options        *shared.Options
)

type commandService struct {
	options *shared.Options
	prompt  shared.PromptService
}

func NewCommand(sharedOptions *shared.Options) *cobra.Command {
	options = sharedOptions

	var err error
	configFilePath, err = shared.DefaultDackupConfigPath()
	if err != nil {
		configFilePath = "config.json"
	}

	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage dackup configuration",
		Long:  "Create and update the dackup configuration file used by the backup command.",
	}

	configInitCmd := &cobra.Command{
		Use:   "init",
		Short: "Create the dackup configuration file",
		Long:  "Interactively create the dackup configuration file containing containers, backup paths, and stop settings.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigInit()
		},
	}

	configAddContainerCmd := &cobra.Command{
		Use:   "add",
		Short: "Add a container to the dackup configuration",
		Long:  "Interactively add a container entry to the active dackup configuration file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigAddContainer()
		},
	}

	configUpdateContainerCmd := &cobra.Command{
		Use:   "update",
		Short: "Update a container in the dackup configuration",
		Long:  "List existing containers, then interactively update one container entry in the active dackup configuration file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigUpdateContainer()
		},
	}

	configRemoveContainerCmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a container from the dackup configuration",
		Long:  "List existing containers, then interactively remove one container entry from the active dackup configuration file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigRemoveContainer()
		},
	}

	configListContainersCmd := &cobra.Command{
		Use:   "list",
		Short: "List containers in the dackup configuration",
		Long:  "List all container entries in the active dackup configuration file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigListContainers()
		},
	}

	configUseFileCmd := &cobra.Command{
		Use:   "use-file <path>",
		Short: "Use a custom containers configuration file",
		Long: `Configure dackup to read containers from a custom file.

The custom file path is stored in the main dackup config file, usually ~/.config/dackup/config.json.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigUseFile(args[0])
		},
	}

	configCmd.PersistentFlags().StringVar(
		&configFilePath,
		"config-file",
		configFilePath,
		"main dackup config file",
	)

	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configAddContainerCmd)
	configCmd.AddCommand(configUpdateContainerCmd)
	configCmd.AddCommand(configRemoveContainerCmd)
	configCmd.AddCommand(configListContainersCmd)
	configCmd.AddCommand(configUseFileCmd)

	return configCmd
}

func newCommandService(reader *bufio.Reader) commandService {
	return commandService{
		options: options,
		prompt:  shared.NewPromptService(reader),
	}
}

func runConfigInit() error {
	service := newCommandService(bufio.NewReader(os.Stdin))

	if shared.FileExists(configFilePath) {
		overwrite, err := service.prompt.Bool(
			fmt.Sprintf("Configuration file already exists at %s. Overwrite it?", configFilePath),
			false,
		)
		if err != nil {
			return err
		}

		if !overwrite {
			fmt.Println("Configuration creation cancelled.")
			return nil
		}
	}

	owner, err := service.prompt.RequiredString("Backup and restore file owner user")
	if err != nil {
		return err
	}

	group, err := service.prompt.RequiredString("Backup and restore file owner group")
	if err != nil {
		return err
	}

	dataDir, err := service.prompt.StringWithDefault("Application data directory", defaultDataDir)
	if err != nil {
		return err
	}

	stagingDir, err := service.prompt.StringWithDefault("Staging directory", defaultStagingDir)
	if err != nil {
		return err
	}

	useCustomFile, err := service.prompt.Bool("Do you want to store containers in a custom config file?", false)
	if err != nil {
		return err
	}

	if useCustomFile {
		return service.createConfigWithCustomContainersFile(owner, group, dataDir, stagingDir)
	}

	containers, err := service.askContainers()
	if err != nil {
		return err
	}

	config := shared.DackupConfig{
		User:       owner,
		Group:      group,
		DataDir:    dataDir,
		StagingDir: stagingDir,
		Containers: containers,
	}

	if err := shared.WriteDackupConfig(configFilePath, config, options); err != nil {
		return err
	}

	fmt.Printf("Configuration file created: %s\n", configFilePath)
	return nil
}

func (service commandService) createConfigWithCustomContainersFile(
	owner string,
	group string,
	dataDir string,
	stagingDir string,
) error {
	customPath, err := service.prompt.RequiredString("Custom containers config file path")
	if err != nil {
		return err
	}

	customPath, err = normalizeConfigPath(customPath)
	if err != nil {
		return err
	}

	mainConfig := shared.DackupConfig{
		User:       owner,
		Group:      group,
		ConfigFile: customPath,
		DataDir:    dataDir,
		StagingDir: stagingDir,
	}

	if err := shared.WriteDackupConfig(configFilePath, mainConfig, options); err != nil {
		return err
	}

	if !shared.FileExists(customPath) {
		createCustom, err := service.prompt.Bool(
			fmt.Sprintf("Custom file does not exist at %s. Create it now?", customPath),
			true,
		)
		if err != nil {
			return err
		}

		if createCustom {
			containers, err := service.askContainers()
			if err != nil {
				return err
			}

			if err := shared.WriteContainerConfigsToPath(customPath, containers, options); err != nil {
				return err
			}
		}
	}

	fmt.Printf("Main config created: %s\n", configFilePath)
	fmt.Printf("Custom containers config: %s\n", customPath)
	return nil
}

func runConfigAddContainer() error {
	service := newCommandService(bufio.NewReader(os.Stdin))

	effectiveConfigPath, err := shared.EffectiveContainersConfigPath(configFilePath)
	if err != nil {
		return err
	}

	configs, err := service.readExistingContainerConfigs(effectiveConfigPath)
	if err != nil {
		return err
	}

	config, err := service.askContainerConfig()
	if err != nil {
		return err
	}

	for _, existingConfig := range configs {
		if existingConfig.Container == config.Container {
			return fmt.Errorf("container %q already exists in %s", config.Container, effectiveConfigPath)
		}
	}

	configs = append(configs, config)

	if err := shared.WriteContainerConfigsToPath(effectiveConfigPath, configs, options); err != nil {
		return err
	}

	fmt.Printf("Container %q added to %s\n", config.Container, effectiveConfigPath)
	return nil
}

func runConfigUpdateContainer() error {
	service := newCommandService(bufio.NewReader(os.Stdin))

	effectiveConfigPath, err := shared.EffectiveContainersConfigPath(configFilePath)
	if err != nil {
		return err
	}

	configs, err := service.readExistingContainerConfigs(effectiveConfigPath)
	if err != nil {
		return err
	}

	if len(configs) == 0 {
		return fmt.Errorf("no containers found in %s", effectiveConfigPath)
	}

	printContainers(configs)

	selectedContainer, err := service.prompt.RequiredString("Container to update")
	if err != nil {
		return err
	}

	selectedIndex := findContainerIndex(configs, selectedContainer)
	if selectedIndex == -1 {
		return fmt.Errorf("container %q was not found in %s", selectedContainer, effectiveConfigPath)
	}

	updatedConfig, err := service.askUpdatedContainerConfig(configs[selectedIndex])
	if err != nil {
		return err
	}

	for index, existingConfig := range configs {
		if index == selectedIndex {
			continue
		}

		if existingConfig.Container == updatedConfig.Container {
			return fmt.Errorf("container %q already exists in %s", updatedConfig.Container, effectiveConfigPath)
		}
	}

	configs[selectedIndex] = updatedConfig

	if err := shared.WriteContainerConfigsToPath(effectiveConfigPath, configs, options); err != nil {
		return err
	}

	fmt.Printf("Container %q updated in %s\n", updatedConfig.Container, effectiveConfigPath)
	return nil
}

func runConfigRemoveContainer() error {
	service := newCommandService(bufio.NewReader(os.Stdin))

	effectiveConfigPath, err := shared.EffectiveContainersConfigPath(configFilePath)
	if err != nil {
		return err
	}

	configs, err := service.readExistingContainerConfigs(effectiveConfigPath)
	if err != nil {
		return err
	}

	if len(configs) == 0 {
		return fmt.Errorf("no containers found in %s", effectiveConfigPath)
	}

	printContainers(configs)

	selectedContainer, err := service.prompt.RequiredString("Container to remove")
	if err != nil {
		return err
	}

	selectedIndex := findContainerIndex(configs, selectedContainer)
	if selectedIndex == -1 {
		return fmt.Errorf("container %q was not found in %s", selectedContainer, effectiveConfigPath)
	}

	removedContainer := configs[selectedIndex].Container

	confirmRemoval, err := service.prompt.Bool(
		fmt.Sprintf("Remove container %q from %s?", removedContainer, effectiveConfigPath),
		false,
	)
	if err != nil {
		return err
	}

	if !confirmRemoval {
		fmt.Println("Container removal cancelled.")
		return nil
	}

	configs = append(configs[:selectedIndex], configs[selectedIndex+1:]...)

	if err := shared.WriteContainerConfigsToPath(effectiveConfigPath, configs, options); err != nil {
		return err
	}

	fmt.Printf("Container %q removed from %s\n", removedContainer, effectiveConfigPath)

	for _, config := range configs {
		if containsString(config.Contains, removedContainer) {
			fmt.Printf("Warning: container %q still lists %q in \"contains\"\n", config.Container, removedContainer)
		}
	}

	return nil
}

func runConfigListContainers() error {
	effectiveConfigPath, err := shared.EffectiveContainersConfigPath(configFilePath)
	if err != nil {
		return err
	}

	if !shared.FileExists(effectiveConfigPath) {
		fmt.Printf("No configuration file found at %s\n", effectiveConfigPath)
		return nil
	}

	configs, err := shared.ReadContainerConfigsFromPath(effectiveConfigPath)
	if err != nil {
		return err
	}

	if len(configs) == 0 {
		fmt.Printf("No containers configured in %s\n", effectiveConfigPath)
		return nil
	}

	fmt.Printf("Configuration file: %s\n\n", effectiveConfigPath)
	printContainers(configs)

	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}

	return false
}

func runConfigUseFile(customPath string) error {
	service := newCommandService(bufio.NewReader(os.Stdin))

	customPath = strings.TrimSpace(customPath)
	if customPath == "" {
		return fmt.Errorf("custom config file path cannot be empty")
	}

	normalizedPath, err := normalizeConfigPath(customPath)
	if err != nil {
		return err
	}

	mainConfig := shared.DackupConfig{}

	if shared.FileExists(configFilePath) {
		config, err := shared.ReadDackupConfig(configFilePath)
		if err != nil {
			return err
		}

		mainConfig = config
	}

	if strings.TrimSpace(mainConfig.User) == "" {
		mainConfig.User, err = service.prompt.RequiredString("Backup and restore file owner user")
		if err != nil {
			return err
		}
	}

	if strings.TrimSpace(mainConfig.Group) == "" {
		mainConfig.Group, err = service.prompt.RequiredString("Backup and restore file owner group")
		if err != nil {
			return err
		}
	}

	if strings.TrimSpace(mainConfig.DataDir) == "" {
		mainConfig.DataDir, err = service.prompt.StringWithDefault("Application data directory", defaultDataDir)
		if err != nil {
			return err
		}
	}

	if strings.TrimSpace(mainConfig.StagingDir) == "" {
		mainConfig.StagingDir, err = service.prompt.StringWithDefault("Staging directory", defaultStagingDir)
		if err != nil {
			return err
		}
	}

	mainConfig.ConfigFile = normalizedPath
	mainConfig.Containers = nil

	if err := shared.WriteDackupConfig(configFilePath, mainConfig, options); err != nil {
		return err
	}

	if !shared.FileExists(normalizedPath) {
		if err := shared.WriteContainerConfigsToPath(normalizedPath, []shared.ContainerConfig{}, options); err != nil {
			return err
		}
	}

	fmt.Printf("Dackup will now read containers from: %s\n", normalizedPath)
	fmt.Printf("This setting was written to: %s\n", configFilePath)

	return nil
}

func (service commandService) askContainers() ([]shared.ContainerConfig, error) {
	var configs []shared.ContainerConfig

	fmt.Println("Creating dackup containers configuration.")
	fmt.Println("You will now be asked to add containers.")
	fmt.Println()

	for {
		config, err := service.askContainerConfig()
		if err != nil {
			return nil, err
		}

		configs = append(configs, config)

		addAnother, err := service.prompt.Bool("Add another container?", true)
		if err != nil {
			return nil, err
		}

		if !addAnother {
			break
		}

		fmt.Println()
	}

	return configs, nil
}

func askContainers(reader *bufio.Reader) ([]shared.ContainerConfig, error) {
	return newCommandService(reader).askContainers()
}

func (service commandService) askContainerConfig() (shared.ContainerConfig, error) {
	container, err := service.prompt.RequiredString("Container name")
	if err != nil {
		return shared.ContainerConfig{}, err
	}

	toStop, err := service.prompt.Bool("Stop this container before backup?", false)
	if err != nil {
		return shared.ContainerConfig{}, err
	}

	paths, err := service.prompt.StringList("Backup paths, separated by commas. Leave empty if none")
	if err != nil {
		return shared.ContainerConfig{}, err
	}

	contains, err := service.prompt.StringList("Contained/dependent containers, separated by commas. Leave empty if none")
	if err != nil {
		return shared.ContainerConfig{}, err
	}

	config := shared.ContainerConfig{
		Container: container,
		ToStop:    toStop,
	}

	if len(paths) > 0 {
		config.Paths = paths
	}

	if len(contains) > 0 {
		config.Contains = contains
	}

	return config, nil
}

func askContainerConfig(reader *bufio.Reader) (shared.ContainerConfig, error) {
	return newCommandService(reader).askContainerConfig()
}

func (service commandService) askUpdatedContainerConfig(
	currentConfig shared.ContainerConfig,
) (shared.ContainerConfig, error) {
	fmt.Printf("Updating container %q. Press Enter to keep the current value.\n", currentConfig.Container)
	fmt.Println()

	container, err := service.prompt.StringWithDefault("Container name", currentConfig.Container)
	if err != nil {
		return shared.ContainerConfig{}, err
	}

	toStop, err := service.prompt.Bool(
		fmt.Sprintf("Stop this container before backup? Current value: %t", currentConfig.ToStop),
		currentConfig.ToStop,
	)
	if err != nil {
		return shared.ContainerConfig{}, err
	}

	paths, err := service.prompt.StringListWithDefault("Backup paths, separated by commas", currentConfig.Paths)
	if err != nil {
		return shared.ContainerConfig{}, err
	}

	contains, err := service.prompt.StringListWithDefault("Contained/dependent containers, separated by commas", currentConfig.Contains)
	if err != nil {
		return shared.ContainerConfig{}, err
	}

	updatedConfig := shared.ContainerConfig{
		Container: container,
		ToStop:    toStop,
	}

	if len(paths) > 0 {
		updatedConfig.Paths = paths
	}

	if len(contains) > 0 {
		updatedConfig.Contains = contains
	}

	return updatedConfig, nil
}

func askUpdatedContainerConfig(
	reader *bufio.Reader,
	currentConfig shared.ContainerConfig,
) (shared.ContainerConfig, error) {
	return newCommandService(reader).askUpdatedContainerConfig(currentConfig)
}

func askRequiredString(reader *bufio.Reader, label string) (string, error) {
	return shared.NewPromptService(reader).RequiredString(label)
}

func askString(reader *bufio.Reader, label string) (string, error) {
	return shared.NewPromptService(reader).String(label)
}

func askStringWithDefault(reader *bufio.Reader, label string, defaultValue string) (string, error) {
	return shared.NewPromptService(reader).StringWithDefault(label, defaultValue)
}

func askBool(reader *bufio.Reader, label string, defaultValue bool) (bool, error) {
	return shared.NewPromptService(reader).Bool(label, defaultValue)
}

func askStringList(reader *bufio.Reader, label string) ([]string, error) {
	return shared.NewPromptService(reader).StringList(label)
}

func askStringListWithDefault(reader *bufio.Reader, label string, defaultValues []string) ([]string, error) {
	return shared.NewPromptService(reader).StringListWithDefault(label, defaultValues)
}

func parseStringList(value string) []string {
	return shared.ParseStringList(value)
}

func (service commandService) readExistingContainerConfigs(path string) ([]shared.ContainerConfig, error) {
	if !shared.FileExists(path) {
		create, err := service.askCreateMissingConfig(path)
		if err != nil {
			return nil, err
		}

		if !create {
			return nil, fmt.Errorf("configuration file does not exist: %s", path)
		}

		if err := shared.WriteContainerConfigsToPath(path, []shared.ContainerConfig{}, options); err != nil {
			return nil, err
		}

		return []shared.ContainerConfig{}, nil
	}

	return shared.ReadContainerConfigsFromPath(path)
}

func readExistingContainerConfigs(path string) ([]shared.ContainerConfig, error) {
	return newCommandService(bufio.NewReader(os.Stdin)).readExistingContainerConfigs(path)
}

func (service commandService) askCreateMissingConfig(path string) (bool, error) {
	return service.prompt.Bool(fmt.Sprintf("Configuration file does not exist at %s. Create it?", path), true)
}

func askCreateMissingConfig(path string) (bool, error) {
	return newCommandService(bufio.NewReader(os.Stdin)).askCreateMissingConfig(path)
}

func printContainers(configs []shared.ContainerConfig) {
	fmt.Println("Existing containers:")
	fmt.Println()

	for index, config := range configs {
		fmt.Printf("%d. %s\n", index+1, config.Container)
		fmt.Printf("   Stop before backup: %t\n", config.ToStop)

		if len(config.Paths) > 0 {
			fmt.Printf("   Paths: %s\n", strings.Join(config.Paths, ", "))
		} else {
			fmt.Println("   Paths: none")
		}

		if len(config.Contains) > 0 {
			fmt.Printf("   Contains: %s\n", strings.Join(config.Contains, ", "))
		} else {
			fmt.Println("   Contains: none")
		}

		fmt.Println()
	}
}

func findContainerIndex(configs []shared.ContainerConfig, containerName string) int {
	containerName = strings.TrimSpace(containerName)

	for index, config := range configs {
		if config.Container == containerName {
			return index
		}
	}

	return -1
}

func normalizeConfigPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("config path cannot be empty")
	}

	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to find user home directory: %w", err)
		}

		path = filepath.Join(homeDir, strings.TrimPrefix(path, "~/"))
	}

	if !filepath.IsAbs(path) {
		absolutePath, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("failed to resolve config file path: %w", err)
		}

		path = absolutePath
	}

	return path, nil
}
