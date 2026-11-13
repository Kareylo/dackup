package swift

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

func validStorage() Storage {
	return Storage{
		Container:         "my-container",
		AuthURL:           "https://keystone.example.com/v3",
		Username:          "dackup",
		EncryptedPassword: "enc:hunter2",
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
	missingAuthURL := validStorage()
	missingAuthURL.AuthURL = ""
	if err := missingAuthURL.Validate(); err == nil {
		t.Fatal("expected error for missing auth_url")
	}

	missingUsername := validStorage()
	missingUsername.Username = ""
	if err := missingUsername.Validate(); err == nil {
		t.Fatal("expected error for missing username")
	}

	missingPassword := validStorage()
	missingPassword.EncryptedPassword = ""
	if err := missingPassword.Validate(); err == nil {
		t.Fatal("expected error for missing encrypted_password")
	}
}

func TestStorage_BuildInvocation(t *testing.T) {
	s := validStorage()
	s.Prefix = "dackup"
	s.TenantName = "myproject"
	s.RegionName = "RegionOne"

	invocation, err := s.BuildInvocation("global", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	want := "swift:my-container:/dackup/global"
	if invocation.Repository != want {
		t.Fatalf("expected repository %q, got %q", want, invocation.Repository)
	}

	for _, want := range []string{
		"OS_AUTH_URL=https://keystone.example.com/v3",
		"OS_USERNAME=dackup",
		"OS_PASSWORD=hunter2",
		"OS_TENANT_NAME=myproject",
		"OS_REGION_NAME=RegionOne",
	} {
		if !containsEnv(invocation.Env, want) {
			t.Fatalf("expected %q in env, got %v", want, invocation.Env)
		}
	}
}

func TestStorage_BuildInvocation_OmitsOptionalEnvWhenUnset(t *testing.T) {
	invocation, err := validStorage().BuildInvocation("global", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	if containsEnvPrefix(invocation.Env, "OS_TENANT_NAME=") || containsEnvPrefix(invocation.Env, "OS_REGION_NAME=") {
		t.Fatalf("expected no optional env vars when unset, got %v", invocation.Env)
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

func containsEnvPrefix(env []string, prefix string) bool {
	for _, value := range env {
		if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func TestStorage_SendsCredentialsInCleartext(t *testing.T) {
	cases := []struct {
		name    string
		authURL string
		want    bool
	}{
		{"http", "http://keystone.example.com/v3", true},
		{"uppercase HTTP", "HTTP://keystone.example.com/v3", true},
		{"no scheme", "keystone.example.com/v3", true},
		{"https", "https://keystone.example.com/v3", false},
		{"uppercase HTTPS", "HTTPS://keystone.example.com/v3", false},
		{"https with surrounding spaces", "  https://keystone.example.com/v3  ", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storage := validStorage()
			storage.AuthURL = tc.authURL

			if got := storage.SendsCredentialsInCleartext(); got != tc.want {
				t.Fatalf("SendsCredentialsInCleartext() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStorage_Validate_RejectsCleartextAuthURLWithoutOptIn(t *testing.T) {
	storage := validStorage()
	storage.AuthURL = "http://keystone.example.com/v3"

	err := storage.Validate()
	if err == nil {
		t.Fatal("expected an error for an http:// auth_url without allow_insecure_http")
	}

	for _, want := range []string{"cleartext", "http://keystone.example.com/v3", "allow_insecure_http"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to mention %q, got: %v", want, err)
		}
	}
}

func TestStorage_Validate_AcceptsCleartextAuthURLWithOptIn(t *testing.T) {
	storage := validStorage()
	storage.AuthURL = "http://localhost:5000/v3"
	storage.AllowInsecureHTTP = true

	if err := storage.Validate(); err != nil {
		t.Fatalf("expected allow_insecure_http to permit an http:// auth_url, got: %v", err)
	}
}

func TestStorage_Validate_MissingFieldErrorWinsOverCleartextError(t *testing.T) {
	storage := validStorage()
	storage.AuthURL = "http://keystone.example.com/v3"
	storage.EncryptedPassword = ""

	err := storage.Validate()
	if err == nil || !strings.Contains(err.Error(), "encrypted_password") {
		t.Fatalf("expected the missing encrypted_password error first, got: %v", err)
	}
}

func TestStorage_AllowInsecureHTTP_JSONFieldName(t *testing.T) {
	var storage Storage
	if err := json.Unmarshal([]byte(`{"auth_url":"http://localhost:5000/v3","allow_insecure_http":true}`), &storage); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if !storage.AllowInsecureHTTP {
		t.Fatal("expected the allow_insecure_http JSON field to set AllowInsecureHTTP")
	}
}
