package backend

import (
	"dackup/internal/backend/default"
	"dackup/internal/shared"
	"encoding/json"
	"fmt"
)

// Factory constructs a Backend by name, injecting the same dependencies
// shared.TransferService takes today so a concrete backend's constructor
// can slot in without changing this signature.
type Factory struct {
	Runner  shared.CommandRunner
	Logger  shared.Logger
	Options *shared.Options
}

// GetBackend resolves name (a DackupConfig.Backend value) and its raw
// settings into a ready-to-use Backend. An empty name returns
// defaultbackend.Backend; any other name is looked up against the
// registered backends and returns an error if none match.
func (factory Factory) GetBackend(name string, settings json.RawMessage) (Backend, error) {
	switch name {
	case "":
		return defaultbackend.Backend{Logger: factory.Logger}, nil
	default:
		return nil, fmt.Errorf("unknown backend %q", name)
	}
}
