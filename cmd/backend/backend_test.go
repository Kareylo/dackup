package backend

import (
	"bufio"
	"dackup/internal/backend/borg"
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

	got, err := service.promptBackendSettings("kopia", "", nil)
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
