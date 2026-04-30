package cmd

import (
	"bufio"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseStringList(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "empty",
			in:   "",
			want: nil,
		},
		{
			name: "spaces only",
			in:   "   ",
			want: nil,
		},
		{
			name: "single item",
			in:   "/data/paperless",
			want: []string{"/data/paperless"},
		},
		{
			name: "multiple items",
			in:   "/data/paperless, /config/paperless, /data/db",
			want: []string{"/data/paperless", "/config/paperless", "/data/db"},
		},
		{
			name: "ignores empty items",
			in:   "/data/paperless,, /config/paperless, ",
			want: []string{"/data/paperless", "/config/paperless"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseStringList(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("expected %#v, got %#v", tt.want, got)
			}
		})
	}
}

func TestAskString(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("paperless\n"))

	got, err := askString(reader, "Container name")
	if err != nil {
		t.Fatalf("askString returned error: %v", err)
	}

	if got != "paperless" {
		t.Fatalf("expected %q, got %q", "paperless", got)
	}
}

func TestAskRequiredString(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("\n   \npaperless\n"))

	got, err := askRequiredString(reader, "Container name")
	if err != nil {
		t.Fatalf("askRequiredString returned error: %v", err)
	}

	if got != "paperless" {
		t.Fatalf("expected %q, got %q", "paperless", got)
	}
}

func TestAskStringWithDefault_EmptyUsesDefault(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("\n"))

	got, err := askStringWithDefault(reader, "Container name", "paperless")
	if err != nil {
		t.Fatalf("askStringWithDefault returned error: %v", err)
	}

	if got != "paperless" {
		t.Fatalf("expected %q, got %q", "paperless", got)
	}
}

func TestAskStringWithDefault_ValueOverridesDefault(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("adguard\n"))

	got, err := askStringWithDefault(reader, "Container name", "paperless")
	if err != nil {
		t.Fatalf("askStringWithDefault returned error: %v", err)
	}

	if got != "adguard" {
		t.Fatalf("expected %q, got %q", "adguard", got)
	}
}

func TestAskBool_EmptyUsesDefaultTrue(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("\n"))

	got, err := askBool(reader, "Stop?", true)
	if err != nil {
		t.Fatalf("askBool returned error: %v", err)
	}

	if !got {
		t.Fatal("expected true, got false")
	}
}

func TestAskBool_EmptyUsesDefaultFalse(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("\n"))

	got, err := askBool(reader, "Stop?", false)
	if err != nil {
		t.Fatalf("askBool returned error: %v", err)
	}

	if got {
		t.Fatal("expected false, got true")
	}
}

func TestAskBool_ParsesYes(t *testing.T) {
	inputs := []string{"y\n", "yes\n", "true\n", "1\n"}

	for _, input := range inputs {
		t.Run(strings.TrimSpace(input), func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(input))

			got, err := askBool(reader, "Stop?", false)
			if err != nil {
				t.Fatalf("askBool returned error: %v", err)
			}

			if !got {
				t.Fatal("expected true, got false")
			}
		})
	}
}

func TestAskBool_ParsesNo(t *testing.T) {
	inputs := []string{"n\n", "no\n", "false\n", "0\n"}

	for _, input := range inputs {
		t.Run(strings.TrimSpace(input), func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(input))

			got, err := askBool(reader, "Stop?", true)
			if err != nil {
				t.Fatalf("askBool returned error: %v", err)
			}

			if got {
				t.Fatal("expected false, got true")
			}
		})
	}
}

func TestAskBool_InvalidThenValid(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("maybe\ny\n"))

	got, err := askBool(reader, "Stop?", false)
	if err != nil {
		t.Fatalf("askBool returned error: %v", err)
	}

	if !got {
		t.Fatal("expected true, got false")
	}
}

func TestAskStringList(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("/data/paperless, /config/paperless\n"))

	got, err := askStringList(reader, "Paths")
	if err != nil {
		t.Fatalf("askStringList returned error: %v", err)
	}

	want := []string{"/data/paperless", "/config/paperless"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestAskStringListWithDefault_EmptyUsesDefault(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("\n"))
	defaults := []string{"/data/paperless"}

	got, err := askStringListWithDefault(reader, "Paths", defaults)
	if err != nil {
		t.Fatalf("askStringListWithDefault returned error: %v", err)
	}

	if !reflect.DeepEqual(got, defaults) {
		t.Fatalf("expected %#v, got %#v", defaults, got)
	}
}

