package shared

import (
	"bufio"
	"reflect"
	"strings"
	"testing"
)

func TestPromptService_String_TrimsAnswer(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("  paperless  \n")))

	got, err := service.String("Container")
	if err != nil {
		t.Fatalf("String returned error: %v", err)
	}

	if got != "paperless" {
		t.Fatalf("expected %q, got %q", "paperless", got)
	}
}

func TestPromptService_String_PropagatesReadError(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("")))

	if _, err := service.String("Container"); err == nil {
		t.Fatal("expected error on empty input with no trailing newline, got nil")
	}
}

func TestPromptService_RequiredString_ReturnsFirstNonEmptyAnswer(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("\n  \nadguard\n")))

	got, err := service.RequiredString("Container")
	if err != nil {
		t.Fatalf("RequiredString returned error: %v", err)
	}

	if got != "adguard" {
		t.Fatalf("expected %q, got %q", "adguard", got)
	}
}

func TestPromptService_RequiredString_PropagatesReadError(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("")))

	if _, err := service.RequiredString("Container"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPromptService_StringWithDefault_UsesDefaultWhenInputIsEmpty(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("\n")))

	got, err := service.StringWithDefault("User", "appuser")
	if err != nil {
		t.Fatalf("StringWithDefault returned error: %v", err)
	}

	if got != "appuser" {
		t.Fatalf("expected %q, got %q", "appuser", got)
	}
}

func TestPromptService_StringWithDefault_ReturnsTrimmedAnswerWhenProvided(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader(" otheruser \n")))

	got, err := service.StringWithDefault("User", "appuser")
	if err != nil {
		t.Fatalf("StringWithDefault returned error: %v", err)
	}

	if got != "otheruser" {
		t.Fatalf("expected %q, got %q", "otheruser", got)
	}
}

func TestPromptService_StringWithDefault_PropagatesReadError(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("")))

	if _, err := service.StringWithDefault("User", "appuser"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPromptService_Bool_EmptyAnswerReturnsDefault(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("\n")))

	got, err := service.Bool("Remove container?", true)
	if err != nil {
		t.Fatalf("Bool returned error: %v", err)
	}

	if !got {
		t.Fatal("expected default value true, got false")
	}
}

func TestPromptService_Bool_RecognizesYesAndNoAnswers(t *testing.T) {
	tests := []struct {
		answer string
		want   bool
	}{
		{answer: "y\n", want: true},
		{answer: "YES\n", want: true},
		{answer: "1\n", want: true},
		{answer: "n\n", want: false},
		{answer: "NO\n", want: false},
		{answer: "0\n", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.answer, func(t *testing.T) {
			service := NewPromptService(bufio.NewReader(strings.NewReader(tt.answer)))

			got, err := service.Bool("Remove container?", false)
			if err != nil {
				t.Fatalf("Bool returned error: %v", err)
			}

			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestPromptService_Bool_RePromptsOnUnrecognizedAnswerThenAcceptsValidOne(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("maybe\nyes\n")))

	got, err := service.Bool("Remove container?", false)
	if err != nil {
		t.Fatalf("Bool returned error: %v", err)
	}

	if !got {
		t.Fatal("expected true after a valid answer, got false")
	}
}

func TestPromptService_Bool_PropagatesReadError(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("")))

	if _, err := service.Bool("Remove container?", false); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPromptService_StringList_PropagatesReadError(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("")))

	if _, err := service.StringList("Backup paths"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPromptService_StringListWithDefault_PropagatesReadError(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("")))

	if _, err := service.StringListWithDefault("Backup paths", nil); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPromptService_StringList_ParsesCommaSeparatedValues(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("paperless, adguard\n")))

	got, err := service.StringList("Backup paths")
	if err != nil {
		t.Fatalf("StringList returned error: %v", err)
	}

	want := []string{"paperless", "adguard"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestPromptService_StringList_EmptyReturnsNil(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("\n")))

	got, err := service.StringList("Backup paths")
	if err != nil {
		t.Fatalf("StringList returned error: %v", err)
	}

	if got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}
}

func TestPromptService_StringListWithDefault_UsesDefaultWhenInputIsEmpty(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("\n")))
	defaultValues := []string{"paperless", "adguard"}

	got, err := service.StringListWithDefault("Backup paths", defaultValues)
	if err != nil {
		t.Fatalf("StringListWithDefault returned error: %v", err)
	}

	if !reflect.DeepEqual(got, defaultValues) {
		t.Fatalf("expected %#v, got %#v", defaultValues, got)
	}
}

func TestPromptService_StringListWithDefault_NoneClearsValues(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("none\n")))

	got, err := service.StringListWithDefault("Backup paths", []string{"paperless"})
	if err != nil {
		t.Fatalf("StringListWithDefault returned error: %v", err)
	}

	if got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}
}

func TestPromptService_StringListWithDefault_ParsesProvidedValues(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("paperless, adguard\n")))

	got, err := service.StringListWithDefault("Backup paths", nil)
	if err != nil {
		t.Fatalf("StringListWithDefault returned error: %v", err)
	}

	want := []string{"paperless", "adguard"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestPromptService_StringListWithDefault_NoDefaultsShowsNoneLabel(t *testing.T) {
	service := NewPromptService(bufio.NewReader(strings.NewReader("\n")))

	got, err := service.StringListWithDefault("Backup paths", nil)
	if err != nil {
		t.Fatalf("StringListWithDefault returned error: %v", err)
	}

	if got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}
}

func TestParseStringList(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{name: "empty", value: "", want: nil},
		{name: "spaces only", value: "   ", want: nil},
		{name: "single value", value: "paperless", want: []string{"paperless"}},
		{name: "multiple values", value: "paperless, adguard", want: []string{"paperless", "adguard"}},
		{
			name:  "trims and skips empty values",
			value: " paperless, , adguard ,, vaultwarden ",
			want:  []string{"paperless", "adguard", "vaultwarden"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseStringList(tt.value)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("expected %#v, got %#v", tt.want, got)
			}
		})
	}
}
