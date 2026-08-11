package backend

import "testing"

type recordingLogger struct {
	messages []string
}

func (logger *recordingLogger) Log(level string, message string) {
	logger.messages = append(logger.messages, level+": "+message)
}

func TestDefaultBackend_Name(t *testing.T) {
	backend := DefaultBackend{}

	if got := backend.Name(); got != DefaultBackendName {
		t.Fatalf("expected name %q, got %q", DefaultBackendName, got)
	}
}

func TestDefaultBackend_BackupIsNoOp(t *testing.T) {
	logger := &recordingLogger{}
	backend := DefaultBackend{Logger: logger}

	if err := backend.Backup("/staging"); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	if len(logger.messages) != 1 {
		t.Fatalf("expected one log message, got %#v", logger.messages)
	}
}

func TestDefaultBackend_RestoreIsNoOp(t *testing.T) {
	logger := &recordingLogger{}
	backend := DefaultBackend{Logger: logger}

	if err := backend.Restore("/staging"); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}

	if len(logger.messages) != 1 {
		t.Fatalf("expected one log message, got %#v", logger.messages)
	}
}

func TestDefaultBackend_NilLoggerDoesNotPanic(t *testing.T) {
	backend := DefaultBackend{}

	if err := backend.Backup("/staging"); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	if err := backend.Restore("/staging"); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
}
