package b2

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

	valid := Storage{Bucket: "my-bucket", AccountID: "acct-id", EncryptedAccountKey: "enc:key"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestStorage_Validate_RequiresEachFieldIndividually(t *testing.T) {
	missingAccountID := Storage{Bucket: "my-bucket", EncryptedAccountKey: "enc:key"}
	if err := missingAccountID.Validate(); err == nil {
		t.Fatal("expected error for missing account_id")
	}

	missingKey := Storage{Bucket: "my-bucket", AccountID: "acct-id"}
	if err := missingKey.Validate(); err == nil {
		t.Fatal("expected error for missing encrypted_account_key")
	}
}

func TestStorage_BuildInvocation(t *testing.T) {
	s := Storage{Bucket: "my-bucket", Prefix: "dackup", AccountID: "acct-id", EncryptedAccountKey: "enc:secretkey"}

	invocation, err := s.BuildInvocation("global", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	want := "b2:my-bucket:dackup/global"
	if invocation.Repository != want {
		t.Fatalf("expected repository %q, got %q", want, invocation.Repository)
	}

	if !containsEnv(invocation.Env, "B2_ACCOUNT_ID=acct-id") || !containsEnv(invocation.Env, "B2_ACCOUNT_KEY=secretkey") {
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
