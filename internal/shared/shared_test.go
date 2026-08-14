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

func TestEffectiveDackupConfig_InlineContainersReturnsMainConfigPath(t *testing.T) {
	tempDir := t.TempDir()
	mainPath := filepath.Join(tempDir, "config.json")

	mainConfig := DackupConfig{
		User:       "user",
		Group:      "group",
		Containers: []ContainerConfig{{Container: "web", ToStop: true}},
	}
	if err := WriteDackupConfig(mainPath, mainConfig, nil); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}

	effectiveConfig, effectiveConfigPath, err := EffectiveDackupConfig(mainPath)
	if err != nil {
		t.Fatalf("EffectiveDackupConfig returned error: %v", err)
	}

	if effectiveConfigPath != mainPath {
		t.Fatalf("expected effective config path %q, got %q", mainPath, effectiveConfigPath)
	}

	if !reflect.DeepEqual(effectiveConfig.Containers, mainConfig.Containers) {
		t.Fatalf("expected containers %#v, got %#v", mainConfig.Containers, effectiveConfig.Containers)
	}
}

func TestEffectiveDackupConfig_ConfigFilePointerMergesContainersFromSeparateFile(t *testing.T) {
	tempDir := t.TempDir()
	mainPath := filepath.Join(tempDir, "config.json")
	containersPath := filepath.Join(tempDir, "containers.json")

	mainConfig := DackupConfig{
		User:       "user",
		Group:      "group",
		ConfigFile: containersPath,
		// A container list on the main config itself, deliberately
		// different from the pointed-to file's, to prove it's ignored.
		Containers: []ContainerConfig{{Container: "should-be-ignored"}},
	}
	if err := WriteDackupConfig(mainPath, mainConfig, nil); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}

	containersConfig := DackupConfig{
		Containers: []ContainerConfig{{Container: "web", ToStop: true}},
	}
	if err := WriteDackupConfig(containersPath, containersConfig, nil); err != nil {
		t.Fatalf("failed to write containers config: %v", err)
	}

	effectiveConfig, effectiveConfigPath, err := EffectiveDackupConfig(mainPath)
	if err != nil {
		t.Fatalf("EffectiveDackupConfig returned error: %v", err)
	}

	if effectiveConfigPath != containersPath {
		t.Fatalf("expected effective config path %q, got %q", containersPath, effectiveConfigPath)
	}

	if !reflect.DeepEqual(effectiveConfig.Containers, containersConfig.Containers) {
		t.Fatalf("expected containers from the pointed-to file %#v, got %#v", containersConfig.Containers, effectiveConfig.Containers)
	}

	if effectiveConfig.User != mainConfig.User {
		t.Fatalf("expected user %q from the main config to be preserved, got %q", mainConfig.User, effectiveConfig.User)
	}
}

func TestEffectiveDackupConfig_MissingMainConfigReturnsError(t *testing.T) {
	_, _, err := EffectiveDackupConfig(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("expected an error for a missing main config file")
	}
}

func TestEffectiveDackupConfig_MissingConfigFileTargetReturnsError(t *testing.T) {
	tempDir := t.TempDir()
	mainPath := filepath.Join(tempDir, "config.json")

	mainConfig := DackupConfig{ConfigFile: filepath.Join(tempDir, "missing-containers.json")}
	if err := WriteDackupConfig(mainPath, mainConfig, nil); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}

	_, _, err := EffectiveDackupConfig(mainPath)
	if err == nil {
		t.Fatal("expected an error when the config_file target does not exist")
	}
}

func TestEffectiveContainersConfigPath_NoMainConfigReturnsMainConfigPathUnchanged(t *testing.T) {
	mainPath := filepath.Join(t.TempDir(), "missing.json")

	got, err := EffectiveContainersConfigPath(mainPath)
	if err != nil {
		t.Fatalf("EffectiveContainersConfigPath returned error: %v", err)
	}

	if got != mainPath {
		t.Fatalf("expected %q, got %q", mainPath, got)
	}
}

func TestEffectiveContainersConfigPath_InlineConfigReturnsMainConfigPath(t *testing.T) {
	tempDir := t.TempDir()
	mainPath := filepath.Join(tempDir, "config.json")

	if err := WriteDackupConfig(mainPath, DackupConfig{User: "user"}, nil); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}

	got, err := EffectiveContainersConfigPath(mainPath)
	if err != nil {
		t.Fatalf("EffectiveContainersConfigPath returned error: %v", err)
	}

	if got != mainPath {
		t.Fatalf("expected %q, got %q", mainPath, got)
	}
}

func TestEffectiveContainersConfigPath_ConfigFilePointerReturnsPointerPath(t *testing.T) {
	tempDir := t.TempDir()
	mainPath := filepath.Join(tempDir, "config.json")
	containersPath := filepath.Join(tempDir, "containers.json")

	if err := WriteDackupConfig(mainPath, DackupConfig{ConfigFile: containersPath}, nil); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}

	got, err := EffectiveContainersConfigPath(mainPath)
	if err != nil {
		t.Fatalf("EffectiveContainersConfigPath returned error: %v", err)
	}

	if got != containersPath {
		t.Fatalf("expected %q, got %q", containersPath, got)
	}
}
