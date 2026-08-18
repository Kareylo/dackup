//go:build integration

// Integration tests in this file drive the real restic CLI against the
// local emulator containers defined in test/compose.yml (start them with
// "docker compose -f test/compose.yml up -d" first). They're excluded from
// the default "go test ./..." run and only run via
// "go test -tags=integration ./internal/backend/restic/...". Each test
// skips gracefully (rather than failing) when its prerequisite isn't
// reachable.
package rest_test

import (
	"dackup/internal/backend/restic"
	"testing"
)

// TestIntegration_Rest backs up to and restores from the test_restic_rest
// service in test/compose.yml (an unauthenticated restic/rest-server
// instance), using test/config.restic-rest.json.
func TestIntegration_Rest(t *testing.T) {
	restic.RequireResticBinary(t)
	restic.RequireReachable(t, "localhost:8010")

	config := restic.LoadIntegrationConfig(t, "config.restic-rest.json")
	backend := restic.NewIntegrationBackend(t, config)

	restic.RunBackupRestoreRoundTrip(t, backend)
}
