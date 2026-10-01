package shared

import (
	"fmt"
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

// ValidateConfiguredPath rejects a configured path that, once cleaned,
// would escape the root it's later joined under (e.g. "../etc") or resolve
// to the whole root itself ("."), since either would let rsync --delete and
// chown -R act outside a single container's data. An empty path is
// rejected too, since filepath.Clean turns it into ".". A root-only path
// ("/") is allowed: it cleans to "", which TransferService already skips
// with a warning.
func ValidateConfiguredPath(configuredPath string) error {
	cleanPath := CleanConfiguredPath(configuredPath)
	if cleanPath == "" {
		return nil
	}

	if cleanPath == "." || !filepath.IsLocal(cleanPath) {
		return fmt.Errorf("configured path %q must stay inside its root directory and cannot be the root itself", configuredPath)
	}

	return nil
}
