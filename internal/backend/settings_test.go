package backend

import (
	"dackup/internal/backend/borg"
	"dackup/internal/backend/kopia"
	"dackup/internal/backend/restic"
	"testing"
)

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
	_, err := ParseSettings("unknown", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for unknown backend name, got nil")
	}
}

func TestParseSettings_BorgReturnsParsedConfig(t *testing.T) {
	got, err := ParseSettings("borg", []byte(`{"encryption":"none"}`))
	if err != nil {
		t.Fatalf("ParseSettings returned error: %v", err)
	}

	config, ok := got.(borg.Config)
	if !ok {
		t.Fatalf("expected borg.Config, got %#v", got)
	}

	if config.Encryption != "none" {
		t.Fatalf("expected encryption %q, got %q", "none", config.Encryption)
	}
}

func TestParseSettings_KopiaReturnsParsedConfig(t *testing.T) {
	got, err := ParseSettings("kopia", []byte(`{"encrypted_password":"enc:secret"}`))
	if err != nil {
		t.Fatalf("ParseSettings returned error: %v", err)
	}

	config, ok := got.(kopia.Config)
	if !ok {
		t.Fatalf("expected kopia.Config, got %#v", got)
	}

	if config.EncryptedPassword != "enc:secret" {
		t.Fatalf("expected encrypted_password %q, got %q", "enc:secret", config.EncryptedPassword)
	}
}

func TestParseSettings_ResticReturnsParsedConfig(t *testing.T) {
	got, err := ParseSettings("restic", []byte(`{"encrypted_password":"enc:secret"}`))
	if err != nil {
		t.Fatalf("ParseSettings returned error: %v", err)
	}

	config, ok := got.(restic.Config)
	if !ok {
		t.Fatalf("expected restic.Config, got %#v", got)
	}

	if config.EncryptedPassword != "enc:secret" {
		t.Fatalf("expected encrypted_password %q, got %q", "enc:secret", config.EncryptedPassword)
	}
}
