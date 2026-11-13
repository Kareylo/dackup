package sftp

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

func TestStorage_ValidateRequiresExactlyOneAuthMethod(t *testing.T) {
	base := Storage{Host: "h", Username: "u", Path: "/p", KnownHostsPath: "/known_hosts"}

	if err := base.Validate(); err == nil {
		t.Fatal("expected error when neither keyfile_path nor encrypted_password is set")
	}

	withKeyfile := base
	withKeyfile.KeyfilePath = "/key"
	if err := withKeyfile.Validate(); err != nil {
		t.Fatalf("expected no error with only keyfile_path set, got %v", err)
	}

	withPassword := base
	withPassword.EncryptedPassword = "enc:pw"
	if err := withPassword.Validate(); err != nil {
		t.Fatalf("expected no error with only encrypted_password set, got %v", err)
	}

	withBoth := base
	withBoth.KeyfilePath = "/key"
	withBoth.EncryptedPassword = "enc:pw"
	if err := withBoth.Validate(); err == nil {
		t.Fatal("expected error when both keyfile_path and encrypted_password are set")
	}
}

func TestStorage_ValidateRequiresHostUsernamePath(t *testing.T) {
	if err := (Storage{}).Validate(); err == nil {
		t.Fatal("expected error for empty Storage")
	}
	if err := (Storage{Host: "h"}).Validate(); err == nil {
		t.Fatal("expected error when username is missing")
	}
	if err := (Storage{Host: "h", Username: "u"}).Validate(); err == nil {
		t.Fatal("expected error when path is missing")
	}
}

// Kopia refuses to connect over SFTP without --known-hosts or
// --known-hosts-data, so a config without known_hosts_path must fail
// Validate (at "dackup backend create" and before any container stops)
// rather than at the first backup.
func TestStorage_ValidateRequiresKnownHostsPath(t *testing.T) {
	base := Storage{Host: "h", Username: "u", Path: "/p", KeyfilePath: "/key"}

	for _, knownHostsPath := range []string{"", "   "} {
		storage := base
		storage.KnownHostsPath = knownHostsPath

		err := storage.Validate()
		if err == nil {
			t.Fatalf("expected an error for known_hosts_path %q", knownHostsPath)
		}

		if !strings.Contains(err.Error(), "known_hosts_path") {
			t.Fatalf("expected the error to name known_hosts_path, got %v", err)
		}
	}

	withKnownHosts := base
	withKnownHosts.KnownHostsPath = "/home/dackup/.ssh/known_hosts"
	if err := withKnownHosts.Validate(); err != nil {
		t.Fatalf("expected no error with known_hosts_path set, got %v", err)
	}
}

func TestStorage_BuildInvocationWithKeyfile(t *testing.T) {
	s := Storage{
		Host:        "backup.example.com",
		Username:    "dackup",
		Path:        "/srv/backups",
		KeyfilePath: "/home/dackup/.ssh/id_ed25519",
	}

	invocation, err := s.BuildInvocation("myrepo", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	wantArgs := []string{"--host=backup.example.com", "--port=22", "--username=dackup", "--path=/srv/backups/myrepo", "--keyfile=/home/dackup/.ssh/id_ed25519"}
	if !equalArgs(invocation.Args, wantArgs) {
		t.Fatalf("expected args %v, got %v", wantArgs, invocation.Args)
	}
}

func TestStorage_BuildInvocationWithPassword(t *testing.T) {
	s := Storage{
		Host:              "backup.example.com",
		Port:              2222,
		Username:          "dackup",
		Path:              "/srv/backups",
		EncryptedPassword: "enc:hunter2",
		KnownHostsPath:    "/home/dackup/.ssh/known_hosts",
	}

	invocation, err := s.BuildInvocation("myrepo", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	wantArgs := []string{"--host=backup.example.com", "--port=2222", "--username=dackup", "--path=/srv/backups/myrepo", "--known-hosts=/home/dackup/.ssh/known_hosts", "--sftp-password=hunter2"}
	if !equalArgs(invocation.Args, wantArgs) {
		t.Fatalf("expected args %v, got %v", wantArgs, invocation.Args)
	}
}

func TestStorage_UsesPasswordAuth(t *testing.T) {
	cases := []struct {
		name    string
		storage Storage
		want    bool
	}{
		{"password only", Storage{EncryptedPassword: "enc:hunter2"}, true},
		{"keyfile only", Storage{KeyfilePath: "/home/dackup/.ssh/id_ed25519"}, false},
		{"keyfile wins over a leftover password", Storage{KeyfilePath: "/home/dackup/.ssh/id_ed25519", EncryptedPassword: "enc:hunter2"}, false},
		{"neither set", Storage{}, false},
		{"whitespace-only password", Storage{EncryptedPassword: "   "}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.storage.UsesPasswordAuth(); got != tc.want {
				t.Fatalf("UsesPasswordAuth() = %v, want %v", got, tc.want)
			}
		})
	}
}
