package rclone

import "testing"

type fakeSecretStore struct{}

func (fakeSecretStore) Encrypt(plaintext string) (string, error) { return "enc:" + plaintext, nil }
func (fakeSecretStore) Decrypt(ciphertext string) (string, error) {
	return ciphertext[len("enc:"):], nil
}

func TestStorage_Validate_RequiresRemoteName(t *testing.T) {
	if err := (Storage{}).Validate(); err == nil {
		t.Fatal("expected error for empty remote_name")
	}

	if err := (Storage{RemoteName: "b2remote"}).Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestStorage_BuildInvocation_Minimal(t *testing.T) {
	invocation, err := Storage{RemoteName: "b2remote"}.BuildInvocation("global", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	if invocation.Repository != "rclone:b2remote:global" {
		t.Fatalf("expected repository %q, got %q", "rclone:b2remote:global", invocation.Repository)
	}

	if len(invocation.Args) != 0 || len(invocation.Env) != 0 {
		t.Fatalf("expected no args or env, got %+v", invocation)
	}
}

func TestStorage_BuildInvocation_WithExeAndConfigPaths(t *testing.T) {
	s := Storage{RemoteName: "b2remote", RemotePath: "dackup", RcloneExePath: "/usr/local/bin/rclone", ConfigFilePath: "/etc/rclone.conf"}

	invocation, err := s.BuildInvocation("global", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	if invocation.Repository != "rclone:b2remote:dackup/global" {
		t.Fatalf("expected repository %q, got %q", "rclone:b2remote:dackup/global", invocation.Repository)
	}

	if len(invocation.Args) != 2 || invocation.Args[1] != "rclone.program=/usr/local/bin/rclone" {
		t.Fatalf("expected rclone.program arg, got %v", invocation.Args)
	}

	if len(invocation.Env) != 1 || invocation.Env[0] != "RCLONE_CONFIG=/etc/rclone.conf" {
		t.Fatalf("expected RCLONE_CONFIG env, got %v", invocation.Env)
	}
}
