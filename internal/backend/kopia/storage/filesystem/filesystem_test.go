package filesystem

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

func TestStorage_ValidateAlwaysSucceeds(t *testing.T) {
	if err := (Storage{}).Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestStorage_BuildInvocation(t *testing.T) {
	s := Storage{ReposRoot: "/repos"}

	invocation, err := s.BuildInvocation("myrepo", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	if invocation.Kind != Name {
		t.Fatalf("expected kind %q, got %q", Name, invocation.Kind)
	}

	if !equalArgs(invocation.Args, []string{"--path=/repos/myrepo"}) {
		t.Fatalf("unexpected args: %v", invocation.Args)
	}

	if len(invocation.Env) != 0 {
		t.Fatalf("expected no extra env vars, got %v", invocation.Env)
	}
}
