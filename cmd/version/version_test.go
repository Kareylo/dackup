package version

import (
	"bytes"
	"dackup/internal/shared"
	"strings"
	"testing"
)

func TestVersionCommandPrintsVersion(t *testing.T) {
	originalVersion := Version
	Version = "1.2.3"
	defer func() { Version = originalVersion }()

	cmd := NewCommand(&shared.Options{})

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if got := out.String(); !strings.Contains(got, "1.2.3") {
		t.Fatalf("expected output to contain version %q, got %q", "1.2.3", got)
	}
}
