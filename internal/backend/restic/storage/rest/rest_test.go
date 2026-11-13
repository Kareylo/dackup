package rest

import (
	"encoding/json"
	"strings"
	"testing"
)

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

func TestStorage_SendsCredentialsInCleartext(t *testing.T) {
	cases := []struct {
		name    string
		storage Storage
		want    bool
	}{
		{"http with credentials", Storage{URL: "http://backup.example.com:8000", Username: "dackup"}, true},
		{"uppercase HTTP with credentials", Storage{URL: "HTTP://backup.example.com:8000", Username: "dackup"}, true},
		{"https with credentials", Storage{URL: "https://backup.example.com:8000", Username: "dackup"}, false},
		{"uppercase HTTPS with credentials", Storage{URL: "HTTPS://backup.example.com:8000", Username: "dackup"}, false},
		{"http without credentials", Storage{URL: "http://backup.example.com:8000"}, false},
		{"no scheme with credentials", Storage{URL: "backup.example.com:8000", Username: "dackup"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.storage.SendsCredentialsInCleartext(); got != tc.want {
				t.Fatalf("SendsCredentialsInCleartext() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStorage_Validate_RejectsCleartextCredentialsWithoutOptIn(t *testing.T) {
	storage := Storage{URL: "http://backup.example.com:8000", Username: "dackup", EncryptedPassword: "enc:secret"}

	err := storage.Validate()
	if err == nil {
		t.Fatal("expected an error for credentials over http:// without allow_insecure_http")
	}

	for _, want := range []string{"cleartext", "http://backup.example.com:8000", "allow_insecure_http"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to mention %q, got: %v", want, err)
		}
	}
}

func TestStorage_Validate_AcceptsCleartextCredentialsWithOptIn(t *testing.T) {
	storage := Storage{URL: "http://localhost:8000", Username: "dackup", EncryptedPassword: "enc:secret", AllowInsecureHTTP: true}

	if err := storage.Validate(); err != nil {
		t.Fatalf("expected allow_insecure_http to permit http:// credentials, got: %v", err)
	}
}

func TestStorage_Validate_AcceptsHTTPWithoutCredentials(t *testing.T) {
	if err := (Storage{URL: "http://localhost:8010"}).Validate(); err != nil {
		t.Fatalf("expected an unauthenticated http:// server to be accepted, got: %v", err)
	}
}

func TestStorage_Validate_AcceptsHTTPSWithCredentials(t *testing.T) {
	storage := Storage{URL: "https://backup.example.com:8000", Username: "dackup", EncryptedPassword: "enc:secret"}

	if err := storage.Validate(); err != nil {
		t.Fatalf("expected https:// with credentials to be accepted, got: %v", err)
	}
}

func TestStorage_AllowInsecureHTTP_JSONFieldName(t *testing.T) {
	var storage Storage
	if err := json.Unmarshal([]byte(`{"url":"http://localhost:8000","allow_insecure_http":true}`), &storage); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if !storage.AllowInsecureHTTP {
		t.Fatal("expected the allow_insecure_http JSON field to set AllowInsecureHTTP")
	}
}
