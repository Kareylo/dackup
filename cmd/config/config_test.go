package config

import (
	"bufio"
	"dackup/internal/shared"
	"reflect"
	"strings"
	"testing"
)

func TestParseStringList(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{
			name:  "empty",
			value: "",
			want:  nil,
		},
		{
			name:  "spaces only",
			value: "   ",
			want:  nil,
		},
		{
			name:  "single value",
			value: "paperless",
			want:  []string{"paperless"},
		},
		{
			name:  "multiple values",
			value: "paperless, adguard, vaultwarden",
			want:  []string{"paperless", "adguard", "vaultwarden"},
		},
		{
			name:  "trims and skips empty values",
			value: " paperless, , adguard ,, vaultwarden ",
			want:  []string{"paperless", "adguard", "vaultwarden"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseStringList(tt.value)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("expected %#v, got %#v", tt.want, got)
			}
		})
	}
}

func TestAskStringWithDefault_UsesDefaultWhenInputIsEmpty(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("\n"))

	got, err := askStringWithDefault(reader, "Label", "default-value")
	if err != nil {
		t.Fatalf("askStringWithDefault returned error: %v", err)
	}

	if got != "default-value" {
		t.Fatalf("expected default value, got %q", got)
	}
}

func TestAskStringWithDefault_UsesProvidedValue(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("custom-value\n"))

	got, err := askStringWithDefault(reader, "Label", "default-value")
	if err != nil {
		t.Fatalf("askStringWithDefault returned error: %v", err)
	}

	if got != "custom-value" {
		t.Fatalf("expected custom value, got %q", got)
	}
}

func TestAskStringListWithDefault_UsesDefaultWhenInputIsEmpty(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("\n"))
	defaultValues := []string{"paperless", "adguard"}

	got, err := askStringListWithDefault(reader, "Label", defaultValues)
	if err != nil {
		t.Fatalf("askStringListWithDefault returned error: %v", err)
	}

	if !reflect.DeepEqual(got, defaultValues) {
		t.Fatalf("expected %#v, got %#v", defaultValues, got)
	}
}

func TestAskStringListWithDefault_NoneClearsValues(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("none\n"))

	got, err := askStringListWithDefault(reader, "Label", []string{"paperless"})
	if err != nil {
		t.Fatalf("askStringListWithDefault returned error: %v", err)
	}

	if got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}
}

func TestAskStringListWithDefault_ParsesProvidedValues(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("paperless, adguard\n"))

	got, err := askStringListWithDefault(reader, "Label", nil)
	if err != nil {
		t.Fatalf("askStringListWithDefault returned error: %v", err)
	}

	want := []string{"paperless", "adguard"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestFindContainerIndex_FindsContainer(t *testing.T) {
	configs := []shared.ContainerConfig{
		{
			Container: "paperless",
		},
		{
			Container: "adguard",
		},
	}

	got := findContainerIndex(configs, "adguard")
	if got != 1 {
		t.Fatalf("expected index 1, got %d", got)
	}
}

func TestFindContainerIndex_TrimsContainerName(t *testing.T) {
	configs := []shared.ContainerConfig{
		{
			Container: "paperless",
		},
	}

	got := findContainerIndex(configs, " paperless ")
	if got != 0 {
		t.Fatalf("expected index 0, got %d", got)
	}
}

func TestFindContainerIndex_ReturnsMinusOneWhenMissing(t *testing.T) {
	configs := []shared.ContainerConfig{
		{
			Container: "paperless",
		},
	}

	got := findContainerIndex(configs, "missing")
	if got != -1 {
		t.Fatalf("expected -1, got %d", got)
	}
}

func TestNormalizeConfigPath_ExpandsRelativePath(t *testing.T) {
	got, err := normalizeConfigPath("config.json")
	if err != nil {
		t.Fatalf("normalizeConfigPath returned error: %v", err)
	}

	if !strings.HasSuffix(got, "config.json") {
		t.Fatalf("expected path to end with config.json, got %q", got)
	}
}

func TestNormalizeConfigPath_ReturnsErrorForEmptyPath(t *testing.T) {
	_, err := normalizeConfigPath("   ")
	if err == nil {
		t.Fatal("expected error for empty path, got nil")
	}
}

func TestContainsString_FindsValue(t *testing.T) {
	if !containsString([]string{"paperless_db", "paperless_broker"}, "paperless_broker") {
		t.Fatal("expected containsString to find the value")
	}
}

func TestContainsString_ReturnsFalseWhenMissing(t *testing.T) {
	if containsString([]string{"paperless_db"}, "adguard") {
		t.Fatal("expected containsString to return false for a missing value")
	}
}

func TestContainsString_ReturnsFalseForEmptySlice(t *testing.T) {
	if containsString(nil, "adguard") {
		t.Fatal("expected containsString to return false for a nil slice")
	}
}
