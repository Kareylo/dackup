package backend

import (
	"bufio"
	"dackup/internal/backend"
	"dackup/internal/backend/borg"
	"dackup/internal/backend/kopia"
	"dackup/internal/backend/kopia/storage/azure"
	"dackup/internal/backend/kopia/storage/b2"
	"dackup/internal/backend/kopia/storage/filesystem"
	"dackup/internal/backend/kopia/storage/gcs"
	"dackup/internal/backend/kopia/storage/rclone"
	"dackup/internal/backend/kopia/storage/s3"
	"dackup/internal/backend/kopia/storage/sftp"
	"dackup/internal/backend/kopia/storage/webdav"
	"dackup/internal/shared"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
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
// so it's gathered once here rather than per-backend. The value is always
// required: every backend implemented so far needs local repository
// storage — even kopia's remote storage types still need it for their
// local per-repository config files (see AGENTS.md's "Backend interface"
// section).
func (service commandService) promptBackendDir(current string) (string, error) {
	return service.promptRequiredStringWithCurrent("Backend repository storage directory (backend_dir)", current)
}

// promptRequiredStringWithCurrent prompts for a field that must not stay
// empty, pre-filling from current (shown as "[current]") when updating an
// already-configured value, and re-prompting until a non-empty answer is
// given.
func (service commandService) promptRequiredStringWithCurrent(label string, current string) (string, error) {
	for {
		value, err := service.promptOptionalStringWithCurrent(label, current)
		if err != nil {
			return "", err
		}

		if value != "" {
			return value, nil
		}

		fmt.Println("This value is required.")
	}
}

// promptRequiredFilePath prompts for a path to a file that must already
// exist on disk (e.g. an SSH keyfile, a GCS service-account JSON file) —
// unlike promptBinPath, empty is not accepted.
func (service commandService) promptRequiredFilePath(label string, current string) (string, error) {
	for {
		value, err := service.promptBinPath(label, current)
		if err != nil {
			return "", err
		}

		if value != "" {
			return value, nil
		}

		fmt.Println("This value is required.")
	}
}

// promptEncryptedSecret prompts for a secret value and encrypts it via
// service.secrets before returning its ciphertext. current is the existing
// ciphertext (empty on a fresh create); an empty answer keeps it unchanged
// when current is set, or is rejected when there's nothing to keep. Honors
// service.options.DryRun the same way promptBorgSettings's own passphrase
// prompt does, to avoid creating a real secret key file during a dry run.
func (service commandService) promptEncryptedSecret(label string, current string) (string, error) {
	promptLabel := label
	if current != "" {
		promptLabel = label + " (leave empty to keep the current one)"
	}

	value, err := service.prompt.String(promptLabel)
	if err != nil {
		return "", err
	}

	switch {
	case value == "" && current != "":
		return current, nil
	case value == "":
		return "", fmt.Errorf("%s is required", label)
	case service.options != nil && service.options.DryRun:
		fmt.Printf("[dry-run] Would encrypt %s and store its ciphertext\n", label)
		return "[dry-run placeholder, not a real ciphertext]", nil
	default:
		return service.secrets.Encrypt(value)
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
	case kopia.Name:
		config := kopia.DefaultConfig()

		if currentBackend == kopia.Name && len(currentSettings) > 0 {
			parsed, err := kopia.ParseConfig(currentSettings)
			if err != nil {
				fmt.Printf("Warning: failed to load the current kopia settings (%v); starting from defaults\n", err)
			} else {
				config = parsed
			}
		}

		return service.promptKopiaSettings(config)
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

// kopiaStorageTypes lists the storage types selectKopiaStorageType offers,
// in the same order as kopia.Config's storage fields, each identified by
// its own subpackage's Name constant (filesystem.Name, s3.Name, ...).
// Adding a storage type means adding it here too, plus a case in
// promptKopiaSettings's switch below.
var kopiaStorageTypes = []string{
	filesystem.Name,
	s3.Name,
	sftp.Name,
	b2.Name,
	azure.Name,
	gcs.Name,
	rclone.Name,
	webdav.Name,
}

// promptKopiaSettings gathers kopia.Config's fields interactively, mirroring
// promptBorgSettings's shape. Unlike borg, kopia repositories are always
// encrypted, so there is no encryption-mode prompt — a password is always
// required. The repository storage root itself (DackupConfig.BackendDir) is
// not prompted for here — see promptBorgSettings's doc comment for why; it
// doubles as the local home for kopia's per-repository config files
// regardless of which storage type is chosen below (see AGENTS.md's
// "Backend interface" section).
func (service commandService) promptKopiaSettings(current kopia.Config) (json.RawMessage, error) {
	config := current

	bin, err := service.promptBinPath("Path to the kopia binary (leave empty to use PATH)", config.Bin)
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

	currentStorageType := config.StorageType
	if currentStorageType == "" {
		currentStorageType = filesystem.Name
	}

	storageType, err := service.selectKopiaStorageType(currentStorageType)
	if err != nil {
		return nil, err
	}
	config.StorageType = storageType

	// Clear every other storage type's settings so switching types doesn't
	// leave a stale, unused block behind in the stored JSON.
	config.S3, config.SFTP, config.B2, config.Azure, config.GCS, config.Rclone, config.WebDAV = nil, nil, nil, nil, nil, nil, nil

	switch storageType {
	case filesystem.Name:
		// No extra settings; repository data lives under backend_dir.
	case s3.Name:
		s3, err := service.promptKopiaS3Settings(current.S3)
		if err != nil {
			return nil, err
		}
		config.S3 = &s3
	case sftp.Name:
		sftp, err := service.promptKopiaSFTPSettings(current.SFTP)
		if err != nil {
			return nil, err
		}
		config.SFTP = &sftp
	case b2.Name:
		b2, err := service.promptKopiaB2Settings(current.B2)
		if err != nil {
			return nil, err
		}
		config.B2 = &b2
	case azure.Name:
		azure, err := service.promptKopiaAzureSettings(current.Azure)
		if err != nil {
			return nil, err
		}
		config.Azure = &azure
	case gcs.Name:
		gcs, err := service.promptKopiaGCSSettings(current.GCS)
		if err != nil {
			return nil, err
		}
		config.GCS = &gcs
	case rclone.Name:
		rclone, err := service.promptKopiaRcloneSettings(current.Rclone)
		if err != nil {
			return nil, err
		}
		config.Rclone = &rclone
	case webdav.Name:
		webdav, err := service.promptKopiaWebDAVSettings(current.WebDAV)
		if err != nil {
			return nil, err
		}
		config.WebDAV = &webdav
	}

	encryptedPassword, err := service.promptEncryptedSecret("Kopia repository password", config.EncryptedPassword)
	if err != nil {
		return nil, err
	}
	config.EncryptedPassword = encryptedPassword

	compression, err := service.promptOptionalStringWithCurrent(
		"Kopia compression algorithm, e.g. zstd (leave empty for kopia's default)",
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

func (service commandService) selectKopiaStorageType(current string) (string, error) {
	fmt.Println("Available kopia storage types:")
	for _, storageType := range kopiaStorageTypes {
		fmt.Printf("- %s\n", storageType)
	}

	for {
		choice, err := service.prompt.StringWithDefault("Kopia storage type", current)
		if err != nil {
			return "", err
		}

		for _, storageType := range kopiaStorageTypes {
			if storageType == choice {
				return choice, nil
			}
		}

		fmt.Println("Please choose one of the listed storage types.")
	}
}

// promptKopiaS3Settings gathers s3.Storage's fields, using current (nil
// on a fresh create, or the already-configured settings on an update) as
// the pre-filled default at each prompt.
func (service commandService) promptKopiaS3Settings(current *s3.Storage) (s3.Storage, error) {
	config := s3.Storage{}
	if current != nil {
		config = *current
	}

	bucket, err := service.promptRequiredStringWithCurrent("S3 bucket", config.Bucket)
	if err != nil {
		return s3.Storage{}, err
	}
	config.Bucket = bucket

	accessKeyID, err := service.promptRequiredStringWithCurrent("S3 access key ID", config.AccessKeyID)
	if err != nil {
		return s3.Storage{}, err
	}
	config.AccessKeyID = accessKeyID

	encryptedSecretAccessKey, err := service.promptEncryptedSecret("S3 secret access key", config.EncryptedSecretAccessKey)
	if err != nil {
		return s3.Storage{}, err
	}
	config.EncryptedSecretAccessKey = encryptedSecretAccessKey

	endpoint, err := service.promptOptionalStringWithCurrent("S3 endpoint, host[:port] only — no http(s):// (leave empty for AWS S3)", config.Endpoint)
	if err != nil {
		return s3.Storage{}, err
	}

	endpoint, impliedDisableTLS := splitEndpointScheme(endpoint)
	config.Endpoint = endpoint

	region, err := service.promptOptionalStringWithCurrent("S3 region (leave empty if not required)", config.Region)
	if err != nil {
		return s3.Storage{}, err
	}
	config.Region = region

	prefix, err := service.promptOptionalStringWithCurrent("S3 key prefix (leave empty for none)", config.Prefix)
	if err != nil {
		return s3.Storage{}, err
	}
	config.Prefix = prefix

	if impliedDisableTLS != nil {
		config.DisableTLS = *impliedDisableTLS
		fmt.Printf("Detected a scheme in the S3 endpoint; disable_tls set to %v automatically\n", config.DisableTLS)
	} else {
		disableTLS, err := service.prompt.Bool("Disable TLS for the S3 endpoint?", config.DisableTLS)
		if err != nil {
			return s3.Storage{}, err
		}
		config.DisableTLS = disableTLS
	}

	return config, nil
}

// splitEndpointScheme strips a leading "http://" or "https://" from a
// user-typed S3 endpoint. kopia's --endpoint flag wants a bare host[:port]
// and controls the scheme separately via --disable-tls — passing it a full
// URL makes kopia reject the connection, so this both fixes the value and
// tells the caller what disable_tls should be, before promptKopiaS3Settings
// asks (or skips asking) that question. Returns the stripped endpoint and,
// when a scheme was found, the disable_tls value it implies (true for
// http, false for https) — nil when no scheme was given, meaning the
// caller should still ask.
func splitEndpointScheme(endpoint string) (string, *bool) {
	lower := strings.ToLower(endpoint)

	switch {
	case strings.HasPrefix(lower, "https://"):
		disableTLS := false
		return endpoint[len("https://"):], &disableTLS
	case strings.HasPrefix(lower, "http://"):
		disableTLS := true
		return endpoint[len("http://"):], &disableTLS
	default:
		return endpoint, nil
	}
}

// promptKopiaSFTPSettings gathers sftp.Storage's fields. Auth is
// mutually exclusive (see sftp.Storage.Validate): a keyfile path is
// tried first, and only if left empty is a password prompted for.
func (service commandService) promptKopiaSFTPSettings(current *sftp.Storage) (sftp.Storage, error) {
	config := sftp.Storage{}
	if current != nil {
		config = *current
	}

	host, err := service.promptRequiredStringWithCurrent("SFTP host", config.Host)
	if err != nil {
		return sftp.Storage{}, err
	}
	config.Host = host

	portDefault := strconv.Itoa(sftp.DefaultPort)
	if config.Port != 0 {
		portDefault = strconv.Itoa(config.Port)
	}

	portInput, err := service.prompt.StringWithDefault("SFTP port", portDefault)
	if err != nil {
		return sftp.Storage{}, err
	}

	port, err := strconv.Atoi(strings.TrimSpace(portInput))
	if err != nil || port <= 0 {
		return sftp.Storage{}, fmt.Errorf("invalid SFTP port %q", portInput)
	}
	config.Port = port

	username, err := service.promptRequiredStringWithCurrent("SFTP username", config.Username)
	if err != nil {
		return sftp.Storage{}, err
	}
	config.Username = username

	remotePath, err := service.promptRequiredStringWithCurrent("SFTP remote base path", config.Path)
	if err != nil {
		return sftp.Storage{}, err
	}
	config.Path = remotePath

	keyfilePath, err := service.promptBinPath("Path to an SSH private key file (leave empty to use a password instead)", config.KeyfilePath)
	if err != nil {
		return sftp.Storage{}, err
	}
	config.KeyfilePath = keyfilePath

	if config.KeyfilePath == "" {
		encryptedPassword, err := service.promptEncryptedSecret("SFTP password", config.EncryptedPassword)
		if err != nil {
			return sftp.Storage{}, err
		}
		config.EncryptedPassword = encryptedPassword
	} else {
		config.EncryptedPassword = ""
	}

	knownHostsPath, err := service.promptBinPath("Path to a known_hosts file (leave empty to skip host key verification)", config.KnownHostsPath)
	if err != nil {
		return sftp.Storage{}, err
	}
	config.KnownHostsPath = knownHostsPath

	return config, nil
}

func (service commandService) promptKopiaB2Settings(current *b2.Storage) (b2.Storage, error) {
	config := b2.Storage{}
	if current != nil {
		config = *current
	}

	bucket, err := service.promptRequiredStringWithCurrent("B2 bucket", config.Bucket)
	if err != nil {
		return b2.Storage{}, err
	}
	config.Bucket = bucket

	keyID, err := service.promptRequiredStringWithCurrent("B2 key ID", config.KeyID)
	if err != nil {
		return b2.Storage{}, err
	}
	config.KeyID = keyID

	encryptedApplicationKey, err := service.promptEncryptedSecret("B2 application key", config.EncryptedApplicationKey)
	if err != nil {
		return b2.Storage{}, err
	}
	config.EncryptedApplicationKey = encryptedApplicationKey

	prefix, err := service.promptOptionalStringWithCurrent("B2 key prefix (leave empty for none)", config.Prefix)
	if err != nil {
		return b2.Storage{}, err
	}
	config.Prefix = prefix

	return config, nil
}

func (service commandService) promptKopiaAzureSettings(current *azure.Storage) (azure.Storage, error) {
	config := azure.Storage{}
	if current != nil {
		config = *current
	}

	container, err := service.promptRequiredStringWithCurrent("Azure container", config.Container)
	if err != nil {
		return azure.Storage{}, err
	}
	config.Container = container

	storageAccount, err := service.promptRequiredStringWithCurrent("Azure storage account", config.StorageAccount)
	if err != nil {
		return azure.Storage{}, err
	}
	config.StorageAccount = storageAccount

	encryptedStorageKey, err := service.promptEncryptedSecret("Azure storage key", config.EncryptedStorageKey)
	if err != nil {
		return azure.Storage{}, err
	}
	config.EncryptedStorageKey = encryptedStorageKey

	prefix, err := service.promptOptionalStringWithCurrent("Azure blob prefix (leave empty for none)", config.Prefix)
	if err != nil {
		return azure.Storage{}, err
	}
	config.Prefix = prefix

	return config, nil
}

func (service commandService) promptKopiaGCSSettings(current *gcs.Storage) (gcs.Storage, error) {
	config := gcs.Storage{}
	if current != nil {
		config = *current
	}

	bucket, err := service.promptRequiredStringWithCurrent("GCS bucket", config.Bucket)
	if err != nil {
		return gcs.Storage{}, err
	}
	config.Bucket = bucket

	credentialsFilePath, err := service.promptRequiredFilePath("Path to a GCS service account credentials JSON file", config.CredentialsFilePath)
	if err != nil {
		return gcs.Storage{}, err
	}
	config.CredentialsFilePath = credentialsFilePath

	prefix, err := service.promptOptionalStringWithCurrent("GCS object prefix (leave empty for none)", config.Prefix)
	if err != nil {
		return gcs.Storage{}, err
	}
	config.Prefix = prefix

	return config, nil
}

// promptKopiaRcloneSettings gathers rclone.Storage's fields. Unlike
// the other storage types, rclone needs no secret from dackup: its
// credentials for RemoteName live in the operator's own rclone.conf
// (referenced only by RcloneStorage.ConfigFilePath when non-default), the
// same "externally-managed path, not a value we encrypt" reasoning as
// SFTPStorage.KeyfilePath.
func (service commandService) promptKopiaRcloneSettings(current *rclone.Storage) (rclone.Storage, error) {
	config := rclone.Storage{}
	if current != nil {
		config = *current
	}

	remoteName, err := service.promptRequiredStringWithCurrent("Rclone remote name (as configured in rclone.conf, e.g. b2remote)", config.RemoteName)
	if err != nil {
		return rclone.Storage{}, err
	}
	config.RemoteName = remoteName

	remotePath, err := service.promptOptionalStringWithCurrent("Base path within the rclone remote (leave empty for its root)", config.RemotePath)
	if err != nil {
		return rclone.Storage{}, err
	}
	config.RemotePath = remotePath

	rcloneExePath, err := service.promptBinPath("Path to the rclone binary (leave empty to use PATH)", config.RcloneExePath)
	if err != nil {
		return rclone.Storage{}, err
	}
	config.RcloneExePath = rcloneExePath

	configFilePath, err := service.promptBinPath("Path to a non-default rclone.conf (leave empty to use rclone's own default)", config.ConfigFilePath)
	if err != nil {
		return rclone.Storage{}, err
	}
	config.ConfigFilePath = configFilePath

	return config, nil
}

// promptKopiaWebDAVSettings gathers webdav.Storage's fields. Username
// and password are prompted together: leaving username empty configures an
// unauthenticated server and skips the password prompt entirely, matching
// WebDAVStorage.Validate's "both set or both empty" rule.
func (service commandService) promptKopiaWebDAVSettings(current *webdav.Storage) (webdav.Storage, error) {
	config := webdav.Storage{}
	if current != nil {
		config = *current
	}

	url, err := service.promptRequiredStringWithCurrent("WebDAV base URL", config.URL)
	if err != nil {
		return webdav.Storage{}, err
	}
	config.URL = url

	username, err := service.promptOptionalStringWithCurrent("WebDAV username (leave empty for an unauthenticated server)", config.Username)
	if err != nil {
		return webdav.Storage{}, err
	}
	config.Username = username

	if config.Username == "" {
		config.EncryptedPassword = ""
	} else {
		encryptedPassword, err := service.promptEncryptedSecret("WebDAV password", config.EncryptedPassword)
		if err != nil {
			return webdav.Storage{}, err
		}
		config.EncryptedPassword = encryptedPassword
	}

	return config, nil
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

// maskEncryptedSettings masks any "encrypted_*" field in raw backend_settings
// JSON before display, at any nesting depth, so "dackup backend show" never
// prints an encrypted secret's ciphertext — kopia's storage settings nest
// theirs under a sub-object (e.g. s3.encrypted_secret_access_key) rather
// than keeping them top-level like borg's encrypted_passphrase. Falls back
// to the raw bytes unchanged if they don't decode as JSON at all.
func maskEncryptedSettings(raw json.RawMessage) json.RawMessage {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return raw
	}

	masked, err := json.Marshal(maskEncryptedValue(value))
	if err != nil {
		return raw
	}

	return masked
}

func maskEncryptedValue(value any) any {
	fields, ok := value.(map[string]any)
	if !ok {
		return value
	}

	masked := make(map[string]any, len(fields))
	for key, fieldValue := range fields {
		if str, isString := fieldValue.(string); isString && str != "" && strings.HasPrefix(key, "encrypted_") {
			masked[key] = "[set]"
			continue
		}

		masked[key] = maskEncryptedValue(fieldValue)
	}

	return masked
}
