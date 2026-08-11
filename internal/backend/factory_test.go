package backend

import "testing"

func TestFactory_GetBackend_EmptyNameReturnsDefaultBackend(t *testing.T) {
	factory := Factory{}

	got, err := factory.GetBackend("", nil)
	if err != nil {
		t.Fatalf("GetBackend returned error: %v", err)
	}

	if _, ok := got.(DefaultBackend); !ok {
		t.Fatalf("expected DefaultBackend, got %#v", got)
	}
}

func TestFactory_GetBackend_UnknownNameReturnsError(t *testing.T) {
	factory := Factory{}

	_, err := factory.GetBackend("borg", nil)
	if err == nil {
		t.Fatal("expected error for unknown backend name, got nil")
	}
}
