package backend

import "testing"

func TestParseSettings_EmptyNameReturnsNil(t *testing.T) {
	got, err := ParseSettings("", nil)
	if err != nil {
		t.Fatalf("ParseSettings returned error: %v", err)
	}

	if got != nil {
		t.Fatalf("expected nil settings, got %#v", got)
	}
}

func TestParseSettings_UnknownNameReturnsError(t *testing.T) {
	_, err := ParseSettings("borg", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for unknown backend name, got nil")
	}
}
