// Package s3 implements storage.Provider for kopia repositories stored in
// an S3 (or S3-compatible: MinIO, Wasabi, B2's S3 endpoint, ...) bucket.
package s3

import (
	"dackup/internal/backend/kopia/storage"
	"dackup/internal/shared"
	"fmt"
	"strings"
)

// Name is the identifier written to kopia.Config.StorageType (and kopia's
// own "repository create/connect s3" subcommand).
const Name = "s3"

// Storage configures an S3 bucket as repository storage.
type Storage struct {
	// Bucket is the S3 bucket name.
	Bucket string `json:"bucket,omitempty"`

	// Endpoint overrides the default AWS S3 endpoint, for S3-compatible
	// services. Must be a bare host[:port] — kopia's --endpoint flag
	// doesn't take a scheme, that's controlled separately by DisableTLS.
	Endpoint string `json:"endpoint,omitempty"`

	// Region is the bucket's AWS region. Not required for every
	// S3-compatible service.
	Region string `json:"region,omitempty"`

	// Prefix is prepended to each repository's own name-derived prefix,
	// letting several dackup deployments share one bucket.
	Prefix string `json:"prefix,omitempty"`

	// DisableTLS connects over plain HTTP instead of HTTPS. Only useful
	// against a local/private S3-compatible endpoint.
	DisableTLS bool `json:"disable_tls,omitempty"`

	// AccessKeyID is not a secret on its own (it's an identifier, not a
	// credential), so it's stored as plain text.
	AccessKeyID string `json:"access_key_id,omitempty"`

	// EncryptedSecretAccessKey is ciphertext produced by a
	// shared.SecretStore, never a plaintext secret key.
	EncryptedSecretAccessKey string `json:"encrypted_secret_access_key,omitempty"`
}

// Validate reports whether the S3 settings are well-formed.
func (s Storage) Validate() error {
	if strings.TrimSpace(s.Bucket) == "" {
		return fmt.Errorf("kopia s3 storage requires bucket")
	}

	if strings.TrimSpace(s.AccessKeyID) == "" {
		return fmt.Errorf("kopia s3 storage requires access_key_id")
	}

	if strings.TrimSpace(s.EncryptedSecretAccessKey) == "" {
		return fmt.Errorf("kopia s3 storage requires encrypted_secret_access_key")
	}

	if strings.Contains(s.Endpoint, "://") {
		return fmt.Errorf("kopia s3 endpoint must be a bare host[:port] (no %q scheme) — set disable_tls instead", strings.SplitN(s.Endpoint, "://", 2)[0])
	}

	return nil
}

func (s Storage) BuildInvocation(repoName string, secrets shared.SecretStore) (storage.Invocation, error) {
	secretAccessKey, err := secrets.Decrypt(s.EncryptedSecretAccessKey)
	if err != nil {
		return storage.Invocation{}, fmt.Errorf("failed to decrypt kopia s3 secret access key: %w", err)
	}

	args := []string{"--bucket=" + s.Bucket, "--prefix=" + storage.ObjectPrefix(s.Prefix, repoName)}
	if s.Endpoint != "" {
		args = append(args, "--endpoint="+s.Endpoint)
	}
	if s.Region != "" {
		args = append(args, "--region="+s.Region)
	}
	if s.DisableTLS {
		args = append(args, "--disable-tls")
	}

	return storage.Invocation{
		Kind: Name,
		Args: args,
		Env:  []string{"AWS_ACCESS_KEY_ID=" + s.AccessKeyID, "AWS_SECRET_ACCESS_KEY=" + secretAccessKey},
	}, nil
}
