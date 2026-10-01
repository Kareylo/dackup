package shared

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// ErrPromptInterrupted is returned when the user aborts an interactive
// selection with Ctrl-C.
var ErrPromptInterrupted = errors.New("prompt interrupted")

// Terminal switches the input terminal into raw mode so a selection prompt
// can read single key presses. MakeRaw returns the function restoring the
// previous mode.
type Terminal interface {
	MakeRaw() (func() error, error)
}

type fileTerminal struct {
	fd int
}

func (terminal fileTerminal) MakeRaw() (func() error, error) {
	state, err := term.MakeRaw(terminal.fd)
	if err != nil {
		return nil, err
	}

	return func() error {
		return term.Restore(terminal.fd, state)
	}, nil
}

// StdinTerminal returns a Terminal for os.Stdin, or nil when stdin or
// stdout is not a terminal, in which case selection prompts fall back to
// typed answers.
func StdinTerminal() Terminal {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil
	}

	return fileTerminal{fd: fd}
}

// SelectOne prompts for exactly one of options, pre-selecting defaultValue
// when it is one of them. With a Terminal it shows a checkbox list driven by
// the arrow keys; otherwise it accepts an option's number or name.
func (service PromptService) SelectOne(label string, options []string, defaultValue string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("%s: no options to choose from", label)
	}

	if !containsOption(options, defaultValue) {
		defaultValue = ""
	}

	if service.Terminal != nil {
		checked := checkedOptions(options, []string{defaultValue})

		checked, err := service.selectInteractive(label, options, checked, false)
		if err != nil {
			return "", err
		}

		return checkedValues(options, checked)[0], nil
	}

	printOptions(label, options, checkedOptions(options, []string{defaultValue}))

	for {
		var (
			answer string
			err    error
		)

		if defaultValue == "" {
			answer, err = service.String(label)
		} else {
			answer, err = service.StringWithDefault(label, defaultValue)
		}

		if err != nil {
			return "", err
		}

		if answer == "" {
			fmt.Println("This value is required.")
			continue
		}

		index, ok := resolveOption(options, answer)
		if !ok {
			fmt.Println("Please choose one of the listed options.")
			continue
		}

		return options[index], nil
	}
}

// SelectMany prompts for any number of options, pre-ticking defaultValues.
// With a Terminal it shows a checkbox list toggled with space; otherwise it
// accepts a comma-separated list of option numbers or names, where an empty
// answer keeps defaultValues and "none" clears the selection. The result
// follows options' order and is nil when nothing is selected.
func (service PromptService) SelectMany(label string, options []string, defaultValues []string) ([]string, error) {
	checked := checkedOptions(options, defaultValues)

	if service.Terminal != nil {
		checked, err := service.selectInteractive(label, options, checked, true)
		if err != nil {
			return nil, err
		}

		return checkedValues(options, checked), nil
	}

	printOptions(label, options, checked)

	for {
		answers, err := service.StringListWithDefault(label+", separated by commas", defaultValues)
		if err != nil {
			return nil, err
		}

		selected := make([]bool, len(options))
		valid := true

		for _, answer := range answers {
			index, ok := resolveOption(options, answer)
			if !ok {
				valid = false
				break
			}

			selected[index] = true
		}

		if !valid {
			fmt.Println("Please choose only listed options.")
			continue
		}

		return checkedValues(options, selected), nil
	}
}

// selectInteractive draws options as a checkbox list and updates it on each
// key press until enter is pressed. In single mode the checked box follows
// the cursor; in multi mode space toggles the box under the cursor.
func (service PromptService) selectInteractive(label string, options []string, checked []bool, multi bool) ([]bool, error) {
	restore, err := service.Terminal.MakeRaw()
	if err != nil {
		return nil, err
	}
	defer restore()

	cursor := 0
	for index, isChecked := range checked {
		if isChecked {
			cursor = index
			break
		}
	}

	hint := "up/down to move, enter to confirm"
	if multi {
		hint = "up/down to move, space to toggle, enter to confirm"
	}

	fmt.Printf("%s (%s):\r\n", label, hint)

	for drawn := false; ; drawn = true {
		if !multi {
			checked = make([]bool, len(options))
			checked[cursor] = true
		}

		if drawn {
			fmt.Printf("\x1b[%dA", len(options))
		}

		for index, option := range options {
			// In single mode the ticked box already marks the cursor.
			pointer := ""
			if multi {
				pointer = "  "
				if index == cursor {
					pointer = "> "
				}
			}

			fmt.Printf("\r\x1b[2K%s%s %s\r\n", pointer, checkbox(checked[index]), option)
		}

		key, err := service.readKey()
		if err != nil {
			return nil, err
		}

		switch key {
		case "up":
			cursor = (cursor - 1 + len(options)) % len(options)
		case "down":
			cursor = (cursor + 1) % len(options)
		case "space":
			if multi {
				checked[cursor] = !checked[cursor]
			}
		case "enter":
			return checked, nil
		case "interrupt":
			return nil, ErrPromptInterrupted
		}
	}
}

// readKey reads one key press from a raw-mode terminal and names it, or
// returns "" for keys selection prompts ignore.
func (service PromptService) readKey() (string, error) {
	key, err := service.Reader.ReadByte()
	if err != nil {
		return "", err
	}

	switch key {
	case 0x03:
		return "interrupt", nil
	case '\r', '\n':
		return "enter", nil
	case ' ':
		return "space", nil
	case 'k':
		return "up", nil
	case 'j':
		return "down", nil
	case 0x1b:
		sequence := make([]byte, 2)
		for index := range sequence {
			if sequence[index], err = service.Reader.ReadByte(); err != nil {
				return "", err
			}
		}

		switch string(sequence) {
		case "[A":
			return "up", nil
		case "[B":
			return "down", nil
		}
	}

	return "", nil
}

func printOptions(label string, options []string, checked []bool) {
	fmt.Printf("%s:\n", label)

	for index, option := range options {
		fmt.Printf("  %s %d. %s\n", checkbox(checked[index]), index+1, option)
	}
}

func checkbox(checked bool) string {
	if checked {
		return "[x]"
	}

	return "[ ]"
}

// resolveOption matches answer against an option's 1-based number or its
// name.
func resolveOption(options []string, answer string) (int, bool) {
	answer = strings.TrimSpace(answer)

	if number, err := strconv.Atoi(answer); err == nil {
		if number >= 1 && number <= len(options) {
			return number - 1, true
		}

		return 0, false
	}

	for index, option := range options {
		if option == answer {
			return index, true
		}
	}

	return 0, false
}

func containsOption(options []string, value string) bool {
	for _, option := range options {
		if option == value {
			return true
		}
	}

	return false
}

func checkedOptions(options []string, values []string) []bool {
	checked := make([]bool, len(options))

	for index, option := range options {
		for _, value := range values {
			if option == value {
				checked[index] = true
			}
		}
	}

	return checked
}

func checkedValues(options []string, checked []bool) []string {
	var values []string

	for index, option := range options {
		if checked[index] {
			values = append(values, option)
		}
	}

	return values
}
