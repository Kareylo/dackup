package backup

import (
	"dackup/internal/backend"
	"dackup/internal/shared"
	"path/filepath"
	"reflect"
	"testing"
)

func testContainerConfigs() []shared.ContainerConfig {
	return []shared.ContainerConfig{
		{
			Container: "paperless",
			ToStop:    true,
			Paths:     []string{"/data/paperless"},
			Contains:  []string{"paperless_db", "paperless_broker"},
		},
		{
			Container: "paperless_db",
			ToStop:    true,
			Paths:     []string{"/data/paperless_db"},
		},
		{
			Container: "paperless_broker",
			ToStop:    true,
			Paths:     []string{"/data/paperless_broker"},
			Contains:  []string{"redis"},
		},
		{
			Container: "redis",
			ToStop:    true,
			Paths:     []string{"/data/redis"},
		},
		{
			Container: "paperless_gotenberg",
			ToStop:    false,
			Paths:     []string{"/data/paperless_gotenberg"},
		},
		{
			Container: "paperless_tika",
			ToStop:    false,
			Paths:     []string{"/data/paperless_tika"},
		},
		{
			Container: "adguard",
			ToStop:    true,
			Paths:     []string{"/config/adguard"},
		},
	}
}

func assertContainerNames(t *testing.T, configs []shared.ContainerConfig, want []string) {
	t.Helper()

	got := make([]string, 0, len(configs))
	for _, config := range configs {
		got = append(got, config.Container)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected container names %#v, got %#v", want, got)
	}
}

func assertPathEqual(t *testing.T, got string, want string) {
	t.Helper()

	got = filepath.Clean(got)
	want = filepath.Clean(want)

	if got != want {
		t.Fatalf("expected path %q, got %q", want, got)
	}
}

func TestFilterConfigsForBackup_NoRequestedContainersReturnsAll(t *testing.T) {
	configs := testContainerConfigs()

	got, err := filterConfigsForBackup(configs, nil)
	if err != nil {
		t.Fatalf("filterConfigsForBackup returned error: %v", err)
	}

	if !reflect.DeepEqual(got, configs) {
		t.Fatalf("expected all configs, got %#v", got)
	}
}

func TestFilterConfigsForBackup_SelectsRequestedContainer(t *testing.T) {
	configs := testContainerConfigs()

	got, err := filterConfigsForBackup(configs, []string{"adguard"})
	if err != nil {
		t.Fatalf("filterConfigsForBackup returned error: %v", err)
	}

	wantNames := []string{"adguard"}
	assertContainerNames(t, got, wantNames)
}

func TestFilterConfigsForBackup_SelectsContainedContainersRecursively(t *testing.T) {
	configs := testContainerConfigs()

	got, err := filterConfigsForBackup(configs, []string{"paperless"})
	if err != nil {
		t.Fatalf("filterConfigsForBackup returned error: %v", err)
	}

	wantNames := []string{
		"paperless",
		"paperless_db",
		"paperless_broker",
		"redis",
	}

	assertContainerNames(t, got, wantNames)
}

func TestFilterConfigsForBackup_SelectsMultipleRequestedContainers(t *testing.T) {
	configs := testContainerConfigs()

	got, err := filterConfigsForBackup(configs, []string{"paperless", "adguard"})
	if err != nil {
		t.Fatalf("filterConfigsForBackup returned error: %v", err)
	}

	wantNames := []string{
		"paperless",
		"paperless_db",
		"paperless_broker",
		"redis",
		"adguard",
	}

	assertContainerNames(t, got, wantNames)
}

func TestFilterConfigsForBackup_UnknownContainerReturnsError(t *testing.T) {
	configs := testContainerConfigs()

	_, err := filterConfigsForBackup(configs, []string{"unknown"})
	if err == nil {
		t.Fatal("expected error for unknown container, got nil")
	}
}

func TestFilterConfigsForBackup_IgnoresEmptyRequestedContainer(t *testing.T) {
	configs := testContainerConfigs()

	_, err := filterConfigsForBackup(configs, []string{"   "})
	if err == nil {
		t.Fatal("expected error when only empty containers are requested")
	}
}

