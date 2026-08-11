package backend

import (
	"dackup/internal/backend/default"
	"testing"
)

func TestFactory_GetBackend_EmptyNameReturnsDefaultBackend(t *testing.T) {
	factory := Factory{}

	got, err := factory.GetBackend("", nil)
	if err != nil {
		t.Fatalf("GetBackend returned error: %v", err)
	}

	if _, ok := got.(defaultbackend.Backend); !ok {
		t.Fatalf("expected defaultbackend.Backend, got %#v", got)
	}
}

func TestFactory_GetBackend_UnknownNameReturnsError(t *testing.T) {
	factory := Factory{}

	_, err := factory.GetBackend("borg", nil)
	if err == nil {
		t.Fatal("expected error for unknown backend name, got nil")
	}
}
