// Package backend implements the "backend" command and its create/show/
// update/remove subcommands, managing the Backend/BackendSettings fields on
// the main dackup config file. It's split one-file-per-responsibility, per
// AGENTS.md's "File Naming Guidelines":
//
//   - backend.go (this file) — command wiring and the create/show/update/
//     remove handlers.
//   - prompts.go — prompt helpers shared across backends, and the
//     per-backend settings dispatch.
//   - borg.go — borg-specific settings prompting.
//   - kopia.go — kopia-specific settings prompting, including its
//     per-storage-type sub-prompts.
//   - print.go — "backend show" rendering and secret masking.
package backend

import (
	"bufio"
	"dackup/internal/backend"
	"dackup/internal/shared"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	configFilePath string
	options        *shared.Options
)

// commandService bundles the dependencies every backend subcommand needs:
// interactive prompting, secret encryption, and filesystem access for
// validating file-path answers (e.g. an SSH keyfile).
type commandService struct {
	options *shared.Options
	prompt  shared.PromptService
	secrets shared.SecretStore
	fs      shared.FileSystem
}

// NewCommand builds the "backend" command and its create/show/update/remove
// subcommands, which manage the Backend/BackendSettings fields on the main
// dackup config file.
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
		secrets: shared.AESFileSecretStore{},
		fs:      shared.OSFileSystem{},
	}
}

func (service commandService) fileSystem() shared.FileSystem {
	if service.fs != nil {
		return service.fs
	}

	return shared.OSFileSystem{}
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

	fmt.Println("Current backend configuration:")
	printBackend(config)
	fmt.Println()

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
	config.BackendDir = ""
	config.BackendSettings = nil

	if err := shared.WriteDackupConfig(configFilePath, config, options); err != nil {
		return err
	}

	fmt.Printf("Backend removed from %s\n", configFilePath)
	return nil
}

// configureBackend prompts for a backend name, its repository storage
// directory (BackendDir), and its settings, and returns the updated config.
// The second return value is false when there was nothing to configure (no
// backend implemented yet) and the caller should not write anything.
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

	backendDir, err := service.promptBackendDir(config.BackendDir)
	if err != nil {
		return config, false, err
	}

	settings, err := service.promptBackendSettings(name, config.Backend, config.BackendSettings)
	if err != nil {
		return config, false, err
	}

	config.Backend = name
	config.BackendDir = backendDir
	config.BackendSettings = settings

	return config, true, nil
}
