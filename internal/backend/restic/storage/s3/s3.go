// Package s3 implements storage.Provider for restic repositories stored in
// an S3 (or S3-compatible: MinIO, Wasabi, ...) bucket.
package s3

import (
	"dackup/internal/backend/restic/storage"
	"dackup/internal/shared"
	"fmt"
	"strings"
)

// Name is the identifier written to restic.Config.StorageType.
const Name = "s3"

// Storage configures an S3 bucket as repository storage. Restic addresses
// S3 via a "s3:scheme://endpoint/bucket/path" repository URI (verified
// against restic's own "Preparing a new repository" docs — both an explicit
// "s3:https://server:port/bucket_name" S3-compatible example and an
// explicit "s3:http://localhost:9000/restic" MinIO example are documented,
// so the scheme is always spelled out here rather than left to restic's own
// default, mirroring kopia's own s3 storage type's DisableTLS field).
type Storage struct {
	// Endpoint is a bare host[:port], e.g. "s3.us-east-1.amazonaws.com" or
	// "localhost:9000" for MinIO — no scheme; DisableTLS controls that
	// separately.
	Endpoint string `json:"endpoint,omitempty"`

	// Bucket is the S3 bucket name.
	Bucket string `json:"bucket,omitempty"`

	// Prefix is prepended to each repository's own name-derived path,
	// letting several dackup deployments share one bucket.
	Prefix string `json:"prefix,omitempty"`

	// Region is the bucket's AWS region, sent via AWS_DEFAULT_REGION. Not
	// required for every S3-compatible service.
	Region string `json:"region,omitempty"`

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
	if strings.TrimSpace(s.Endpoint) == "" {
		return fmt.Errorf("restic s3 storage requires endpoint")
	}

	if strings.TrimSpace(s.Bucket) == "" {
		return fmt.Errorf("restic s3 storage requires bucket")
	}

	if strings.TrimSpace(s.AccessKeyID) == "" {
		return fmt.Errorf("restic s3 storage requires access_key_id")
	}

	if strings.TrimSpace(s.EncryptedSecretAccessKey) == "" {
		return fmt.Errorf("restic s3 storage requires encrypted_secret_access_key")
	}

	if strings.Contains(s.Endpoint, "://") {
		return fmt.Errorf("restic s3 endpoint must be a bare host[:port] (no %q scheme) — set disable_tls instead", strings.SplitN(s.Endpoint, "://", 2)[0])
	}

	return nil
}

// BuildInvocation implements storage.Provider.
func (s Storage) BuildInvocation(repoName string, secrets shared.SecretStore) (storage.Invocation, error) {
	secretAccessKey, err := secrets.Decrypt(s.EncryptedSecretAccessKey)
	if err != nil {
		return storage.Invocation{}, fmt.Errorf("failed to decrypt restic s3 secret access key: %w", err)
	}

	scheme := "https"
	if s.DisableTLS {
		scheme = "http"
	}

	repository := fmt.Sprintf("s3:%s://%s/%s/%s", scheme, s.Endpoint, s.Bucket, storage.RepoPath(s.Prefix, repoName))

	env := []string{"AWS_ACCESS_KEY_ID=" + s.AccessKeyID, "AWS_SECRET_ACCESS_KEY=" + secretAccessKey}
	if s.Region != "" {
		env = append(env, "AWS_DEFAULT_REGION="+s.Region)
	}

	return storage.Invocation{Repository: repository, Env: env}, nil
}
