package backend

import (
	"dackup/internal/backend/borg"
	"encoding/json"
	"fmt"
)

// ParseSettings decodes raw backend_settings JSON into the typed Config
// struct owned by the named backend. An empty name has no settings to parse.
func ParseSettings(name string, raw json.RawMessage) (any, error) {
	switch name {
	case "":
		return nil, nil
	case borg.Name:
		return borg.ParseConfig(raw)
	default:
		return nil, fmt.Errorf("unknown backend %q", name)
	}
}
