//go:build integration

// Integration tests in this file drive the real restic CLI against the
// local emulator containers defined in test/compose.yml (start them with
// "docker compose -f test/compose.yml up -d" first). They're excluded from
// the default "go test ./..." run — which must stay hermetic and not
// depend on Docker or a "restic" binary being present — and only run via
// "go test -tags=integration ./internal/backend/restic/...". Each test
// skips gracefully (rather than failing) when its prerequisite (the restic
// binary, or the target container) isn't reachable.
package s3_test

import (
	"dackup/internal/backend/restic"
	"testing"
)

// TestIntegration_S3 backs up to and restores from the test_minio service
// in test/compose.yml (the same container kopia's own s3 integration test
// uses), using test/config.restic-s3.json.
func TestIntegration_S3(t *testing.T) {
	restic.RequireResticBinary(t)
	restic.RequireReachable(t, "localhost:9000")

	config := restic.LoadIntegrationConfig(t, "config.restic-s3.json")
	backend := restic.NewIntegrationBackend(t, config)

	restic.RunBackupRestoreRoundTrip(t, backend)
}
