package gcs

import "testing"

type fakeSecretStore struct{}

func (fakeSecretStore) Encrypt(plaintext string) (string, error) { return "enc:" + plaintext, nil }
func (fakeSecretStore) Decrypt(ciphertext string) (string, error) {
	return ciphertext[len("enc:"):], nil
}

func TestStorage_Validate_RequiresCredentialsUnlessEmulatorHostSet(t *testing.T) {
	if err := (Storage{}).Validate(); err == nil {
		t.Fatal("expected error for missing bucket")
	}

	if err := (Storage{Bucket: "my-bucket"}).Validate(); err == nil {
		t.Fatal("expected error when neither credentials_file_path nor emulator_host is set")
	}

	withCredentials := Storage{Bucket: "my-bucket", CredentialsFilePath: "/creds.json"}
	if err := withCredentials.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	withEmulator := Storage{Bucket: "my-bucket", EmulatorHost: "localhost:4443"}
	if err := withEmulator.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestStorage_BuildInvocation(t *testing.T) {
	s := Storage{Bucket: "my-bucket", Prefix: "dackup", ProjectID: "proj", CredentialsFilePath: "/creds.json"}

	invocation, err := s.BuildInvocation("global", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	want := "gs:my-bucket:/dackup/global"
	if invocation.Repository != want {
		t.Fatalf("expected repository %q, got %q", want, invocation.Repository)
	}

	if !containsEnv(invocation.Env, "GOOGLE_PROJECT_ID=proj") || !containsEnv(invocation.Env, "GOOGLE_APPLICATION_CREDENTIALS=/creds.json") {
		t.Fatalf("expected credentials in env, got %v", invocation.Env)
	}
}

func TestStorage_BuildInvocation_EmulatorHost(t *testing.T) {
	s := Storage{Bucket: "my-bucket", EmulatorHost: "localhost:4443"}

	invocation, err := s.BuildInvocation("global", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	if !containsEnv(invocation.Env, "STORAGE_EMULATOR_HOST=localhost:4443") {
		t.Fatalf("expected emulator host in env, got %v", invocation.Env)
	}

	if !containsEnv(invocation.Env, "GOOGLE_ACCESS_TOKEN="+dummyEmulatorAccessToken) {
		t.Fatalf("expected dummy access token in env, got %v", invocation.Env)
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
