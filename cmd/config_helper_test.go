package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWriteAndReadDackupConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	want := dackupConfig{
		User:  "test-user",
		Group: "test-group",
		Containers: []containerConfig{
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

	want := []containerConfig{
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
		Containers: []containerConfig{
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

func TestReadDackupConfig_MissingFileReturnsError(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "missing.json")

	_, err := readDackupConfig(configPath)
	if err == nil {
		t.Fatal("expected error for missing config file, got nil")
	}
}
