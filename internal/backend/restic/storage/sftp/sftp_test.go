package sftp

import (
	"strings"
	"testing"
)

type fakeSecretStore struct{}

func (fakeSecretStore) Encrypt(plaintext string) (string, error) { return "enc:" + plaintext, nil }
func (fakeSecretStore) Decrypt(ciphertext string) (string, error) {
	return ciphertext[len("enc:"):], nil
}

func TestStorage_Validate_RequiresFields(t *testing.T) {
	if err := (Storage{}).Validate(); err == nil {
		t.Fatal("expected error for empty storage")
	}

	valid := Storage{Host: "backup.example.com", Username: "dackup", Path: "/srv/backups"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestStorage_Validate_RequiresEachFieldIndividually(t *testing.T) {
	missingUsername := Storage{Host: "backup.example.com", Path: "/srv/backups"}
	if err := missingUsername.Validate(); err == nil {
		t.Fatal("expected error for missing username")
	}

	missingPath := Storage{Host: "backup.example.com", Username: "dackup"}
	if err := missingPath.Validate(); err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestStorage_BuildInvocation_DefaultPortNoArgs(t *testing.T) {
	s := Storage{Host: "backup.example.com", Username: "dackup", Path: "/srv/backups"}

	invocation, err := s.BuildInvocation("global", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	want := "sftp:dackup@backup.example.com:/srv/backups/global"
	if invocation.Repository != want {
		t.Fatalf("expected repository %q, got %q", want, invocation.Repository)
	}

	if len(invocation.Args) != 0 {
		t.Fatalf("expected no extra args for the default port, got %v", invocation.Args)
	}
}

func TestStorage_BuildInvocation_NonDefaultPortSetsSFTPCommand(t *testing.T) {
	s := Storage{Host: "backup.example.com", Port: 2222, Username: "dackup", Path: "/srv/backups"}

	invocation, err := s.BuildInvocation("global", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	if len(invocation.Args) != 2 || invocation.Args[0] != "-o" || !strings.Contains(invocation.Args[1], "-p 2222") {
		t.Fatalf("expected an sftp.command override with -p 2222, got %v", invocation.Args)
	}
}

func TestStorage_BuildInvocation_KeyfileSetsSFTPCommand(t *testing.T) {
	s := Storage{Host: "backup.example.com", Username: "dackup", Path: "/srv/backups", KeyfilePath: "/home/dackup/.ssh/id_ed25519"}

	invocation, err := s.BuildInvocation("global", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	if len(invocation.Args) != 2 || !strings.Contains(invocation.Args[1], "-i /home/dackup/.ssh/id_ed25519") {
		t.Fatalf("expected an sftp.command override with -i, got %v", invocation.Args)
	}
}

func TestStorage_BuildInvocation_KnownHostsSetsSFTPCommand(t *testing.T) {
	s := Storage{Host: "backup.example.com", Username: "dackup", Path: "/srv/backups", KnownHostsPath: "/tmp/known_hosts"}

	invocation, err := s.BuildInvocation("global", fakeSecretStore{})
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	if len(invocation.Args) != 2 || !strings.Contains(invocation.Args[1], "-o UserKnownHostsFile=/tmp/known_hosts") {
		t.Fatalf("expected an sftp.command override with UserKnownHostsFile, got %v", invocation.Args)
	}
}

func TestStorage_Validate_RejectsValuesThatWouldSplitOrInjectSSHArguments(t *testing.T) {
	cases := map[string]Storage{
		"host with space":             {Host: "backup.example.com -oProxyCommand=x", Username: "dackup", Path: "/srv"},
		"host starting with dash":     {Host: "-oProxyCommand=x", Username: "dackup", Path: "/srv"},
		"username with tab":           {Host: "backup.example.com", Username: "dack\tup", Path: "/srv"},
		"username starting with dash": {Host: "backup.example.com", Username: "-oProxyCommand=x", Path: "/srv"},
		"keyfile with space":          {Host: "backup.example.com", Username: "dackup", Path: "/srv", KeyfilePath: "/a b"},
		"keyfile with newline":        {Host: "backup.example.com", Username: "dackup", Path: "/srv", KeyfilePath: "/a\nb"},
		"keyfile starting with dash":  {Host: "backup.example.com", Username: "dackup", Path: "/srv", KeyfilePath: "-oProxyCommand=x"},
		"keyfile with quote":          {Host: "backup.example.com", Username: "dackup", Path: "/srv", KeyfilePath: `/a"b`},
		"known_hosts with space":      {Host: "backup.example.com", Username: "dackup", Path: "/srv", KnownHostsPath: "/a b"},
		"known_hosts with backslash":  {Host: "backup.example.com", Username: "dackup", Path: "/srv", KnownHostsPath: `/a\b`},
	}

	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			if err := s.Validate(); err == nil {
				t.Fatalf("expected an error for %+v", s)
			}
		})
	}
}

func TestStorage_Validate_ErrorNamesTheOffendingField(t *testing.T) {
	s := Storage{Host: "backup.example.com", Username: "dackup", Path: "/srv", KeyfilePath: "/a b"}

	err := s.Validate()
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "keyfile_path") {
		t.Fatalf("expected the error to name keyfile_path, got %v", err)
	}
}

func TestStorage_Validate_AcceptsOrdinaryKeyfileAndKnownHostsPaths(t *testing.T) {
	s := Storage{
		Host:           "backup.example.com",
		Username:       "dackup",
		Path:           "/srv/backups",
		KeyfilePath:    "/home/dackup/.ssh/id_ed25519",
		KnownHostsPath: "/home/dackup/.ssh/known_hosts",
	}

	if err := s.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
