package backend

import (
	"dackup/internal/backend/default"
	"testing"
)

func TestDefaultBackendName_MatchesDefaultBackendPackage(t *testing.T) {
	if DefaultBackendName != defaultbackend.Name {
		t.Fatalf("expected DefaultBackendName to match defaultbackend.Name, got %q vs %q", DefaultBackendName, defaultbackend.Name)
	}
}
