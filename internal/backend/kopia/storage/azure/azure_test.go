package azure

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

func TestStorage_ValidateRequiresFields(t *testing.T) {
	if err := (Storage{}).Validate(); err == nil {
		t.Fatal("expected error for empty Storage")
	}
	if err := (Storage{Container: "c"}).Validate(); err == nil {
		t.Fatal("expected error when storage_account is missing")
	}
	if err := (Storage{Container: "c", StorageAccount: "a"}).Validate(); err == nil {
		t.Fatal("expected error when encrypted_storage_key is missing")
	}
	if err := (Storage{Container: "c", StorageAccount: "a", EncryptedStorageKey: "enc:k"}).Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestStorage_BuildInvocation(t *testing.T) {
	s := Storage{Container: "my-container", StorageAccount: "myaccount", EncryptedStorageKey: "enc:storagekey"}

	invocation, err := s.BuildInvocation("myrepo", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	wantArgs := []string{"--container=my-container", "--storage-account=myaccount", "--storage-key=storagekey", "--prefix=myrepo/"}
	if !equalArgs(invocation.Args, wantArgs) {
		t.Fatalf("expected args %v, got %v", wantArgs, invocation.Args)
	}
}
