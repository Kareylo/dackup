package backend

import (
	"dackup/internal/backend/restic"
	"dackup/internal/backend/restic/storage/azure"
	"dackup/internal/backend/restic/storage/b2"
	"dackup/internal/backend/restic/storage/filesystem"
	"dackup/internal/backend/restic/storage/gcs"
	"dackup/internal/backend/restic/storage/rclone"
	"dackup/internal/backend/restic/storage/rest"
	"dackup/internal/backend/restic/storage/s3"
	"dackup/internal/backend/restic/storage/sftp"
	"dackup/internal/backend/restic/storage/swift"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// resticStorageTypes lists the storage types selectResticStorageType offers,
// in the same order as restic.Config's storage fields, each identified by
// its own subpackage's Name constant. Adding a storage type means adding it
// here too, plus a case in promptResticSettings's switch below — mirrors
// kopiaStorageTypes.
var resticStorageTypes = []string{
	filesystem.Name,
	s3.Name,
	sftp.Name,
	b2.Name,
	azure.Name,
	gcs.Name,
	rclone.Name,
	rest.Name,
	swift.Name,
}

// promptResticSettings gathers restic.Config's fields interactively,
// mirroring promptKopiaSettings's shape. Like kopia (and unlike borg),
// restic repositories are always encrypted, so there is no encryption-mode
// prompt — a password is always required. Unlike either, restic has no
// per-repository compression setting to prompt for (it compresses
// automatically). The repository storage root itself
// (DackupConfig.BackendDir) is not prompted for here — see
// promptBorgSettings's doc comment for why.
func (service commandService) promptResticSettings(current restic.Config) (json.RawMessage, error) {
	config := current

	bin, err := service.promptBinPath("Path to the restic binary (leave empty to use PATH)", config.Bin)
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

	storageType, err := service.selectResticStorageType(currentStorageType)
	if err != nil {
		return nil, err
	}
	config.StorageType = storageType

	// Clear every other storage type's settings so switching types doesn't
	// leave a stale, unused block behind in the stored JSON.
	config.S3, config.SFTP, config.B2, config.Azure, config.GCS, config.Rclone, config.Rest, config.Swift = nil, nil, nil, nil, nil, nil, nil, nil

	switch storageType {
	case filesystem.Name:
		// No extra settings; repository data lives under backend_dir.
	case s3.Name:
		s3, err := service.promptResticS3Settings(current.S3)
		if err != nil {
			return nil, err
		}
		config.S3 = &s3
	case sftp.Name:
		sftp, err := service.promptResticSFTPSettings(current.SFTP)
		if err != nil {
			return nil, err
		}
		config.SFTP = &sftp
	case b2.Name:
		b2, err := service.promptResticB2Settings(current.B2)
		if err != nil {
			return nil, err
		}
		config.B2 = &b2
	case azure.Name:
		azure, err := service.promptResticAzureSettings(current.Azure)
		if err != nil {
			return nil, err
		}
		config.Azure = &azure
	case gcs.Name:
		gcs, err := service.promptResticGCSSettings(current.GCS)
		if err != nil {
			return nil, err
		}
		config.GCS = &gcs
	case rclone.Name:
		rclone, err := service.promptResticRcloneSettings(current.Rclone)
		if err != nil {
			return nil, err
		}
		config.Rclone = &rclone
	case rest.Name:
		rest, err := service.promptResticRestSettings(current.Rest)
		if err != nil {
			return nil, err
		}
		config.Rest = &rest
	case swift.Name:
		swift, err := service.promptResticSwiftSettings(current.Swift)
		if err != nil {
			return nil, err
		}
		config.Swift = &swift
	}

	encryptedPassword, err := service.promptEncryptedSecret("Restic repository password", config.EncryptedPassword)
	if err != nil {
		return nil, err
	}
	config.EncryptedPassword = encryptedPassword

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return json.Marshal(config)
}

// selectResticStorageType prompts for one of resticStorageTypes,
// re-prompting until a listed one is chosen.
func (service commandService) selectResticStorageType(current string) (string, error) {
	fmt.Println("Available restic storage types:")
	for _, storageType := range resticStorageTypes {
		fmt.Printf("- %s\n", storageType)
	}

	for {
		choice, err := service.prompt.StringWithDefault("Restic storage type", current)
		if err != nil {
			return "", err
		}

		for _, storageType := range resticStorageTypes {
			if storageType == choice {
				return choice, nil
			}
		}

		fmt.Println("Please choose one of the listed storage types.")
	}
}

// promptResticS3Settings gathers s3.Storage's fields, using current (nil on
// a fresh create, or the already-configured settings on an update) as the
// pre-filled default at each prompt.
func (service commandService) promptResticS3Settings(current *s3.Storage) (s3.Storage, error) {
	config := s3.Storage{}
	if current != nil {
		config = *current
	}

	endpoint, err := service.promptRequiredStringWithCurrent("S3 endpoint, host[:port] only — no http(s):// (e.g. s3.us-east-1.amazonaws.com)", config.Endpoint)
	if err != nil {
		return s3.Storage{}, err
	}

	endpoint, impliedDisableTLS := splitEndpointScheme(endpoint)
	config.Endpoint = endpoint

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

// promptResticSFTPSettings gathers sftp.Storage's fields. Unlike kopia's
// SFTP storage type, restic's has no password option at all — see
// sftp.Storage's doc comment — so there is no auth-method branch here, just
// an optional keyfile path.
func (service commandService) promptResticSFTPSettings(current *sftp.Storage) (sftp.Storage, error) {
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

	keyfilePath, err := service.promptBinPath("Path to an SSH private key file (leave empty to use ssh's own default identity/agent)", config.KeyfilePath)
	if err != nil {
		return sftp.Storage{}, err
	}
	config.KeyfilePath = keyfilePath

	knownHostsPath, err := service.promptBinPath("Path to a known_hosts file (leave empty to use ssh's own default host-key verification)", config.KnownHostsPath)
	if err != nil {
		return sftp.Storage{}, err
	}
	config.KnownHostsPath = knownHostsPath

	return config, nil
}

// promptResticB2Settings gathers b2.Storage's fields, using current (nil on
// a fresh create, or the already-configured settings on an update) as the
// pre-filled default at each prompt.
func (service commandService) promptResticB2Settings(current *b2.Storage) (b2.Storage, error) {
	config := b2.Storage{}
	if current != nil {
		config = *current
	}

	bucket, err := service.promptRequiredStringWithCurrent("B2 bucket", config.Bucket)
	if err != nil {
		return b2.Storage{}, err
	}
	config.Bucket = bucket

	accountID, err := service.promptRequiredStringWithCurrent("B2 account ID", config.AccountID)
	if err != nil {
		return b2.Storage{}, err
	}
	config.AccountID = accountID

	encryptedAccountKey, err := service.promptEncryptedSecret("B2 account key", config.EncryptedAccountKey)
	if err != nil {
		return b2.Storage{}, err
	}
	config.EncryptedAccountKey = encryptedAccountKey

	prefix, err := service.promptOptionalStringWithCurrent("B2 path prefix (leave empty for none)", config.Prefix)
	if err != nil {
		return b2.Storage{}, err
	}
	config.Prefix = prefix

	return config, nil
}

// promptResticAzureSettings gathers azure.Storage's fields, using current
// (nil on a fresh create, or the already-configured settings on an update)
// as the pre-filled default at each prompt.
func (service commandService) promptResticAzureSettings(current *azure.Storage) (azure.Storage, error) {
	config := azure.Storage{}
	if current != nil {
		config = *current
	}

	container, err := service.promptRequiredStringWithCurrent("Azure container", config.Container)
	if err != nil {
		return azure.Storage{}, err
	}
	config.Container = container

	accountName, err := service.promptRequiredStringWithCurrent("Azure storage account name", config.AccountName)
	if err != nil {
		return azure.Storage{}, err
	}
	config.AccountName = accountName

	encryptedAccountKey, err := service.promptEncryptedSecret("Azure storage account key", config.EncryptedAccountKey)
	if err != nil {
		return azure.Storage{}, err
	}
	config.EncryptedAccountKey = encryptedAccountKey

	prefix, err := service.promptOptionalStringWithCurrent("Azure blob path prefix (leave empty for none)", config.Prefix)
	if err != nil {
		return azure.Storage{}, err
	}
	config.Prefix = prefix

	return config, nil
}

// promptResticGCSSettings gathers gcs.Storage's fields, using current (nil
// on a fresh create, or the already-configured settings on an update) as
// the pre-filled default at each prompt.
func (service commandService) promptResticGCSSettings(current *gcs.Storage) (gcs.Storage, error) {
	config := gcs.Storage{}
	if current != nil {
		config = *current
	}

	bucket, err := service.promptRequiredStringWithCurrent("GCS bucket", config.Bucket)
	if err != nil {
		return gcs.Storage{}, err
	}
	config.Bucket = bucket

	projectID, err := service.promptOptionalStringWithCurrent("GCS project ID (leave empty if not required)", config.ProjectID)
	if err != nil {
		return gcs.Storage{}, err
	}
	config.ProjectID = projectID

	credentialsFilePath, err := service.promptBinPath("Path to a GCS service account credentials JSON file (leave empty when targeting a local emulator)", config.CredentialsFilePath)
	if err != nil {
		return gcs.Storage{}, err
	}
	config.CredentialsFilePath = credentialsFilePath

	prefix, err := service.promptOptionalStringWithCurrent("GCS path prefix (leave empty for none)", config.Prefix)
	if err != nil {
		return gcs.Storage{}, err
	}
	config.Prefix = prefix

	return config, nil
}

// promptResticRcloneSettings gathers rclone.Storage's fields. Like kopia's
// rclone storage type, restic needs no secret from dackup: its credentials
// for RemoteName live in the operator's own rclone.conf.
func (service commandService) promptResticRcloneSettings(current *rclone.Storage) (rclone.Storage, error) {
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

// promptResticRestSettings gathers rest.Storage's fields. Username and
// password are prompted together: leaving username empty configures an
// unauthenticated server and skips the password prompt entirely, matching
// rest.Storage.Validate's "both set or both empty" rule — mirrors
// promptKopiaWebDAVSettings.
func (service commandService) promptResticRestSettings(current *rest.Storage) (rest.Storage, error) {
	config := rest.Storage{}
	if current != nil {
		config = *current
	}

	url, err := service.promptRequiredStringWithCurrent("REST server base URL (including scheme), e.g. https://backup.example.com:8000", config.URL)
	if err != nil {
		return rest.Storage{}, err
	}
	config.URL = url

	username, err := service.promptOptionalStringWithCurrent("REST username (leave empty for an unauthenticated server)", config.Username)
	if err != nil {
		return rest.Storage{}, err
	}
	config.Username = username

	if config.Username == "" {
		config.EncryptedPassword = ""
	} else {
		encryptedPassword, err := service.promptEncryptedSecret("REST password", config.EncryptedPassword)
		if err != nil {
			return rest.Storage{}, err
		}
		config.EncryptedPassword = encryptedPassword
	}

	return config, nil
}

// promptResticSwiftSettings gathers swift.Storage's fields, using current
// (nil on a fresh create, or the already-configured settings on an update)
// as the pre-filled default at each prompt.
func (service commandService) promptResticSwiftSettings(current *swift.Storage) (swift.Storage, error) {
	config := swift.Storage{}
	if current != nil {
		config = *current
	}

	container, err := service.promptRequiredStringWithCurrent("Swift container", config.Container)
	if err != nil {
		return swift.Storage{}, err
	}
	config.Container = container

	authURL, err := service.promptRequiredStringWithCurrent("Swift Keystone auth URL", config.AuthURL)
	if err != nil {
		return swift.Storage{}, err
	}
	config.AuthURL = authURL

	username, err := service.promptRequiredStringWithCurrent("Swift username", config.Username)
	if err != nil {
		return swift.Storage{}, err
	}
	config.Username = username

	encryptedPassword, err := service.promptEncryptedSecret("Swift password", config.EncryptedPassword)
	if err != nil {
		return swift.Storage{}, err
	}
	config.EncryptedPassword = encryptedPassword

	tenantName, err := service.promptOptionalStringWithCurrent("Swift tenant/project name (leave empty if not required)", config.TenantName)
	if err != nil {
		return swift.Storage{}, err
	}
	config.TenantName = tenantName

	regionName, err := service.promptOptionalStringWithCurrent("Swift region name (leave empty if not required)", config.RegionName)
	if err != nil {
		return swift.Storage{}, err
	}
	config.RegionName = regionName

	prefix, err := service.promptOptionalStringWithCurrent("Swift path prefix (leave empty for none)", config.Prefix)
	if err != nil {
		return swift.Storage{}, err
	}
	config.Prefix = prefix

	return config, nil
}
