package shared

import (
	"bufio"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Raw key sequences a terminal sends in raw mode.
const (
	keyUp    = "\x1b[A"
	keyDown  = "\x1b[B"
	keyEnter = "\r"
	keySpace = " "
	keyCtrlC = "\x03"
)

type fakeTerminal struct {
	rawCalls     int
	restoreCalls int
}

func (terminal *fakeTerminal) MakeRaw() (func() error, error) {
	terminal.rawCalls++

	return func() error {
		terminal.restoreCalls++
		return nil
	}, nil
}

func newInteractivePromptService(input string) (PromptService, *fakeTerminal) {
	terminal := &fakeTerminal{}
	service := NewPromptService(bufio.NewReader(strings.NewReader(input)))
	service.Terminal = terminal

	return service, terminal
}

var backendOptions = []string{"borg", "kopia", "restic"}

// --- Text fallback (no Terminal: pipes, scripts, tests) ---

func TestPromptService_SelectOne_Text_AcceptsNumber(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("2\n")))

	got, err := service.SelectOne("Backend", backendOptions, "")
	if err != nil {
		t.Fatalf("SelectOne returned error: %v", err)
	}

	if got != "kopia" {
		t.Fatalf("expected %q, got %q", "kopia", got)
	}
}

func TestPromptService_SelectOne_Text_AcceptsName(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader(" restic \n")))

	got, err := service.SelectOne("Backend", backendOptions, "")
	if err != nil {
		t.Fatalf("SelectOne returned error: %v", err)
	}

	if got != "restic" {
		t.Fatalf("expected %q, got %q", "restic", got)
	}
}

func TestPromptService_SelectOne_Text_EmptyKeepsDefault(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("\n")))

	got, err := service.SelectOne("Backend", backendOptions, "kopia")
	if err != nil {
		t.Fatalf("SelectOne returned error: %v", err)
	}

	if got != "kopia" {
		t.Fatalf("expected %q, got %q", "kopia", got)
	}
}

func TestPromptService_SelectOne_Text_RepromptsUntilValid(t *testing.T) {
	// empty with no default, out-of-range number, unknown name, then valid.
	service := NewPromptService(bufio.NewReader(strings.NewReader("\n9\ndropbox\n1\n")))

	got, err := service.SelectOne("Backend", backendOptions, "")
	if err != nil {
		t.Fatalf("SelectOne returned error: %v", err)
	}

	if got != "borg" {
		t.Fatalf("expected %q, got %q", "borg", got)
	}
}

func TestPromptService_SelectOne_Text_PropagatesReadError(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("")))

	if _, err := service.SelectOne("Backend", backendOptions, ""); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPromptService_SelectMany_Text_AcceptsNumbersAndNamesInOptionOrder(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("restic, 1\n")))

	got, err := service.SelectMany("Contains", backendOptions, nil)
	if err != nil {
		t.Fatalf("SelectMany returned error: %v", err)
	}

	want := []string{"borg", "restic"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestPromptService_SelectMany_Text_EmptyKeepsDefaults(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("\n")))

	got, err := service.SelectMany("Contains", backendOptions, []string{"kopia"})
	if err != nil {
		t.Fatalf("SelectMany returned error: %v", err)
	}

	if !reflect.DeepEqual(got, []string{"kopia"}) {
		t.Fatalf("expected %#v, got %#v", []string{"kopia"}, got)
	}
}

func TestPromptService_SelectMany_Text_NoneClearsSelection(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("NONE\n")))

	got, err := service.SelectMany("Contains", backendOptions, []string{"kopia"})
	if err != nil {
		t.Fatalf("SelectMany returned error: %v", err)
	}

	if got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}
}

func TestPromptService_SelectMany_Text_RepromptsOnUnknownItem(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("borg, dropbox\n2\n")))

	got, err := service.SelectMany("Contains", backendOptions, nil)
	if err != nil {
		t.Fatalf("SelectMany returned error: %v", err)
	}

	if !reflect.DeepEqual(got, []string{"kopia"}) {
		t.Fatalf("expected %#v, got %#v", []string{"kopia"}, got)
	}
}

