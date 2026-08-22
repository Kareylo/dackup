//go:build integration

// Integration tests in this file drive the real restic CLI against the
// local emulator containers defined in test/compose.yml (start them with
// "docker compose -f test/compose.yml up -d" first). They're excluded from
// the default "go test ./..." run and only run via
// "go test -tags=integration ./internal/backend/restic/...". Each test
// skips gracefully (rather than failing) when its prerequisite isn't
// reachable.
package gcs_test

import (
	"dackup/internal/backend/restic"
	"testing"
)

// TestIntegration_GCS backs up to and restores from the test_gcs service in
// test/compose.yml (the same fake-gcs-server container kopia's own gcs
// integration test uses), using test/config.restic-gcs.json. Like kopia's
// own gcs storage type, whether restic's GCS backend actually honors
// STORAGE_EMULATOR_HOST is unconfirmed without a real emulator to test
// against — see gcs.Storage's doc comment — so this test is exactly that
// confirmation.
func TestIntegration_GCS(t *testing.T) {
	restic.RequireResticBinary(t)
	restic.RequireReachable(t, "localhost:4443")

	config := restic.LoadIntegrationConfig(t, "config.restic-gcs.json")
	backend := restic.NewIntegrationBackend(t, config)

	restic.RunBackupRestoreRoundTrip(t, backend)
}
