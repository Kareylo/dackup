package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSecretFileSystem lets secrets_test.go inject failures at each step of
// loadOrCreateKey/readKey/createKey that a real OSFileSystem can't easily
// trigger on demand. OpenFile delegates to the real os package (so callers
// get a real *os.File, matching the FileSystem interface), but when
// closeOpenedFile is set it immediately closes the file it just opened
// before returning it, so any Read/Write the caller does next fails with
// "file already closed" — a reliable way to exercise readKey's io.ReadAll
// error branch and createKey's WriteString error branch without a custom
// io.Writer/io.Reader (FileSystem.OpenFile returns a concrete *os.File, not
// an interface).
type fakeSecretFileSystem struct {
	statErr  error
	mkdirErr error
	openErr  error

	closeOpenedFile bool
}

func (fs fakeSecretFileSystem) Stat(name string) (os.FileInfo, error) {
	if fs.statErr != nil {
		return nil, fs.statErr
	}

	return os.Stat(name)
}

func (fs fakeSecretFileSystem) MkdirAll(path string, perm os.FileMode) error {
	if fs.mkdirErr != nil {
		return fs.mkdirErr
	}

	return os.MkdirAll(path, perm)
}

func (fs fakeSecretFileSystem) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	if fs.openErr != nil {
		return nil, fs.openErr
	}

	file, err := os.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}

	if fs.closeOpenedFile {
		file.Close()
	}

	return file, nil
}

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

func TestDefaultSecretKeyPath_PropagatesErrorWhenHomeDirIsUnknown(t *testing.T) {
	t.Setenv("HOME", "")

	if _, err := DefaultSecretKeyPath(); err == nil {
		t.Fatal("expected an error when the home directory can't be determined")
	}
}

func TestAESFileSecretStore_Encrypt_PropagatesKeyPathError(t *testing.T) {
	t.Setenv("HOME", "")

	// An empty KeyPath falls back to DefaultSecretKeyPath, which fails
	// without a resolvable home directory.
	store := AESFileSecretStore{}

	if _, err := store.Encrypt("value"); err == nil {
		t.Fatal("expected an error when the key path can't be determined")
	}
}

func TestAESFileSecretStore_Decrypt_PropagatesCipherError(t *testing.T) {
	t.Setenv("HOME", "")

	store := AESFileSecretStore{}

	if _, err := store.Decrypt(secretEncodingPrefix + "QQ=="); err == nil {
		t.Fatal("expected an error when the key path can't be determined")
	}
}

func TestAESFileSecretStore_Decrypt_RejectsInvalidBase64(t *testing.T) {
	store := AESFileSecretStore{KeyPath: filepath.Join(t.TempDir(), "secret.key")}

	if _, err := store.Decrypt(secretEncodingPrefix + "not-valid-base64!!!"); err == nil {
		t.Fatal("expected an error for invalid base64 ciphertext")
	}
}

func TestAESFileSecretStore_Decrypt_RejectsCiphertextShorterThanNonce(t *testing.T) {
	store := AESFileSecretStore{KeyPath: filepath.Join(t.TempDir(), "secret.key")}

	// A single encoded byte can never contain a full GCM nonce.
	if _, err := store.Decrypt(secretEncodingPrefix + "QQ=="); err == nil {
		t.Fatal("expected an error for ciphertext shorter than the nonce size")
	}
}

func TestAESFileSecretStore_ReadKey_RejectsMalformedKeyFile(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "secret.key")
	if err := os.WriteFile(keyPath, []byte("not a valid key"), 0o600); err != nil {
		t.Fatalf("failed to write malformed key file: %v", err)
	}

	store := AESFileSecretStore{KeyPath: keyPath}

	if _, err := store.Encrypt("value"); err == nil {
		t.Fatal("expected an error for a malformed key file")
	}
}

func TestAESFileSecretStore_LoadOrCreateKey_ReadKeyOpenFileError(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "secret.key")
	if err := os.WriteFile(keyPath, []byte("irrelevant"), 0o600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	store := AESFileSecretStore{
		KeyPath: keyPath,
		FS:      fakeSecretFileSystem{openErr: fmt.Errorf("permission denied")},
	}

	if _, err := store.Encrypt("value"); err == nil {
		t.Fatal("expected an error when opening the existing key file fails")
	}
}

func TestAESFileSecretStore_LoadOrCreateKey_ReadKeyReadError(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "secret.key")
	if err := os.WriteFile(keyPath, []byte("irrelevant"), 0o600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	store := AESFileSecretStore{
		KeyPath: keyPath,
		FS:      fakeSecretFileSystem{closeOpenedFile: true},
	}

	if _, err := store.Encrypt("value"); err == nil {
		t.Fatal("expected an error when reading the existing key file fails")
	}
}

func TestAESFileSecretStore_LoadOrCreateKey_CreateKeyMkdirError(t *testing.T) {
	store := AESFileSecretStore{
		KeyPath: filepath.Join(t.TempDir(), "nested", "secret.key"),
		FS:      fakeSecretFileSystem{mkdirErr: fmt.Errorf("permission denied")},
	}

	if _, err := store.Encrypt("value"); err == nil {
		t.Fatal("expected an error when the key directory can't be created")
	}
}

func TestAESFileSecretStore_LoadOrCreateKey_CreateKeyOpenFileError(t *testing.T) {
	store := AESFileSecretStore{
		KeyPath: filepath.Join(t.TempDir(), "secret.key"),
		FS:      fakeSecretFileSystem{openErr: fmt.Errorf("permission denied")},
	}

	if _, err := store.Encrypt("value"); err == nil {
		t.Fatal("expected an error when the new key file can't be created")
	}
}

func TestAESFileSecretStore_LoadOrCreateKey_CreateKeyWriteError(t *testing.T) {
	store := AESFileSecretStore{
		KeyPath: filepath.Join(t.TempDir(), "secret.key"),
		FS:      fakeSecretFileSystem{closeOpenedFile: true},
	}

	if _, err := store.Encrypt("value"); err == nil {
		t.Fatal("expected an error when writing the new key file fails")
	}
}
