package shared

import (
	"reflect"
	"testing"
)

func TestSelectContainerAndContained_SelectsRecursively(t *testing.T) {
	configByContainer := map[string]ContainerConfig{
		"app":   {Container: "app", Contains: []string{"db", "cache"}},
		"db":    {Container: "db"},
		"cache": {Container: "cache"},
	}
	selected := make(map[string]bool)

	SelectContainerAndContained("app", configByContainer, selected)

	want := map[string]bool{"app": true, "db": true, "cache": true}
	if !reflect.DeepEqual(selected, want) {
		t.Fatalf("selected = %v, want %v", selected, want)
	}
}

func TestSelectContainerAndContained_HandlesCycles(t *testing.T) {
	configByContainer := map[string]ContainerConfig{
		"a": {Container: "a", Contains: []string{"b"}},
		"b": {Container: "b", Contains: []string{"a"}},
	}
	selected := make(map[string]bool)

	SelectContainerAndContained("a", configByContainer, selected)

	want := map[string]bool{"a": true, "b": true}
	if !reflect.DeepEqual(selected, want) {
		t.Fatalf("selected = %v, want %v", selected, want)
	}
}

func TestSelectContainerAndContained_UnknownContainerIsNoOp(t *testing.T) {
	configByContainer := map[string]ContainerConfig{}
	selected := make(map[string]bool)

	SelectContainerAndContained("missing", configByContainer, selected)

	if len(selected) != 0 {
		t.Fatalf("expected nothing selected, got %v", selected)
	}
}

func TestSelectContainerAndContained_EmptyNameIsNoOp(t *testing.T) {
	configByContainer := map[string]ContainerConfig{"app": {Container: "app"}}
	selected := make(map[string]bool)

	SelectContainerAndContained("  ", configByContainer, selected)

	if len(selected) != 0 {
		t.Fatalf("expected nothing selected, got %v", selected)
	}
}

func TestFilterContainerConfigs_NoRequestedContainersReturnsAll(t *testing.T) {
	configs := []ContainerConfig{{Container: "app"}, {Container: "db"}}

	filtered, err := FilterContainerConfigs(configs, nil, "backup")
	if err != nil {
		t.Fatalf("FilterContainerConfigs returned error: %v", err)
	}

	if !reflect.DeepEqual(filtered, configs) {
		t.Fatalf("filtered = %v, want %v", filtered, configs)
	}
}

func TestFilterContainerConfigs_SelectsRequestedContainerAndContained(t *testing.T) {
	configs := []ContainerConfig{
		{Container: "app", Contains: []string{"db"}},
		{Container: "db"},
		{Container: "unrelated"},
	}

	filtered, err := FilterContainerConfigs(configs, []string{"app"}, "backup")
	if err != nil {
		t.Fatalf("FilterContainerConfigs returned error: %v", err)
	}

	var names []string
	for _, config := range filtered {
		names = append(names, config.Container)
	}

	want := []string{"app", "db"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
}

func TestFilterContainerConfigs_SelectsMultipleRequestedContainers(t *testing.T) {
	configs := []ContainerConfig{
		{Container: "app"},
		{Container: "db"},
		{Container: "unrelated"},
	}

	filtered, err := FilterContainerConfigs(configs, []string{"app", "db"}, "backup")
	if err != nil {
		t.Fatalf("FilterContainerConfigs returned error: %v", err)
	}

	if len(filtered) != 2 {
		t.Fatalf("expected 2 configs, got %d: %v", len(filtered), filtered)
	}
}

func TestFilterContainerConfigs_IgnoresEmptyRequestedContainer(t *testing.T) {
	configs := []ContainerConfig{{Container: "app"}}

	filtered, err := FilterContainerConfigs(configs, []string{"app", "  "}, "backup")
	if err != nil {
		t.Fatalf("FilterContainerConfigs returned error: %v", err)
	}

	if len(filtered) != 1 || filtered[0].Container != "app" {
		t.Fatalf("filtered = %v, want just app", filtered)
	}
}

func TestFilterContainerConfigs_UnknownContainerReturnsError(t *testing.T) {
	configs := []ContainerConfig{{Container: "app"}}

	_, err := FilterContainerConfigs(configs, []string{"missing"}, "backup")
	if err == nil {
		t.Fatal("expected error for unknown container, got nil")
	}
}

func TestFilterContainerConfigs_AllRequestsEmptyReturnsNoContainersSelectedError(t *testing.T) {
	configs := []ContainerConfig{{Container: "app"}}

	_, err := FilterContainerConfigs(configs, []string{"  "}, "restore")
	if err == nil {
		t.Fatal("expected error when nothing is selected, got nil")
	}
}

func TestContainersToStopFromConfig_ListsToStopAndContainedInOrderWithoutDuplicates(t *testing.T) {
	configs := []ContainerConfig{
		{Container: "app", ToStop: true, Contains: []string{"db", "app"}},
		{Container: "db", ToStop: false},
		{Container: "cache", ToStop: true},
	}

	containers := ContainersToStopFromConfig(configs)

	want := []string{"app", "db", "cache"}
	if !reflect.DeepEqual(containers, want) {
		t.Fatalf("containers = %v, want %v", containers, want)
	}
}

func TestContainersToStopFromConfig_NoneToStopReturnsEmpty(t *testing.T) {
	configs := []ContainerConfig{{Container: "app", ToStop: false}}

	containers := ContainersToStopFromConfig(configs)

	if len(containers) != 0 {
		t.Fatalf("expected no containers, got %v", containers)
	}
}

func TestAddUniqueContainer_AppendsTrimmedAndDeduplicates(t *testing.T) {
	seen := make(map[string]bool)
	var containers []string

	AddUniqueContainer(" app ", seen, &containers)
	AddUniqueContainer("app", seen, &containers)
	AddUniqueContainer("db", seen, &containers)

	want := []string{"app", "db"}
	if !reflect.DeepEqual(containers, want) {
		t.Fatalf("containers = %v, want %v", containers, want)
	}
}

func TestAddUniqueContainer_IgnoresEmpty(t *testing.T) {
	seen := make(map[string]bool)
	var containers []string

	AddUniqueContainer("   ", seen, &containers)

	if len(containers) != 0 {
		t.Fatalf("expected no containers, got %v", containers)
	}
}
