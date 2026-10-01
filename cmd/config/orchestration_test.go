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

	// No other container is configured yet, so the contains prompt is skipped.
	input := "owner\ngroup\n\n\nn\nweb\ny\n/data\nn\n"

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

func TestRunConfigInitWithReader_CustomContainersFileCreatesBothFiles(t *testing.T) {
	withConfigFilePath(t, filepath.Join(t.TempDir(), "config.json"))
	customPath := filepath.Join(t.TempDir(), "containers.json")

	// owner, group, dataDir, stagingDir -> (default), useCustomFile -> y,
	// customPath, createCustom -> y, container -> web, toStop -> n,
	// paths -> /data, contains -> (empty), addAnother -> n.
	input := "owner\ngroup\n\n\ny\n" + customPath + "\ny\nweb\nn\n/data\nn\n"

	if err := runConfigInitWithReader(readerFor(input)); err != nil {
		t.Fatalf("runConfigInitWithReader returned error: %v", err)
	}

	mainConfig, err := shared.ReadDackupConfig(configFilePath)
	if err != nil {
		t.Fatalf("failed to read main config: %v", err)
	}

	if mainConfig.ConfigFile != customPath {
		t.Fatalf("expected config_file %q, got %q", customPath, mainConfig.ConfigFile)
	}

	containers, err := shared.ReadContainerConfigsFromPath(customPath)
	if err != nil {
		t.Fatalf("failed to read custom containers file: %v", err)
	}

	want := []shared.ContainerConfig{{Container: "web", ToStop: false, Paths: []string{"/data"}}}
	if !reflect.DeepEqual(containers, want) {
		t.Fatalf("expected containers %#v, got %#v", want, containers)
	}
}

func TestRunConfigInitWithReader_CustomContainersFileDeclinedCreationLeavesItMissing(t *testing.T) {
	withConfigFilePath(t, filepath.Join(t.TempDir(), "config.json"))
	customPath := filepath.Join(t.TempDir(), "containers.json")

	// owner, group, dataDir, stagingDir -> (default), useCustomFile -> y,
	// customPath, createCustom -> n.
	input := "owner\ngroup\n\n\ny\n" + customPath + "\nn\n"

	if err := runConfigInitWithReader(readerFor(input)); err != nil {
		t.Fatalf("runConfigInitWithReader returned error: %v", err)
	}

	if shared.FileExists(customPath) {
		t.Fatal("expected the custom containers file to not be created")
	}
}

func TestReadExistingContainerConfigs_CreatesFileWhenMissingAndConfirmed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "containers.json")
	service := commandService{prompt: shared.NewPromptService(readerFor("y\n"))}

	configs, err := service.readExistingContainerConfigs(path)
	if err != nil {
		t.Fatalf("readExistingContainerConfigs returned error: %v", err)
	}

	if len(configs) != 0 {
		t.Fatalf("expected no containers, got %#v", configs)
	}

	if !shared.FileExists(path) {
		t.Fatal("expected the containers file to be created")
	}
}

