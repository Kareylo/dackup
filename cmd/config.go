package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var configFilePath string

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage dackup configuration",
	Long:  "Create and update the dackup configuration file used by the backup command.",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create the dackup configuration file",
	Long:  "Interactively create the dackup configuration file containing containers, backup paths, and stop settings.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfigInit()
	},
}

var configAddContainerCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a container to the dackup configuration",
	Long:  "Interactively add a container entry to the active dackup configuration file.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfigAddContainer()
	},
}

var configUpdateContainerCmd = &cobra.Command{
	Use:   "update",
	Short: "Update a container in the dackup configuration",
	Long:  "List existing containers, then interactively update one container entry in the active dackup configuration file.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfigUpdateContainer()
	},
}

var configUseFileCmd = &cobra.Command{
	Use:   "use-file <path>",
	Short: "Use a custom containers configuration file",
	Long: `Configure dackup to read containers from a custom file.

The custom file path is stored in the main dackup config file, usually ~/.config/dackup/config.json.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfigUseFile(args[0])
	},
}

func init() {
	var err error
	configFilePath, err = defaultDackupConfigPath()
	if err != nil {
		configFilePath = "config.json"
	}

	rootCmd.AddCommand(configCmd)

	configCmd.PersistentFlags().StringVar(
		&configFilePath,
		"config-file",
		configFilePath,
		"main dackup config file",
	)

	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configAddContainerCmd)
	configCmd.AddCommand(configUpdateContainerCmd)
	configCmd.AddCommand(configUseFileCmd)
}

func runConfigInit() error {
	reader := bufio.NewReader(os.Stdin)

	if fileExists(configFilePath) {
		overwrite, err := askBool(
			reader,
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

	owner, err := askRequiredString(reader, "Backup and restore file owner user")
	if err != nil {
		return err
	}

	group, err := askRequiredString(reader, "Backup and restore file owner group")
	if err != nil {
		return err
	}

	useCustomFile, err := askBool(reader, "Do you want to store containers in a custom config file?", false)
	if err != nil {
		return err
	}

	if useCustomFile {
		customPath, err := askRequiredString(reader, "Custom containers config file path")
		if err != nil {
			return err
		}

		customPath, err = normalizeConfigPath(customPath)
		if err != nil {
			return err
		}

		mainConfig := dackupConfig{
			User:       owner,
			Group:      group,
			ConfigFile: customPath,
		}

		if err := writeDackupConfig(configFilePath, mainConfig); err != nil {
			return err
		}

		if !fileExists(customPath) {
			createCustom, err := askBool(
				reader,
				fmt.Sprintf("Custom file does not exist at %s. Create it now?", customPath),
				true,
			)
			if err != nil {
				return err
			}

			if createCustom {
				containers, err := askContainers(reader)
				if err != nil {
					return err
				}

				if err := writeContainerConfigsToPath(customPath, containers); err != nil {
					return err
				}
			}
		}

		fmt.Printf("Main config created: %s\n", configFilePath)
		fmt.Printf("Custom containers config: %s\n", customPath)
		return nil
	}

	containers, err := askContainers(reader)
	if err != nil {
		return err
	}

	config := dackupConfig{
		User:       owner,
		Group:      group,
		Containers: containers,
	}

	if err := writeDackupConfig(configFilePath, config); err != nil {
		return err
	}

	fmt.Printf("Configuration file created: %s\n", configFilePath)
	return nil
}

func runConfigAddContainer() error {
	reader := bufio.NewReader(os.Stdin)

	effectiveConfigPath, err := effectiveContainersConfigPath(configFilePath)
	if err != nil {
		return err
	}

	configs, err := readExistingContainerConfigs(effectiveConfigPath)
	if err != nil {
		return err
	}

	config, err := askContainerConfig(reader)
	if err != nil {
		return err
	}

	for _, existingConfig := range configs {
		if existingConfig.Container == config.Container {
			return fmt.Errorf("container %q already exists in %s", config.Container, effectiveConfigPath)
		}
	}

	configs = append(configs, config)

	if err := writeContainerConfigsToPath(effectiveConfigPath, configs); err != nil {
		return err
	}

	fmt.Printf("Container %q added to %s\n", config.Container, effectiveConfigPath)
	return nil
}

func runConfigUpdateContainer() error {
	reader := bufio.NewReader(os.Stdin)

	effectiveConfigPath, err := effectiveContainersConfigPath(configFilePath)
	if err != nil {
		return err
	}

	configs, err := readExistingContainerConfigs(effectiveConfigPath)
	if err != nil {
		return err
	}

	if len(configs) == 0 {
		return fmt.Errorf("no containers found in %s", effectiveConfigPath)
	}

	printContainers(configs)

	selectedContainer, err := askRequiredString(reader, "Container to update")
	if err != nil {
		return err
	}

	selectedIndex := findContainerIndex(configs, selectedContainer)
	if selectedIndex == -1 {
		return fmt.Errorf("container %q was not found in %s", selectedContainer, effectiveConfigPath)
	}

	updatedConfig, err := askUpdatedContainerConfig(reader, configs[selectedIndex])
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

	if err := writeContainerConfigsToPath(effectiveConfigPath, configs); err != nil {
		return err
	}

	fmt.Printf("Container %q updated in %s\n", updatedConfig.Container, effectiveConfigPath)
	return nil
}

func runConfigUseFile(customPath string) error {
	customPath = strings.TrimSpace(customPath)
	if customPath == "" {
		return fmt.Errorf("custom config file path cannot be empty")
	}

	normalizedPath, err := normalizeConfigPath(customPath)
	if err != nil {
		return err
	}

	mainConfig := dackupConfig{}

	if fileExists(configFilePath) {
		config, err := readDackupConfig(configFilePath)
		if err != nil {
			return err
		}

		mainConfig = config
	}

	reader := bufio.NewReader(os.Stdin)

	if strings.TrimSpace(mainConfig.User) == "" {
		mainConfig.User, err = askRequiredString(reader, "Backup and restore file owner user")
		if err != nil {
			return err
		}
	}

	if strings.TrimSpace(mainConfig.Group) == "" {
		mainConfig.Group, err = askRequiredString(reader, "Backup and restore file owner group")
		if err != nil {
			return err
		}
	}

	mainConfig.ConfigFile = normalizedPath
	mainConfig.Containers = nil

	if err := writeDackupConfig(configFilePath, mainConfig); err != nil {
		return err
	}

	if !fileExists(normalizedPath) {
		if err := writeContainerConfigsToPath(normalizedPath, []containerConfig{}); err != nil {
			return err
		}
	}

	fmt.Printf("Dackup will now read containers from: %s\n", normalizedPath)
	fmt.Printf("This setting was written to: %s\n", configFilePath)

	return nil
}

func askContainers(reader *bufio.Reader) ([]containerConfig, error) {
	var configs []containerConfig

	fmt.Println("Creating dackup containers configuration.")
	fmt.Println("You will now be asked to add containers.")
	fmt.Println()

	for {
		config, err := askContainerConfig(reader)
		if err != nil {
			return nil, err
		}

		configs = append(configs, config)

		addAnother, err := askBool(reader, "Add another container?", true)
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

func askContainerConfig(reader *bufio.Reader) (containerConfig, error) {
	container, err := askRequiredString(reader, "Container name")
	if err != nil {
		return containerConfig{}, err
	}

	toStop, err := askBool(reader, "Stop this container before backup?", false)
	if err != nil {
		return containerConfig{}, err
	}

	paths, err := askStringList(reader, "Backup paths, separated by commas. Leave empty if none")
	if err != nil {
		return containerConfig{}, err
	}

	contains, err := askStringList(reader, "Contained/dependent containers, separated by commas. Leave empty if none")
	if err != nil {
		return containerConfig{}, err
	}

	config := containerConfig{
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

func askUpdatedContainerConfig(reader *bufio.Reader, currentConfig containerConfig) (containerConfig, error) {
	fmt.Printf("Updating container %q. Press Enter to keep the current value.\n", currentConfig.Container)
	fmt.Println()

	container, err := askStringWithDefault(reader, "Container name", currentConfig.Container)
	if err != nil {
		return containerConfig{}, err
	}

	toStop, err := askBool(
		reader,
		fmt.Sprintf("Stop this container before backup? Current value: %t", currentConfig.ToStop),
		currentConfig.ToStop,
	)
	if err != nil {
		return containerConfig{}, err
	}

	paths, err := askStringListWithDefault(reader, "Backup paths, separated by commas", currentConfig.Paths)
	if err != nil {
		return containerConfig{}, err
	}

	contains, err := askStringListWithDefault(reader, "Contained/dependent containers, separated by commas", currentConfig.Contains)
	if err != nil {
		return containerConfig{}, err
	}

	updatedConfig := containerConfig{
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

func askRequiredString(reader *bufio.Reader, label string) (string, error) {
	for {
		value, err := askString(reader, label)
		if err != nil {
			return "", err
		}

		if value != "" {
			return value, nil
		}

		fmt.Println("This value is required.")
	}
}

func askString(reader *bufio.Reader, label string) (string, error) {
	fmt.Printf("%s: ", label)

	value, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(value), nil
}

func askStringWithDefault(reader *bufio.Reader, label string, defaultValue string) (string, error) {
	fmt.Printf("%s [%s]: ", label, defaultValue)

	value, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue, nil
	}

	return value, nil
}

func askBool(reader *bufio.Reader, label string, defaultValue bool) (bool, error) {
	defaultLabel := "y/N"
	if defaultValue {
		defaultLabel = "Y/n"
	}

	for {
		fmt.Printf("%s [%s]: ", label, defaultLabel)

		value, err := reader.ReadString('\n')
		if err != nil {
			return false, err
		}

		value = strings.ToLower(strings.TrimSpace(value))

		if value == "" {
			return defaultValue, nil
		}

		switch value {
		case "y", "yes", "true", "1":
			return true, nil
		case "n", "no", "false", "0":
			return false, nil
		default:
			fmt.Println("Please answer yes or no.")
		}
	}
}

func askStringList(reader *bufio.Reader, label string) ([]string, error) {
	value, err := askString(reader, label)
	if err != nil {
		return nil, err
	}

	return parseStringList(value), nil
}

func askStringListWithDefault(reader *bufio.Reader, label string, defaultValues []string) ([]string, error) {
	defaultLabel := "none"
	if len(defaultValues) > 0 {
		defaultLabel = strings.Join(defaultValues, ", ")
	}

	fmt.Printf("%s [%s]: ", label, defaultLabel)

	value, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValues, nil
	}

	if strings.EqualFold(value, "none") {
		return nil, nil
	}

	return parseStringList(value), nil
}

func parseStringList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))

	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}

		items = append(items, item)
	}

	return items
}

func readExistingContainerConfigs(path string) ([]containerConfig, error) {
	if !fileExists(path) {
		create, err := askCreateMissingConfig(path)
		if err != nil {
			return nil, err
		}

		if !create {
			return nil, fmt.Errorf("configuration file does not exist: %s", path)
		}

		if err := writeContainerConfigsToPath(path, []containerConfig{}); err != nil {
			return nil, err
		}

		return []containerConfig{}, nil
	}

	return readContainerConfigsFromPath(path)
}

func askCreateMissingConfig(path string) (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	return askBool(reader, fmt.Sprintf("Configuration file does not exist at %s. Create it?", path), true)
}

func printContainers(configs []containerConfig) {
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

func findContainerIndex(configs []containerConfig, containerName string) int {
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
