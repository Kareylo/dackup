//go:build integration

// Integration tests in this file drive the real kopia CLI against the
// local emulator containers defined in test/compose.yml (start them with
// "docker compose -f test/compose.yml up -d" first). They're excluded from
// the default "go test ./..." run — which must stay hermetic and not
// depend on Docker or a "kopia" binary being present — and only run via
// "go test -tags=integration ./internal/backend/kopia/...". Each test
// skips gracefully (rather than failing) when its prerequisite (the kopia
// binary, or the target container) isn't reachable, so the suite degrades
// cleanly on a machine that hasn't started the containers.
package gcs_test

import (
	"dackup/internal/backend/kopia"
	"testing"
)

// TestIntegration_GCS backs up to and restores from the test_gcs service
// in test/compose.yml, using test/config.gcs.json.
//
// gcs.Storage.EmulatorHost (STORAGE_EMULATOR_HOST) is a best-effort bet on
// kopia's GCS backend using the standard Google Cloud Storage Go client
// under the hood, since kopia itself documents no gcs endpoint override
// flag — see gcs.Storage.EmulatorHost's doc comment. A failure here may
// mean that bet didn't pay off, not a bug in this test.
func TestIntegration_GCS(t *testing.T) {
	kopia.RequireKopiaBinary(t)
	kopia.RequireReachable(t, "localhost:4443")

	config := kopia.LoadIntegrationConfig(t, "config.gcs.json")
	backend := kopia.NewIntegrationBackend(t, config)

	kopia.RunBackupRestoreRoundTrip(t, backend)
}
