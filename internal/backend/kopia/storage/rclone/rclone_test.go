package rclone

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

func TestStorage_ValidateRequiresRemoteName(t *testing.T) {
	if err := (Storage{}).Validate(); err == nil {
		t.Fatal("expected error for empty Storage")
	}
	if err := (Storage{RemoteName: "b2remote"}).Validate(); err != nil {
		t.Fatalf("expected no error once remote_name is set, got %v", err)
	}
}

func TestStorage_BuildInvocationMinimal(t *testing.T) {
	s := Storage{RemoteName: "b2remote"}

	invocation, err := s.BuildInvocation("myrepo", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	if invocation.Kind != Name {
		t.Fatalf("expected kind %q, got %q", Name, invocation.Kind)
	}

	wantArgs := []string{"--remote-path=b2remote:myrepo"}
	if !equalArgs(invocation.Args, wantArgs) {
		t.Fatalf("expected args %v, got %v", wantArgs, invocation.Args)
	}

	if len(invocation.Env) != 0 {
		t.Fatalf("expected no env vars when config_file_path is unset, got %v", invocation.Env)
	}
}

func TestStorage_BuildInvocationWithAllFields(t *testing.T) {
	s := Storage{
		RemoteName:     "b2remote",
		RemotePath:     "dackup",
		RcloneExePath:  "/usr/local/bin/rclone",
		ConfigFilePath: "/etc/dackup/rclone.conf",
	}

	invocation, err := s.BuildInvocation("myrepo", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	wantArgs := []string{"--remote-path=b2remote:dackup/myrepo", "--rclone-exe=/usr/local/bin/rclone"}
	if !equalArgs(invocation.Args, wantArgs) {
		t.Fatalf("expected args %v, got %v", wantArgs, invocation.Args)
	}

	if !containsEnv(invocation.Env, "RCLONE_CONFIG=/etc/dackup/rclone.conf") {
		t.Fatalf("expected RCLONE_CONFIG env var, got %v", invocation.Env)
	}
}
