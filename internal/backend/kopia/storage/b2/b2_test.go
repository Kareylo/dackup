package b2

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

func assertSecretNotInArgs(t *testing.T, args []string, secret string) {
	t.Helper()

	for _, arg := range args {
		if strings.Contains(arg, secret) {
			t.Fatalf("expected secret %q to stay out of argv, found it in %q", secret, arg)
		}
	}
}

func TestStorage_ValidateRequiresFields(t *testing.T) {
	if err := (Storage{}).Validate(); err == nil {
		t.Fatal("expected error for empty Storage")
	}
	if err := (Storage{Bucket: "b"}).Validate(); err == nil {
		t.Fatal("expected error when key_id is missing")
	}
	if err := (Storage{Bucket: "b", KeyID: "id"}).Validate(); err == nil {
		t.Fatal("expected error when encrypted_application_key is missing")
	}
	if err := (Storage{Bucket: "b", KeyID: "id", EncryptedApplicationKey: "enc:k"}).Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestStorage_BuildInvocation(t *testing.T) {
	s := Storage{Bucket: "my-bucket", KeyID: "key-id", EncryptedApplicationKey: "enc:appkey"}

	invocation, err := s.BuildInvocation("myrepo", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	wantArgs := []string{"--bucket=my-bucket", "--key-id=key-id", "--prefix=myrepo/"}
	if !equalArgs(invocation.Args, wantArgs) {
		t.Fatalf("expected args %v, got %v", wantArgs, invocation.Args)
	}

	wantEnv := []string{"B2_KEY=appkey"}
	if !equalArgs(invocation.Env, wantEnv) {
		t.Fatalf("expected env %v, got %v", wantEnv, invocation.Env)
	}

	assertSecretNotInArgs(t, invocation.Args, "appkey")
}
