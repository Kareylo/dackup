package backend

import (
	"dackup/internal/backend/borg"
	"dackup/internal/backend/default"
	"dackup/internal/shared"
	"encoding/json"
	"fmt"
	"strings"
)

// Factory constructs a Backend by name, injecting the same dependencies
// shared.TransferService takes today so a concrete backend's constructor
// can slot in without changing this signature. BackendDir is the top-level
// DackupConfig.BackendDir; Secrets decrypts any stored backend credential
// (e.g. borg's encrypted_passphrase).
type Factory struct {
	Runner     shared.CommandRunner
	Logger     shared.Logger
	Options    *shared.Options
	BackendDir string
	Secrets    shared.SecretStore
}

// GetBackend resolves name (a DackupConfig.Backend value) and its raw
// settings into a ready-to-use Backend. An empty name returns
// defaultbackend.Backend; any other name is looked up against the
// registered backends and returns an error if none match.
func (factory Factory) GetBackend(name string, settings json.RawMessage) (Backend, error) {
	switch name {
	case "":
		return defaultbackend.Backend{Logger: factory.Logger}, nil
	case borg.Name:
		return factory.getBorgBackend(settings)
	default:
		return nil, fmt.Errorf("unknown backend %q", name)
	}
}

func (factory Factory) getBorgBackend(settings json.RawMessage) (Backend, error) {
	if strings.TrimSpace(factory.BackendDir) == "" {
		return nil, fmt.Errorf("backend %q requires backend_dir to be set in the main config", borg.Name)
	}

	config, err := borg.ParseConfig(settings)
	if err != nil {
		return nil, err
	}

	return borg.Backend{
		Config:    config,
		ReposRoot: factory.BackendDir,
		Runner:    factory.Runner,
		Logger:    factory.Logger,
		Options:   factory.Options,
		Secrets:   factory.Secrets,
	}, nil
}