func TestAskStringListWithDefault_NoneClearsList(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("none\n"))
	defaults := []string{"/data/paperless"}

	got, err := askStringListWithDefault(reader, "Paths", defaults)
	if err != nil {
		t.Fatalf("askStringListWithDefault returned error: %v", err)
	}

	if got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}
}

func TestAskContainerConfig(t *testing.T) {
	input := strings.Join([]string{
		"paperless",
		"y",
		"/data/paperless, /config/paperless",
		"paperless_db, paperless_broker",
		"",
	}, "\n")

	reader := bufio.NewReader(strings.NewReader(input))

	got, err := askContainerConfig(reader)
	if err != nil {
		t.Fatalf("askContainerConfig returned error: %v", err)
	}

	want := containerConfig{
		Container: "paperless",
		ToStop:    true,
		Paths:     []string{"/data/paperless", "/config/paperless"},
		Contains:  []string{"paperless_db", "paperless_broker"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestAskUpdatedContainerConfig_KeepsCurrentValues(t *testing.T) {
	current := containerConfig{
		Container: "paperless",
		ToStop:    true,
		Paths:     []string{"/data/paperless"},
		Contains:  []string{"paperless_db"},
	}

	input := strings.Join([]string{
		"",
		"",
		"",
		"",
		"",
	}, "\n")

	reader := bufio.NewReader(strings.NewReader(input))

	got, err := askUpdatedContainerConfig(reader, current)
	if err != nil {
		t.Fatalf("askUpdatedContainerConfig returned error: %v", err)
	}

	if !reflect.DeepEqual(got, current) {
		t.Fatalf("expected %#v, got %#v", current, got)
	}
}

func TestAskUpdatedContainerConfig_OverridesValues(t *testing.T) {
	current := containerConfig{
		Container: "paperless",
		ToStop:    true,
		Paths:     []string{"/data/paperless"},
		Contains:  []string{"paperless_db"},
	}

	input := strings.Join([]string{
		"paperless-new",
		"n",
		"/data/paperless-new",
		"none",
		"",
	}, "\n")

	reader := bufio.NewReader(strings.NewReader(input))

	got, err := askUpdatedContainerConfig(reader, current)
	if err != nil {
		t.Fatalf("askUpdatedContainerConfig returned error: %v", err)
	}

	want := containerConfig{
		Container: "paperless-new",
		ToStop:    false,
		Paths:     []string{"/data/paperless-new"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestFindContainerIndex(t *testing.T) {
	configs := []containerConfig{
		{
			Container: "adguard",
		},
		{
			Container: "paperless",
		},
	}

	got := findContainerIndex(configs, "paperless")
	if got != 1 {
		t.Fatalf("expected index 1, got %d", got)
	}

	got = findContainerIndex(configs, "missing")
	if got != -1 {
		t.Fatalf("expected index -1, got %d", got)
	}
}

func TestNormalizeConfigPath_AbsolutePath(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "config.json")

	got, err := normalizeConfigPath(path)
	if err != nil {
		t.Fatalf("normalizeConfigPath returned error: %v", err)
	}

	assertPathEqual(t, got, path)
}

func TestNormalizeConfigPath_RelativePath(t *testing.T) {
	got, err := normalizeConfigPath("config.json")
	if err != nil {
		t.Fatalf("normalizeConfigPath returned error: %v", err)
	}

	if !filepath.IsAbs(got) {
		t.Fatalf("expected absolute path, got %q", got)
	}
}

func TestNormalizeConfigPath_EmptyPathReturnsError(t *testing.T) {
	_, err := normalizeConfigPath("   ")
	if err == nil {
		t.Fatal("expected error for empty path, got nil")
	}
}

func TestFileExists(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "file.txt")

	if fileExists(path) {
		t.Fatal("expected file to not exist")
	}

	touchFile(t, path)

	if !fileExists(path) {
		t.Fatal("expected file to exist")
	}
}
