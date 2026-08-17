//go:build integration

// Integration tests in this file drive the real restic CLI against the
// local emulator containers defined in test/compose.yml (start them with
// "docker compose -f test/compose.yml up -d" first). They're excluded from
// the default "go test ./..." run and only run via
// "go test -tags=integration ./internal/backend/restic/...". Each test
// skips gracefully (rather than failing) when its prerequisite isn't
// reachable.
package azure_test

import (
	"dackup/internal/backend/restic"
	"testing"
)

// TestIntegration_Azure backs up to and restores from the test_azurite
// service in test/compose.yml (the same container kopia's own azure
// integration test uses), using test/config.restic-azure.json.
func TestIntegration_Azure(t *testing.T) {
	restic.RequireResticBinary(t)
	restic.RequireReachable(t, "localhost:10000")

	config := restic.LoadIntegrationConfig(t, "config.restic-azure.json")
	backend := restic.NewIntegrationBackend(t, config)

	restic.RunBackupRestoreRoundTrip(t, backend)
}
