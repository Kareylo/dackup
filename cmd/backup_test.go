package cmd

import (
	"reflect"
	"testing"
)

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
		"paperless_gotenberg",
		"paperless_tika",
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

	selectContainerAndContainedForBackup("a", configByContainer, selected)

	if !selected["a"] {
		t.Fatal("expected container a to be selected")
	}

	if !selected["b"] {
		t.Fatal("expected container b to be selected")
	}
}
