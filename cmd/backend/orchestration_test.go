package backend

import (
	"bufio"
	"dackup/internal/shared"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withBackendConfigFilePath(t *testing.T, path string) {
	t.Helper()

	original := configFilePath
	configFilePath = path

	t.Cleanup(func() {
		configFilePath = original
	})
}

func backendReaderFor(input string) *bufio.Reader {
	return bufio.NewReader(strings.NewReader(input))
}

func TestNewCommandService_BuildsServiceWithDependencies(t *testing.T) {
	service := newCommandService(backendReaderFor(""))

	if service.prompt.Reader == nil {
		t.Fatal("expected prompt.Reader to be set")
	}

	if service.secrets == nil {
		t.Fatal("expected secrets to be set")
	}

	if service.fs == nil {
		t.Fatal("expected fs to be set")
	}
}

func TestRunBackendShow_NoConfigFileReturnsNilError(t *testing.T) {
	withBackendConfigFilePath(t, filepath.Join(t.TempDir(), "missing-config.json"))

	if err := runBackendShow(); err != nil {
		t.Fatalf("expected nil error for a missing config file, got %v", err)
	}
}

func TestRunBackendShow_PrintsConfiguredBackend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withBackendConfigFilePath(t, path)

	seed := shared.DackupConfig{
		User:            "owner",
		Group:           "group",
		Backend:         "borg",
		BackendSettings: []byte(`{"global_repo_name":"global","encryption":"none"}`),
	}
	if err := shared.WriteDackupConfig(path, seed, nil); err != nil {
		t.Fatalf("failed to seed config: %v", err)
	}

	if err := runBackendShow(); err != nil {
		t.Fatalf("runBackendShow returned error: %v", err)
	}
}

func TestRunBackendShow_InvalidConfigReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withBackendConfigFilePath(t, path)

	if err := os.WriteFile(path, []byte("{invalid-json"), 0o644); err != nil {
		t.Fatalf("failed to seed invalid config: %v", err)
	}

	if err := runBackendShow(); err == nil {
		t.Fatal("expected an error for an invalid config file")
	}
}

func TestRunBackendCreateWithReader_MissingConfigFileReturnsError(t *testing.T) {
	withBackendConfigFilePath(t, filepath.Join(t.TempDir(), "missing-config.json"))

	err := runBackendCreateWithReader(backendReaderFor(""))
	if err == nil {
		t.Fatal("expected an error when the config file does not exist")
	}
}

func TestRunBackendCreateWithReader_ConfiguresAndWritesBackend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withBackendConfigFilePath(t, path)

	seed := shared.DackupConfig{User: "owner", Group: "group"}
	if err := shared.WriteDackupConfig(path, seed, nil); err != nil {
		t.Fatalf("failed to seed config: %v", err)
	}

	// name -> borg, backend_dir, bin -> (empty), global_repo_name ->
	// (default), encryption -> none, compression -> (empty).
	input := "borg\n/mnt/backup/borg-repos\n\n\nnone\n\n"

	if err := runBackendCreateWithReader(backendReaderFor(input)); err != nil {
		t.Fatalf("runBackendCreateWithReader returned error: %v", err)
	}

	got, err := shared.ReadDackupConfig(path)
	if err != nil {
		t.Fatalf("failed to read written config: %v", err)
	}

	if got.Backend != "borg" {
		t.Fatalf("expected backend %q, got %q", "borg", got.Backend)
	}

	if got.BackendDir != "/mnt/backup/borg-repos" {
		t.Fatalf("expected backend_dir %q, got %q", "/mnt/backup/borg-repos", got.BackendDir)
	}
}

func TestRunBackendCreateWithReader_DeclinesOverwriteWhenAlreadyConfigured(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withBackendConfigFilePath(t, path)

	seed := shared.DackupConfig{
		User:            "owner",
		Group:           "group",
		Backend:         "borg",
		BackendSettings: []byte(`{"global_repo_name":"global","encryption":"none"}`),
	}
	if err := shared.WriteDackupConfig(path, seed, nil); err != nil {
		t.Fatalf("failed to seed config: %v", err)
	}

	if err := runBackendCreateWithReader(backendReaderFor("n\n")); err != nil {
		t.Fatalf("runBackendCreateWithReader returned error: %v", err)
	}

	got, err := shared.ReadDackupConfig(path)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	if got.Backend != "borg" {
		t.Fatalf("expected backend to stay unchanged, got %q", got.Backend)
	}
}

func TestRunBackendUpdateWithReader_MissingConfigFileReturnsError(t *testing.T) {
	withBackendConfigFilePath(t, filepath.Join(t.TempDir(), "missing-config.json"))

	if err := runBackendUpdateWithReader(backendReaderFor("")); err == nil {
		t.Fatal("expected an error when the config file does not exist")
	}
}

