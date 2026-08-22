package backend

import (
	"dackup/internal/backend/borg"
	"dackup/internal/backend/default"
	"dackup/internal/backend/kopia"
	"dackup/internal/backend/restic"
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

	_, err := factory.GetBackend("unknown", nil)
	if err == nil {
		t.Fatal("expected error for unknown backend name, got nil")
	}
}

func TestFactory_GetBackend_BorgWithoutBackendDirReturnsError(t *testing.T) {
	factory := Factory{}

	_, err := factory.GetBackend(borg.Name, []byte(`{"encryption":"none"}`))
	if err == nil {
		t.Fatal("expected error when backend_dir is not set")
	}
}

func TestFactory_GetBackend_BorgWithInvalidSettingsReturnsError(t *testing.T) {
	factory := Factory{BackendDir: "/mnt/backup/borg-repos"}

	_, err := factory.GetBackend(borg.Name, []byte(`{"encryption":"repokey"}`))
	if err == nil {
		t.Fatal("expected error when repokey encryption is set without a passphrase")
	}
}

func TestFactory_GetBackend_BorgWithValidSettingsReturnsBorgBackend(t *testing.T) {
	factory := Factory{BackendDir: "/mnt/backup/borg-repos"}

	got, err := factory.GetBackend(borg.Name, []byte(`{"encryption":"none"}`))
	if err != nil {
		t.Fatalf("GetBackend returned error: %v", err)
	}

	borgBackend, ok := got.(borg.Backend)
	if !ok {
		t.Fatalf("expected borg.Backend, got %#v", got)
	}

	if borgBackend.ReposRoot != "/mnt/backup/borg-repos" {
		t.Fatalf("expected ReposRoot %q, got %q", "/mnt/backup/borg-repos", borgBackend.ReposRoot)
	}
}

func TestFactory_GetBackend_KopiaWithoutBackendDirReturnsError(t *testing.T) {
	factory := Factory{}

	_, err := factory.GetBackend(kopia.Name, []byte(`{"encrypted_password":"enc:secret"}`))
	if err == nil {
		t.Fatal("expected error when backend_dir is not set")
	}
}

func TestFactory_GetBackend_KopiaWithInvalidSettingsReturnsError(t *testing.T) {
	factory := Factory{BackendDir: "/mnt/backup/kopia-repos"}

	_, err := factory.GetBackend(kopia.Name, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error when encrypted_password is not set")
	}
}

func TestFactory_GetBackend_KopiaWithValidSettingsReturnsKopiaBackend(t *testing.T) {
	factory := Factory{BackendDir: "/mnt/backup/kopia-repos"}

	got, err := factory.GetBackend(kopia.Name, []byte(`{"encrypted_password":"enc:secret"}`))
	if err != nil {
		t.Fatalf("GetBackend returned error: %v", err)
	}

	kopiaBackend, ok := got.(kopia.Backend)
	if !ok {
		t.Fatalf("expected kopia.Backend, got %#v", got)
	}

	if kopiaBackend.ReposRoot != "/mnt/backup/kopia-repos" {
		t.Fatalf("expected ReposRoot %q, got %q", "/mnt/backup/kopia-repos", kopiaBackend.ReposRoot)
	}
}

func TestFactory_GetBackend_ResticWithoutBackendDirReturnsError(t *testing.T) {
	factory := Factory{}

	_, err := factory.GetBackend(restic.Name, []byte(`{"encrypted_password":"enc:secret"}`))
	if err == nil {
		t.Fatal("expected error when backend_dir is not set")
	}
}

func TestFactory_GetBackend_ResticWithInvalidSettingsReturnsError(t *testing.T) {
	factory := Factory{BackendDir: "/mnt/backup/restic-repos"}

	_, err := factory.GetBackend(restic.Name, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error when encrypted_password is not set")
	}
}

func TestFactory_GetBackend_ResticWithValidSettingsReturnsResticBackend(t *testing.T) {
	factory := Factory{BackendDir: "/mnt/backup/restic-repos"}

	got, err := factory.GetBackend(restic.Name, []byte(`{"encrypted_password":"enc:secret"}`))
	if err != nil {
		t.Fatalf("GetBackend returned error: %v", err)
	}

	resticBackend, ok := got.(restic.Backend)
	if !ok {
		t.Fatalf("expected restic.Backend, got %#v", got)
	}

	if resticBackend.ReposRoot != "/mnt/backup/restic-repos" {
		t.Fatalf("expected ReposRoot %q, got %q", "/mnt/backup/restic-repos", resticBackend.ReposRoot)
	}
}
