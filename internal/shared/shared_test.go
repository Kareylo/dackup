package shared

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWriteAndReadDackupConfig_BackendFields(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	want := DackupConfig{
		User:            "test-user",
		Group:           "test-group",
		Backend:         "borg",
		BackendSettings: []byte(`{"repository":"/backups/repo"}`),
	}

	if err := WriteDackupConfig(configPath, want, nil); err != nil {
		t.Fatalf("WriteDackupConfig returned error: %v", err)
	}

	got, err := ReadDackupConfig(configPath)
	if err != nil {
		t.Fatalf("ReadDackupConfig returned error: %v", err)
	}

	if got.Backend != want.Backend {
		t.Fatalf("expected backend %q, got %q", want.Backend, got.Backend)
	}

	var gotSettings, wantSettings map[string]any
	if err := json.Unmarshal(got.BackendSettings, &gotSettings); err != nil {
		t.Fatalf("failed to unmarshal got backend settings: %v", err)
	}
	if err := json.Unmarshal(want.BackendSettings, &wantSettings); err != nil {
		t.Fatalf("failed to unmarshal want backend settings: %v", err)
	}

	if !reflect.DeepEqual(gotSettings, wantSettings) {
		t.Fatalf("expected backend settings %#v, got %#v", wantSettings, gotSettings)
	}
}

func TestWriteDackupConfig_OmitsEmptyBackendFields(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	config := DackupConfig{
		User:  "test-user",
		Group: "test-group",
	}

	if err := WriteDackupConfig(configPath, config, nil); err != nil {
		t.Fatalf("WriteDackupConfig returned error: %v", err)
	}

	got, err := ReadDackupConfig(configPath)
	if err != nil {
		t.Fatalf("ReadDackupConfig returned error: %v", err)
	}

	if got.Backend != "" {
		t.Fatalf("expected empty backend, got %q", got.Backend)
	}

	if got.BackendSettings != nil {
		t.Fatalf("expected nil backend settings, got %s", got.BackendSettings)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read raw config file: %v", err)
	}

	if strings.Contains(string(content), "backend") {
		t.Fatalf("expected written config to omit backend fields entirely, got:\n%s", content)
	}
}