func TestRunBackendUpdateWithReader_NoBackendConfiguredReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withBackendConfigFilePath(t, path)

	seed := shared.DackupConfig{User: "owner", Group: "group"}
	if err := shared.WriteDackupConfig(path, seed, nil); err != nil {
		t.Fatalf("failed to seed config: %v", err)
	}

	if err := runBackendUpdateWithReader(backendReaderFor("")); err == nil {
		t.Fatal("expected an error when no backend is configured yet")
	}
}

func TestRunBackendUpdateWithReader_UpdatesConfiguredBackend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withBackendConfigFilePath(t, path)

	seed := shared.DackupConfig{
		User:            "owner",
		Group:           "group",
		Backend:         "borg",
		BackendDir:      "/mnt/backup/borg-repos",
		BackendSettings: []byte(`{"global_repo_name":"global","encryption":"none"}`),
	}
	if err := shared.WriteDackupConfig(path, seed, nil); err != nil {
		t.Fatalf("failed to seed config: %v", err)
	}

	// name -> borg, backend_dir -> (kept), bin -> (empty),
	// global_repo_name -> (kept), encryption -> none, compression -> (empty).
	input := "borg\n\n\n\nnone\n\n"

	if err := runBackendUpdateWithReader(backendReaderFor(input)); err != nil {
		t.Fatalf("runBackendUpdateWithReader returned error: %v", err)
	}

	got, err := shared.ReadDackupConfig(path)
	if err != nil {
		t.Fatalf("failed to read written config: %v", err)
	}

	if got.BackendDir != "/mnt/backup/borg-repos" {
		t.Fatalf("expected backend_dir to stay %q, got %q", "/mnt/backup/borg-repos", got.BackendDir)
	}
}

func TestRunBackendRemoveWithReader_MissingConfigFileReturnsError(t *testing.T) {
	withBackendConfigFilePath(t, filepath.Join(t.TempDir(), "missing-config.json"))

	if err := runBackendRemoveWithReader(backendReaderFor("")); err == nil {
		t.Fatal("expected an error when the config file does not exist")
	}
}

func TestRunBackendRemoveWithReader_NoBackendConfiguredReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withBackendConfigFilePath(t, path)

	seed := shared.DackupConfig{User: "owner", Group: "group"}
	if err := shared.WriteDackupConfig(path, seed, nil); err != nil {
		t.Fatalf("failed to seed config: %v", err)
	}

	if err := runBackendRemoveWithReader(backendReaderFor("")); err == nil {
		t.Fatal("expected an error when no backend is configured")
	}
}

func TestRunBackendRemoveWithReader_ConfirmedRemovalClearsBackend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withBackendConfigFilePath(t, path)

	seed := shared.DackupConfig{
		User:            "owner",
		Group:           "group",
		Backend:         "borg",
		BackendDir:      "/mnt/backup/borg-repos",
		BackendSettings: []byte(`{"global_repo_name":"global","encryption":"none"}`),
	}
	if err := shared.WriteDackupConfig(path, seed, nil); err != nil {
		t.Fatalf("failed to seed config: %v", err)
	}

	if err := runBackendRemoveWithReader(backendReaderFor("y\n")); err != nil {
		t.Fatalf("runBackendRemoveWithReader returned error: %v", err)
	}

	got, err := shared.ReadDackupConfig(path)
	if err != nil {
		t.Fatalf("failed to read written config: %v", err)
	}

	if got.Backend != "" || got.BackendDir != "" || got.BackendSettings != nil {
		t.Fatalf("expected backend fields to be cleared, got %#v", got)
	}
}

func TestRunBackendRemoveWithReader_DeclinedRemovalLeavesConfigUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withBackendConfigFilePath(t, path)

	seed := shared.DackupConfig{
		User:            "owner",
		Group:           "group",
		Backend:         "borg",
		BackendDir:      "/mnt/backup/borg-repos",
		BackendSettings: []byte(`{"global_repo_name":"global","encryption":"none"}`),
	}
	if err := shared.WriteDackupConfig(path, seed, nil); err != nil {
		t.Fatalf("failed to seed config: %v", err)
	}

	if err := runBackendRemoveWithReader(backendReaderFor("n\n")); err != nil {
		t.Fatalf("runBackendRemoveWithReader returned error: %v", err)
	}

	got, err := shared.ReadDackupConfig(path)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	if got.Backend != "borg" {
		t.Fatalf("expected backend to stay unchanged, got %q", got.Backend)
	}
}
