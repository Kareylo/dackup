package s3

import (
	"strings"
	"testing"
)

type fakeSecretStore struct{}

func (fakeSecretStore) Encrypt(plaintext string) (string, error) { return "enc:" + plaintext, nil }
func (fakeSecretStore) Decrypt(ciphertext string) (string, error) {
	return ciphertext[len("enc:"):], nil
}

func validStorage() Storage {
	return Storage{
		Endpoint:                 "s3.us-east-1.amazonaws.com",
		Bucket:                   "my-bucket",
		AccessKeyID:              "AKID",
		EncryptedSecretAccessKey: "enc:secretkey",
	}
}

func TestStorage_Validate_RequiresFields(t *testing.T) {
	if err := (Storage{}).Validate(); err == nil {
		t.Fatal("expected error for empty storage")
	}

	if err := validStorage().Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestStorage_Validate_RequiresEachFieldIndividually(t *testing.T) {
	missingBucket := validStorage()
	missingBucket.Bucket = ""
	if err := missingBucket.Validate(); err == nil {
		t.Fatal("expected error for missing bucket")
	}

	missingAccessKeyID := validStorage()
	missingAccessKeyID.AccessKeyID = ""
	if err := missingAccessKeyID.Validate(); err == nil {
		t.Fatal("expected error for missing access_key_id")
	}

	missingSecret := validStorage()
	missingSecret.EncryptedSecretAccessKey = ""
	if err := missingSecret.Validate(); err == nil {
		t.Fatal("expected error for missing encrypted_secret_access_key")
	}
}

func TestStorage_Validate_RejectsEndpointWithScheme(t *testing.T) {
	s := validStorage()
	s.Endpoint = "https://s3.us-east-1.amazonaws.com"

	if err := s.Validate(); err == nil {
		t.Fatal("expected error for endpoint with scheme")
	}
}

func TestStorage_BuildInvocation_HTTPS(t *testing.T) {
	invocation, err := validStorage().BuildInvocation("global", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	want := "s3:https://s3.us-east-1.amazonaws.com/my-bucket/global"
	if invocation.Repository != want {
		t.Fatalf("expected repository %q, got %q", want, invocation.Repository)
	}

	if !containsEnv(invocation.Env, "AWS_ACCESS_KEY_ID=AKID") || !containsEnv(invocation.Env, "AWS_SECRET_ACCESS_KEY=secretkey") {
		t.Fatalf("expected decrypted credentials in env, got %v", invocation.Env)
	}
}

func TestStorage_BuildInvocation_DisableTLSUsesHTTP(t *testing.T) {
	s := validStorage()
	s.DisableTLS = true

	invocation, err := s.BuildInvocation("global", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	if !strings.HasPrefix(invocation.Repository, "s3:http://") {
		t.Fatalf("expected http scheme, got %q", invocation.Repository)
	}
}

func TestStorage_BuildInvocation_IncludesRegionWhenSet(t *testing.T) {
	s := validStorage()
	s.Region = "us-east-1"

	invocation, err := s.BuildInvocation("global", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	if !containsEnv(invocation.Env, "AWS_DEFAULT_REGION=us-east-1") {
		t.Fatalf("expected region in env, got %v", invocation.Env)
	}
}

func containsEnv(env []string, want string) bool {
	for _, value := range env {
		if value == want {
			return true
		}
	}
	return false
}
