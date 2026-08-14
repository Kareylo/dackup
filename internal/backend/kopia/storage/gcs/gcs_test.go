package gcs

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
		t.Fatal("expected error when credentials_file_path is missing")
	}
	if err := (Storage{Bucket: "b", CredentialsFilePath: "/creds.json"}).Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestStorage_ValidateAllowsMissingCredentialsFilePathWithEmulatorHost(t *testing.T) {
	if err := (Storage{Bucket: "b", EmulatorHost: "localhost:4443"}).Validate(); err != nil {
		t.Fatalf("expected no error when emulator_host is set, got %v", err)
	}
}

func TestStorage_BuildInvocation(t *testing.T) {
	s := Storage{Bucket: "my-bucket", CredentialsFilePath: "/etc/dackup/gcs.json"}

	invocation, err := s.BuildInvocation("myrepo", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	wantArgs := []string{"--bucket=my-bucket", "--credentials-file=/etc/dackup/gcs.json", "--prefix=myrepo/"}
	if !equalArgs(invocation.Args, wantArgs) {
		t.Fatalf("expected args %v, got %v", wantArgs, invocation.Args)
	}
}

func TestStorage_BuildInvocationWithEmulatorHost(t *testing.T) {
	s := Storage{Bucket: "my-bucket", EmulatorHost: "localhost:4443"}

	invocation, err := s.BuildInvocation("myrepo", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	wantArgs := []string{"--bucket=my-bucket", "--prefix=myrepo/"}
	if !equalArgs(invocation.Args, wantArgs) {
		t.Fatalf("expected args %v (no --credentials-file), got %v", wantArgs, invocation.Args)
	}

	if !containsEnv(invocation.Env, "STORAGE_EMULATOR_HOST=localhost:4443") {
		t.Fatalf("expected STORAGE_EMULATOR_HOST env var, got %v", invocation.Env)
	}
}
