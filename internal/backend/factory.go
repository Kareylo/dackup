package backend

import (
	"dackup/internal/backend/borg"
	"dackup/internal/backend/default"
	"dackup/internal/backend/kopia"
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
	case kopia.Name:
		return factory.getKopiaBackend(settings)
	default:
		return nil, fmt.Errorf("unknown backend %q", name)
	}
}

func requireBackendDir(name string, backendDir string) error {
	if strings.TrimSpace(backendDir) == "" {
		return fmt.Errorf("backend %q requires backend_dir to be set in the main config", name)
	}

	return nil
}

func (factory Factory) getBorgBackend(settings json.RawMessage) (Backend, error) {
	if err := requireBackendDir(borg.Name, factory.BackendDir); err != nil {
		return nil, err
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

func (factory Factory) getKopiaBackend(settings json.RawMessage) (Backend, error) {
	if err := requireBackendDir(kopia.Name, factory.BackendDir); err != nil {
		return nil, err
	}

	config, err := kopia.ParseConfig(settings)
	if err != nil {
		return nil, err
	}

	return kopia.Backend{
		Config:    config,
		ReposRoot: factory.BackendDir,
		Runner:    factory.Runner,
		Logger:    factory.Logger,
		Options:   factory.Options,
		Secrets:   factory.Secrets,
	}, nil
}
