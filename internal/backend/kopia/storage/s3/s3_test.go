package s3

import (
	"fmt"
	"strings"
	"testing"
)

type fakeSecretStore struct{}

func (fakeSecretStore) Encrypt(plaintext string) (string, error) {
	return "enc:" + plaintext, nil
}

func (fakeSecretStore) Decrypt(ciphertext string) (string, error) {
	value, ok := strings.CutPrefix(ciphertext, "enc:")
	if !ok {
		return "", fmt.Errorf("not encrypted with fakeSecretStore")
	}
	return value, nil
}

func equalArgs(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func containsEnv(env []string, want string) bool {
	for _, value := range env {
		if value == want {
			return true
		}
	}
	return false
}

func TestStorage_ValidateRequiresFields(t *testing.T) {
	if err := (Storage{}).Validate(); err == nil {
		t.Fatal("expected error for empty Storage")
	}
	if err := (Storage{Bucket: "b"}).Validate(); err == nil {
		t.Fatal("expected error when access_key_id is missing")
	}
	if err := (Storage{Bucket: "b", AccessKeyID: "id"}).Validate(); err == nil {
		t.Fatal("expected error when encrypted_secret_access_key is missing")
	}
	if err := (Storage{Bucket: "b", AccessKeyID: "id", EncryptedSecretAccessKey: "enc:s"}).Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestStorage_ValidateRejectsSchemeInEndpoint(t *testing.T) {
	base := Storage{Bucket: "b", AccessKeyID: "id", EncryptedSecretAccessKey: "enc:s"}

	withHTTP := base
	withHTTP.Endpoint = "http://localhost:9000"
	if err := withHTTP.Validate(); err == nil {
		t.Fatal("expected error for an http:// scheme in endpoint")
	}

	withHTTPS := base
	withHTTPS.Endpoint = "https://s3.example.com"
	if err := withHTTPS.Validate(); err == nil {
		t.Fatal("expected error for an https:// scheme in endpoint")
	}

	bare := base
	bare.Endpoint = "localhost:9000"
	if err := bare.Validate(); err != nil {
		t.Fatalf("expected no error for a bare host:port endpoint, got %v", err)
	}
}

func TestStorage_BuildInvocation(t *testing.T) {
	s := Storage{
		Bucket:                   "my-bucket",
		Endpoint:                 "s3.example.com",
		Region:                   "us-east-1",
		Prefix:                   "dackup",
		DisableTLS:               true,
		AccessKeyID:              "AKID",
		EncryptedSecretAccessKey: "enc:secret",
	}

	invocation, err := s.BuildInvocation("myrepo", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	if invocation.Kind != Name {
		t.Fatalf("expected kind %q, got %q", Name, invocation.Kind)
	}

	wantArgs := []string{"--bucket=my-bucket", "--prefix=dackup/myrepo/", "--endpoint=s3.example.com", "--region=us-east-1", "--disable-tls"}
	if !equalArgs(invocation.Args, wantArgs) {
		t.Fatalf("expected args %v, got %v", wantArgs, invocation.Args)
	}

	if !containsEnv(invocation.Env, "AWS_ACCESS_KEY_ID=AKID") || !containsEnv(invocation.Env, "AWS_SECRET_ACCESS_KEY=secret") {
		t.Fatalf("expected AWS credential env vars, got %v", invocation.Env)
	}
}

func TestStorage_BuildInvocationDecryptErrorPropagates(t *testing.T) {
	s := Storage{Bucket: "b", AccessKeyID: "id", EncryptedSecretAccessKey: "not-encrypted"}

	if _, err := s.BuildInvocation("myrepo", fakeSecretStore{}); err == nil {
		t.Fatal("expected error when the secret store can't decrypt the secret access key")
	}
}
