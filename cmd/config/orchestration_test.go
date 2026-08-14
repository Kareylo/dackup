package config

import (
	"bufio"
	"dackup/internal/shared"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func withConfigFilePath(t *testing.T, path string) {
	t.Helper()

	original := configFilePath
	configFilePath = path

	t.Cleanup(func() {
		configFilePath = original
	})
}

func readerFor(input string) *bufio.Reader {
	return bufio.NewReader(strings.NewReader(input))
}

func TestRunConfigInitWithReader_CreatesConfigFileWithContainer(t *testing.T) {
	withConfigFilePath(t, filepath.Join(t.TempDir(), "config.json"))

	input := "owner\ngroup\n\n\nn\nweb\ny\n/data\n\nn\n"

	if err := runConfigInitWithReader(readerFor(input)); err != nil {
		t.Fatalf("runConfigInitWithReader returned error: %v", err)
	}

	got, err := shared.ReadDackupConfig(configFilePath)
	if err != nil {
		t.Fatalf("failed to read written config: %v", err)
	}

	if got.User != "owner" || got.Group != "group" {
		t.Fatalf("expected owner/group %q/%q, got %q/%q", "owner", "group", got.User, got.Group)
	}

	if got.DataDir != defaultDataDir || got.StagingDir != defaultStagingDir {
		t.Fatalf("expected default dirs %q/%q, got %q/%q", defaultDataDir, defaultStagingDir, got.DataDir, got.StagingDir)
	}

	want := []shared.ContainerConfig{{Container: "web", ToStop: true, Paths: []string{"/data"}}}
	if !reflect.DeepEqual(got.Containers, want) {
		t.Fatalf("expected containers %#v, got %#v", want, got.Containers)
	}
}

func TestRunConfigInitWithReader_DeclinedOverwriteLeavesFileUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withConfigFilePath(t, path)

	original := shared.DackupConfig{User: "original-owner", Group: "original-group"}
	if err := shared.WriteDackupConfig(path, original, nil); err != nil {
		t.Fatalf("failed to seed existing config: %v", err)
	}

	if err := runConfigInitWithReader(readerFor("n\n")); err != nil {
		t.Fatalf("runConfigInitWithReader returned error: %v", err)
	}

	got, err := shared.ReadDackupConfig(path)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	if got.User != "original-owner" {
		t.Fatalf("expected existing config to be left unchanged, got user %q", got.User)
	}
}

func TestRunConfigAddContainerWithReader_AppendsContainer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withConfigFilePath(t, path)

	seed := shared.DackupConfig{
		User:  "owner",
		Group: "group",
		Containers: []shared.ContainerConfig{
			{Container: "existing", ToStop: true},
		},
	}
	if err := shared.WriteDackupConfig(path, seed, nil); err != nil {
		t.Fatalf("failed to seed existing config: %v", err)
	}

	input := "newcontainer\nn\n\n\n"

	if err := runConfigAddContainerWithReader(readerFor(input)); err != nil {
		t.Fatalf("runConfigAddContainerWithReader returned error: %v", err)
	}

	configs, err := shared.ReadContainerConfigsFromPath(path)
	if err != nil {
		t.Fatalf("failed to read containers: %v", err)
	}

	want := []shared.ContainerConfig{
		{Container: "existing", ToStop: true},
		{Container: "newcontainer", ToStop: false},
	}
	if !reflect.DeepEqual(configs, want) {
		t.Fatalf("expected containers %#v, got %#v", want, configs)
	}
}

func TestRunConfigAddContainerWithReader_DuplicateContainerReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withConfigFilePath(t, path)

	seed := shared.DackupConfig{
		User:  "owner",
		Group: "group",
		Containers: []shared.ContainerConfig{
			{Container: "web", ToStop: true},
		},
	}
	if err := shared.WriteDackupConfig(path, seed, nil); err != nil {
		t.Fatalf("failed to seed existing config: %v", err)
	}

	input := "web\nn\n\n\n"

	if err := runConfigAddContainerWithReader(readerFor(input)); err == nil {
		t.Fatal("expected an error when adding a container that already exists")
	}
}