func TestReadExistingContainerConfigs_ReturnsErrorWhenMissingAndDeclined(t *testing.T) {
	path := filepath.Join(t.TempDir(), "containers.json")
	service := commandService{prompt: shared.NewPromptService(readerFor("n\n"))}

	if _, err := service.readExistingContainerConfigs(path); err == nil {
		t.Fatal("expected an error when creation is declined")
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

func TestRunConfigInitWithReader_LaterContainerCanContainEarlierOne(t *testing.T) {
	withConfigFilePath(t, filepath.Join(t.TempDir(), "config.json"))

	// db: no stop, no paths, (contains skipped), add another; web: no stop,
	// no paths, contains option 1 (db), stop adding.
	input := "owner\ngroup\n\n\nn\ndb\nn\n\ny\nweb\nn\n\n1\nn\n"

	if err := runConfigInitWithReader(readerFor(input)); err != nil {
		t.Fatalf("runConfigInitWithReader returned error: %v", err)
	}

	got, err := shared.ReadDackupConfig(configFilePath)
	if err != nil {
		t.Fatalf("failed to read written config: %v", err)
	}

	want := []shared.ContainerConfig{
		{Container: "db"},
		{Container: "web", Contains: []string{"db"}},
	}
	if !reflect.DeepEqual(got.Containers, want) {
		t.Fatalf("expected containers %#v, got %#v", want, got.Containers)
	}
}

func TestRunConfigAddContainerWithReader_ContainsPicksFromConfiguredContainers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withConfigFilePath(t, path)

	seed := shared.DackupConfig{
		User:  "owner",
		Group: "group",
		Containers: []shared.ContainerConfig{
			{Container: "existing"},
			{Container: "db"},
		},
	}
	if err := shared.WriteDackupConfig(path, seed, nil); err != nil {
		t.Fatalf("failed to seed existing config: %v", err)
	}

	// web, no stop, no paths, contains option 2 (db).
	input := "web\nn\n\n2\n"

	if err := runConfigAddContainerWithReader(readerFor(input)); err != nil {
		t.Fatalf("runConfigAddContainerWithReader returned error: %v", err)
	}

	configs, err := shared.ReadContainerConfigsFromPath(path)
	if err != nil {
		t.Fatalf("failed to read containers: %v", err)
	}

	want := shared.ContainerConfig{Container: "web", Contains: []string{"db"}}
	if !reflect.DeepEqual(configs[2], want) {
		t.Fatalf("expected new container %#v, got %#v", want, configs[2])
	}
}

func TestRunConfigUpdateContainerWithReader_KeepsCurrentContainsIncludingUnconfiguredOnes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withConfigFilePath(t, path)

	seed := shared.DackupConfig{
		User:  "owner",
		Group: "group",
		Containers: []shared.ContainerConfig{
			{Container: "web", Contains: []string{"gone"}},
			{Container: "db"},
		},
	}
	if err := shared.WriteDackupConfig(path, seed, nil); err != nil {
		t.Fatalf("failed to seed existing config: %v", err)
	}

	// container 1 (web), then keep name, stop, paths and contains.
	input := "1\n\n\n\n\n"

	if err := runConfigUpdateContainerWithReader(readerFor(input)); err != nil {
		t.Fatalf("runConfigUpdateContainerWithReader returned error: %v", err)
	}

	configs, err := shared.ReadContainerConfigsFromPath(path)
	if err != nil {
		t.Fatalf("failed to read containers: %v", err)
	}

	if !reflect.DeepEqual(configs[0].Contains, []string{"gone"}) {
		t.Fatalf("expected contains %#v to be kept, got %#v", []string{"gone"}, configs[0].Contains)
	}
}

func TestRunConfigUpdateContainerWithReader_ContainsCanBeReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withConfigFilePath(t, path)

	seed := shared.DackupConfig{
		User:  "owner",
		Group: "group",
		Containers: []shared.ContainerConfig{
			{Container: "web", Contains: []string{"gone"}},
			{Container: "db"},
		},
	}
	if err := shared.WriteDackupConfig(path, seed, nil); err != nil {
		t.Fatalf("failed to seed existing config: %v", err)
	}

	// Options are [db, gone]: pick db only.
	input := "web\n\n\n\ndb\n"

	if err := runConfigUpdateContainerWithReader(readerFor(input)); err != nil {
		t.Fatalf("runConfigUpdateContainerWithReader returned error: %v", err)
	}

	configs, err := shared.ReadContainerConfigsFromPath(path)
	if err != nil {
		t.Fatalf("failed to read containers: %v", err)
	}

	if !reflect.DeepEqual(configs[0].Contains, []string{"db"}) {
		t.Fatalf("expected contains %#v, got %#v", []string{"db"}, configs[0].Contains)
	}
}

func TestRunConfigRemoveContainerWithReader_AcceptsListedNumber(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withConfigFilePath(t, path)

	seed := shared.DackupConfig{
		User:  "owner",
		Group: "group",
		Containers: []shared.ContainerConfig{
			{Container: "web"},
			{Container: "db"},
		},
	}
	if err := shared.WriteDackupConfig(path, seed, nil); err != nil {
		t.Fatalf("failed to seed existing config: %v", err)
	}

	if err := runConfigRemoveContainerWithReader(readerFor("2\ny\n")); err != nil {
		t.Fatalf("runConfigRemoveContainerWithReader returned error: %v", err)
	}

	configs, err := shared.ReadContainerConfigsFromPath(path)
	if err != nil {
		t.Fatalf("failed to read containers: %v", err)
	}

	if !reflect.DeepEqual(configs, []shared.ContainerConfig{{Container: "web"}}) {
		t.Fatalf("expected only web to remain, got %#v", configs)
	}
}

func TestNewCommand_RegistersSubcommands(t *testing.T) {
	originalOptions, originalPath, originalTerminal := options, configFilePath, terminal
	t.Cleanup(func() {
		options, configFilePath, terminal = originalOptions, originalPath, originalTerminal
	})

	cmd := NewCommand(&shared.Options{})

	if len(cmd.Commands()) != 6 {
		t.Fatalf("expected 6 config subcommands, got %d", len(cmd.Commands()))
	}
}

type fakeTerminal struct{}

func (fakeTerminal) MakeRaw() (func() error, error) {
	return func() error { return nil }, nil
}

func TestNewCommandService_UsesPackageTerminal(t *testing.T) {
	originalTerminal := terminal
	t.Cleanup(func() { terminal = originalTerminal })

	terminal = fakeTerminal{}

	service := newCommandService(readerFor(""))
	if service.prompt.Terminal != terminal {
		t.Fatalf("expected prompt terminal %#v, got %#v", terminal, service.prompt.Terminal)
	}
}

