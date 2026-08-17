package rest

import "testing"

type fakeSecretStore struct{}

func (fakeSecretStore) Encrypt(plaintext string) (string, error) { return "enc:" + plaintext, nil }
func (fakeSecretStore) Decrypt(ciphertext string) (string, error) {
	return ciphertext[len("enc:"):], nil
}

func TestStorage_Validate_RequiresURL(t *testing.T) {
	if err := (Storage{}).Validate(); err == nil {
		t.Fatal("expected error for empty url")
	}

	if err := (Storage{URL: "https://backup.example.com:8000"}).Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestStorage_Validate_RequiresUsernameAndPasswordTogether(t *testing.T) {
	usernameOnly := Storage{URL: "https://backup.example.com:8000", Username: "dackup"}
	if err := usernameOnly.Validate(); err == nil {
		t.Fatal("expected error when username is set without encrypted_password")
	}

	passwordOnly := Storage{URL: "https://backup.example.com:8000", EncryptedPassword: "enc:secret"}
	if err := passwordOnly.Validate(); err == nil {
		t.Fatal("expected error when encrypted_password is set without username")
	}
}

func TestStorage_BuildInvocation_Unauthenticated(t *testing.T) {
	invocation, err := Storage{URL: "https://backup.example.com:8000"}.BuildInvocation("global", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	want := "rest:https://backup.example.com:8000/global"
	if invocation.Repository != want {
		t.Fatalf("expected repository %q, got %q", want, invocation.Repository)
	}

	if len(invocation.Env) != 0 {
		t.Fatalf("expected no env for an unauthenticated server, got %v", invocation.Env)
	}
}

func TestStorage_BuildInvocation_Authenticated(t *testing.T) {
	s := Storage{URL: "https://backup.example.com:8000", Username: "dackup", EncryptedPassword: "enc:hunter2"}

	invocation, err := s.BuildInvocation("global", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	if !containsEnv(invocation.Env, "RESTIC_REST_USERNAME=dackup") || !containsEnv(invocation.Env, "RESTIC_REST_PASSWORD=hunter2") {
		t.Fatalf("expected decrypted credentials in env, got %v", invocation.Env)
	}
}

func TestStorage_BuildInvocation_TrailingSlashInURLIsHandled(t *testing.T) {
	invocation, err := Storage{URL: "https://backup.example.com:8000/"}.BuildInvocation("global", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	want := "rest:https://backup.example.com:8000/global"
	if invocation.Repository != want {
		t.Fatalf("expected repository %q, got %q", want, invocation.Repository)
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
