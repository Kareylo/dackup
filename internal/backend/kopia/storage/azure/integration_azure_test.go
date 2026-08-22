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
package azure_test

import (
	"dackup/internal/backend/kopia"
	"strings"
	"testing"
)

// TestIntegration_Azure backs up to and restores from the test_azurite
// service in test/compose.yml, using test/config.azure.json and Azurite's
// well-known local-dev account/key.
//
// This one is the least certain of the five: kopia's azure provider's
// --storage-domain flag (which config.azure.json's storage_domain field
// sets) overrides the domain in an otherwise virtual-hosted-style URL
// (https://<account>.<domain>/<container>/...), but Azurite serves
// path-style URLs instead (http://<host>:<port>/<account>/<container>/...).
// kopia's azure CLI exposes no override for scheme or addressing style, so
// this is a genuine upstream limitation, not something fixable in dackup —
// confirmed by driving the real kopia CLI against a real Azurite container
// and getting the identical error every time. Rather than failing
// permanently (or skipping unconditionally, which would hide a real
// regression), Backup's error is checked for that exact, stable signature:
// if it matches, the test skips with an explanation instead of failing, but
// any other error — or no error at all, e.g. once kopia adds Azurite
// support or this is pointed at a real Azure account — is treated as a real
// result and runs (or fails) the rest of the round trip normally. See
// azure.Storage.StorageDomain's doc comment for the full explanation.
func TestIntegration_Azure(t *testing.T) {
	kopia.RequireKopiaBinary(t)
	kopia.RequireReachable(t, "localhost:10000")

	config := kopia.LoadIntegrationConfig(t, "config.azure.json")
	backend := kopia.NewIntegrationBackend(t, config)

	stagingDir, testFilePath, testContent := kopia.WriteIntegrationTestFile(t)

	err := backend.Backup(stagingDir)
	if err != nil {
		if isKnownAzuriteAddressingIncompatibility(err) {
			t.Skipf("skipping: kopia's azure client can't reach Azurite (kopia always speaks virtual-hosted-style HTTPS via --storage-domain, Azurite serves path-style HTTP, and kopia's azure CLI has no override for either) — see azure.Storage.StorageDomain's doc comment: %v", err)
		}
		t.Fatalf("Backup returned error: %v", err)
	}

	kopia.VerifyRestoreRoundTrip(t, backend, stagingDir, testFilePath, testContent)
}

// isKnownAzuriteAddressingIncompatibility reports whether err matches the
// specific, confirmed kopia/Azurite incompatibility documented on
// TestIntegration_Azure and azure.Storage.StorageDomain. Any other error is
// treated as a genuine failure, not this known limitation, so a real
// regression still fails loudly instead of being silently swallowed.
func isKnownAzuriteAddressingIncompatibility(err error) bool {
	return strings.Contains(err.Error(), "server gave HTTP response to HTTPS client")
}
