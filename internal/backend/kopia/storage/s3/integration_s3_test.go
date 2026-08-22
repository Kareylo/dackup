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
package s3_test

import (
	"dackup/internal/backend/kopia"
	"testing"
)

// TestIntegration_S3 backs up to and restores from the test_minio service
// in test/compose.yml, using test/config.s3.json.
func TestIntegration_S3(t *testing.T) {
	kopia.RequireKopiaBinary(t)
	kopia.RequireReachable(t, "localhost:9000")

	config := kopia.LoadIntegrationConfig(t, "config.s3.json")
	backend := kopia.NewIntegrationBackend(t, config)

	kopia.RunBackupRestoreRoundTrip(t, backend)
}
