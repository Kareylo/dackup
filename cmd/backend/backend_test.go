package backend

import (
	"bufio"
	"dackup/internal/shared"
	"strings"
	"testing"
)

func newTestService(input string) commandService {
	return commandService{
		options: &shared.Options{},
		prompt:  shared.NewPromptService(bufio.NewReader(strings.NewReader(input))),
	}
}

func TestConfigureBackend_NoBackendsImplementedYet(t *testing.T) {
	service := newTestService("")

	config, configured, err := service.configureBackend(shared.DackupConfig{User: "test-user"})
	if err != nil {
		t.Fatalf("configureBackend returned error: %v", err)
	}

	if configured {
		t.Fatal("expected configured to be false when no backends are registered")
	}

	if config.Backend != "" {
		t.Fatalf("expected backend to remain unset, got %q", config.Backend)
	}
}

func TestSelectBackendName_RejectsUnknownThenAcceptsValid(t *testing.T) {
	service := newTestService("bogus\nborg\n")

	got, err := service.selectBackendName([]string{"borg"})
	if err != nil {
		t.Fatalf("selectBackendName returned error: %v", err)
	}

	if got != "borg" {
		t.Fatalf("expected %q, got %q", "borg", got)
	}
}

func TestPromptBackendSettings_UnknownBackendReturnsNil(t *testing.T) {
	service := newTestService("")

	got, err := service.promptBackendSettings("borg")
	if err != nil {
		t.Fatalf("promptBackendSettings returned error: %v", err)
	}

	if got != nil {
		t.Fatalf("expected nil settings, got %#v", got)
	}
}

func TestPrintBackend_NoPanicWhenUnset(t *testing.T) {
	printBackend(shared.DackupConfig{})
}

func TestPrintBackend_NoPanicWhenConfigured(t *testing.T) {
	printBackend(shared.DackupConfig{
		Backend:         "borg",
		BackendSettings: []byte(`{"repository":"/backups/repo"}`),
	})
}
