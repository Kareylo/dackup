package shared

import (
	"bufio"
	"reflect"
	"strings"
	"testing"
)

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
