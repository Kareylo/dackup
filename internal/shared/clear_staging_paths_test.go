package shared

import (
	"os"
	"reflect"
	"testing"
)

// removeRecordingFS records RemoveAll calls; every other method is unused
// by ClearStagingPaths.
type removeRecordingFS struct {
	removed *[]string
}

func (fs removeRecordingFS) Stat(name string) (os.FileInfo, error) { return nil, os.ErrNotExist }
func (fs removeRecordingFS) MkdirAll(path string, perm os.FileMode) error {
	return nil
}
func (fs removeRecordingFS) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return nil, os.ErrInvalid
}
func (fs removeRecordingFS) RemoveAll(path string) error {
	*fs.removed = append(*fs.removed, path)
	return nil
}

func TestClearStagingPaths_RemovesEachConfiguredPathAndSkipsTheRoot(t *testing.T) {
	var removed []string

	err := ClearStagingPaths(removeRecordingFS{removed: &removed}, "/backups/in", []string{"data/paperless", "/", "/config/adguard"})
	if err != nil {
		t.Fatalf("ClearStagingPaths returned error: %v", err)
	}

	want := []string{"/backups/in/data/paperless", "/backups/in/config/adguard"}
	if !reflect.DeepEqual(removed, want) {
		t.Fatalf("expected removed paths %v, got %v", want, removed)
	}
}

// A path that escapes the staging root must be refused before anything is
// deleted, even the valid paths listed before it, so the call either
// clears every path or none.
func TestClearStagingPaths_PathEscapingTheRootRemovesNothing(t *testing.T) {
	for _, escaping := range []string{"../etc", "data/../../x", "data/../.."} {
		t.Run(escaping, func(t *testing.T) {
			var removed []string

			err := ClearStagingPaths(removeRecordingFS{removed: &removed}, "/backups/in", []string{"data/paperless", escaping})
			if err == nil {
				t.Fatalf("expected an error for %q", escaping)
			}

			if len(removed) != 0 {
				t.Fatalf("expected nothing removed, got %v", removed)
			}
		})
	}
}
