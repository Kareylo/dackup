package backend

import (
	"dackup/internal/backend/borg"
	"encoding/json"
	"fmt"
	"strings"
)

// promptBorgSettings gathers borg.Config's fields interactively, using
// current as both the starting point and the pre-filled default shown at
// each prompt — current is borg.DefaultConfig() on a fresh create, or the
// already-configured borg.Config on an update. The repository storage root
// itself (DackupConfig.BackendDir) is not prompted for here — it's a
// top-level config field gathered once by configureBackend's own
// promptBackendDir, not part of backend_settings (see AGENTS.md's "Backend
// interface" section for why).
func (service commandService) promptBorgSettings(current borg.Config) (json.RawMessage, error) {
	config := current

	bin, err := service.promptBinPath("Path to the borg binary (leave empty to use PATH)", config.Bin)
	if err != nil {
		return nil, err
	}
	config.Bin = bin

	globalRepoName, err := service.prompt.StringWithDefault(
		"Name of the global repository (a full mirror, in addition to one per container group)",
		config.GlobalRepoName,
	)
	if err != nil {
		return nil, err
	}
	config.GlobalRepoName = globalRepoName

	encryption, err := service.prompt.StringWithDefault(
		"Borg encryption mode (none, repokey, repokey-blake2, keyfile, keyfile-blake2, authenticated, authenticated-blake2)",
		config.Encryption,
	)
	if err != nil {
		return nil, err
	}
	config.Encryption = encryption

	if strings.TrimSpace(encryption) == "none" {
		config.EncryptedPassphrase = ""
	} else {
		passphraseLabel := "Borg repository passphrase"
		if config.EncryptedPassphrase != "" {
			passphraseLabel = "Borg repository passphrase (leave empty to keep the current one)"
		}

		passphrase, err := service.prompt.String(passphraseLabel)
		if err != nil {
			return nil, err
		}

		switch {
		case passphrase == "" && config.EncryptedPassphrase != "":
			// Keep the existing encrypted_passphrase unchanged.
		case passphrase == "":
			return nil, fmt.Errorf("a passphrase is required for encryption %q", encryption)
		case service.options != nil && service.options.DryRun:
			fmt.Println("[dry-run] Would encrypt the passphrase and store it as encrypted_passphrase")
			config.EncryptedPassphrase = "[dry-run placeholder, not a real ciphertext]"
		default:
			encryptedPassphrase, err := service.secrets.Encrypt(passphrase)
			if err != nil {
				return nil, err
			}

			config.EncryptedPassphrase = encryptedPassphrase
		}
	}

	compression, err := service.promptOptionalStringWithCurrent(
		"Borg compression, e.g. zstd,6 (leave empty for borg's default)",
		config.Compression,
	)
	if err != nil {
		return nil, err
	}
	config.Compression = compression

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return json.Marshal(config)
}
