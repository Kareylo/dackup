package backend

import (
	"dackup/internal/backend/kopia"
	"dackup/internal/backend/kopia/storage/azure"
	"dackup/internal/backend/kopia/storage/b2"
	"dackup/internal/backend/kopia/storage/filesystem"
	"dackup/internal/backend/kopia/storage/gcs"
	"dackup/internal/backend/kopia/storage/rclone"
	"dackup/internal/backend/kopia/storage/s3"
	"dackup/internal/backend/kopia/storage/sftp"
	"dackup/internal/backend/kopia/storage/webdav"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

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

// selectKopiaStorageType prompts for one of kopiaStorageTypes, re-prompting
// until a listed one is chosen.
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

// promptKopiaB2Settings gathers b2.Storage's fields, using current (nil on
// a fresh create, or the already-configured settings on an update) as the
// pre-filled default at each prompt.
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

// promptKopiaAzureSettings gathers azure.Storage's fields, using current
// (nil on a fresh create, or the already-configured settings on an update)
// as the pre-filled default at each prompt.
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

// promptKopiaGCSSettings gathers gcs.Storage's fields, using current (nil
// on a fresh create, or the already-configured settings on an update) as
// the pre-filled default at each prompt.
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