// --- Interactive checkboxes (Terminal set: arrow keys, space, enter) ---

func TestPromptService_SelectOne_Interactive_EnterPicksCursor(t *testing.T) {
	service, terminal := newInteractivePromptService(keyDown + keyDown + keyEnter)

	got, err := service.SelectOne("Backend", backendOptions, "")
	if err != nil {
		t.Fatalf("SelectOne returned error: %v", err)
	}

	if got != "restic" {
		t.Fatalf("expected %q, got %q", "restic", got)
	}

	if terminal.rawCalls != 1 || terminal.restoreCalls != 1 {
		t.Fatalf("expected raw mode entered and restored once, got %d/%d", terminal.rawCalls, terminal.restoreCalls)
	}
}

func TestPromptService_SelectOne_Interactive_CursorStartsOnDefault(t *testing.T) {
	service, _ := newInteractivePromptService(keyEnter)

	got, err := service.SelectOne("Backend", backendOptions, "kopia")
	if err != nil {
		t.Fatalf("SelectOne returned error: %v", err)
	}

	if got != "kopia" {
		t.Fatalf("expected %q, got %q", "kopia", got)
	}
}

func TestPromptService_SelectOne_Interactive_CursorWrapsAround(t *testing.T) {
	service, _ := newInteractivePromptService(keyUp + keyEnter)

	got, err := service.SelectOne("Backend", backendOptions, "")
	if err != nil {
		t.Fatalf("SelectOne returned error: %v", err)
	}

	if got != "restic" {
		t.Fatalf("expected up from the first option to wrap to %q, got %q", "restic", got)
	}
}

func TestPromptService_SelectOne_Interactive_SupportsVimKeys(t *testing.T) {
	service, _ := newInteractivePromptService("jjk" + keyEnter)

	got, err := service.SelectOne("Backend", backendOptions, "")
	if err != nil {
		t.Fatalf("SelectOne returned error: %v", err)
	}

	if got != "kopia" {
		t.Fatalf("expected %q, got %q", "kopia", got)
	}
}

func TestPromptService_SelectMany_Interactive_SpaceTogglesAndEnterConfirms(t *testing.T) {
	// tick borg, move to restic, tick it, move back to borg, untick it.
	input := keySpace + keyDown + keyDown + keySpace + keyDown + keySpace + keyEnter
	service, terminal := newInteractivePromptService(input)

	got, err := service.SelectMany("Contains", backendOptions, nil)
	if err != nil {
		t.Fatalf("SelectMany returned error: %v", err)
	}

	if !reflect.DeepEqual(got, []string{"restic"}) {
		t.Fatalf("expected %#v, got %#v", []string{"restic"}, got)
	}

	if terminal.restoreCalls != 1 {
		t.Fatalf("expected terminal restored once, got %d", terminal.restoreCalls)
	}
}