func TestContainersToStopFromConfig(t *testing.T) {
	configs := []shared.ContainerConfig{
		{
			Container: "paperless",
			ToStop:    true,
			Contains:  []string{"paperless_db", "paperless_broker"},
		},
		{
			Container: "paperless_db",
			ToStop:    true,
		},
		{
			Container: "vaultwarden",
			ToStop:    false,
		},
		{
			Container: "adguard",
			ToStop:    true,
		},
	}

	got := containersToStopFromConfig(configs)
	want := []string{
		"paperless",
		"paperless_db",
		"paperless_broker",
		"adguard",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestAddContainer_DeduplicatesAndTrims(t *testing.T) {
	seen := make(map[string]bool)
	var containers []string

	addContainer(" paperless ", seen, &containers)
	addContainer("paperless", seen, &containers)
	addContainer("", seen, &containers)
	addContainer("   ", seen, &containers)
	addContainer("adguard", seen, &containers)

	want := []string{"paperless", "adguard"}

	if !reflect.DeepEqual(containers, want) {
		t.Fatalf("expected %#v, got %#v", want, containers)
	}
}

func TestCleanConfiguredPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "absolute path",
			in:   "/data/paperless",
			want: "data/paperless",
		},
		{
			name: "relative path",
			in:   "data/paperless",
			want: "data/paperless",
		},
		{
			name: "cleans path",
			in:   "/data/../config/adguard",
			want: "config/adguard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanConfiguredPath(tt.in)
			assertPathEqual(t, got, tt.want)
		})
	}
}

func TestSelectContainerAndContainedForBackup_HandlesCycles(t *testing.T) {
	configs := []shared.ContainerConfig{
		{
			Container: "a",
			Contains:  []string{"b"},
		},
		{
			Container: "b",
			Contains:  []string{"a"},
		},
	}

	configByContainer := make(map[string]shared.ContainerConfig)
	for _, config := range configs {
		configByContainer[config.Container] = config
	}

	selected := make(map[string]bool)

	selectContainerAndContainedForBackup("a", configByContainer, selected)

	if !selected["a"] {
		t.Fatal("expected container a to be selected")
	}

	if !selected["b"] {
		t.Fatal("expected container b to be selected")
	}
}

func TestResolveBackend_EmptyNameReturnsDefaultBackend(t *testing.T) {
	got, err := resolveBackend(commandService{}, shared.DackupConfig{})
	if err != nil {
		t.Fatalf("resolveBackend returned error: %v", err)
	}

	if got.Name() != backend.DefaultBackendName {
		t.Fatalf("expected backend name %q, got %q", backend.DefaultBackendName, got.Name())
	}
}

func TestResolveBackend_UnknownNameReturnsError(t *testing.T) {
	_, err := resolveBackend(commandService{}, shared.DackupConfig{Backend: "borg"})
	if err == nil {
		t.Fatal("expected error for unknown backend name, got nil")
	}
}

func TestApplyBackupDirectoryConfig_UsesConfigValuesWhenFlagsAreNotChanged(t *testing.T) {
	originalSrcDir := backupSrcDir
	originalDstDir := backupDstDir
	defer func() {
		backupSrcDir = originalSrcDir
		backupDstDir = originalDstDir
	}()

	backupSrcDir = "/default/src"
	backupDstDir = "/default/dst"

	config := shared.DackupConfig{
		BackupSrcDir: "/config/src",
		BackupDstDir: "/config/dst",
	}

	applyBackupDirectoryConfig(config, false, false)

	if backupSrcDir != config.BackupSrcDir {
		t.Fatalf("expected backupSrcDir %q, got %q", config.BackupSrcDir, backupSrcDir)
	}

	if backupDstDir != config.BackupDstDir {
		t.Fatalf("expected backupDstDir %q, got %q", config.BackupDstDir, backupDstDir)
	}
}

func TestApplyBackupDirectoryConfig_KeepsFlagValuesWhenFlagsAreChanged(t *testing.T) {
	originalSrcDir := backupSrcDir
	originalDstDir := backupDstDir
	defer func() {
		backupSrcDir = originalSrcDir
		backupDstDir = originalDstDir
	}()

	backupSrcDir = "/flag/src"
	backupDstDir = "/flag/dst"

	config := shared.DackupConfig{
		BackupSrcDir: "/config/src",
		BackupDstDir: "/config/dst",
	}

	applyBackupDirectoryConfig(config, true, true)

	if backupSrcDir != "/flag/src" {
		t.Fatalf("expected backupSrcDir to keep flag value %q, got %q", "/flag/src", backupSrcDir)
	}

	if backupDstDir != "/flag/dst" {
		t.Fatalf("expected backupDstDir to keep flag value %q, got %q", "/flag/dst", backupDstDir)
	}
}

func TestApplyBackupDirectoryConfig_IgnoresEmptyConfigValues(t *testing.T) {
	originalSrcDir := backupSrcDir
	originalDstDir := backupDstDir
	defer func() {
		backupSrcDir = originalSrcDir
		backupDstDir = originalDstDir
	}()

	backupSrcDir = "/default/src"
	backupDstDir = "/default/dst"

	config := shared.DackupConfig{
		BackupSrcDir: "   ",
		BackupDstDir: "",
	}

	applyBackupDirectoryConfig(config, false, false)

	if backupSrcDir != "/default/src" {
		t.Fatalf("expected backupSrcDir to keep default value %q, got %q", "/default/src", backupSrcDir)
	}

	if backupDstDir != "/default/dst" {
		t.Fatalf("expected backupDstDir to keep default value %q, got %q", "/default/dst", backupDstDir)
	}
}
