package backend

import (
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

func (factory Factory) GetBackend(name string, settings json.RawMessage) (Backend, error) {
	switch name {
	case "":
		return DefaultBackend{Logger: factory.Logger}, nil
	default:
		return nil, fmt.Errorf("unknown backend %q", name)
	}
}
