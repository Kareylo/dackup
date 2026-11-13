package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// writeConfigWithMode writes content to a new file under t.TempDir() and
// chmods it to mode explicitly, so the result doesn't depend on the
// process umask.
func writeConfigWithMode(t *testing.T, name string, content string, mode os.FileMode) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}

	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("failed to chmod %s: %v", path, err)
	}

	return path
}

func TestReadDackupConfig_AcceptsFileNotWritableByOthers(t *testing.T) {
	for _, mode := range []os.FileMode{0o600, 0o640, 0o644, 0o400} {
		t.Run(fmt.Sprintf("%#o", mode), func(t *testing.T) {
			path := writeConfigWithMode(t, "config.json", `{"user":"dackup"}`, mode)

			config, err := ReadDackupConfig(path)
			if err != nil {
				t.Fatalf("expected mode %#o to be accepted, got error: %v", mode, err)
			}

			if config.User != "dackup" {
				t.Fatalf("expected user %q, got %q", "dackup", config.User)
			}
		})
	}
}

func TestReadDackupConfig_RejectsFileWritableByGroupOrOthers(t *testing.T) {
	for _, mode := range []os.FileMode{0o620, 0o602, 0o664, 0o666, 0o777} {
		t.Run(fmt.Sprintf("%#o", mode), func(t *testing.T) {
			path := writeConfigWithMode(t, "config.json", `{"user":"dackup"}`, mode)

			_, err := ReadDackupConfig(path)
			if err == nil {
				t.Fatalf("expected mode %#o to be rejected", mode)
			}

			for _, want := range []string{path, fmt.Sprintf("%o", mode), "chmod 600"} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("expected error to mention %q, got: %v", want, err)
				}
			}
		})
	}
}

func TestReadDackupConfig_ChecksTrustBeforeParsing(t *testing.T) {
	path := writeConfigWithMode(t, "config.json", "{not valid json", 0o666)

	_, err := ReadDackupConfig(path)
	if err == nil {
		t.Fatal("expected an error")
	}

	if strings.Contains(err.Error(), "failed to parse") {
		t.Fatalf("expected the permission check to fail before parsing, got: %v", err)
	}
}

func TestEffectiveDackupConfig_RejectsWritableContainersConfigFile(t *testing.T) {
	containersPath := writeConfigWithMode(t, "containers.json", `{"containers":[{"container":"app"}]}`, 0o666)
	mainPath := writeConfigWithMode(t, "config.json", fmt.Sprintf(`{"user":"dackup","config_file":%q}`, containersPath), 0o600)

	_, _, err := EffectiveDackupConfig(mainPath)
	if err == nil {
		t.Fatal("expected a group/other-writable config_file target to be rejected")
	}

	if !strings.Contains(err.Error(), containersPath) {
		t.Fatalf("expected error to name %s, got: %v", containersPath, err)
	}
}

// fakeConfigFileInfo is an os.FileInfo with a controllable mode and owner
// uid, since a test can't create a file owned by another user without
// running as root.
type fakeConfigFileInfo struct {
	mode os.FileMode
	sys  any
}

func (info fakeConfigFileInfo) Name() string       { return "config.json" }
func (info fakeConfigFileInfo) Size() int64        { return 0 }
func (info fakeConfigFileInfo) Mode() os.FileMode  { return info.mode }
func (info fakeConfigFileInfo) ModTime() time.Time { return time.Time{} }
func (info fakeConfigFileInfo) IsDir() bool        { return false }
func (info fakeConfigFileInfo) Sys() any           { return info.sys }

func TestCheckConfigFileTrust_AcceptsFileOwnedByCurrentUser(t *testing.T) {
	info := fakeConfigFileInfo{mode: 0o600, sys: &syscall.Stat_t{Uid: uint32(os.Geteuid())}}

	if err := checkConfigFileTrust("/etc/dackup/config.json", info); err != nil {
		t.Fatalf("expected a file owned by the current user to be accepted, got: %v", err)
	}
}

func TestCheckConfigFileTrust_AcceptsFileOwnedByRoot(t *testing.T) {
	info := fakeConfigFileInfo{mode: 0o644, sys: &syscall.Stat_t{Uid: 0}}

	if err := checkConfigFileTrust("/etc/dackup/config.json", info); err != nil {
		t.Fatalf("expected a root-owned file to be accepted, got: %v", err)
	}
}

func TestCheckConfigFileTrust_RejectsFileOwnedByAnotherUser(t *testing.T) {
	otherUID := uint32(os.Geteuid()) + 4242
	info := fakeConfigFileInfo{mode: 0o600, sys: &syscall.Stat_t{Uid: otherUID}}

	err := checkConfigFileTrust("/etc/dackup/config.json", info)
	if err == nil {
		t.Fatalf("expected a file owned by uid %d to be rejected", otherUID)
	}

	for _, want := range []string{"/etc/dackup/config.json", fmt.Sprintf("%d", otherUID)} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to mention %q, got: %v", want, err)
		}
	}
}

func TestCheckConfigFileTrust_ChecksModeWhenOwnerIsUnknown(t *testing.T) {
	if err := checkConfigFileTrust("/etc/dackup/config.json", fakeConfigFileInfo{mode: 0o600}); err != nil {
		t.Fatalf("expected an owner-unknown 0600 file to be accepted, got: %v", err)
	}

	if err := checkConfigFileTrust("/etc/dackup/config.json", fakeConfigFileInfo{mode: 0o666}); err == nil {
		t.Fatal("expected an owner-unknown 0666 file to still be rejected on mode")
	}
}
