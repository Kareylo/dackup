package backend

import (
	"dackup/internal/shared"
	"encoding/json"
	"fmt"
	"strings"
)

// printBackend prints the currently configured backend and its settings,
// or reports that none is configured. Used by both "backend show" and as
// the "current configuration" preview at the start of "backend update".
func printBackend(config shared.DackupConfig) {
	if config.Backend == "" {
		fmt.Println("No backend configured (using the default no-op backend)")
		return
	}

	fmt.Printf("Backend: %s\n", config.Backend)

	if config.BackendDir != "" {
		fmt.Printf("Backend directory: %s\n", config.BackendDir)
	} else {
		fmt.Println("Backend directory: (not set)")
	}

	if len(config.BackendSettings) == 0 {
		fmt.Println("Settings: none")
		return
	}

	pretty, err := json.MarshalIndent(maskEncryptedSettings(config.BackendSettings), "", "  ")
	if err != nil {
		fmt.Printf("Settings: %s\n", config.BackendSettings)
		return
	}

	fmt.Printf("Settings:\n%s\n", pretty)
}

// maskEncryptedSettings masks any "encrypted_*" field in raw backend_settings
// JSON before display, at any nesting depth, so "dackup backend show" never
// prints an encrypted secret's ciphertext — kopia's storage settings nest
// theirs under a sub-object (e.g. s3.encrypted_secret_access_key) rather
// than keeping them top-level like borg's encrypted_passphrase. Falls back
// to the raw bytes unchanged if they don't decode as JSON at all.
func maskEncryptedSettings(raw json.RawMessage) json.RawMessage {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return raw
	}

	masked, err := json.Marshal(maskEncryptedValue(value))
	if err != nil {
		return raw
	}

	return masked
}

func maskEncryptedValue(value any) any {
	fields, ok := value.(map[string]any)
	if !ok {
		return value
	}

	masked := make(map[string]any, len(fields))
	for key, fieldValue := range fields {
		if str, isString := fieldValue.(string); isString && str != "" && strings.HasPrefix(key, "encrypted_") {
			masked[key] = "[set]"
			continue
		}

		masked[key] = maskEncryptedValue(fieldValue)
	}

	return masked
}
