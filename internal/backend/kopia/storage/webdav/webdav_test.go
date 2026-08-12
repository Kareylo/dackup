package webdav

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestStorage_ValidateRequiresURLAndPairedAuth(t *testing.T) {
	if err := (Storage{}).Validate(); err == nil {
		t.Fatal("expected error for empty Storage")
	}

	unauthenticated := Storage{URL: "https://webdav.example.com"}
	if err := unauthenticated.Validate(); err != nil {
		t.Fatalf("expected no error for an unauthenticated webdav server, got %v", err)
	}

	authenticated := Storage{URL: "https://webdav.example.com", Username: "u", EncryptedPassword: "enc:pw"}
	if err := authenticated.Validate(); err != nil {
		t.Fatalf("expected no error when both username and encrypted_password are set, got %v", err)
	}

	usernameOnly := Storage{URL: "https://webdav.example.com", Username: "u"}
	if err := usernameOnly.Validate(); err == nil {
		t.Fatal("expected error when username is set without encrypted_password")
	}

	passwordOnly := Storage{URL: "https://webdav.example.com", EncryptedPassword: "enc:pw"}
	if err := passwordOnly.Validate(); err == nil {
		t.Fatal("expected error when encrypted_password is set without username")
	}
}

func TestStorage_BuildInvocationUnauthenticated(t *testing.T) {
	s := Storage{URL: "https://webdav.example.com/backups"}

	invocation, err := s.BuildInvocation("myrepo", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	wantArgs := []string{"--url=https://webdav.example.com/backups/myrepo"}
	if !equalArgs(invocation.Args, wantArgs) {
		t.Fatalf("expected args %v, got %v", wantArgs, invocation.Args)
	}
}

func TestStorage_BuildInvocationWithAuth(t *testing.T) {
	s := Storage{URL: "https://webdav.example.com/backups/", Username: "dackup", EncryptedPassword: "enc:hunter2"}

	invocation, err := s.BuildInvocation("myrepo", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	wantArgs := []string{"--url=https://webdav.example.com/backups/myrepo", "--webdav-username=dackup", "--webdav-password=hunter2"}
	if !equalArgs(invocation.Args, wantArgs) {
		t.Fatalf("expected args %v, got %v", wantArgs, invocation.Args)
	}
}

func TestStorage_EnsureCollectionSendsMKCOLWithAuth(t *testing.T) {
	var gotMethod, gotPath, gotUser, gotPassword string
	var gotAuthOK bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotUser, gotPassword, gotAuthOK = r.BasicAuth()
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	s := Storage{URL: server.URL, Username: "dackup", EncryptedPassword: "enc:hunter2"}

	if err := s.EnsureCollection("myrepo", fakeSecretStore{}); err != nil {
		t.Fatalf("EnsureCollection returned error: %v", err)
	}

	if gotMethod != "MKCOL" {
		t.Fatalf("expected MKCOL request, got %q", gotMethod)
	}
	if gotPath != "/myrepo" {
		t.Fatalf("expected request path %q, got %q", "/myrepo", gotPath)
	}
	if !gotAuthOK || gotUser != "dackup" || gotPassword != "hunter2" {
		t.Fatalf("expected basic auth dackup/hunter2, got ok=%v user=%q password=%q", gotAuthOK, gotUser, gotPassword)
	}
}

func TestStorage_EnsureCollectionAlreadyExistsIsNotAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer server.Close()

	s := Storage{URL: server.URL}

	if err := s.EnsureCollection("myrepo", fakeSecretStore{}); err != nil {
		t.Fatalf("expected 405 (already exists) to not be an error, got %v", err)
	}
}

func TestStorage_EnsureCollectionReturnsErrorOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	s := Storage{URL: server.URL}

	if err := s.EnsureCollection("myrepo", fakeSecretStore{}); err == nil {
		t.Fatal("expected error for a non-2xx, non-405 status")
	}
}
