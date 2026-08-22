package shared

import (
	"bufio"
	"fmt"
	"strings"
)

// PromptService reads interactive terminal answers from Reader.
type PromptService struct {
	Reader *bufio.Reader
}

// NewPromptService returns a PromptService reading from reader.
func NewPromptService(reader *bufio.Reader) PromptService {
	return PromptService{
		Reader: reader,
	}
}

// RequiredString prompts for label, re-prompting until a non-empty answer
// is given.
func (service PromptService) RequiredString(label string) (string, error) {
	for {
		value, err := service.String(label)
		if err != nil {
			return "", err
		}

		if value != "" {
			return value, nil
		}

		fmt.Println("This value is required.")
	}
}

// String prompts for label and returns the trimmed answer, which may be
// empty.
func (service PromptService) String(label string) (string, error) {
	fmt.Printf("%s: ", label)

	value, err := service.Reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(value), nil
}

// StringWithDefault prompts for label, showing defaultValue and returning
// it unchanged when the answer is empty.
func (service PromptService) StringWithDefault(label string, defaultValue string) (string, error) {
	fmt.Printf("%s [%s]: ", label, defaultValue)

	value, err := service.Reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue, nil
	}

	return value, nil
}

// Bool prompts for a yes/no answer to label, showing defaultValue and
// re-prompting until a recognized answer (or an empty one, which returns
// defaultValue) is given.
func (service PromptService) Bool(label string, defaultValue bool) (bool, error) {
	defaultLabel := "y/N"
	if defaultValue {
		defaultLabel = "Y/n"
	}

	for {
		fmt.Printf("%s [%s]: ", label, defaultLabel)

		value, err := service.Reader.ReadString('\n')
		if err != nil {
			return false, err
		}

		value = strings.ToLower(strings.TrimSpace(value))

		if value == "" {
			return defaultValue, nil
		}

		switch value {
		case "y", "yes", "true", "1":
			return true, nil
		case "n", "no", "false", "0":
			return false, nil
		default:
			fmt.Println("Please answer yes or no.")
		}
	}
}

// StringList prompts for label and parses the answer as a comma-separated
// list via ParseStringList.
func (service PromptService) StringList(label string) ([]string, error) {
	value, err := service.String(label)
	if err != nil {
		return nil, err
	}

	return ParseStringList(value), nil
}

// StringListWithDefault prompts for label, showing defaultValues and
// returning them unchanged when the answer is empty. Answering "none"
// (case-insensitive) clears the list.
func (service PromptService) StringListWithDefault(label string, defaultValues []string) ([]string, error) {
	defaultLabel := "none"
	if len(defaultValues) > 0 {
		defaultLabel = strings.Join(defaultValues, ", ")
	}

	fmt.Printf("%s [%s]: ", label, defaultLabel)

	value, err := service.Reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValues, nil
	}

	if strings.EqualFold(value, "none") {
		return nil, nil
	}

	return ParseStringList(value), nil
}

// ParseStringList splits value on commas, trims each item, and drops empty
// ones. An all-whitespace or empty value returns nil.
func ParseStringList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))

	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}

		items = append(items, item)
	}

	return items
}
