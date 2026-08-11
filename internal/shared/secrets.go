package shared

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DefaultSecretKeyRelativePath is DefaultSecretKeyPath's path, relative
	// to the user's home directory.
	DefaultSecretKeyRelativePath = ".config/dackup/secret.key"

	secretEncodingPrefix = "v1:"
	secretKeySize        = 32
)

// SecretStore turns a plaintext secret into ciphertext safe to store in a
// config file, and back. It knows nothing about backends or what the secret
// is for — any backend needing a stored credential (a Borg passphrase, a
// Kopia password, ...) depends on this interface rather than rolling its
// own encryption.
type SecretStore interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// AESFileSecretStore implements SecretStore with AES-256-GCM, keyed by a
// random key lazily created on first use. This protects a secret from
// appearing in plaintext in a config file (e.g. one accidentally committed
// to a dotfiles repo); it does not protect against an attacker who already
// has read access to the same machine, since the key lives right next to
// what it protects. A different SecretStore implementation (an OS keyring,
// a remote secrets manager) can be swapped in later without changing any
// caller, since they only depend on the SecretStore interface.
type AESFileSecretStore struct {
	KeyPath string
	FS      FileSystem
}

// DefaultSecretKeyPath returns the default location of the secret key file,
// ~/.config/dackup/secret.key.
func DefaultSecretKeyPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to find user home directory: %w", err)
	}

	return filepath.Join(homeDir, DefaultSecretKeyRelativePath), nil
}

func (store AESFileSecretStore) Encrypt(plaintext string) (string, error) {
	gcm, err := store.cipher()
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return secretEncodingPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

func (store AESFileSecretStore) Decrypt(ciphertext string) (string, error) {
	encoded, ok := strings.CutPrefix(ciphertext, secretEncodingPrefix)
	if !ok {
		return "", fmt.Errorf("unrecognized secret encoding")
	}

	sealed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("failed to decode secret: %w", err)
	}

	gcm, err := store.cipher()
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(sealed) < nonceSize {
		return "", fmt.Errorf("secret ciphertext is too short")
	}

	nonce, encrypted := sealed[:nonceSize], sealed[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt secret (wrong or missing key file?): %w", err)
	}

	return string(plaintext), nil
}

func (store AESFileSecretStore) cipher() (cipher.AEAD, error) {
	key, err := store.loadOrCreateKey()
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize GCM: %w", err)
	}

	return gcm, nil
}

func (store AESFileSecretStore) loadOrCreateKey() ([]byte, error) {
	fs := store.fileSystem()

	keyPath, err := store.keyPath()
	if err != nil {
		return nil, err
	}

	if _, err := fs.Stat(keyPath); err == nil {
		return store.readKey(fs, keyPath)
	}

	return store.createKey(fs, keyPath)
}

func (store AESFileSecretStore) readKey(fs FileSystem, keyPath string) ([]byte, error) {
	file, err := fs.OpenFile(keyPath, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open secret key file %s: %w", keyPath, err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read secret key file %s: %w", keyPath, err)
	}

	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(key) != secretKeySize {
		return nil, fmt.Errorf("secret key file %s does not contain a valid key", keyPath)
	}

	return key, nil
}

func (store AESFileSecretStore) createKey(fs FileSystem, keyPath string) ([]byte, error) {
	if err := fs.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return nil, fmt.Errorf("failed to create secret key directory: %w", err)
	}

	key := make([]byte, secretKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate secret key: %w", err)
	}

	file, err := fs.OpenFile(keyPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to create secret key file %s: %w", keyPath, err)
	}
	defer file.Close()

	if _, err := file.WriteString(base64.StdEncoding.EncodeToString(key)); err != nil {
		return nil, fmt.Errorf("failed to write secret key file %s: %w", keyPath, err)
	}

	return key, nil
}

func (store AESFileSecretStore) fileSystem() FileSystem {
	if store.FS != nil {
		return store.FS
	}

	return OSFileSystem{}
}

func (store AESFileSecretStore) keyPath() (string, error) {
	if store.KeyPath != "" {
		return store.KeyPath, nil
	}

	return DefaultSecretKeyPath()
}
