package backend

import (
	"bufio"
	"dackup/internal/backend/borg"
	"dackup/internal/backend/kopia"
	"dackup/internal/backend/kopia/storage/filesystem"
	"dackup/internal/backend/kopia/storage/s3"
	"dackup/internal/backend/kopia/storage/sftp"
	"dackup/internal/shared"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestService(input string) commandService {
	return commandService{
		options: &shared.Options{},
		prompt:  shared.NewPromptService(bufio.NewReader(strings.NewReader(input))),
	}
}

func newTestServiceWithSecretKey(t *testing.T, input string) commandService {
	t.Helper()

	return commandService{
		options: &shared.Options{},
		prompt:  shared.NewPromptService(bufio.NewReader(strings.NewReader(input))),
		secrets: shared.AESFileSecretStore{KeyPath: filepath.Join(t.TempDir(), "secret.key")},
	}
}

func TestConfigureBackend_ConfiguresBorg(t *testing.T) {
	// name -> borg, backend_dir -> /mnt/backup/borg-repos, bin -> (empty),
	// global_repo_name -> (default), encryption -> none, compression -> (empty)
	service := newTestServiceWithSecretKey(t, "borg\n/mnt/backup/borg-repos\n\n\nnone\n\n")

	config, configured, err := service.configureBackend(shared.DackupConfig{User: "test-user"})
	if err != nil {
		t.Fatalf("configureBackend returned error: %v", err)
	}

	if !configured {
		t.Fatal("expected configured to be true")
	}

	if config.Backend != "borg" {
		t.Fatalf("expected backend %q, got %q", "borg", config.Backend)
	}

	if config.BackendDir != "/mnt/backup/borg-repos" {
		t.Fatalf("expected backend_dir %q, got %q", "/mnt/backup/borg-repos", config.BackendDir)
	}

	if len(config.BackendSettings) == 0 {
		t.Fatal("expected non-empty backend settings")
	}
}

func TestConfigureBackend_UpdatePrefillsExistingBackendDir(t *testing.T) {
	// name -> borg, backend_dir -> (kept via empty input, pre-filled from
	// current), bin -> (empty), global_repo_name -> (default),
	// encryption -> none, compression -> (empty)
	service := newTestServiceWithSecretKey(t, "borg\n\n\n\nnone\n\n")

	config, configured, err := service.configureBackend(shared.DackupConfig{
		User:       "test-user",
		BackendDir: "/mnt/backup/borg-repos",
	})
	if err != nil {
		t.Fatalf("configureBackend returned error: %v", err)
	}

	if !configured {
		t.Fatal("expected configured to be true")
	}

	if config.BackendDir != "/mnt/backup/borg-repos" {
		t.Fatalf("expected backend_dir to stay %q, got %q", "/mnt/backup/borg-repos", config.BackendDir)
	}
}

func TestPromptBackendDir_RequiresNonEmptyValueOnCreate(t *testing.T) {
	service := newTestService("\n\n/mnt/backup/borg-repos\n")

	got, err := service.promptBackendDir("")
	if err != nil {
		t.Fatalf("promptBackendDir returned error: %v", err)
	}

	if got != "/mnt/backup/borg-repos" {
		t.Fatalf("expected %q, got %q", "/mnt/backup/borg-repos", got)
	}
}

func TestPromptBinPath_EmptyIsAcceptedWithoutChecking(t *testing.T) {
	service := newTestService("\n")

	got, err := service.promptBinPath("Path to the borg binary", "")
	if err != nil {
		t.Fatalf("promptBinPath returned error: %v", err)
	}

	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestPromptBinPath_AcceptsExistingFile(t *testing.T) {
	binPath := filepath.Join(t.TempDir(), "borg")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to create fake binary: %v", err)
	}

	service := newTestService(binPath + "\n")

	got, err := service.promptBinPath("Path to the borg binary", "")
	if err != nil {
		t.Fatalf("promptBinPath returned error: %v", err)
	}

	if got != binPath {
		t.Fatalf("expected %q, got %q", binPath, got)
	}
}

func TestPromptBinPath_ReprocessesUntilAnExistingFileIsGiven(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "borg")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to create fake binary: %v", err)
	}

	missingPath := filepath.Join(dir, "does-not-exist")

	service := newTestService(missingPath + "\n" + dir + "\n" + binPath + "\n")

	got, err := service.promptBinPath("Path to the borg binary", "")
	if err != nil {
		t.Fatalf("promptBinPath returned error: %v", err)
	}

	if got != binPath {
		t.Fatalf("expected %q after rejecting a missing path and a directory, got %q", binPath, got)
	}
}