func TestPromptService_SelectMany_Interactive_DefaultsArePreTicked(t *testing.T) {
	service, _ := newInteractivePromptService(keyEnter)

	got, err := service.SelectMany("Contains", backendOptions, []string{"restic", "borg"})
	if err != nil {
		t.Fatalf("SelectMany returned error: %v", err)
	}

	want := []string{"borg", "restic"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestPromptService_SelectMany_Interactive_NothingTickedReturnsNil(t *testing.T) {
	service, _ := newInteractivePromptService(keyEnter)

	got, err := service.SelectMany("Contains", backendOptions, nil)
	if err != nil {
		t.Fatalf("SelectMany returned error: %v", err)
	}

	if got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}
}

func TestPromptService_Select_Interactive_CtrlCAbortsAndRestoresTerminal(t *testing.T) {
	service, terminal := newInteractivePromptService(keyDown + keyCtrlC)

	_, err := service.SelectOne("Backend", backendOptions, "")
	if !errors.Is(err, ErrPromptInterrupted) {
		t.Fatalf("expected ErrPromptInterrupted, got %v", err)
	}

	if terminal.restoreCalls != 1 {
		t.Fatalf("expected terminal restored once, got %d", terminal.restoreCalls)
	}
}

func TestPromptService_Select_Interactive_ReadErrorRestoresTerminal(t *testing.T) {
	service, terminal := newInteractivePromptService(keyDown)

	if _, err := service.SelectMany("Contains", backendOptions, nil); err == nil {
		t.Fatal("expected error when input ends before enter, got nil")
	}

	if terminal.restoreCalls != 1 {
		t.Fatalf("expected terminal restored once, got %d", terminal.restoreCalls)
	}
}

func TestPromptService_Bool_Interactive_EnterKeepsDefault(t *testing.T) {
	for _, defaultValue := range []bool{true, false} {
		service, terminal := newInteractivePromptService(keyEnter)

		got, err := service.Bool("Create it?", defaultValue)
		if err != nil {
			t.Fatalf("Bool returned error: %v", err)
		}

		if got != defaultValue {
			t.Fatalf("expected default %t, got %t", defaultValue, got)
		}

		if terminal.rawCalls != 1 || terminal.restoreCalls != 1 {
			t.Fatalf("expected raw mode entered and restored once, got %d/%d", terminal.rawCalls, terminal.restoreCalls)
		}
	}
}

func TestPromptService_Bool_Interactive_MovingSelectsOtherAnswer(t *testing.T) {
	service, _ := newInteractivePromptService(keyDown + keyEnter)

	got, err := service.Bool("Create it?", true)
	if err != nil {
		t.Fatalf("Bool returned error: %v", err)
	}

	if got {
		t.Fatal("expected moving from Yes to No to return false")
	}
}

func TestPromptService_Bool_Interactive_CtrlCAborts(t *testing.T) {
	service, _ := newInteractivePromptService(keyCtrlC)

	if _, err := service.Bool("Create it?", true); !errors.Is(err, ErrPromptInterrupted) {
		t.Fatalf("expected ErrPromptInterrupted, got %v", err)
	}
}

type failingTerminal struct{}

func (failingTerminal) MakeRaw() (func() error, error) {
	return nil, errors.New("raw mode unavailable")
}

func TestPromptService_SelectOne_NoOptionsReturnsError(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("1\n")))

	if _, err := service.SelectOne("Backend", nil, ""); err == nil {
		t.Fatal("expected error when there is nothing to choose from, got nil")
	}
}

func TestPromptService_SelectMany_Text_PropagatesReadError(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("")))

	if _, err := service.SelectMany("Contains", backendOptions, nil); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPromptService_Select_Interactive_PropagatesMakeRawError(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader(keyEnter)))
	service.Terminal = failingTerminal{}

	if _, err := service.SelectOne("Backend", backendOptions, ""); err == nil {
		t.Fatal("expected raw mode error, got nil")
	}
}

func TestPromptService_Select_Interactive_IgnoresUnknownKeys(t *testing.T) {
	// "x" and the right arrow are ignored; the cursor stays on borg.
	service, _ := newInteractivePromptService("x\x1b[C" + keyEnter)

	got, err := service.SelectOne("Backend", backendOptions, "")
	if err != nil {
		t.Fatalf("SelectOne returned error: %v", err)
	}

	if got != "borg" {
		t.Fatalf("expected %q, got %q", "borg", got)
	}
}

func TestPromptService_Select_Interactive_TruncatedEscapeSequenceReturnsError(t *testing.T) {
	service, terminal := newInteractivePromptService("\x1b[")

	if _, err := service.SelectOne("Backend", backendOptions, ""); err == nil {
		t.Fatal("expected error on a truncated escape sequence, got nil")
	}

	if terminal.restoreCalls != 1 {
		t.Fatalf("expected terminal restored once, got %d", terminal.restoreCalls)
	}
}

func TestFileTerminal_MakeRaw_FailsOnNonTerminal(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "not-a-terminal")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer file.Close()

	if _, err := (fileTerminal{fd: int(file.Fd())}).MakeRaw(); err == nil {
		t.Fatal("expected MakeRaw on a regular file to fail, got nil")
	}
}

