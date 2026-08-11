package shared

import (
	"reflect"
	"testing"
)

func TestBackendGroupsFromContainerGroups_UnionsAndDedupesMemberPaths(t *testing.T) {
	containerGroups := []ContainerGroup{
		{
			Name: "paperless",
			Containers: []ContainerConfig{
				{Container: "paperless", Paths: []string{"/data/paperless", "/config/shared"}},
				{Container: "paperless_db", Paths: []string{"/data/paperless_db", "/config/shared"}},
			},
		},
		{
			Name: "adguard",
			Containers: []ContainerConfig{
				{Container: "adguard", Paths: []string{"/config/adguard"}},
			},
		},
	}

	groups := BackendGroupsFromContainerGroups(containerGroups)

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d: %+v", len(groups), groups)
	}

	if groups[0].Name != "paperless" {
		t.Fatalf("expected first group name %q, got %q", "paperless", groups[0].Name)
	}

	wantPaperlessPaths := []string{"data/paperless", "config/shared", "data/paperless_db"}
	if !reflect.DeepEqual(groups[0].Paths, wantPaperlessPaths) {
		t.Fatalf("expected paperless group paths %v, got %v", wantPaperlessPaths, groups[0].Paths)
	}

	if groups[1].Name != "adguard" {
		t.Fatalf("expected second group name %q, got %q", "adguard", groups[1].Name)
	}

	wantAdguardPaths := []string{"config/adguard"}
	if !reflect.DeepEqual(groups[1].Paths, wantAdguardPaths) {
		t.Fatalf("expected adguard group paths %v, got %v", wantAdguardPaths, groups[1].Paths)
	}
}

func TestBackendGroupsFromContainerGroups_SkipsPathsThatCleanToEmpty(t *testing.T) {
	containerGroups := []ContainerGroup{
		{
			Name: "solo",
			Containers: []ContainerConfig{
				{Container: "solo", Paths: []string{"/", "data"}},
			},
		},
	}

	groups := BackendGroupsFromContainerGroups(containerGroups)

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d: %+v", len(groups), groups)
	}

	wantPaths := []string{"data"}
	if !reflect.DeepEqual(groups[0].Paths, wantPaths) {
		t.Fatalf("expected paths %v (root path cleaned away), got %v", wantPaths, groups[0].Paths)
	}
}

func TestBackendGroupsFromContainerGroups_EmptyInputReturnsEmptySlice(t *testing.T) {
	groups := BackendGroupsFromContainerGroups(nil)

	if len(groups) != 0 {
		t.Fatalf("expected no groups, got %+v", groups)
	}
}
