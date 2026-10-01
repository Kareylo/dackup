package webdav

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func assertSecretNotInArgs(t *testing.T, args []string, secret string) {
	t.Helper()

	for _, arg := range args {
		if strings.Contains(arg, secret) {
			t.Fatalf("expected secret %q to stay out of argv, found it in %q", secret, arg)
		}
	}
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

	wantArgs := []string{"--url=https://webdav.example.com/backups/myrepo", "--webdav-username=dackup"}
	if !equalArgs(invocation.Args, wantArgs) {
		t.Fatalf("expected args %v, got %v", wantArgs, invocation.Args)
	}

	wantEnv := []string{"KOPIA_WEBDAV_PASSWORD=hunter2"}
	if !equalArgs(invocation.Env, wantEnv) {
		t.Fatalf("expected env %v, got %v", wantEnv, invocation.Env)
	}

	assertSecretNotInArgs(t, invocation.Args, "hunter2")
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

func TestStorage_ValidateRejectsCredentialsOverPlainHTTP(t *testing.T) {
	cleartext := Storage{URL: "http://webdav.example.com", Username: "u", EncryptedPassword: "enc:pw"}
	if err := cleartext.Validate(); err == nil {
		t.Fatal("expected error for credentials over a plain http:// URL")
	}

	optedIn := Storage{URL: "http://webdav.example.com", Username: "u", EncryptedPassword: "enc:pw", AllowInsecureHTTP: true}
	if err := optedIn.Validate(); err != nil {
		t.Fatalf("expected no error when allow_insecure_http is set, got %v", err)
	}

	unauthenticated := Storage{URL: "http://webdav.example.com"}
	if err := unauthenticated.Validate(); err != nil {
		t.Fatalf("expected no error for an unauthenticated plain http:// server, got %v", err)
	}

	upperCaseScheme := Storage{URL: "HTTPS://webdav.example.com", Username: "u", EncryptedPassword: "enc:pw"}
	if err := upperCaseScheme.Validate(); err != nil {
		t.Fatalf("expected an upper-case https scheme to count as https, got %v", err)
	}
}

func TestStorage_SendsCredentialsInCleartext(t *testing.T) {
	cases := []struct {
		storage Storage
		want    bool
	}{
		{Storage{URL: "http://webdav.example.com", Username: "u"}, true},
		{Storage{URL: "https://webdav.example.com", Username: "u"}, false},
		{Storage{URL: "http://webdav.example.com"}, false},
		{Storage{URL: "webdav.example.com", Username: "u"}, true},
	}

	for _, tc := range cases {
		if got := tc.storage.SendsCredentialsInCleartext(); got != tc.want {
			t.Fatalf("SendsCredentialsInCleartext(%#v) = %v, want %v", tc.storage, got, tc.want)
		}
	}
}

func TestMKCOLClient_HasTimeout(t *testing.T) {
	if mkcolClient.Timeout != mkcolTimeout || mkcolTimeout <= 0 {
		t.Fatalf("expected mkcolClient to use a positive mkcolTimeout, got client timeout %v (mkcolTimeout %v)", mkcolClient.Timeout, mkcolTimeout)
	}
}

func TestStorage_EnsureCollectionTimesOutOnUnresponsiveServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	originalClient := mkcolClient
	mkcolClient = &http.Client{Timeout: 100 * time.Millisecond}
	defer func() { mkcolClient = originalClient }()

	s := Storage{URL: server.URL}

	start := time.Now()
	err := s.EnsureCollection("myrepo", fakeSecretStore{})
	if err == nil {
		t.Fatal("expected error from an unresponsive server")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("expected EnsureCollection to give up after the client timeout, took %v", elapsed)
	}
}
