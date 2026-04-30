package cmd

import (
	"reflect"
	"testing"
)

func TestFilterConfigsForRestore_NoRequestedContainersReturnsAll(t *testing.T) {
	configs := testContainerConfigs()

	got, err := filterConfigsForRestore(configs, nil)
	if err != nil {
		t.Fatalf("filterConfigsForRestore returned error: %v", err)
	}

	if !reflect.DeepEqual(got, configs) {
		t.Fatalf("expected all configs, got %#v", got)
	}
}

func TestFilterConfigsForRestore_SelectsRequestedContainer(t *testing.T) {
	configs := testContainerConfigs()

	got, err := filterConfigsForRestore(configs, []string{"adguard"})
	if err != nil {
		t.Fatalf("filterConfigsForRestore returned error: %v", err)
	}

	wantNames := []string{"adguard"}
	assertContainerNames(t, got, wantNames)
}

func TestFilterConfigsForRestore_SelectsContainedContainersRecursively(t *testing.T) {
	configs := testContainerConfigs()

	got, err := filterConfigsForRestore(configs, []string{"paperless"})
	if err != nil {
		t.Fatalf("filterConfigsForRestore returned error: %v", err)
	}

	wantNames := []string{
		"paperless",
		"paperless_db",
		"paperless_broker",
		"redis",
		"paperless_gotenberg",
		"paperless_tika",
	}

	assertContainerNames(t, got, wantNames)
}

func TestFilterConfigsForRestore_SelectsMultipleRequestedContainers(t *testing.T) {
	configs := testContainerConfigs()

	got, err := filterConfigsForRestore(configs, []string{"paperless", "adguard"})
	if err != nil {
		t.Fatalf("filterConfigsForRestore returned error: %v", err)
	}

	wantNames := []string{
		"adguard",
		"paperless",
		"paperless_db",
		"paperless_broker",
		"redis",
		"paperless_gotenberg",
		"paperless_tika",
	}

	assertContainerNames(t, got, wantNames)
}

func TestFilterConfigsForRestore_UnknownContainerReturnsError(t *testing.T) {
	configs := testContainerConfigs()

	_, err := filterConfigsForRestore(configs, []string{"unknown"})
	if err == nil {
		t.Fatal("expected error for unknown container, got nil")
	}
}

func TestFilterConfigsForRestore_IgnoresEmptyRequestedContainer(t *testing.T) {
	configs := testContainerConfigs()

	_, err := filterConfigsForRestore(configs, []string{"   "})
	if err == nil {
		t.Fatal("expected error when only empty containers are requested")
	}
}

func TestRestoreContainersToStopFromConfig(t *testing.T) {
	configs := []containerConfig{
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

	got := restoreContainersToStopFromConfig(configs)
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

func TestRestoreAddContainer_DeduplicatesAndTrims(t *testing.T) {
	seen := make(map[string]bool)
	var containers []string

	restoreAddContainer(" paperless ", seen, &containers)
	restoreAddContainer("paperless", seen, &containers)
	restoreAddContainer("", seen, &containers)
	restoreAddContainer("   ", seen, &containers)
	restoreAddContainer("adguard", seen, &containers)

	want := []string{"paperless", "adguard"}

	if !reflect.DeepEqual(containers, want) {
		t.Fatalf("expected %#v, got %#v", want, containers)
	}
}

func TestRestoreCleanConfiguredPath(t *testing.T) {
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
			got := restoreCleanConfiguredPath(tt.in)
			assertPathEqual(t, got, tt.want)
		})
	}
}

func TestSelectContainerAndContainedForRestore_HandlesCycles(t *testing.T) {
	configs := []containerConfig{
		{
			Container: "a",
			Contains:  []string{"b"},
		},
		{
			Container: "b",
			Contains:  []string{"a"},
		},
	}

	configByContainer := make(map[string]containerConfig)
	for _, config := range configs {
		configByContainer[config.Container] = config
	}

	selected := make(map[string]bool)

	selectContainerAndContainedForRestore("a", configByContainer, selected)

	if !selected["a"] {
		t.Fatal("expected container a to be selected")
	}

	if !selected["b"] {
		t.Fatal("expected container b to be selected")
	}
}

func TestApplyRestoreDirectoryConfig_UsesBackupConfigValuesInReverseWhenFlagsAreNotChanged(t *testing.T) {
	originalSrcDir := restoreSrcDir
	originalDstDir := restoreDstDir
	defer func() {
		restoreSrcDir = originalSrcDir
		restoreDstDir = originalDstDir
	}()

	restoreSrcDir = "/default/restore/src"
	restoreDstDir = "/default/restore/dst"

	config := dackupConfig{
		BackupSrcDir: "/config/backup/src",
		BackupDstDir: "/config/backup/dst",
	}

	applyRestoreDirectoryConfig(config, false, false)

	if restoreSrcDir != config.BackupDstDir {
		t.Fatalf("expected restoreSrcDir %q, got %q", config.BackupDstDir, restoreSrcDir)
	}

	if restoreDstDir != config.BackupSrcDir {
		t.Fatalf("expected restoreDstDir %q, got %q", config.BackupSrcDir, restoreDstDir)
	}
}

func TestApplyRestoreDirectoryConfig_KeepsFlagValuesWhenFlagsAreChanged(t *testing.T) {
	originalSrcDir := restoreSrcDir
	originalDstDir := restoreDstDir
	defer func() {
		restoreSrcDir = originalSrcDir
		restoreDstDir = originalDstDir
	}()

	restoreSrcDir = "/flag/restore/src"
	restoreDstDir = "/flag/restore/dst"

	config := dackupConfig{
		BackupSrcDir: "/config/backup/src",
		BackupDstDir: "/config/backup/dst",
	}

	applyRestoreDirectoryConfig(config, true, true)

	if restoreSrcDir != "/flag/restore/src" {
		t.Fatalf("expected restoreSrcDir to keep flag value %q, got %q", "/flag/restore/src", restoreSrcDir)
	}

	if restoreDstDir != "/flag/restore/dst" {
		t.Fatalf("expected restoreDstDir to keep flag value %q, got %q", "/flag/restore/dst", restoreDstDir)
	}
}

func TestApplyRestoreDirectoryConfig_IgnoresEmptyConfigValues(t *testing.T) {
	originalSrcDir := restoreSrcDir
	originalDstDir := restoreDstDir
	defer func() {
		restoreSrcDir = originalSrcDir
		restoreDstDir = originalDstDir
	}()

	restoreSrcDir = "/default/restore/src"
	restoreDstDir = "/default/restore/dst"

	config := dackupConfig{
		BackupSrcDir: "",
		BackupDstDir: "   ",
	}

	applyRestoreDirectoryConfig(config, false, false)

	if restoreSrcDir != "/default/restore/src" {
		t.Fatalf("expected restoreSrcDir to keep default value %q, got %q", "/default/restore/src", restoreSrcDir)
	}

	if restoreDstDir != "/default/restore/dst" {
		t.Fatalf("expected restoreDstDir to keep default value %q, got %q", "/default/restore/dst", restoreDstDir)
	}
}
