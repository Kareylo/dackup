package backend

import (
	"bufio"
	"dackup/internal/backend"
	"dackup/internal/backend/borg"
	"dackup/internal/shared"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	configFilePath string
	options        *shared.Options
)

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

// promptBackendDir prompts for DackupConfig.BackendDir — the backend's
// repository storage root. It's a top-level config field (not part of
// backend_settings, see AGENTS.md's "Backend interface" section for why),
// so it's gathered once here rather than per-backend. current pre-fills the
// prompt when updating an already-configured backend; a fresh create has no
// default and must be typed. The value is always required: every backend
// implemented so far needs local repository storage.
func (service commandService) promptBackendDir(current string) (string, error) {
	label := "Backend repository storage directory (backend_dir)"

	for {
		var value string
		var err error

		if current != "" {
			value, err = service.prompt.StringWithDefault(label, current)
		} else {
			value, err = service.prompt.String(label)
		}

		if err != nil {
			return "", err
		}

		if value != "" {
			return value, nil
		}

		fmt.Println("This value is required.")
	}
}

// promptBinPath prompts for an optional explicit path to a CLI tool's
// binary, re-prompting until the path exists as a file (not a directory).
// Empty is always accepted without checking anything — it means "resolve
// the bare command name via PATH at runtime" (see e.g. borg.Config.bin's
// fallback to DefaultBin), not "use this specific file". current pre-fills
// the prompt (shown as "[current]") when updating an already-configured
// backend; empty means there's nothing to default to yet.
func (service commandService) promptBinPath(label string, current string) (string, error) {
	fs := service.fileSystem()

	for {
		value, err := service.promptOptionalStringWithCurrent(label, current)
		if err != nil {
			return "", err
		}

		if value == "" {
			return "", nil
		}

		info, err := fs.Stat(value)
		if err != nil {
			fmt.Printf("No file found at %q: %v\n", value, err)
			continue
		}

		if info.IsDir() {
			fmt.Printf("%q is a directory, not a binary\n", value)
			continue
		}

		return value, nil
	}
}

// promptOptionalStringWithCurrent prompts for a field that may legitimately
// stay empty. When current is non-empty it's shown and kept on an empty
// answer (shared.PromptService.StringWithDefault); otherwise a plain prompt
// is used, so a fresh create doesn't show a misleading "[]" default.
func (service commandService) promptOptionalStringWithCurrent(label string, current string) (string, error) {
	if current != "" {
		return service.prompt.StringWithDefault(label, current)
	}

	return service.prompt.String(label)
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

// promptBackendSettings gathers backend-specific settings, pre-filling from
// currentSettings when the backend being configured (name) is the same one
// currentSettings belongs to (currentBackend) — e.g. "dackup backend
// update" without switching to a different backend. Adding a backend means
// adding a case here, matching the "one case per backend" pattern used by
// internal/backend.ParseSettings and internal/backend.Factory.
func (service commandService) promptBackendSettings(name string, currentBackend string, currentSettings json.RawMessage) (json.RawMessage, error) {
	switch name {
	case borg.Name:
		config := borg.DefaultConfig()

		if currentBackend == borg.Name && len(currentSettings) > 0 {
			parsed, err := borg.ParseConfig(currentSettings)
			if err != nil {
				fmt.Printf("Warning: failed to load the current borg settings (%v); starting from defaults\n", err)
			} else {
				config = parsed
			}
		}

		return service.promptBorgSettings(config)
	default:
		return nil, nil
	}
}

// promptBorgSettings gathers borg.Config's fields interactively, using
// current as both the starting point and the pre-filled default shown at
// each prompt — current is borg.DefaultConfig() on a fresh create, or the
// already-configured borg.Config on an update. The repository storage root
// itself (DackupConfig.BackendDir) is not prompted for here — it's a
// top-level config field gathered once by configureBackend's own
// promptBackendDir, not part of backend_settings (see AGENTS.md's "Backend
// interface" section for why).
func (service commandService) promptBorgSettings(current borg.Config) (json.RawMessage, error) {
	config := current

	bin, err := service.promptBinPath("Path to the borg binary (leave empty to use PATH)", config.Bin)
	if err != nil {
		return nil, err
	}
	config.Bin = bin

	globalRepoName, err := service.prompt.StringWithDefault(
		"Name of the global repository (a full mirror, in addition to one per container group)",
		config.GlobalRepoName,
	)
	if err != nil {
		return nil, err
	}
	config.GlobalRepoName = globalRepoName

	encryption, err := service.prompt.StringWithDefault(
		"Borg encryption mode (none, repokey, repokey-blake2, keyfile, keyfile-blake2, authenticated, authenticated-blake2)",
		config.Encryption,
	)
	if err != nil {
		return nil, err
	}
	config.Encryption = encryption

	if strings.TrimSpace(encryption) == "none" {
		config.EncryptedPassphrase = ""
	} else {
		passphraseLabel := "Borg repository passphrase"
		if config.EncryptedPassphrase != "" {
			passphraseLabel = "Borg repository passphrase (leave empty to keep the current one)"
		}

		passphrase, err := service.prompt.String(passphraseLabel)
		if err != nil {
			return nil, err
		}

		switch {
		case passphrase == "" && config.EncryptedPassphrase != "":
			// Keep the existing encrypted_passphrase unchanged.
		case passphrase == "":
			return nil, fmt.Errorf("a passphrase is required for encryption %q", encryption)
		case service.options != nil && service.options.DryRun:
			fmt.Println("[dry-run] Would encrypt the passphrase and store it as encrypted_passphrase")
			config.EncryptedPassphrase = "[dry-run placeholder, not a real ciphertext]"
		default:
			encryptedPassphrase, err := service.secrets.Encrypt(passphrase)
			if err != nil {
				return nil, err
			}

			config.EncryptedPassphrase = encryptedPassphrase
		}
	}

	compression, err := service.promptOptionalStringWithCurrent(
		"Borg compression, e.g. zstd,6 (leave empty for borg's default)",
		config.Compression,
	)
	if err != nil {
		return nil, err
	}
	config.Compression = compression

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return json.Marshal(config)
}

func printBackend(config shared.DackupConfig) {
	if config.Backend == "" {
		fmt.Println("No backend configured (using the default no-op backend)")
		return
	}

	fmt.Printf("Backend: %s\n", config.Backend)

	if config.BackendDir != "" {
		fmt.Printf("Backend directory: %s\n", config.BackendDir)
	} else {
		fmt.Println("Backend directory: (not set)")
	}

	if len(config.BackendSettings) == 0 {
		fmt.Println("Settings: none")
		return
	}

	pretty, err := json.MarshalIndent(maskEncryptedSettings(config.BackendSettings), "", "  ")
	if err != nil {
		fmt.Printf("Settings: %s\n", config.BackendSettings)
		return
	}

	fmt.Printf("Settings:\n%s\n", pretty)
}

// maskEncryptedSettings masks any top-level "encrypted_*" field in raw
// backend_settings JSON before display, so "dackup backend show" never
// prints an encrypted secret's ciphertext. Falls back to the raw bytes
// unchanged if they don't decode as a flat JSON object.
func maskEncryptedSettings(raw json.RawMessage) json.RawMessage {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return raw
	}

	for key, value := range fields {
		if strings.HasPrefix(key, "encrypted_") && string(value) != `""` && string(value) != "null" {
			fields[key] = json.RawMessage(`"[set]"`)
		}
	}

	masked, err := json.Marshal(fields)
	if err != nil {
		return raw
	}

	return masked
}
