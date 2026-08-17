package azure

import "testing"

type fakeSecretStore struct{}

func (fakeSecretStore) Encrypt(plaintext string) (string, error) { return "enc:" + plaintext, nil }
func (fakeSecretStore) Decrypt(ciphertext string) (string, error) {
	return ciphertext[len("enc:"):], nil
}

func TestStorage_Validate_RequiresFields(t *testing.T) {
	if err := (Storage{}).Validate(); err == nil {
		t.Fatal("expected error for empty storage")
	}

	valid := Storage{Container: "my-container", AccountName: "myaccount", EncryptedAccountKey: "enc:key"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestStorage_Validate_RequiresEachFieldIndividually(t *testing.T) {
	missingAccountName := Storage{Container: "my-container", EncryptedAccountKey: "enc:key"}
	if err := missingAccountName.Validate(); err == nil {
		t.Fatal("expected error for missing account_name")
	}

	missingKey := Storage{Container: "my-container", AccountName: "myaccount"}
	if err := missingKey.Validate(); err == nil {
		t.Fatal("expected error for missing encrypted_account_key")
	}
}

func TestStorage_BuildInvocation(t *testing.T) {
	s := Storage{Container: "my-container", Prefix: "dackup", AccountName: "myaccount", EncryptedAccountKey: "enc:storagekey"}

	invocation, err := s.BuildInvocation("global", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	want := "azure:my-container:/dackup/global"
	if invocation.Repository != want {
		t.Fatalf("expected repository %q, got %q", want, invocation.Repository)
	}

	if !containsEnv(invocation.Env, "AZURE_ACCOUNT_NAME=myaccount") || !containsEnv(invocation.Env, "AZURE_ACCOUNT_KEY=storagekey") {
		t.Fatalf("expected decrypted credentials in env, got %v", invocation.Env)
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