func TestPromptBinPath_EmptyInputKeepsCurrentValue(t *testing.T) {
	binPath := filepath.Join(t.TempDir(), "borg")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to create fake binary: %v", err)
	}

	service := newTestService("\n")

	got, err := service.promptBinPath("Path to the borg binary", binPath)
	if err != nil {
		t.Fatalf("promptBinPath returned error: %v", err)
	}

	if got != binPath {
		t.Fatalf("expected empty input to keep current value %q, got %q", binPath, got)
	}
}

func TestSelectBackendName_RejectsUnknownThenAcceptsValid(t *testing.T) {
	service := newTestService("bogus\nborg\n")

	got, err := service.selectBackendName([]string{"borg"})
	if err != nil {
		t.Fatalf("selectBackendName returned error: %v", err)
	}

	if got != "borg" {
		t.Fatalf("expected %q, got %q", "borg", got)
	}
}

func TestPromptBackendSettings_UnknownBackendReturnsNil(t *testing.T) {
	service := newTestService("")

	got, err := service.promptBackendSettings("restic", "", nil)
	if err != nil {
		t.Fatalf("promptBackendSettings returned error: %v", err)
	}

	if got != nil {
		t.Fatalf("expected nil settings, got %#v", got)
	}
}

func TestPromptBackendSettings_PrefillsFromCurrentSettingsForSameBackend(t *testing.T) {
	current := json.RawMessage(`{"global_repo_name":"prod","encryption":"none"}`)

	// bin -> (empty), global_repo_name -> (kept via empty input),
	// encryption -> (kept via empty input), compression -> (empty)
	service := newTestService("\n\n\n\n")

	got, err := service.promptBackendSettings(borg.Name, borg.Name, current)
	if err != nil {
		t.Fatalf("promptBackendSettings returned error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatalf("failed to unmarshal settings: %v", err)
	}

	if settings["global_repo_name"] != "prod" {
		t.Fatalf("expected global_repo_name to stay %q, got %v", "prod", settings["global_repo_name"])
	}
}

func TestPromptBackendSettings_IgnoresCurrentSettingsWhenSwitchingBackend(t *testing.T) {
	current := json.RawMessage(`{"global_repo_name":"prod","encryption":"none"}`)

	// bin -> (empty), global_repo_name -> (empty, so the builtin default
	// "global" applies, not "prod" from a different backend's settings),
	// encryption -> (empty, defaults to repokey), passphrase -> hunter2,
	// compression -> (empty)
	service := newTestServiceWithSecretKey(t, "\n\n\nhunter2\n\n")

	got, err := service.promptBackendSettings(borg.Name, "some-other-backend", current)
	if err != nil {
		t.Fatalf("promptBackendSettings returned error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatalf("failed to unmarshal settings: %v", err)
	}

	if settings["global_repo_name"] != borg.DefaultGlobalRepoName {
		t.Fatalf("expected global_repo_name to fall back to the default %q, got %v", borg.DefaultGlobalRepoName, settings["global_repo_name"])
	}
}

func TestPromptBorgSettings_EncryptsPassphraseBeforeStoring(t *testing.T) {
	service := newTestServiceWithSecretKey(t, "\n\nrepokey\nhunter2\n\n")

	got, err := service.promptBorgSettings(borg.DefaultConfig())
	if err != nil {
		t.Fatalf("promptBorgSettings returned error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatalf("failed to unmarshal settings: %v", err)
	}

	if settings["encryption"] != "repokey" {
		t.Fatalf("expected encryption %q, got %v", "repokey", settings["encryption"])
	}

	encryptedPassphrase, _ := settings["encrypted_passphrase"].(string)
	if encryptedPassphrase == "" || encryptedPassphrase == "hunter2" {
		t.Fatalf("expected encrypted_passphrase to be set and not equal the plaintext, got %q", encryptedPassphrase)
	}

	decrypted, err := service.secrets.Decrypt(encryptedPassphrase)
	if err != nil {
		t.Fatalf("failed to decrypt stored passphrase: %v", err)
	}

	if decrypted != "hunter2" {
		t.Fatalf("expected decrypted passphrase %q, got %q", "hunter2", decrypted)
	}
}

func TestPromptBorgSettings_EmptyPassphraseKeepsCurrentEncryptedPassphrase(t *testing.T) {
	current := borg.DefaultConfig()
	current.EncryptedPassphrase = "v1:existing-ciphertext"

	// bin -> (empty), global_repo_name -> (kept), encryption -> (kept,
	// repokey), passphrase -> (empty, keep current), compression -> (empty)
	service := newTestServiceWithSecretKey(t, "\n\n\n\n\n")

	got, err := service.promptBorgSettings(current)
	if err != nil {
		t.Fatalf("promptBorgSettings returned error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatalf("failed to unmarshal settings: %v", err)
	}

	if settings["encrypted_passphrase"] != "v1:existing-ciphertext" {
		t.Fatalf("expected encrypted_passphrase to stay %q, got %v", "v1:existing-ciphertext", settings["encrypted_passphrase"])
	}
}

func TestPromptBorgSettings_SwitchingToNoneEncryptionClearsPassphrase(t *testing.T) {
	current := borg.DefaultConfig()
	current.EncryptedPassphrase = "v1:existing-ciphertext"

	// bin -> (empty), global_repo_name -> (kept), encryption -> none,
	// compression -> (empty)
	service := newTestServiceWithSecretKey(t, "\n\nnone\n\n")

	got, err := service.promptBorgSettings(current)
	if err != nil {
		t.Fatalf("promptBorgSettings returned error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatalf("failed to unmarshal settings: %v", err)
	}

	if value, ok := settings["encrypted_passphrase"]; ok && value != "" {
		t.Fatalf("expected encrypted_passphrase to be cleared, got %v", value)
	}
}

func TestPromptBorgSettings_EmptyPassphraseWithNoCurrentOneIsRejected(t *testing.T) {
	// bin -> (empty), global_repo_name -> (kept), encryption -> (kept,
	// repokey), passphrase -> (empty, nothing to keep)
	service := newTestServiceWithSecretKey(t, "\n\n\n\n")

	if _, err := service.promptBorgSettings(borg.DefaultConfig()); err == nil {
		t.Fatal("expected an error when no passphrase is given and none is already set")
	}
}

func TestPromptBorgSettings_DryRunSkipsRealEncryption(t *testing.T) {
	service := newTestServiceWithSecretKey(t, "\n\nrepokey\nhunter2\n\n")
	service.options = &shared.Options{DryRun: true}

	got, err := service.promptBorgSettings(borg.DefaultConfig())
	if err != nil {
		t.Fatalf("promptBorgSettings returned error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatalf("failed to unmarshal settings: %v", err)
	}

	if _, err := os.Stat(service.secrets.(shared.AESFileSecretStore).KeyPath); err == nil {
		t.Fatal("expected dry-run to not create a real secret key file")
	}
}

func TestPromptKopiaSettings_EncryptsPasswordBeforeStoring(t *testing.T) {
	// bin -> (empty), global_repo_name -> (default), storage_type ->
	// (default, filesystem), password -> hunter2, compression -> (empty)
	service := newTestServiceWithSecretKey(t, "\n\n\nhunter2\n\n")

	got, err := service.promptKopiaSettings(kopia.DefaultConfig())
	if err != nil {
		t.Fatalf("promptKopiaSettings returned error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatalf("failed to unmarshal settings: %v", err)
	}

	encryptedPassword, _ := settings["encrypted_password"].(string)
	if encryptedPassword == "" || encryptedPassword == "hunter2" {
		t.Fatalf("expected encrypted_password to be set and not equal the plaintext, got %q", encryptedPassword)
	}

	decrypted, err := service.secrets.Decrypt(encryptedPassword)
	if err != nil {
		t.Fatalf("failed to decrypt stored password: %v", err)
	}

	if decrypted != "hunter2" {
		t.Fatalf("expected decrypted password %q, got %q", "hunter2", decrypted)
	}
}

func TestPromptKopiaSettings_EmptyPasswordKeepsCurrentEncryptedPassword(t *testing.T) {
	current := kopia.DefaultConfig()
	current.EncryptedPassword = "v1:existing-ciphertext"

	// bin -> (empty), global_repo_name -> (kept), storage_type -> (default,
	// filesystem), password -> (empty, keep current), compression -> (empty)
	service := newTestServiceWithSecretKey(t, "\n\n\n\n\n")

	got, err := service.promptKopiaSettings(current)
	if err != nil {
		t.Fatalf("promptKopiaSettings returned error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatalf("failed to unmarshal settings: %v", err)
	}

	if settings["encrypted_password"] != "v1:existing-ciphertext" {
		t.Fatalf("expected encrypted_password to stay %q, got %v", "v1:existing-ciphertext", settings["encrypted_password"])
	}
}

func TestPromptKopiaSettings_EmptyPasswordWithNoCurrentOneIsRejected(t *testing.T) {
	// bin -> (empty), global_repo_name -> (kept), storage_type -> (default,
	// filesystem), password -> (empty, nothing to keep)
	service := newTestServiceWithSecretKey(t, "\n\n\n\n")

	if _, err := service.promptKopiaSettings(kopia.DefaultConfig()); err == nil {
		t.Fatal("expected an error when no password is given and none is already set")
	}
}

func TestPromptKopiaSettings_DryRunSkipsRealEncryption(t *testing.T) {
	// bin -> (empty), global_repo_name -> (default), storage_type ->
	// (default, filesystem), password -> hunter2, compression -> (empty)
	service := newTestServiceWithSecretKey(t, "\n\n\nhunter2\n\n")
	service.options = &shared.Options{DryRun: true}

	got, err := service.promptKopiaSettings(kopia.DefaultConfig())
	if err != nil {
		t.Fatalf("promptKopiaSettings returned error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatalf("failed to unmarshal settings: %v", err)
	}

	if _, err := os.Stat(service.secrets.(shared.AESFileSecretStore).KeyPath); err == nil {
		t.Fatal("expected dry-run to not create a real secret key file")
	}
}

func TestSelectKopiaStorageType_RejectsUnknownThenAcceptsValid(t *testing.T) {
	service := newTestService("dropbox\ns3\n")

	got, err := service.selectKopiaStorageType(filesystem.Name)
	if err != nil {
		t.Fatalf("selectKopiaStorageType returned error: %v", err)
	}

	if got != s3.Name {
		t.Fatalf("expected %q, got %q", s3.Name, got)
	}
}

func TestPromptKopiaSettings_SelectsS3StorageAndGathersItsSettings(t *testing.T) {
	// bin -> (empty), global_repo_name -> (default), storage_type -> s3,
	// bucket, access_key_id, secret_access_key, endpoint -> (empty),
	// region -> (empty), prefix -> (empty), disable_tls -> (default, no),
	// password -> hunter2, compression -> (empty)
	service := newTestServiceWithSecretKey(t, "\n\ns3\nmy-bucket\nAKID\nsecretkey\n\n\n\n\nhunter2\n\n")

	got, err := service.promptKopiaSettings(kopia.DefaultConfig())
	if err != nil {
		t.Fatalf("promptKopiaSettings returned error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatalf("failed to unmarshal settings: %v", err)
	}

	if settings["storage_type"] != "s3" {
		t.Fatalf("expected storage_type %q, got %v", "s3", settings["storage_type"])
	}

	s3, ok := settings["s3"].(map[string]any)
	if !ok {
		t.Fatalf("expected an s3 settings block, got %#v", settings["s3"])
	}

	if s3["bucket"] != "my-bucket" {
		t.Fatalf("expected bucket %q, got %v", "my-bucket", s3["bucket"])
	}

	if s3["access_key_id"] != "AKID" {
		t.Fatalf("expected access_key_id %q, got %v", "AKID", s3["access_key_id"])
	}

	encryptedSecretAccessKey, _ := s3["encrypted_secret_access_key"].(string)
	if encryptedSecretAccessKey == "" || encryptedSecretAccessKey == "secretkey" {
		t.Fatalf("expected encrypted_secret_access_key to be set and not equal the plaintext, got %q", encryptedSecretAccessKey)
	}

	decrypted, err := service.secrets.Decrypt(encryptedSecretAccessKey)
	if err != nil {
		t.Fatalf("failed to decrypt stored secret access key: %v", err)
	}
	if decrypted != "secretkey" {
		t.Fatalf("expected decrypted secret access key %q, got %q", "secretkey", decrypted)
	}
}

func TestPromptKopiaSettings_SwitchingStorageTypeClearsThePreviousOne(t *testing.T) {
	current := kopia.DefaultConfig()
	current.EncryptedPassword = "v1:existing-ciphertext"
	current.StorageType = sftp.Name
	current.SFTP = &sftp.Storage{Host: "h", Username: "u", Path: "/p", KeyfilePath: "/key"}

	// bin -> (empty), global_repo_name -> (default), storage_type ->
	// filesystem (explicit, overriding the current "sftp" default),
	// password -> (empty, keep current), compression -> (empty)
	service := newTestServiceWithSecretKey(t, "\n\nfilesystem\n\n\n")

	got, err := service.promptKopiaSettings(current)
	if err != nil {
		t.Fatalf("promptKopiaSettings returned error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatalf("failed to unmarshal settings: %v", err)
	}

	if _, ok := settings["sftp"]; ok {
		t.Fatalf("expected the stale sftp settings block to be cleared, got %#v", settings["sftp"])
	}
}

func TestPromptKopiaS3Settings_GathersAllFields(t *testing.T) {
	service := newTestServiceWithSecretKey(t, "my-bucket\nAKID\nsecretkey\nendpoint.example.com\nus-east-1\ndackup\ny\n")

	got, err := service.promptKopiaS3Settings(nil)
	if err != nil {
		t.Fatalf("promptKopiaS3Settings returned error: %v", err)
	}

	if got.Bucket != "my-bucket" || got.AccessKeyID != "AKID" || got.Endpoint != "endpoint.example.com" || got.Region != "us-east-1" || got.Prefix != "dackup" || !got.DisableTLS {
		t.Fatalf("unexpected S3Storage: %#v", got)
	}
}

func TestPromptKopiaS3Settings_HTTPEndpointStripsSchemeAndSkipsTLSPrompt(t *testing.T) {
	// bucket, access_key_id, secret_access_key, endpoint (with scheme),
	// region -> (empty), prefix -> (empty). No answer for the disable_tls
	// prompt — it must be skipped, or ReadString would hit EOF and error.
	service := newTestServiceWithSecretKey(t, "my-bucket\nAKID\nsecretkey\nhttp://localhost:9000\n\n\n")

	got, err := service.promptKopiaS3Settings(nil)
	if err != nil {
		t.Fatalf("promptKopiaS3Settings returned error: %v", err)
	}

	if got.Endpoint != "localhost:9000" {
		t.Fatalf("expected the http:// scheme to be stripped, got endpoint %q", got.Endpoint)
	}

	if !got.DisableTLS {
		t.Fatal("expected disable_tls to be true when an http:// endpoint was given")
	}
}

func TestPromptKopiaS3Settings_HTTPSEndpointStripsSchemeAndSkipsTLSPrompt(t *testing.T) {
	service := newTestServiceWithSecretKey(t, "my-bucket\nAKID\nsecretkey\nhttps://s3.example.com\n\n\n")

	got, err := service.promptKopiaS3Settings(nil)
	if err != nil {
		t.Fatalf("promptKopiaS3Settings returned error: %v", err)
	}

	if got.Endpoint != "s3.example.com" {
		t.Fatalf("expected the https:// scheme to be stripped, got endpoint %q", got.Endpoint)
	}

	if got.DisableTLS {
		t.Fatal("expected disable_tls to be false when an https:// endpoint was given")
	}
}

func TestSplitEndpointScheme(t *testing.T) {
	cases := []struct {
		input          string
		wantEndpoint   string
		wantDisableTLS *bool
	}{
		{"localhost:9000", "localhost:9000", nil},
		{"", "", nil},
		{"http://localhost:9000", "localhost:9000", boolPtr(true)},
		{"HTTP://localhost:9000", "localhost:9000", boolPtr(true)},
		{"https://s3.example.com", "s3.example.com", boolPtr(false)},
		{"HTTPS://s3.example.com", "s3.example.com", boolPtr(false)},
	}

	for _, testCase := range cases {
		endpoint, disableTLS := splitEndpointScheme(testCase.input)

		if endpoint != testCase.wantEndpoint {
			t.Fatalf("splitEndpointScheme(%q): expected endpoint %q, got %q", testCase.input, testCase.wantEndpoint, endpoint)
		}

		switch {
		case testCase.wantDisableTLS == nil && disableTLS != nil:
			t.Fatalf("splitEndpointScheme(%q): expected nil disableTLS, got %v", testCase.input, *disableTLS)
		case testCase.wantDisableTLS != nil && disableTLS == nil:
			t.Fatalf("splitEndpointScheme(%q): expected disableTLS %v, got nil", testCase.input, *testCase.wantDisableTLS)
		case testCase.wantDisableTLS != nil && *testCase.wantDisableTLS != *disableTLS:
			t.Fatalf("splitEndpointScheme(%q): expected disableTLS %v, got %v", testCase.input, *testCase.wantDisableTLS, *disableTLS)
		}
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func TestPromptKopiaSFTPSettings_KeyfileAuthSkipsPasswordPrompt(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("fake key"), 0o600); err != nil {
		t.Fatalf("failed to create fake keyfile: %v", err)
	}

	// host, port -> (default 22), username, path, keyfile_path,
	// known_hosts_path -> (empty)
	service := newTestServiceWithSecretKey(t, "backup.example.com\n\ndackup\n/srv/backups\n"+keyPath+"\n\n")

	got, err := service.promptKopiaSFTPSettings(nil)
	if err != nil {
		t.Fatalf("promptKopiaSFTPSettings returned error: %v", err)
	}

	if got.Host != "backup.example.com" || got.Port != sftp.DefaultPort || got.Username != "dackup" || got.Path != "/srv/backups" {
		t.Fatalf("unexpected SFTPStorage: %#v", got)
	}

	if got.KeyfilePath != keyPath {
		t.Fatalf("expected keyfile_path %q, got %q", keyPath, got.KeyfilePath)
	}

	if got.EncryptedPassword != "" {
		t.Fatalf("expected no password when a keyfile is given, got %q", got.EncryptedPassword)
	}
}

func TestPromptKopiaSFTPSettings_PasswordAuthWhenNoKeyfileGiven(t *testing.T) {
	// host, port -> 2222, username, path, keyfile_path -> (empty),
	// password, known_hosts_path -> (empty)
	service := newTestServiceWithSecretKey(t, "backup.example.com\n2222\ndackup\n/srv/backups\n\nhunter2\n\n")

	got, err := service.promptKopiaSFTPSettings(nil)
	if err != nil {
		t.Fatalf("promptKopiaSFTPSettings returned error: %v", err)
	}

	if got.Port != 2222 {
		t.Fatalf("expected port 2222, got %d", got.Port)
	}

	if got.KeyfilePath != "" {
		t.Fatalf("expected no keyfile_path, got %q", got.KeyfilePath)
	}

	decrypted, err := service.secrets.Decrypt(got.EncryptedPassword)
	if err != nil {
		t.Fatalf("failed to decrypt stored password: %v", err)
	}
	if decrypted != "hunter2" {
		t.Fatalf("expected decrypted password %q, got %q", "hunter2", decrypted)
	}
}

func TestPromptKopiaB2Settings_GathersAllFields(t *testing.T) {
	service := newTestServiceWithSecretKey(t, "my-bucket\nkey-id\napplicationkey\ndackup\n")

	got, err := service.promptKopiaB2Settings(nil)
	if err != nil {
		t.Fatalf("promptKopiaB2Settings returned error: %v", err)
	}

	if got.Bucket != "my-bucket" || got.KeyID != "key-id" || got.Prefix != "dackup" {
		t.Fatalf("unexpected B2Storage: %#v", got)
	}

	decrypted, err := service.secrets.Decrypt(got.EncryptedApplicationKey)
	if err != nil {
		t.Fatalf("failed to decrypt stored application key: %v", err)
	}
	if decrypted != "applicationkey" {
		t.Fatalf("expected decrypted application key %q, got %q", "applicationkey", decrypted)
	}
}

func TestPromptKopiaAzureSettings_GathersAllFields(t *testing.T) {
	service := newTestServiceWithSecretKey(t, "my-container\nmyaccount\nstoragekey\ndackup\n")

	got, err := service.promptKopiaAzureSettings(nil)
	if err != nil {
		t.Fatalf("promptKopiaAzureSettings returned error: %v", err)
	}

	if got.Container != "my-container" || got.StorageAccount != "myaccount" || got.Prefix != "dackup" {
		t.Fatalf("unexpected AzureStorage: %#v", got)
	}

	decrypted, err := service.secrets.Decrypt(got.EncryptedStorageKey)
	if err != nil {
		t.Fatalf("failed to decrypt stored storage key: %v", err)
	}
	if decrypted != "storagekey" {
		t.Fatalf("expected decrypted storage key %q, got %q", "storagekey", decrypted)
	}
}

func TestPromptKopiaGCSSettings_RequiresAnExistingCredentialsFile(t *testing.T) {
	credentialsPath := filepath.Join(t.TempDir(), "gcs-credentials.json")
	if err := os.WriteFile(credentialsPath, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("failed to create fake credentials file: %v", err)
	}

	service := newTestServiceWithSecretKey(t, "my-bucket\n"+credentialsPath+"\ndackup\n")

	got, err := service.promptKopiaGCSSettings(nil)
	if err != nil {
		t.Fatalf("promptKopiaGCSSettings returned error: %v", err)
	}

	if got.Bucket != "my-bucket" || got.CredentialsFilePath != credentialsPath || got.Prefix != "dackup" {
		t.Fatalf("unexpected GCSStorage: %#v", got)
	}
}

func TestPromptKopiaRcloneSettings_MinimalOnlyRequiresRemoteName(t *testing.T) {
	// remote_name, remote_path -> (empty), rclone_exe_path -> (empty),
	// config_file_path -> (empty)
	service := newTestServiceWithSecretKey(t, "b2remote\n\n\n\n")

	got, err := service.promptKopiaRcloneSettings(nil)
	if err != nil {
		t.Fatalf("promptKopiaRcloneSettings returned error: %v", err)
	}

	if got.RemoteName != "b2remote" || got.RemotePath != "" || got.RcloneExePath != "" || got.ConfigFilePath != "" {
		t.Fatalf("unexpected RcloneStorage: %#v", got)
	}
}

func TestPromptKopiaRcloneSettings_GathersAllFields(t *testing.T) {
	dir := t.TempDir()

	rcloneExePath := filepath.Join(dir, "rclone")
	if err := os.WriteFile(rcloneExePath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to create fake rclone binary: %v", err)
	}

	rcloneConfigPath := filepath.Join(dir, "rclone.conf")
	if err := os.WriteFile(rcloneConfigPath, []byte("[b2remote]\n"), 0o600); err != nil {
		t.Fatalf("failed to create fake rclone.conf: %v", err)
	}

	service := newTestServiceWithSecretKey(t, "b2remote\ndackup\n"+rcloneExePath+"\n"+rcloneConfigPath+"\n")

	got, err := service.promptKopiaRcloneSettings(nil)
	if err != nil {
		t.Fatalf("promptKopiaRcloneSettings returned error: %v", err)
	}

	if got.RemoteName != "b2remote" || got.RemotePath != "dackup" {
		t.Fatalf("unexpected RcloneStorage: %#v", got)
	}

	if got.RcloneExePath != rcloneExePath {
		t.Fatalf("expected rclone_exe_path %q, got %q", rcloneExePath, got.RcloneExePath)
	}

	if got.ConfigFilePath != rcloneConfigPath {
		t.Fatalf("expected config_file_path %q, got %q", rcloneConfigPath, got.ConfigFilePath)
	}
}

func TestPromptKopiaWebDAVSettings_UnauthenticatedSkipsPasswordPrompt(t *testing.T) {
	service := newTestServiceWithSecretKey(t, "https://webdav.example.com\n\n")

	got, err := service.promptKopiaWebDAVSettings(nil)
	if err != nil {
		t.Fatalf("promptKopiaWebDAVSettings returned error: %v", err)
	}

	if got.URL != "https://webdav.example.com" || got.Username != "" || got.EncryptedPassword != "" {
		t.Fatalf("unexpected WebDAVStorage: %#v", got)
	}
}

func TestPromptKopiaWebDAVSettings_GathersCredentialsWhenUsernameGiven(t *testing.T) {
	service := newTestServiceWithSecretKey(t, "https://webdav.example.com\ndackup\nhunter2\n")

	got, err := service.promptKopiaWebDAVSettings(nil)
	if err != nil {
		t.Fatalf("promptKopiaWebDAVSettings returned error: %v", err)
	}

	if got.URL != "https://webdav.example.com" || got.Username != "dackup" {
		t.Fatalf("unexpected WebDAVStorage: %#v", got)
	}

	decrypted, err := service.secrets.Decrypt(got.EncryptedPassword)
	if err != nil {
		t.Fatalf("failed to decrypt stored password: %v", err)
	}
	if decrypted != "hunter2" {
		t.Fatalf("expected decrypted password %q, got %q", "hunter2", decrypted)
	}
}

func TestPromptKopiaSettings_SelectsRcloneStorageAndGathersItsSettings(t *testing.T) {
	// bin -> (empty), global_repo_name -> (default), storage_type ->
	// rclone, remote_name, remote_path -> (empty), rclone_exe_path ->
	// (empty), config_file_path -> (empty), password -> hunter2,
	// compression -> (empty)
	service := newTestServiceWithSecretKey(t, "\n\nrclone\nb2remote\n\n\n\nhunter2\n\n")

	got, err := service.promptKopiaSettings(kopia.DefaultConfig())
	if err != nil {
		t.Fatalf("promptKopiaSettings returned error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatalf("failed to unmarshal settings: %v", err)
	}

	if settings["storage_type"] != "rclone" {
		t.Fatalf("expected storage_type %q, got %v", "rclone", settings["storage_type"])
	}

	rclone, ok := settings["rclone"].(map[string]any)
	if !ok {
		t.Fatalf("expected a rclone settings block, got %#v", settings["rclone"])
	}

	if rclone["remote_name"] != "b2remote" {
		t.Fatalf("expected remote_name %q, got %v", "b2remote", rclone["remote_name"])
	}
}

func TestMaskEncryptedSettings_MasksNestedEncryptedFields(t *testing.T) {
	raw := json.RawMessage(`{"storage_type":"s3","s3":{"bucket":"b","access_key_id":"AKID","encrypted_secret_access_key":"v1:abc123"}}`)

	masked := maskEncryptedSettings(raw)

	var fields map[string]any
	if err := json.Unmarshal(masked, &fields); err != nil {
		t.Fatalf("failed to unmarshal masked settings: %v", err)
	}

	s3, ok := fields["s3"].(map[string]any)
	if !ok {
		t.Fatalf("expected an s3 block, got %#v", fields["s3"])
	}

	if s3["bucket"] != "b" || s3["access_key_id"] != "AKID" {
		t.Fatalf("expected non-secret fields to be untouched, got %#v", s3)
	}

	if s3["encrypted_secret_access_key"] != "[set]" {
		t.Fatalf("expected the nested encrypted_secret_access_key to be masked, got %v", s3["encrypted_secret_access_key"])
	}
}

func TestPrintBackend_NoPanicWhenUnset(t *testing.T) {
	printBackend(shared.DackupConfig{})
}

func TestPrintBackend_NoPanicWhenConfigured(t *testing.T) {
	printBackend(shared.DackupConfig{
		Backend:         "borg",
		BackendSettings: []byte(`{"global_repo_name":"global","encrypted_passphrase":"v1:abc123"}`),
	})
}

func TestMaskEncryptedSettings_MasksEncryptedFieldsOnly(t *testing.T) {
	raw := json.RawMessage(`{"global_repo_name":"global","encrypted_passphrase":"v1:abc123"}`)

	masked := maskEncryptedSettings(raw)

	var fields map[string]string
	if err := json.Unmarshal(masked, &fields); err != nil {
		t.Fatalf("failed to unmarshal masked settings: %v", err)
	}

	if fields["global_repo_name"] != "global" {
		t.Fatalf("expected global_repo_name to be untouched, got %q", fields["global_repo_name"])
	}

	if fields["encrypted_passphrase"] != "[set]" {
		t.Fatalf("expected encrypted_passphrase to be masked, got %q", fields["encrypted_passphrase"])
	}
}
