package borg

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// exitCodeError stands in for *exec.ExitError: borg's exit code is read
// through an ExitCode() method, which *exec.ExitError provides.
type exitCodeError struct {
	code int
}

func (e exitCodeError) Error() string { return fmt.Sprintf("exit status %d", e.code) }
func (e exitCodeError) ExitCode() int { return e.code }

// borgWarnings returns the WARN lines about borg finishing with warnings.
func borgWarnings(logger *fakeLogger) []string {
	var warnings []string
	for _, message := range logger.messages {
		if strings.HasPrefix(message, "WARN: ") && strings.Contains(message, "finished with warnings") {
			warnings = append(warnings, message)
		}
	}

	return warnings
}

// restoreRunner returns a runner whose "borg list" finds one archive and
// whose RunInDirWithEnv (the extract call) returns runErr.
func restoreRunner(runErr error) *fakeEnvRunner {
	return &fakeEnvRunner{
		runErr: runErr,
		outputs: map[string][]byte{
			"list /repos/global --last 1 --short": []byte("20260810-020000\n"),
		},
	}
}

func TestIsBorgWarning(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"exit code 1", exitCodeError{1}, true},
		{"exit code 1, wrapped", fmt.Errorf("%w: borg output", exitCodeError{1}), true},
		{"exit code 2", exitCodeError{2}, false},
		{"killed by signal", exitCodeError{137}, false},
		{"no exit code", errors.New("borg: executable file not found in $PATH"), false},
		{"nil", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isBorgWarning(tc.err); got != tc.want {
				t.Fatalf("isBorgWarning(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsBorgWarning_RealProcessExitCodes(t *testing.T) {
	warning := exec.Command("sh", "-c", "exit 1").Run()
	if !isBorgWarning(warning) {
		t.Fatalf("expected a real exit status 1 to count as a borg warning, got %v", warning)
	}

	failure := exec.Command("sh", "-c", "exit 2").Run()
	if isBorgWarning(failure) {
		t.Fatalf("expected a real exit status 2 to count as an error, got %v", failure)
	}
}

func TestBackend_Backup_CreateWarningIsLoggedNotFailed(t *testing.T) {
	logger := &fakeLogger{}
	backend := testBackend(&fakeEnvRunner{runErr: exitCodeError{1}}, newFakeFileSystem("/repos/global/config"), logger)
	backend.Config.EncryptedPassphrase = "enc:hunter2"

	if err := backend.Backup("/staging"); err != nil {
		t.Fatalf("expected borg exit code 1 on create to be a warning, got error: %v", err)
	}

	warnings := borgWarnings(logger)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "/repos/global::") {
		t.Fatalf("expected one warning naming the archive, got %v", logger.messages)
	}
}

func TestBackend_Backup_CreateErrorStillFails(t *testing.T) {
	for _, runErr := range []error{exitCodeError{2}, errors.New("exec: borg not found")} {
		t.Run(runErr.Error(), func(t *testing.T) {
			backend := testBackend(&fakeEnvRunner{runErr: runErr}, newFakeFileSystem("/repos/global/config"), &fakeLogger{})
			backend.Config.EncryptedPassphrase = "enc:hunter2"

			if err := backend.Backup("/staging"); err == nil {
				t.Fatalf("expected %v on create to fail the backup", runErr)
			}
		})
	}
}

func TestBackend_Restore_ExtractWarningIsLoggedNotFailed(t *testing.T) {
	logger := &fakeLogger{}
	backend := testBackend(restoreRunner(exitCodeError{1}), newFakeFileSystem("/repos/global/config"), logger)
	backend.Config.EncryptedPassphrase = "enc:hunter2"

	if err := backend.Restore("/staging"); err != nil {
		t.Fatalf("expected borg exit code 1 on extract to be a warning, got error: %v", err)
	}

	warnings := borgWarnings(logger)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "/repos/global::20260810-020000") {
		t.Fatalf("expected one warning naming the archive, got %v", logger.messages)
	}
}

func TestBackend_Restore_ExtractErrorStillFails(t *testing.T) {
	for _, runErr := range []error{exitCodeError{2}, errors.New("exec: borg not found")} {
		t.Run(runErr.Error(), func(t *testing.T) {
			backend := testBackend(restoreRunner(runErr), newFakeFileSystem("/repos/global/config"), &fakeLogger{})
			backend.Config.EncryptedPassphrase = "enc:hunter2"

			if err := backend.Restore("/staging"); err == nil {
				t.Fatalf("expected %v on extract to fail the restore", runErr)
			}
		})
	}
}

func TestBackend_Backup_InitWarningStillFails(t *testing.T) {
	// A repository that borg init only half-created is not safe to write
	// to, so init keeps treating any non-zero exit as an error.
	backend := testBackend(&fakeEnvRunner{runErr: exitCodeError{1}}, newFakeFileSystem(), &fakeLogger{})
	backend.Config.EncryptedPassphrase = "enc:hunter2"

	if err := backend.Backup("/staging"); err == nil {
		t.Fatal("expected exit code 1 on borg init to still fail the backup")
	}
}
