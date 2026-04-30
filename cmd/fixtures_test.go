package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func testContainerConfigs() []containerConfig {
	return []containerConfig{
		{
			Container: "adguard",
			ToStop:    true,
			Paths:     []string{"/data/adguard", "/config/adguard"},
		},
		{
			Container: "paperless",
			ToStop:    true,
			Paths:     []string{"/data/paperless"},
			Contains: []string{
				"paperless_db",
				"paperless_broker",
				"paperless_gotenberg",
				"paperless_tika",
			},
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
			ToStop:    true,
		},
		{
			Container: "paperless_tika",
			ToStop:    true,
		},
		{
			Container: "vaultwarden",
			ToStop:    false,
			Paths:     []string{"/data/vaultwarden", "/config/vaultwarden"},
		},
	}
}

func assertContainerNames(t *testing.T, configs []containerConfig, want []string) {
	t.Helper()

	got := make([]string, 0, len(configs))
	for _, config := range configs {
		got = append(got, config.Container)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected containers %#v, got %#v", want, got)
	}
}

func touchFile(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}

	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
}

func assertPathEqual(t *testing.T, got string, want string) {
	t.Helper()

	got = filepath.ToSlash(filepath.Clean(got))
	want = filepath.ToSlash(filepath.Clean(want))

	if got != want {
		t.Fatalf("expected path %q, got %q", want, got)
	}
}

func assertPathSliceEqual(t *testing.T, got []string, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("expected %d paths, got %d: %#v", len(want), len(got), got)
	}

	normalizedGot := make([]string, 0, len(got))
	for _, path := range got {
		normalizedGot = append(normalizedGot, filepath.ToSlash(filepath.Clean(path)))
	}

	normalizedWant := make([]string, 0, len(want))
	for _, path := range want {
		normalizedWant = append(normalizedWant, filepath.ToSlash(filepath.Clean(path)))
	}

	if !reflect.DeepEqual(normalizedGot, normalizedWant) {
		t.Fatalf("expected paths %#v, got %#v", normalizedWant, normalizedGot)
	}
}
