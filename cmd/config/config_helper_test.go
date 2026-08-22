package config

import (
	"dackup/internal/shared"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func assertPathEqual(t *testing.T, got string, want string) {
	t.Helper()

	got = filepath.Clean(got)
	want = filepath.Clean(want)

	if got != want {
		t.Fatalf("expected path %q, got %q", want, got)
	}
}

func TestWriteAndReadDackupConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	want := dackupConfig{
		User:  "test-user",
		Group: "test-group",
		Containers: []shared.ContainerConfig{
			{
				Container: "paperless",
				ToStop:    true,
				Paths:     []string{"/data/paperless"},
				Contains:  []string{"paperless_db", "paperless_broker"},
			},
			{
				Container: "adguard",
				ToStop:    false,
				Paths:     []string{"/config/adguard"},
			},
		},
	}

	if err := writeDackupConfig(configPath, want); err != nil {
		t.Fatalf("writeDackupConfig returned error: %v", err)
	}

	got, err := readDackupConfig(configPath)
	if err != nil {
		t.Fatalf("readDackupConfig returned error: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestWriteAndReadContainerConfigsFromPath(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "containers.json")

	want := []shared.ContainerConfig{
		{
			Container: "paperless",
			ToStop:    true,
			Paths:     []string{"/data/paperless"},
		},
		{
			Container: "adguard",
			ToStop:    false,
			Paths:     []string{"/config/adguard"},
		},
	}

	if err := writeContainerConfigsToPath(configPath, want); err != nil {
		t.Fatalf("writeContainerConfigsToPath returned error: %v", err)
	}

	got, err := readContainerConfigsFromPath(configPath)
	if err != nil {
		t.Fatalf("readContainerConfigsFromPath returned error: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestEffectiveContainersConfigPath_WhenMainConfigDoesNotExist(t *testing.T) {
	tempDir := t.TempDir()
	mainConfigPath := filepath.Join(tempDir, "missing.json")

	got, err := effectiveContainersConfigPath(mainConfigPath)
	if err != nil {
		t.Fatalf("effectiveContainersConfigPath returned error: %v", err)
	}

	assertPathEqual(t, got, mainConfigPath)
}

func TestEffectiveContainersConfigPath_WhenMainConfigHasNoCustomFile(t *testing.T) {
	tempDir := t.TempDir()
	mainConfigPath := filepath.Join(tempDir, "config.json")

	config := dackupConfig{
		User:  "test-user",
		Group: "test-group",
		Containers: []shared.ContainerConfig{
			{
				Container: "adguard",
				ToStop:    true,
			},
		},
	}

	if err := writeDackupConfig(mainConfigPath, config); err != nil {
		t.Fatalf("writeDackupConfig returned error: %v", err)
	}

	got, err := effectiveContainersConfigPath(mainConfigPath)
	if err != nil {
		t.Fatalf("effectiveContainersConfigPath returned error: %v", err)
	}

	assertPathEqual(t, got, mainConfigPath)
}

func TestEffectiveContainersConfigPath_WhenMainConfigHasCustomFile(t *testing.T) {
	tempDir := t.TempDir()
	mainConfigPath := filepath.Join(tempDir, "config.json")
	customConfigPath := filepath.Join(tempDir, "custom.json")

	config := dackupConfig{
		User:       "test-user",
		Group:      "test-group",
		ConfigFile: customConfigPath,
	}

	if err := writeDackupConfig(mainConfigPath, config); err != nil {
		t.Fatalf("writeDackupConfig returned error: %v", err)
	}

	got, err := effectiveContainersConfigPath(mainConfigPath)
	if err != nil {
		t.Fatalf("effectiveContainersConfigPath returned error: %v", err)
	}

	assertPathEqual(t, got, customConfigPath)
}

func TestReadDackupConfig_InvalidJSONReturnsError(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	if err := os.WriteFile(configPath, []byte("{invalid-json"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	_, err := readDackupConfig(configPath)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestDefaultDackupConfigPath_ReturnsNonEmptyPath(t *testing.T) {
	got, err := defaultDackupConfigPath()
	if err != nil {
		t.Fatalf("defaultDackupConfigPath returned error: %v", err)
	}

	if got == "" {
		t.Fatal("expected a non-empty default config path")
	}
}

func TestEffectiveDackupConfig_ReturnsInlineContainers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	config := dackupConfig{
		User:  "u",
		Group: "g",
		Containers: []shared.ContainerConfig{
			{Container: "web", ToStop: true},
		},
	}
	if err := writeDackupConfig(path, config); err != nil {
		t.Fatalf("writeDackupConfig returned error: %v", err)
	}

	got, effectivePath, err := effectiveDackupConfig(path)
	if err != nil {
		t.Fatalf("effectiveDackupConfig returned error: %v", err)
	}

	assertPathEqual(t, effectivePath, path)

	if len(got.Containers) != 1 || got.Containers[0].Container != "web" {
		t.Fatalf("expected inline containers to be returned, got %#v", got.Containers)
	}
}

func TestFileExists_ReturnsTrueForExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := writeDackupConfig(path, dackupConfig{User: "u", Group: "g"}); err != nil {
		t.Fatalf("writeDackupConfig returned error: %v", err)
	}

	if !fileExists(path) {
		t.Fatalf("expected fileExists to return true for %q", path)
	}
}

func TestFileExists_ReturnsFalseForMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")

	if fileExists(path) {
		t.Fatalf("expected fileExists to return false for %q", path)
	}
}

func TestReadDackupConfig_MissingFileReturnsError(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "missing.json")

	_, err := readDackupConfig(configPath)
	if err == nil {
		t.Fatal("expected error for missing config file, got nil")
	}
}