// seedContainers writes a main config holding containers and points
// configFilePath at it.
func seedContainers(t *testing.T, containers ...shared.ContainerConfig) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	withConfigFilePath(t, path)

	seed := shared.DackupConfig{User: "owner", Group: "group", Containers: containers}
	if err := shared.WriteDackupConfig(path, seed, nil); err != nil {
		t.Fatalf("failed to seed existing config: %v", err)
	}
}

func TestRunConfigInitWithReader_PropagatesContainerReadError(t *testing.T) {
	withConfigFilePath(t, filepath.Join(t.TempDir(), "config.json"))

	// owner, group, dirs, no custom file, then input ends before a container.
	if err := runConfigInitWithReader(readerFor("owner\ngroup\n\n\nn\n")); err == nil {
		t.Fatal("expected read error, got nil")
	}
}

func TestRunConfigAddContainerWithReader_PropagatesContainerReadError(t *testing.T) {
	seedContainers(t, shared.ContainerConfig{Container: "db"})

	if err := runConfigAddContainerWithReader(readerFor("")); err == nil {
		t.Fatal("expected read error, got nil")
	}
}

func TestRunConfigAddContainerWithReader_PropagatesContainsReadError(t *testing.T) {
	seedContainers(t, shared.ContainerConfig{Container: "db"})

	// web, no stop, no paths, then input ends at the contains choice.
	if err := runConfigAddContainerWithReader(readerFor("web\nn\n\n")); err == nil {
		t.Fatal("expected read error, got nil")
	}
}

func TestRunConfigUpdateContainerWithReader_PropagatesSelectionReadError(t *testing.T) {
	seedContainers(t, shared.ContainerConfig{Container: "web"})

	if err := runConfigUpdateContainerWithReader(readerFor("")); err == nil {
		t.Fatal("expected read error, got nil")
	}
}

func TestRunConfigUpdateContainerWithReader_PropagatesContainsReadError(t *testing.T) {
	seedContainers(t, shared.ContainerConfig{Container: "web"}, shared.ContainerConfig{Container: "db"})

	// web, keep name, stop and paths, then input ends at the contains choice.
	if err := runConfigUpdateContainerWithReader(readerFor("web\n\n\n\n")); err == nil {
		t.Fatal("expected read error, got nil")
	}
}

func TestRunConfigRemoveContainerWithReader_PropagatesSelectionReadError(t *testing.T) {
	seedContainers(t, shared.ContainerConfig{Container: "web"})

	if err := runConfigRemoveContainerWithReader(readerFor("")); err == nil {
		t.Fatal("expected read error, got nil")
	}
}

func TestRunConfigAddContainerWithReader_RejectsPathEscapingRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withConfigFilePath(t, path)

	seed := shared.DackupConfig{
		User:       "owner",
		Group:      "group",
		Containers: []shared.ContainerConfig{{Container: "existing", ToStop: true}},
	}
	if err := shared.WriteDackupConfig(path, seed, nil); err != nil {
		t.Fatalf("failed to seed existing config: %v", err)
	}

	// name, to_stop, paths (escaping the root), contains
	input := "newcontainer\nn\n/app,../../etc\n\n"

	if err := runConfigAddContainerWithReader(readerFor(input)); err == nil {
		t.Fatal("expected an error for a path escaping the root")
	}

	configs, err := shared.ReadContainerConfigsFromPath(path)
	if err != nil {
		t.Fatalf("failed to read containers: %v", err)
	}
	if !reflect.DeepEqual(configs, seed.Containers) {
		t.Fatalf("expected config to be left unchanged, got %#v", configs)
	}
}

func TestRunConfigUpdateContainerWithReader_RejectsPathEscapingRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	withConfigFilePath(t, path)

	seed := shared.DackupConfig{
		User:       "owner",
		Group:      "group",
		Containers: []shared.ContainerConfig{{Container: "web", ToStop: true, Paths: []string{"/data"}}},
	}
	if err := shared.WriteDackupConfig(path, seed, nil); err != nil {
		t.Fatalf("failed to seed existing config: %v", err)
	}

	// container to update, keep name, keep to_stop, paths (escaping the root), keep contains
	input := "web\n\n\n../etc\n\n"

	if err := runConfigUpdateContainerWithReader(readerFor(input)); err == nil {
		t.Fatal("expected an error for a path escaping the root")
	}

	configs, err := shared.ReadContainerConfigsFromPath(path)
	if err != nil {
		t.Fatalf("failed to read containers: %v", err)
	}
	if !reflect.DeepEqual(configs, seed.Containers) {
		t.Fatalf("expected config to be left unchanged, got %#v", configs)
	}
}