func TestStdinTerminal_ReturnsNilWhenStdinIsNotATerminal(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer file.Close()

	withStdio(t, file, file)

	if terminal := StdinTerminal(); terminal != nil {
		t.Fatalf("expected nil terminal, got %#v", terminal)
	}
}

// withStdio swaps os.Stdin/os.Stdout for the duration of the test.
func withStdio(t *testing.T, stdin *os.File, stdout *os.File) {
	t.Helper()

	originalStdin, originalStdout := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = stdin, stdout

	t.Cleanup(func() {
		os.Stdin, os.Stdout = originalStdin, originalStdout
	})
}

// --- Secret ---

func TestSecret_WithoutTerminalReadsTrimmedLine(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader(" hunter2 \n")))

	got, err := service.Secret("Password")
	if err != nil {
		t.Fatalf("Secret returned error: %v", err)
	}
	if got != "hunter2" {
		t.Fatalf("expected %q, got %q", "hunter2", got)
	}
}

func TestSecret_WithTerminalDoesNotEchoInput(t *testing.T) {
	service, terminal := newInteractivePromptService("hunter2\r")

	var got string
	output := captureStdout(t, func() {
		var err error
		got, err = service.Secret("Password")
		if err != nil {
			t.Fatalf("Secret returned error: %v", err)
		}
	})

	if got != "hunter2" {
		t.Fatalf("expected %q, got %q", "hunter2", got)
	}
	if strings.Contains(output, "hunter2") {
		t.Fatalf("expected the secret not to be echoed, got output %q", output)
	}
	if !strings.Contains(output, "Password") {
		t.Fatalf("expected the label to be printed, got output %q", output)
	}
	if terminal.rawCalls != 1 || terminal.restoreCalls != 1 {
		t.Fatalf("expected raw mode entered and restored once, got %d/%d", terminal.rawCalls, terminal.restoreCalls)
	}
}

func TestSecret_WithTerminalHandlesLineEditing(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{"newline ends input", "abc\n", "abc"},
		{"DEL removes last char", "hunterx\x7f2\r", "hunter2"},
		{"BS removes last char", "hunterx\x082\r", "hunter2"},
		{"backspace on empty is a no-op", "\x7fab\r", "ab"},
		{"backspace removes a whole multibyte rune", "pé\x7f\r", "p"},
		{"arrow keys are ignored", "a\x1b[Ab\x1b[Dc\r", "abc"},
		{"surrounding spaces are trimmed like String", " ab \r", "ab"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			service, _ := newInteractivePromptService(testCase.input)

			var got string
			captureStdout(t, func() {
				var err error
				got, err = service.Secret("Password")
				if err != nil {
					t.Fatalf("Secret returned error: %v", err)
				}
			})

			if got != testCase.want {
				t.Fatalf("expected %q, got %q", testCase.want, got)
			}
		})
	}
}

func TestSecret_WithTerminalCtrlCInterruptsAndRestores(t *testing.T) {
	service, terminal := newInteractivePromptService("hun\x03")

	var err error
	captureStdout(t, func() {
		_, err = service.Secret("Password")
	})

	if !errors.Is(err, ErrPromptInterrupted) {
		t.Fatalf("expected ErrPromptInterrupted, got %v", err)
	}
	if terminal.restoreCalls != 1 {
		t.Fatalf("expected terminal restored once, got %d", terminal.restoreCalls)
	}
}

func TestSecret_WithTerminalEOFBeforeEnterReturnsErrorAndRestores(t *testing.T) {
	service, terminal := newInteractivePromptService("hunter2")

	var err error
	captureStdout(t, func() {
		_, err = service.Secret("Password")
	})

	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF, got %v", err)
	}
	if terminal.restoreCalls != 1 {
		t.Fatalf("expected terminal restored once, got %d", terminal.restoreCalls)
	}
}

func TestSecret_RawModeFailureReturnsError(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("hunter2\r")))
	service.Terminal = failingTerminal{}

	var err error
	captureStdout(t, func() {
		_, err = service.Secret("Password")
	})

	if err == nil {
		t.Fatal("expected an error when raw mode is unavailable")
	}
}
