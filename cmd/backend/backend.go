package backend

import (
	"bufio"
	"dackup/internal/backend"
	"dackup/internal/shared"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
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

	backendCmd := &cobra.Command{
		Use:   "backend",
		Short: "Manage the dackup backup backend configuration",
		Long:  "Create, show, update, or remove the backup backend configured in the dackup configuration file.",
	}

	backendCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "Configure a backup backend",
		Long:  "Interactively select and configure the backup backend used by backup and restore.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackendCreate()
		},
	}

	backendShowCmd := &cobra.Command{
		Use:   "show",
		Short: "Show the configured backup backend",
		Long:  "Print the backup backend currently configured, or report that none is configured.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackendShow()
		},
	}

	backendUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "Update the configured backup backend",
		Long:  "Interactively change the backup backend currently configured.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackendUpdate()
		},
	}

	backendRemoveCmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove the configured backup backend",
		Long:  "Clear the backup backend configuration, reverting to the default no-op backend.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackendRemove()
		},
	}

	backendCmd.PersistentFlags().StringVar(
		&configFilePath,
		"config-file",
		configFilePath,
		"main dackup config file",
	)

	backendCmd.AddCommand(backendCreateCmd)
	backendCmd.AddCommand(backendShowCmd)
	backendCmd.AddCommand(backendUpdateCmd)
	backendCmd.AddCommand(backendRemoveCmd)

	return backendCmd
}

func newCommandService(reader *bufio.Reader) commandService {
	return commandService{
		options: options,
		prompt:  shared.NewPromptService(reader),
	}
}

func runBackendCreate() error {
	service := newCommandService(bufio.NewReader(os.Stdin))

	if !shared.FileExists(configFilePath) {
		return fmt.Errorf("configuration file not found at %s; run \"dackup config init\" first", configFilePath)
	}

	config, err := shared.ReadDackupConfig(configFilePath)
	if err != nil {
		return err
	}

	if config.Backend != "" {
		overwrite, err := service.prompt.Bool(
			fmt.Sprintf("A backend is already configured (%s). Overwrite it?", config.Backend),
			false,
		)
		if err != nil {
			return err
		}

		if !overwrite {
			fmt.Println("Backend configuration cancelled.")
			return nil
		}
	}

	updatedConfig, configured, err := service.configureBackend(config)
	if err != nil {
		return err
	}

	if !configured {
		return nil
	}

	if err := shared.WriteDackupConfig(configFilePath, updatedConfig, options); err != nil {
		return err
	}

	fmt.Printf("Backend %q configured in %s\n", updatedConfig.Backend, configFilePath)
	return nil
}

func runBackendShow() error {
	if !shared.FileExists(configFilePath) {
		fmt.Printf("No configuration file found at %s\n", configFilePath)
		return nil
	}

	config, err := shared.ReadDackupConfig(configFilePath)
	if err != nil {
		return err
	}

	printBackend(config)
	return nil
}

func runBackendUpdate() error {
	service := newCommandService(bufio.NewReader(os.Stdin))

	if !shared.FileExists(configFilePath) {
		return fmt.Errorf("configuration file not found at %s; run \"dackup config init\" first", configFilePath)
	}

	config, err := shared.ReadDackupConfig(configFilePath)
	if err != nil {
		return err
	}

	if config.Backend == "" {
		return fmt.Errorf("no backend configured in %s; run \"dackup backend create\" first", configFilePath)
	}

	updatedConfig, configured, err := service.configureBackend(config)
	if err != nil {
		return err
	}

	if !configured {
		return nil
	}

	if err := shared.WriteDackupConfig(configFilePath, updatedConfig, options); err != nil {
		return err
	}

	fmt.Printf("Backend %q configured in %s\n", updatedConfig.Backend, configFilePath)
	return nil
}

func runBackendRemove() error {
	service := newCommandService(bufio.NewReader(os.Stdin))

	if !shared.FileExists(configFilePath) {
		return fmt.Errorf("configuration file not found at %s; run \"dackup config init\" first", configFilePath)
	}

	config, err := shared.ReadDackupConfig(configFilePath)
	if err != nil {
		return err
	}

	if config.Backend == "" {
		return fmt.Errorf("no backend configured in %s", configFilePath)
	}

	confirmRemoval, err := service.prompt.Bool(
		fmt.Sprintf("Remove backend %q from %s?", config.Backend, configFilePath),
		false,
	)
	if err != nil {
		return err
	}

	if !confirmRemoval {
		fmt.Println("Backend removal cancelled.")
		return nil
	}

	config.Backend = ""
	config.BackendSettings = nil

	if err := shared.WriteDackupConfig(configFilePath, config, options); err != nil {
		return err
	}

	fmt.Printf("Backend removed from %s\n", configFilePath)
	return nil
}

// configureBackend prompts for a backend name and its settings, and returns
// the updated config. The second return value is false when there was
// nothing to configure (no backend implemented yet) and the caller should
// not write anything.
func (service commandService) configureBackend(config shared.DackupConfig) (shared.DackupConfig, bool, error) {
	available := backend.AvailableBackends()
	if len(available) == 0 {
		fmt.Println("No backends are implemented yet; the default no-op backend will be used.")
		return config, false, nil
	}

	name, err := service.selectBackendName(available)
	if err != nil {
		return config, false, err
	}

	settings, err := service.promptBackendSettings(name)
	if err != nil {
		return config, false, err
	}

	config.Backend = name
	config.BackendSettings = settings

	return config, true, nil
}

func (service commandService) selectBackendName(available []string) (string, error) {
	fmt.Println("Available backends:")
	for index, name := range available {
		fmt.Printf("%d. %s\n", index+1, name)
	}

	for {
		choice, err := service.prompt.RequiredString("Backend name")
		if err != nil {
			return "", err
		}

		for _, name := range available {
			if name == choice {
				return name, nil
			}
		}

		fmt.Println("Please choose one of the listed backend names.")
	}
}

// promptBackendSettings gathers backend-specific settings. Adding a backend
// means adding a case here, matching the "one case per backend" pattern
// used by internal/backend.ParseSettings and internal/backend.Factory.
func (service commandService) promptBackendSettings(name string) (json.RawMessage, error) {
	switch name {
	default:
		return nil, nil
	}
}

func printBackend(config shared.DackupConfig) {
	if config.Backend == "" {
		fmt.Println("No backend configured (using the default no-op backend)")
		return
	}

	fmt.Printf("Backend: %s\n", config.Backend)

	if len(config.BackendSettings) == 0 {
		fmt.Println("Settings: none")
		return
	}

	pretty, err := json.MarshalIndent(json.RawMessage(config.BackendSettings), "", "  ")
	if err != nil {
		fmt.Printf("Settings: %s\n", config.BackendSettings)
		return
	}

	fmt.Printf("Settings:\n%s\n", pretty)
}
