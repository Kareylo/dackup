package shared

import "sort"

// ContainerGroup is a set of containers that should share a single backend
// repository, because they're connected via contains (directly or
// transitively, in either direction). A container with no contains
// relationship at all is its own group of one.
type ContainerGroup struct {
	Name       string
	Containers []ContainerConfig
}

// ContainerGroups partitions configs into connected components of the
// contains relationship, treated as undirected: if A contains B, A and B
// land in the same group regardless of which one is walked first (unlike
// SelectContainerAndContained, which only walks parent -> contained).
// Each group's Name is the one member not listed in any other member's
// Contains (the "root"); ties (no single root, or more than one) fall back
// to the lexicographically first container name in the group, for a
// deterministic result. Groups are returned in the order their first member
// appears in configs.
func ContainerGroups(configs []ContainerConfig) []ContainerGroup {
	configByContainer := make(map[string]ContainerConfig, len(configs))
	for _, config := range configs {
		configByContainer[config.Container] = config
	}

	adjacency := make(map[string][]string, len(configs))
	containedBy := make(map[string][]string, len(configs))

	for _, config := range configs {
		for _, contained := range config.Contains {
			if _, exists := configByContainer[contained]; !exists {
				continue
			}

			adjacency[config.Container] = append(adjacency[config.Container], contained)
			adjacency[contained] = append(adjacency[contained], config.Container)
			containedBy[contained] = append(containedBy[contained], config.Container)
		}
	}

	visited := make(map[string]bool, len(configs))
	var groups []ContainerGroup

	for _, config := range configs {
		if visited[config.Container] {
			continue
		}

		memberNames := connectedComponent(config.Container, adjacency, visited)

		var members []ContainerConfig
		for _, original := range configs {
			if containsName(memberNames, original.Container) {
				members = append(members, original)
			}
		}

		groups = append(groups, ContainerGroup{
			Name:       groupName(memberNames, containedBy),
			Containers: members,
		})
	}

	return groups
}

func connectedComponent(start string, adjacency map[string][]string, visited map[string]bool) []string {
	var component []string
	queue := []string{start}
	visited[start] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		component = append(component, current)

		for _, neighbor := range adjacency[current] {
			if visited[neighbor] {
				continue
			}

			visited[neighbor] = true
			queue = append(queue, neighbor)
		}
	}

	return component
}

func groupName(memberNames []string, containedBy map[string][]string) string {
	var roots []string
	for _, name := range memberNames {
		if len(containedBy[name]) == 0 {
			roots = append(roots, name)
		}
	}

	if len(roots) == 1 {
		return roots[0]
	}

	sorted := append([]string(nil), memberNames...)
	sort.Strings(sorted)
	return sorted[0]
}

func containsName(names []string, target string) bool {
	for _, name := range names {
		if name == target {
			return true
		}
	}

	return false
}

// BackendGroup is the backend-facing view of a ContainerGroup: just a name
// and the cleaned, deduped relative paths its members contribute, without
// exposing the rest of ContainerConfig. It lives here rather than in
// internal/backend so a concrete backend package (e.g.
// internal/backend/borg) can reference it while only depending on
// internal/shared — internal/backend's Factory already has to import every
// concrete backend subpackage to construct it, so the reverse import would
// be a cycle.
type BackendGroup struct {
	Name  string
	Paths []string
}

// BackendGroupsFromContainerGroups converts ContainerGroups output into
// BackendGroup values: one per container-group, carrying the union of its
// members' cleaned paths. A path referenced by containers in two different
// groups appears in both — each group gets whatever data it needs, even if
// that duplicates a path across repositories.
func BackendGroupsFromContainerGroups(containerGroups []ContainerGroup) []BackendGroup {
	groups := make([]BackendGroup, 0, len(containerGroups))

	for _, containerGroup := range containerGroups {
		seen := make(map[string]bool)
		var paths []string

		for _, container := range containerGroup.Containers {
			for _, path := range container.Paths {
				cleanPath := CleanConfiguredPath(path)
				if cleanPath == "" || seen[cleanPath] {
					continue
				}

				seen[cleanPath] = true
				paths = append(paths, cleanPath)
			}
		}

		groups = append(groups, BackendGroup{
			Name:  containerGroup.Name,
			Paths: paths,
		})
	}

	return groups
}
