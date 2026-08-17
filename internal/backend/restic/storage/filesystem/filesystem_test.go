package filesystem

import (
	"dackup/internal/shared"
	"testing"
)

type fakeSecretStore struct{}

func (fakeSecretStore) Encrypt(plaintext string) (string, error) { return "enc:" + plaintext, nil }
func (fakeSecretStore) Decrypt(ciphertext string) (string, error) {
	return ciphertext[len("enc:"):], nil
}

var _ shared.SecretStore = fakeSecretStore{}

func TestStorage_Validate_AlwaysSucceeds(t *testing.T) {
	if err := (Storage{}).Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestStorage_BuildInvocation_JoinsReposRootAndRepoName(t *testing.T) {
	invocation, err := Storage{ReposRoot: "/repos"}.BuildInvocation("global", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	if invocation.Repository != "/repos/global" {
		t.Fatalf("expected repository %q, got %q", "/repos/global", invocation.Repository)
	}

	if len(invocation.Env) != 0 || len(invocation.Args) != 0 {
		t.Fatalf("expected no env or args, got %+v", invocation)
	}
}
