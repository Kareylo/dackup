package defaultbackend

import "testing"

type recordingLogger struct {
	messages []string
}

func (logger *recordingLogger) Log(level string, message string) {
	logger.messages = append(logger.messages, level+": "+message)
}

func TestBackend_Name(t *testing.T) {
	backend := Backend{}

	if got := backend.Name(); got != Name {
		t.Fatalf("expected name %q, got %q", Name, got)
	}
}

func TestBackend_BackupIsNoOp(t *testing.T) {
	logger := &recordingLogger{}
	backend := Backend{Logger: logger}

	if err := backend.Backup("/staging"); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	if len(logger.messages) != 1 {
		t.Fatalf("expected one log message, got %#v", logger.messages)
	}
}

func TestBackend_RestoreIsNoOp(t *testing.T) {
	logger := &recordingLogger{}
	backend := Backend{Logger: logger}

	if err := backend.Restore("/staging"); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}

	if len(logger.messages) != 1 {
		t.Fatalf("expected one log message, got %#v", logger.messages)
	}
}

func TestBackend_NilLoggerDoesNotPanic(t *testing.T) {
	backend := Backend{}

	if err := backend.Backup("/staging"); err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}

	if err := backend.Restore("/staging"); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
}
