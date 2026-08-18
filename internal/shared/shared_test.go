package shared

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDefaultDackupConfigPath_JoinsHomeDirAndDefaultRelativePath(t *testing.T) {
	t.Setenv("HOME", "/home/dackup-test-user")

	got, err := DefaultDackupConfigPath()
	if err != nil {
		t.Fatalf("DefaultDackupConfigPath returned error: %v", err)
	}

	want := filepath.Join("/home/dackup-test-user", DefaultConfigRelativePath)
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestDefaultDackupConfigPath_PropagatesErrorWhenHomeDirIsUnknown(t *testing.T) {
	t.Setenv("HOME", "")

	if _, err := DefaultDackupConfigPath(); err == nil {
		t.Fatal("expected an error when the home directory can't be determined")
	}
}

func TestReadDackupConfig_MissingFileReturnsError(t *testing.T) {
	if _, err := ReadDackupConfig(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected an error for a missing config file")
	}
}

func TestReadDackupConfig_MalformedJSONReturnsError(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("failed to write malformed config: %v", err)
	}

	if _, err := ReadDackupConfig(configPath); err == nil {
		t.Fatal("expected an error for a malformed config file")
	}
}

func TestWriteDackupConfig_DryRunDoesNotTouchDisk(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")

	err := WriteDackupConfig(configPath, DackupConfig{User: "user"}, &Options{DryRun: true})
	if err != nil {
		t.Fatalf("WriteDackupConfig returned error: %v", err)
	}

	if FileExists(configPath) {
		t.Fatal("expected dry-run to not write the config file")
	}
}

func TestWriteDackupConfig_DryRunMarshalFailureReturnsError(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")

	// json.MarshalIndent re-parses the encoded output to indent it, so
	// invalid raw bytes here (json.RawMessage.MarshalJSON does not itself
	// validate them) surface as a marshal error.
	config := DackupConfig{BackendSettings: json.RawMessage("not valid json")}

	err := WriteDackupConfig(configPath, config, &Options{DryRun: true})
	if err == nil {
		t.Fatal("expected an error when the config can't be marshaled")
	}
}

func TestWriteDackupConfig_MarshalFailureReturnsError(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")

	config := DackupConfig{BackendSettings: json.RawMessage("not valid json")}

	err := WriteDackupConfig(configPath, config, nil)
	if err == nil {
		t.Fatal("expected an error when the config can't be marshaled")
	}
}

func TestWriteDackupConfig_MkdirFailureReturnsError(t *testing.T) {
	tempDir := t.TempDir()
	blockerPath := filepath.Join(tempDir, "blocker")
	if err := os.WriteFile(blockerPath, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("failed to write blocker file: %v", err)
	}

	configPath := filepath.Join(blockerPath, "subdir", "config.json")

	if err := WriteDackupConfig(configPath, DackupConfig{User: "user"}, nil); err == nil {
		t.Fatal("expected an error when the config directory can't be created")
	}
}

func TestWriteDackupConfig_WriteFileFailureReturnsError(t *testing.T) {
	// A path that is itself an existing directory can never be written to
	// as a file.
	configPath := t.TempDir()

	if err := WriteDackupConfig(configPath, DackupConfig{User: "user"}, nil); err == nil {
		t.Fatal("expected an error when the config path is a directory")
	}
}

func TestReadContainerConfigsFromPath_ReturnsContainers(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	config := DackupConfig{Containers: []ContainerConfig{{Container: "web", ToStop: true}}}
	if err := WriteDackupConfig(configPath, config, nil); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	got, err := ReadContainerConfigsFromPath(configPath)
	if err != nil {
		t.Fatalf("ReadContainerConfigsFromPath returned error: %v", err)
	}

	if !reflect.DeepEqual(got, config.Containers) {
		t.Fatalf("expected %#v, got %#v", config.Containers, got)
	}
}

func TestReadContainerConfigsFromPath_MissingFileReturnsError(t *testing.T) {
	if _, err := ReadContainerConfigsFromPath(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected an error for a missing config file")
	}
}

func TestWriteContainerConfigsToPath_CreatesNewFileWhenNoneExists(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	containers := []ContainerConfig{{Container: "web", ToStop: true}}

	if err := WriteContainerConfigsToPath(configPath, containers, nil); err != nil {
		t.Fatalf("WriteContainerConfigsToPath returned error: %v", err)
	}

	got, err := ReadDackupConfig(configPath)
	if err != nil {
		t.Fatalf("ReadDackupConfig returned error: %v", err)
	}

	if !reflect.DeepEqual(got.Containers, containers) {
		t.Fatalf("expected containers %#v, got %#v", containers, got.Containers)
	}
}

func TestWriteContainerConfigsToPath_PreservesOtherExistingFields(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	existing := DackupConfig{
		User:       "test-user",
		Group:      "test-group",
		Containers: []ContainerConfig{{Container: "old"}},
	}
	if err := WriteDackupConfig(configPath, existing, nil); err != nil {
		t.Fatalf("failed to write existing config: %v", err)
	}

	newContainers := []ContainerConfig{{Container: "new", ToStop: true}}
	if err := WriteContainerConfigsToPath(configPath, newContainers, nil); err != nil {
		t.Fatalf("WriteContainerConfigsToPath returned error: %v", err)
	}

	got, err := ReadDackupConfig(configPath)
	if err != nil {
		t.Fatalf("ReadDackupConfig returned error: %v", err)
	}

	if got.User != existing.User || got.Group != existing.Group {
		t.Fatalf("expected user/group to be preserved, got %+v", got)
	}

	if !reflect.DeepEqual(got.Containers, newContainers) {
		t.Fatalf("expected containers %#v, got %#v", newContainers, got.Containers)
	}
}

func TestWriteContainerConfigsToPath_MalformedExistingFileReturnsError(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("failed to write malformed config: %v", err)
	}

	err := WriteContainerConfigsToPath(configPath, []ContainerConfig{{Container: "web"}}, nil)
	if err == nil {
		t.Fatal("expected an error when the existing config file is malformed")
	}
}

func TestFileExists(t *testing.T) {
	tempDir := t.TempDir()
	existingPath := filepath.Join(tempDir, "exists.json")
	if err := os.WriteFile(existingPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	if !FileExists(existingPath) {
		t.Fatal("expected FileExists to report true for an existing file")
	}

	if FileExists(filepath.Join(tempDir, "missing.json")) {
		t.Fatal("expected FileExists to report false for a missing file")
	}
}

func TestEffectiveContainersConfigPath_MalformedMainConfigReturnsError(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("failed to write malformed config: %v", err)
	}

	if _, err := EffectiveContainersConfigPath(configPath); err == nil {
		t.Fatal("expected an error for a malformed main config file")
	}
}

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
