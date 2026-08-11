package shared

import "testing"

func TestContainerGroups_GroupsConnectedContainersAndKeepsStandaloneSeparate(t *testing.T) {
	configs := []ContainerConfig{
		{Container: "paperless", Contains: []string{"paperless_db", "paperless_broker"}},
		{Container: "paperless_db"},
		{Container: "adguard"},
		{Container: "paperless_broker"},
	}

	groups := ContainerGroups(configs)

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d: %+v", len(groups), groups)
	}

	paperlessGroup := groups[0]
	if paperlessGroup.Name != "paperless" {
		t.Fatalf("expected first group name %q, got %q", "paperless", paperlessGroup.Name)
	}

	gotMembers := containerNames(paperlessGroup.Containers)
	wantMembers := []string{"paperless", "paperless_db", "paperless_broker"}
	assertSameNames(t, wantMembers, gotMembers)

	adguardGroup := groups[1]
	if adguardGroup.Name != "adguard" {
		t.Fatalf("expected second group name %q, got %q", "adguard", adguardGroup.Name)
	}

	if len(adguardGroup.Containers) != 1 || adguardGroup.Containers[0].Container != "adguard" {
		t.Fatalf("expected adguard to be its own group of one, got %+v", adguardGroup.Containers)
	}
}

func TestContainerGroups_GroupsAcrossDirectionRegardlessOfWalkOrder(t *testing.T) {
	// paperless_db and paperless_broker don't list "contains" themselves; only
	// paperless does. Starting the walk from paperless_db first must still
	// land all three in the same group.
	configs := []ContainerConfig{
		{Container: "paperless_db"},
		{Container: "paperless_broker"},
		{Container: "paperless", Contains: []string{"paperless_db", "paperless_broker"}},
	}

	groups := ContainerGroups(configs)

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d: %+v", len(groups), groups)
	}

	if groups[0].Name != "paperless" {
		t.Fatalf("expected group name %q, got %q", "paperless", groups[0].Name)
	}

	assertSameNames(t, []string{"paperless_db", "paperless_broker", "paperless"}, containerNames(groups[0].Containers))
}

func TestContainerGroups_IgnoresContainsReferencingUnknownContainer(t *testing.T) {
	configs := []ContainerConfig{
		{Container: "paperless", Contains: []string{"nonexistent"}},
	}

	groups := ContainerGroups(configs)

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d: %+v", len(groups), groups)
	}

	if len(groups[0].Containers) != 1 {
		t.Fatalf("expected group to only contain the configured container, got %+v", groups[0].Containers)
	}
}

func TestContainerGroups_MultipleParentsFallBackToSortedName(t *testing.T) {
	configs := []ContainerConfig{
		{Container: "zeta", Contains: []string{"shared_db"}},
		{Container: "alpha", Contains: []string{"shared_db"}},
		{Container: "shared_db"},
	}

	groups := ContainerGroups(configs)

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d: %+v", len(groups), groups)
	}

	if groups[0].Name != "alpha" {
		t.Fatalf("expected deterministic fallback name %q, got %q", "alpha", groups[0].Name)
	}
}

func TestContainerGroups_CycleFallsBackToSortedName(t *testing.T) {
	configs := []ContainerConfig{
		{Container: "beta", Contains: []string{"alpha"}},
		{Container: "alpha", Contains: []string{"beta"}},
	}

	groups := ContainerGroups(configs)

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d: %+v", len(groups), groups)
	}

	if groups[0].Name != "alpha" {
		t.Fatalf("expected deterministic fallback name %q, got %q", "alpha", groups[0].Name)
	}
}

func TestContainerGroups_EmptyConfigsReturnsNoGroups(t *testing.T) {
	groups := ContainerGroups(nil)

	if len(groups) != 0 {
		t.Fatalf("expected no groups, got %+v", groups)
	}
}

func containerNames(configs []ContainerConfig) []string {
	names := make([]string, 0, len(configs))
	for _, config := range configs {
		names = append(names, config.Container)
	}

	return names
}

func assertSameNames(t *testing.T, want []string, got []string) {
	t.Helper()

	if len(want) != len(got) {
		t.Fatalf("expected names %v, got %v", want, got)
	}

	seen := make(map[string]bool, len(got))
	for _, name := range got {
		seen[name] = true
	}

	for _, name := range want {
		if !seen[name] {
			t.Fatalf("expected names %v to contain %q, got %v", want, name, got)
		}
	}
}
