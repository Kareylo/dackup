package shared

import (
	"os"
	"path/filepath"
	"strings"
)

type PathResolver struct {
	SourceRoot      string
	DestinationRoot string
}

func (resolver PathResolver) SourcePath(configuredPath string) string {
	return filepath.Join(resolver.SourceRoot, CleanConfiguredPath(configuredPath))
}

func (resolver PathResolver) DestinationPath(configuredPath string) string {
	return filepath.Join(resolver.DestinationRoot, CleanConfiguredPath(configuredPath))
}

func CleanConfiguredPath(configuredPath string) string {
	return strings.TrimPrefix(filepath.Clean(configuredPath), string(os.PathSeparator))
}
