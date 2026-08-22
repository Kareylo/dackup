package shared

import (
	"os"
	"path/filepath"
	"strings"
)

// PathResolver joins a configured container path under either the source
// or destination root.
type PathResolver struct {
	SourceRoot      string
	DestinationRoot string
}

// SourcePath resolves configuredPath under SourceRoot.
func (resolver PathResolver) SourcePath(configuredPath string) string {
	return filepath.Join(resolver.SourceRoot, CleanConfiguredPath(configuredPath))
}

// DestinationPath resolves configuredPath under DestinationRoot.
func (resolver PathResolver) DestinationPath(configuredPath string) string {
	return filepath.Join(resolver.DestinationRoot, CleanConfiguredPath(configuredPath))
}

// CleanConfiguredPath cleans configuredPath and strips its leading
// separator, so an absolute-looking configured path (e.g. "/data/app") is
// treated as relative to whatever root it's later joined under.
func CleanConfiguredPath(configuredPath string) string {
	return strings.TrimPrefix(filepath.Clean(configuredPath), string(os.PathSeparator))
}