func TestRunConfigUpdateContainerWithReader_UpdatesContainer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withConfigFilePath(t, path)

	seed := shared.DackupConfig{
		User:  "owner",
		Group: "group",
		Containers: []shared.ContainerConfig{
			{Container: "web", ToStop: true, Paths: []string{"/data"}},
		},
	}
	if err := shared.WriteDackupConfig(path, seed, nil); err != nil {
		t.Fatalf("failed to seed existing config: %v", err)
	}

	// "web" (container to update), then keep name, flip ToStop to false, keep paths, keep contains.
	input := "web\n\nn\n\n\n"

	if err := runConfigUpdateContainerWithReader(readerFor(input)); err != nil {
		t.Fatalf("runConfigUpdateContainerWithReader returned error: %v", err)
	}

	configs, err := shared.ReadContainerConfigsFromPath(path)
	if err != nil {
		t.Fatalf("failed to read containers: %v", err)
	}

	if len(configs) != 1 || configs[0].Container != "web" || configs[0].ToStop != false {
		t.Fatalf("expected updated container web with ToStop=false, got %#v", configs)
	}
}

func TestRunConfigRemoveContainerWithReader_RemovesContainer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withConfigFilePath(t, path)

	seed := shared.DackupConfig{
		User:  "owner",
		Group: "group",
		Containers: []shared.ContainerConfig{
			{Container: "web", ToStop: true},
		},
	}
	if err := shared.WriteDackupConfig(path, seed, nil); err != nil {
		t.Fatalf("failed to seed existing config: %v", err)
	}

	input := "web\ny\n"

	if err := runConfigRemoveContainerWithReader(readerFor(input)); err != nil {
		t.Fatalf("runConfigRemoveContainerWithReader returned error: %v", err)
	}

	configs, err := shared.ReadContainerConfigsFromPath(path)
	if err != nil {
		t.Fatalf("failed to read containers: %v", err)
	}

	if len(configs) != 0 {
		t.Fatalf("expected container to be removed, got %#v", configs)
	}
}

func TestRunConfigListContainers_ReadsExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withConfigFilePath(t, path)

	seed := shared.DackupConfig{
		User:  "owner",
		Group: "group",
		Containers: []shared.ContainerConfig{
			{Container: "web", ToStop: true},
		},
	}
	if err := shared.WriteDackupConfig(path, seed, nil); err != nil {
		t.Fatalf("failed to seed existing config: %v", err)
	}

	if err := runConfigListContainers(); err != nil {
		t.Fatalf("runConfigListContainers returned error: %v", err)
	}
}

func TestRunConfigListContainers_NoConfigFileReturnsNilError(t *testing.T) {
	withConfigFilePath(t, filepath.Join(t.TempDir(), "missing-config.json"))

	if err := runConfigListContainers(); err != nil {
		t.Fatalf("expected nil error for a missing config file, got %v", err)
	}
}

func TestRunConfigUseFileWithReader_SwitchesToCustomFile(t *testing.T) {
	withConfigFilePath(t, filepath.Join(t.TempDir(), "config.json"))
	customPath := filepath.Join(t.TempDir(), "containers.json")

	input := "owner\ngroup\n\n\n"

	if err := runConfigUseFileWithReader(readerFor(input), customPath); err != nil {
		t.Fatalf("runConfigUseFileWithReader returned error: %v", err)
	}

	mainConfig, err := shared.ReadDackupConfig(configFilePath)
	if err != nil {
		t.Fatalf("failed to read main config: %v", err)
	}

	if mainConfig.ConfigFile != customPath {
		t.Fatalf("expected config_file to be set to %q, got %q", customPath, mainConfig.ConfigFile)
	}

	if !shared.FileExists(customPath) {
		t.Fatal("expected custom containers file to be created")
	}
}
