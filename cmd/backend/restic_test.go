package backend

import (
	"dackup/internal/backend/restic"
	"dackup/internal/backend/restic/storage/filesystem"
	"dackup/internal/backend/restic/storage/s3"
	"dackup/internal/backend/restic/storage/sftp"
	"dackup/internal/shared"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPromptResticSettings_EncryptsPasswordBeforeStoring(t *testing.T) {
	// bin -> (empty), global_repo_name -> (default), storage_type ->
	// (default, filesystem), password -> hunter2
	service := newTestServiceWithSecretKey(t, "\n\n\nhunter2\n")

	got, err := service.promptResticSettings(restic.DefaultConfig())
	if err != nil {
		t.Fatalf("promptResticSettings returned error: %v", err)
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

func TestPromptResticSettings_EmptyPasswordKeepsCurrentEncryptedPassword(t *testing.T) {
	current := restic.DefaultConfig()
	current.EncryptedPassword = "v1:existing-ciphertext"

	// bin -> (empty), global_repo_name -> (kept), storage_type -> (default,
	// filesystem), password -> (empty, keep current)
	service := newTestServiceWithSecretKey(t, "\n\n\n\n")

	got, err := service.promptResticSettings(current)
	if err != nil {
		t.Fatalf("promptResticSettings returned error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatalf("failed to unmarshal settings: %v", err)
	}

	if settings["encrypted_password"] != "v1:existing-ciphertext" {
		t.Fatalf("expected encrypted_password to stay %q, got %v", "v1:existing-ciphertext", settings["encrypted_password"])
	}
}

func TestPromptResticSettings_EmptyPasswordWithNoCurrentOneIsRejected(t *testing.T) {
	// bin -> (empty), global_repo_name -> (kept), storage_type -> (default,
	// filesystem), password -> (empty, nothing to keep)
	service := newTestServiceWithSecretKey(t, "\n\n\n\n")

	if _, err := service.promptResticSettings(restic.DefaultConfig()); err == nil {
		t.Fatal("expected an error when no password is given and none is already set")
	}
}

func TestPromptResticSettings_DryRunSkipsRealEncryption(t *testing.T) {
	service := newTestServiceWithSecretKey(t, "\n\n\nhunter2\n")
	service.options = &shared.Options{DryRun: true}

	got, err := service.promptResticSettings(restic.DefaultConfig())
	if err != nil {
		t.Fatalf("promptResticSettings returned error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatalf("failed to unmarshal settings: %v", err)
	}

	if _, err := os.Stat(service.secrets.(shared.AESFileSecretStore).KeyPath); err == nil {
		t.Fatal("expected dry-run to not create a real secret key file")
	}
}

func TestSelectResticStorageType_RejectsUnknownThenAcceptsValid(t *testing.T) {
	service := newTestService("dropbox\ns3\n")

	got, err := service.selectResticStorageType(filesystem.Name)
	if err != nil {
		t.Fatalf("selectResticStorageType returned error: %v", err)
	}

	if got != s3.Name {
		t.Fatalf("expected %q, got %q", s3.Name, got)
	}
}

func TestPromptResticSettings_SelectsS3StorageAndGathersItsSettings(t *testing.T) {
	// bin -> (empty), global_repo_name -> (default), storage_type -> s3,
	// endpoint, bucket, access_key_id, secret_access_key, region -> (empty),
	// prefix -> (empty), disable_tls -> (default, no), password -> hunter2
	input := "\n\ns3\ns3.us-east-1.amazonaws.com\nmy-bucket\nAKID\nsecretkey\n\n\n\nhunter2\n"
	service := newTestServiceWithSecretKey(t, input)

	got, err := service.promptResticSettings(restic.DefaultConfig())
	if err != nil {
		t.Fatalf("promptResticSettings returned error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatalf("failed to unmarshal settings: %v", err)
	}

	if settings["storage_type"] != "s3" {
		t.Fatalf("expected storage_type %q, got %v", "s3", settings["storage_type"])
	}

	s3Settings, ok := settings["s3"].(map[string]any)
	if !ok {
		t.Fatalf("expected an s3 settings block, got %#v", settings["s3"])
	}

	if s3Settings["bucket"] != "my-bucket" || s3Settings["endpoint"] != "s3.us-east-1.amazonaws.com" {
		t.Fatalf("unexpected s3 settings: %#v", s3Settings)
	}
}

func TestPromptResticSFTPSettings_NoPasswordPrompt(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("fake key"), 0o600); err != nil {
		t.Fatalf("failed to create fake keyfile: %v", err)
	}

	// host, port -> (default), username, path, keyfile_path,
	// known_hosts_path -> (empty)
	input := "backup.example.com\n\ndackup\n/srv/backups\n" + keyPath + "\n\n"
	service := newTestService(input)

	got, err := service.promptResticSFTPSettings(nil)
	if err != nil {
		t.Fatalf("promptResticSFTPSettings returned error: %v", err)
	}

	if got.Host != "backup.example.com" || got.Port != sftp.DefaultPort || got.Username != "dackup" || got.Path != "/srv/backups" {
		t.Fatalf("unexpected SFTPStorage: %#v", got)
	}

	if got.KeyfilePath != keyPath {
		t.Fatalf("expected keyfile_path %q, got %q", keyPath, got.KeyfilePath)
	}
}

func TestPromptResticSettings_SwitchingStorageTypeClearsThePreviousOne(t *testing.T) {
	current := restic.DefaultConfig()
	current.EncryptedPassword = "v1:existing-ciphertext"
	current.StorageType = sftp.Name
	current.SFTP = &sftp.Storage{Host: "h", Username: "u", Path: "/p"}

	// bin -> (empty), global_repo_name -> (default), storage_type ->
	// filesystem (explicit, overriding the current "sftp" default),
	// password -> (empty, keep current)
	service := newTestServiceWithSecretKey(t, "\n\nfilesystem\n\n")

	got, err := service.promptResticSettings(current)
	if err != nil {
		t.Fatalf("promptResticSettings returned error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatalf("failed to unmarshal settings: %v", err)
	}

	if _, ok := settings["sftp"]; ok {
		t.Fatalf("expected the stale sftp settings block to be cleared, got %#v", settings["sftp"])
	}
}

func TestPromptResticB2Settings_GathersAllFields(t *testing.T) {
	service := newTestServiceWithSecretKey(t, "my-bucket\nacct-id\napplicationkey\ndackup\n")

	got, err := service.promptResticB2Settings(nil)
	if err != nil {
		t.Fatalf("promptResticB2Settings returned error: %v", err)
	}

	if got.Bucket != "my-bucket" || got.AccountID != "acct-id" || got.Prefix != "dackup" {
		t.Fatalf("unexpected b2.Storage: %#v", got)
	}

	decrypted, err := service.secrets.Decrypt(got.EncryptedAccountKey)
	if err != nil {
		t.Fatalf("failed to decrypt stored account key: %v", err)
	}
	if decrypted != "applicationkey" {
		t.Fatalf("expected decrypted account key %q, got %q", "applicationkey", decrypted)
	}
}

func TestPromptResticAzureSettings_GathersAllFields(t *testing.T) {
	service := newTestServiceWithSecretKey(t, "my-container\nmyaccount\nstoragekey\ndackup\n")

	got, err := service.promptResticAzureSettings(nil)
	if err != nil {
		t.Fatalf("promptResticAzureSettings returned error: %v", err)
	}

	if got.Container != "my-container" || got.AccountName != "myaccount" || got.Prefix != "dackup" {
		t.Fatalf("unexpected azure.Storage: %#v", got)
	}

	decrypted, err := service.secrets.Decrypt(got.EncryptedAccountKey)
	if err != nil {
		t.Fatalf("failed to decrypt stored account key: %v", err)
	}
	if decrypted != "storagekey" {
		t.Fatalf("expected decrypted account key %q, got %q", "storagekey", decrypted)
	}
}

func TestPromptResticGCSSettings_GathersAllFields(t *testing.T) {
	credentialsPath := filepath.Join(t.TempDir(), "gcs-credentials.json")
	if err := os.WriteFile(credentialsPath, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("failed to create fake credentials file: %v", err)
	}

	service := newTestService("my-bucket\nmy-project\n" + credentialsPath + "\ndackup\n")

	got, err := service.promptResticGCSSettings(nil)
	if err != nil {
		t.Fatalf("promptResticGCSSettings returned error: %v", err)
	}

	if got.Bucket != "my-bucket" || got.ProjectID != "my-project" || got.CredentialsFilePath != credentialsPath || got.Prefix != "dackup" {
		t.Fatalf("unexpected gcs.Storage: %#v", got)
	}
}

func TestPromptResticRcloneSettings_MinimalOnlyRequiresRemoteName(t *testing.T) {
	// remote_name, remote_path -> (empty), rclone_exe_path -> (empty),
	// config_file_path -> (empty)
	service := newTestService("b2remote\n\n\n\n")

	got, err := service.promptResticRcloneSettings(nil)
	if err != nil {
		t.Fatalf("promptResticRcloneSettings returned error: %v", err)
	}

	if got.RemoteName != "b2remote" || got.RemotePath != "" || got.RcloneExePath != "" || got.ConfigFilePath != "" {
		t.Fatalf("unexpected rclone.Storage: %#v", got)
	}
}

func TestPromptResticRcloneSettings_GathersAllFields(t *testing.T) {
	dir := t.TempDir()

	rcloneExePath := filepath.Join(dir, "rclone")
	if err := os.WriteFile(rcloneExePath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to create fake rclone binary: %v", err)
	}

	rcloneConfigPath := filepath.Join(dir, "rclone.conf")
	if err := os.WriteFile(rcloneConfigPath, []byte("[b2remote]\n"), 0o600); err != nil {
		t.Fatalf("failed to create fake rclone.conf: %v", err)
	}

	service := newTestService("b2remote\ndackup\n" + rcloneExePath + "\n" + rcloneConfigPath + "\n")

	got, err := service.promptResticRcloneSettings(nil)
	if err != nil {
		t.Fatalf("promptResticRcloneSettings returned error: %v", err)
	}

	if got.RemoteName != "b2remote" || got.RemotePath != "dackup" {
		t.Fatalf("unexpected rclone.Storage: %#v", got)
	}

	if got.RcloneExePath != rcloneExePath {
		t.Fatalf("expected rclone_exe_path %q, got %q", rcloneExePath, got.RcloneExePath)
	}

	if got.ConfigFilePath != rcloneConfigPath {
		t.Fatalf("expected config_file_path %q, got %q", rcloneConfigPath, got.ConfigFilePath)
	}
}

func TestPromptResticRestSettings_UnauthenticatedSkipsPasswordPrompt(t *testing.T) {
	service := newTestServiceWithSecretKey(t, "https://backup.example.com:8000\n\n")

	got, err := service.promptResticRestSettings(nil)
	if err != nil {
		t.Fatalf("promptResticRestSettings returned error: %v", err)
	}

	if got.URL != "https://backup.example.com:8000" || got.Username != "" || got.EncryptedPassword != "" {
		t.Fatalf("unexpected rest.Storage: %#v", got)
	}
}

func TestPromptResticRestSettings_GathersCredentialsWhenUsernameGiven(t *testing.T) {
	service := newTestServiceWithSecretKey(t, "https://backup.example.com:8000\ndackup\nhunter2\n")

	got, err := service.promptResticRestSettings(nil)
	if err != nil {
		t.Fatalf("promptResticRestSettings returned error: %v", err)
	}

	if got.URL != "https://backup.example.com:8000" || got.Username != "dackup" {
		t.Fatalf("unexpected rest.Storage: %#v", got)
	}

	decrypted, err := service.secrets.Decrypt(got.EncryptedPassword)
	if err != nil {
		t.Fatalf("failed to decrypt stored password: %v", err)
	}
	if decrypted != "hunter2" {
		t.Fatalf("expected decrypted password %q, got %q", "hunter2", decrypted)
	}
}

func TestPromptResticSwiftSettings_GathersAllFields(t *testing.T) {
	input := "my-container\nhttps://keystone.example.com\ndackup\nhunter2\nmyproject\nRegionOne\ndackup\n"
	service := newTestServiceWithSecretKey(t, input)

	got, err := service.promptResticSwiftSettings(nil)
	if err != nil {
		t.Fatalf("promptResticSwiftSettings returned error: %v", err)
	}

	if got.Container != "my-container" || got.AuthURL != "https://keystone.example.com" || got.Username != "dackup" {
		t.Fatalf("unexpected swift.Storage: %#v", got)
	}

	if got.TenantName != "myproject" || got.RegionName != "RegionOne" || got.Prefix != "dackup" {
		t.Fatalf("unexpected optional fields on swift.Storage: %#v", got)
	}

	decrypted, err := service.secrets.Decrypt(got.EncryptedPassword)
	if err != nil {
		t.Fatalf("failed to decrypt stored password: %v", err)
	}
	if decrypted != "hunter2" {
		t.Fatalf("expected decrypted password %q, got %q", "hunter2", decrypted)
	}
}

func TestPromptResticSettings_SelectsB2StorageAndGathersItsSettings(t *testing.T) {
	// bin -> (empty), global_repo_name -> (default), storage_type -> b2,
	// bucket, account_id, application_key, prefix -> (empty), password ->
	// hunter2.
	input := "\n\nb2\nmy-bucket\nacct-id\napplicationkey\n\nhunter2\n"
	service := newTestServiceWithSecretKey(t, input)

	got, err := service.promptResticSettings(restic.DefaultConfig())
	if err != nil {
		t.Fatalf("promptResticSettings returned error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatalf("failed to unmarshal settings: %v", err)
	}

	if settings["storage_type"] != "b2" {
		t.Fatalf("expected storage_type %q, got %v", "b2", settings["storage_type"])
	}

	if _, ok := settings["b2"].(map[string]any); !ok {
		t.Fatalf("expected a b2 settings block, got %#v", settings["b2"])
	}
}

func TestPromptResticSettings_SelectsRestStorageAndGathersItsSettings(t *testing.T) {
	// bin -> (empty), global_repo_name -> (default), storage_type -> rest,
	// url, username -> (empty), password -> hunter2.
	input := "\n\nrest\nhttps://backup.example.com:8000\n\nhunter2\n"
	service := newTestServiceWithSecretKey(t, input)

	got, err := service.promptResticSettings(restic.DefaultConfig())
	if err != nil {
		t.Fatalf("promptResticSettings returned error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatalf("failed to unmarshal settings: %v", err)
	}

	if settings["storage_type"] != "rest" {
		t.Fatalf("expected storage_type %q, got %v", "rest", settings["storage_type"])
	}

	if _, ok := settings["rest"].(map[string]any); !ok {
		t.Fatalf("expected a rest settings block, got %#v", settings["rest"])
	}
}

func TestPromptResticSettings_SelectsSwiftStorageAndGathersItsSettings(t *testing.T) {
	// bin -> (empty), global_repo_name -> (default), storage_type -> swift,
	// container, auth_url, username, password -> hunter2, tenant_name,
	// region_name, prefix -> (empty), password -> hunter2.
	input := "\n\nswift\nmy-container\nhttps://keystone.example.com\ndackup\nhunter2\n\n\n\nhunter2\n"
	service := newTestServiceWithSecretKey(t, input)

	got, err := service.promptResticSettings(restic.DefaultConfig())
	if err != nil {
		t.Fatalf("promptResticSettings returned error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatalf("failed to unmarshal settings: %v", err)
	}

	if settings["storage_type"] != "swift" {
		t.Fatalf("expected storage_type %q, got %v", "swift", settings["storage_type"])
	}

	if _, ok := settings["swift"].(map[string]any); !ok {
		t.Fatalf("expected a swift settings block, got %#v", settings["swift"])
	}
}

func TestPromptBackendSettings_ConfiguresRestic(t *testing.T) {
	// bin -> (empty), global_repo_name -> (default), storage_type ->
	// (default, filesystem), password -> hunter2.
	service := newTestServiceWithSecretKey(t, "\n\n\nhunter2\n")

	got, err := service.promptBackendSettings(restic.Name, "", nil)
	if err != nil {
		t.Fatalf("promptBackendSettings returned error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatalf("failed to unmarshal settings: %v", err)
	}

	if settings["global_repo_name"] != restic.DefaultGlobalRepoName {
		t.Fatalf("expected global_repo_name %q, got %v", restic.DefaultGlobalRepoName, settings["global_repo_name"])
	}
}

func TestPromptBackendSettings_InvalidCurrentResticSettingsFallBackToDefaults(t *testing.T) {
	current := json.RawMessage(`{invalid-json`)

	// bin -> (empty), global_repo_name -> (default), storage_type ->
	// (default, filesystem), password -> hunter2.
	service := newTestServiceWithSecretKey(t, "\n\n\nhunter2\n")

	got, err := service.promptBackendSettings(restic.Name, restic.Name, current)
	if err != nil {
		t.Fatalf("promptBackendSettings returned error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatalf("failed to unmarshal settings: %v", err)
	}

	if settings["global_repo_name"] != restic.DefaultGlobalRepoName {
		t.Fatalf("expected global_repo_name to fall back to default %q, got %v", restic.DefaultGlobalRepoName, settings["global_repo_name"])
	}
}
