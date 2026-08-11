package shared

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAESFileSecretStore_EncryptDecryptRoundTrip(t *testing.T) {
	store := AESFileSecretStore{
		KeyPath: filepath.Join(t.TempDir(), "secret.key"),
	}

	ciphertext, err := store.Encrypt("super-secret-passphrase")
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	if ciphertext == "super-secret-passphrase" {
		t.Fatal("expected ciphertext to differ from plaintext")
	}

	plaintext, err := store.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt returned error: %v", err)
	}

	if plaintext != "super-secret-passphrase" {
		t.Fatalf("expected decrypted plaintext %q, got %q", "super-secret-passphrase", plaintext)
	}
}

func TestAESFileSecretStore_ReusesKeyAcrossInstances(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "secret.key")

	first := AESFileSecretStore{KeyPath: keyPath}
	ciphertext, err := first.Encrypt("hunter2")
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	second := AESFileSecretStore{KeyPath: keyPath}
	plaintext, err := second.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt with a fresh store instance returned error: %v", err)
	}

	if plaintext != "hunter2" {
		t.Fatalf("expected decrypted plaintext %q, got %q", "hunter2", plaintext)
	}
}

func TestAESFileSecretStore_CreatesKeyFileWithRestrictedPermissions(t *testing.T) {
	fs := OSFileSystem{}
	keyPath := filepath.Join(t.TempDir(), "nested", "secret.key")
	store := AESFileSecretStore{KeyPath: keyPath, FS: fs}

	if _, err := store.Encrypt("value"); err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	info, err := fs.Stat(keyPath)
	if err != nil {
		t.Fatalf("expected key file to exist: %v", err)
	}

	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("expected key file permissions 0600, got %o", perm)
	}
}

func TestAESFileSecretStore_DifferentKeysCannotDecryptEachOther(t *testing.T) {
	storeA := AESFileSecretStore{KeyPath: filepath.Join(t.TempDir(), "a.key")}
	storeB := AESFileSecretStore{KeyPath: filepath.Join(t.TempDir(), "b.key")}

	ciphertext, err := storeA.Encrypt("value")
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	if _, err := storeB.Decrypt(ciphertext); err == nil {
		t.Fatal("expected decrypting with a different key to fail")
	}
}

func TestAESFileSecretStore_DecryptRejectsUnrecognizedEncoding(t *testing.T) {
	store := AESFileSecretStore{KeyPath: filepath.Join(t.TempDir(), "secret.key")}

	if _, err := store.Decrypt("plaintext-not-encrypted"); err == nil {
		t.Fatal("expected an error for a value without the expected encoding prefix")
	}
}

func TestAESFileSecretStore_DecryptRejectsTamperedCiphertext(t *testing.T) {
	store := AESFileSecretStore{KeyPath: filepath.Join(t.TempDir(), "secret.key")}

	ciphertext, err := store.Encrypt("value")
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	tampered := ciphertext + "tampered"
	if _, err := store.Decrypt(tampered); err == nil {
		t.Fatal("expected an error for tampered ciphertext")
	}

	// Also verify flipping a character inside the payload (not just appending) is rejected.
	if len(ciphertext) > len(secretEncodingPrefix)+2 {
		runes := []rune(ciphertext)
		lastIndex := len(runes) - 1
		if runes[lastIndex] == 'A' {
			runes[lastIndex] = 'B'
		} else {
			runes[lastIndex] = 'A'
		}
		mutated := string(runes)
		if strings.HasPrefix(mutated, secretEncodingPrefix) {
			if _, err := store.Decrypt(mutated); err == nil {
				t.Fatal("expected an error for mutated ciphertext")
			}
		}
	}
}

func TestDefaultSecretKeyPath(t *testing.T) {
	path, err := DefaultSecretKeyPath()
	if err != nil {
		t.Fatalf("DefaultSecretKeyPath returned error: %v", err)
	}

	if !strings.HasSuffix(path, DefaultSecretKeyRelativePath) {
		t.Fatalf("expected path to end with %q, got %q", DefaultSecretKeyRelativePath, path)
	}
}
