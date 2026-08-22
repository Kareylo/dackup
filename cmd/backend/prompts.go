package backend

import (
	"dackup/internal/backend/borg"
	"dackup/internal/backend/kopia"
	"dackup/internal/backend/restic"
	"encoding/json"
	"fmt"
)

// promptBackendDir prompts for DackupConfig.BackendDir — the backend's
// repository storage root. It's a top-level config field (not part of
// backend_settings, see AGENTS.md's "Backend interface" section for why),
// so it's gathered once here rather than per-backend. The value is always
// required: every backend implemented so far needs local repository
// storage — even kopia's remote storage types still need it for their
// local per-repository config files (see AGENTS.md's "Backend interface"
// section).
func (service commandService) promptBackendDir(current string) (string, error) {
	return service.promptRequiredStringWithCurrent("Backend repository storage directory (backend_dir)", current)
}

// promptRequiredStringWithCurrent prompts for a field that must not stay
// empty, pre-filling from current (shown as "[current]") when updating an
// already-configured value, and re-prompting until a non-empty answer is
// given.
func (service commandService) promptRequiredStringWithCurrent(label string, current string) (string, error) {
	for {
		value, err := service.promptOptionalStringWithCurrent(label, current)
		if err != nil {
			return "", err
		}

		if value != "" {
			return value, nil
		}

		fmt.Println("This value is required.")
	}
}

// promptRequiredFilePath prompts for a path to a file that must already
// exist on disk (e.g. an SSH keyfile, a GCS service-account JSON file) —
// unlike promptBinPath, empty is not accepted.
func (service commandService) promptRequiredFilePath(label string, current string) (string, error) {
	for {
		value, err := service.promptBinPath(label, current)
		if err != nil {
			return "", err
		}

		if value != "" {
			return value, nil
		}

		fmt.Println("This value is required.")
	}
}

// promptEncryptedSecret prompts for a secret value and encrypts it via
// service.secrets before returning its ciphertext. current is the existing
// ciphertext (empty on a fresh create); an empty answer keeps it unchanged
// when current is set, or is rejected when there's nothing to keep. Honors
// service.options.DryRun the same way promptBorgSettings's own passphrase
// prompt does, to avoid creating a real secret key file during a dry run.
func (service commandService) promptEncryptedSecret(label string, current string) (string, error) {
	promptLabel := label
	if current != "" {
		promptLabel = label + " (leave empty to keep the current one)"
	}

	value, err := service.prompt.String(promptLabel)
	if err != nil {
		return "", err
	}

	switch {
	case value == "" && current != "":
		return current, nil
	case value == "":
		return "", fmt.Errorf("%s is required", label)
	case service.options != nil && service.options.DryRun:
		fmt.Printf("[dry-run] Would encrypt %s and store its ciphertext\n", label)
		return "[dry-run placeholder, not a real ciphertext]", nil
	default:
		return service.secrets.Encrypt(value)
	}
}

// promptBinPath prompts for an optional explicit path to a CLI tool's
// binary, re-prompting until the path exists as a file (not a directory).
// Empty is always accepted without checking anything — it means "resolve
// the bare command name via PATH at runtime" (see e.g. borg.Config.bin's
// fallback to DefaultBin), not "use this specific file". current pre-fills
// the prompt (shown as "[current]") when updating an already-configured
// backend; empty means there's nothing to default to yet.
func (service commandService) promptBinPath(label string, current string) (string, error) {
	fs := service.fileSystem()

	for {
		value, err := service.promptOptionalStringWithCurrent(label, current)
		if err != nil {
			return "", err
		}

		if value == "" {
			return "", nil
		}

		info, err := fs.Stat(value)
		if err != nil {
			fmt.Printf("No file found at %q: %v\n", value, err)
			continue
		}

		if info.IsDir() {
			fmt.Printf("%q is a directory, not a binary\n", value)
			continue
		}

		return value, nil
	}
}

// promptOptionalStringWithCurrent prompts for a field that may legitimately
// stay empty. When current is non-empty it's shown and kept on an empty
// answer (shared.PromptService.StringWithDefault); otherwise a plain prompt
// is used, so a fresh create doesn't show a misleading "[]" default.
func (service commandService) promptOptionalStringWithCurrent(label string, current string) (string, error) {
	if current != "" {
		return service.prompt.StringWithDefault(label, current)
	}

	return service.prompt.String(label)
}

// selectBackendName prompts for one of the available backend names,
// re-prompting until a listed one is chosen.
func (service commandService) selectBackendName(available []string) (string, error) {
	fmt.Println("Available backends:")
	for index, name := range available {
		fmt.Printf("%d. %s\n", index+1, name)
	}

	for {
		choice, err := service.prompt.RequiredString("Backend name")
		if err != nil {
			return "", err
		}

		for _, name := range available {
			if name == choice {
				return name, nil
			}
		}

		fmt.Println("Please choose one of the listed backend names.")
	}
}

// promptBackendSettings gathers backend-specific settings, pre-filling from
// currentSettings when the backend being configured (name) is the same one
// currentSettings belongs to (currentBackend) — e.g. "dackup backend
// update" without switching to a different backend. Adding a backend means
// adding a case here, matching the "one case per backend" pattern used by
// internal/backend.ParseSettings and internal/backend.Factory.
func (service commandService) promptBackendSettings(name string, currentBackend string, currentSettings json.RawMessage) (json.RawMessage, error) {
	switch name {
	case borg.Name:
		config := borg.DefaultConfig()

		if currentBackend == borg.Name && len(currentSettings) > 0 {
			parsed, err := borg.ParseConfig(currentSettings)
			if err != nil {
				fmt.Printf("Warning: failed to load the current borg settings (%v); starting from defaults\n", err)
			} else {
				config = parsed
			}
		}

		return service.promptBorgSettings(config)
	case kopia.Name:
		config := kopia.DefaultConfig()

		if currentBackend == kopia.Name && len(currentSettings) > 0 {
			parsed, err := kopia.ParseConfig(currentSettings)
			if err != nil {
				fmt.Printf("Warning: failed to load the current kopia settings (%v); starting from defaults\n", err)
			} else {
				config = parsed
			}
		}

		return service.promptKopiaSettings(config)
	case restic.Name:
		config := restic.DefaultConfig()

		if currentBackend == restic.Name && len(currentSettings) > 0 {
			parsed, err := restic.ParseConfig(currentSettings)
			if err != nil {
				fmt.Printf("Warning: failed to load the current restic settings (%v); starting from defaults\n", err)
			} else {
				config = parsed
			}
		}

		return service.promptResticSettings(config)
	default:
		return nil, nil
	}
}
