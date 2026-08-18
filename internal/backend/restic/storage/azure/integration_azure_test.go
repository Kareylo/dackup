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
	"strings"
	"testing"
)

// TestIntegration_Azure backs up to and restores from the test_azurite
// service in test/compose.yml (the same container kopia's own azure
// integration test uses), using test/config.restic-azure.json, whose
// endpoint_suffix field points restic's AZURE_ENDPOINT_SUFFIX at Azurite.
//
// Like kopia's own azure integration test (see its doc comment on
// TestIntegration_Azure / azure.Storage.StorageDomain), this is the least
// certain of restic's storage integration tests: restic's azure backend
// always builds a virtual-hosted-style HTTPS URL
// ("https://<account>.blob.<suffix>/<container>") with no override for
// scheme or path-style addressing, but Azurite serves plain-HTTP
// path-style URLs — a genuine upstream limitation, not something fixable
// in dackup. Rather than failing permanently (or skipping unconditionally,
// which would hide a real regression), Backup's error is checked for that
// exact, stable signature: if it matches, the test skips with an
// explanation instead of failing, but any other error — or no error at
// all, e.g. once restic adds Azurite support or this is pointed at a real
// Azure account — is treated as a real result and runs (or fails) the rest
// of the round trip normally.
//
// This retries for a while before returning that error: restic wraps every
// backend (including azure) in a retry loop with a hardcoded 15-minute
// MaxElapsedTime (confirmed against restic 0.19.1's
// internal/global/global.go — retry.New(be, 15*time.Minute, ...) — with no
// CLI flag or env var to shorten it), so a persistent-looking connection
// error like this one gets retried with exponential backoff for a while.
// restic.NewIntegrationBackend's CommandRunner bounds every restic
// invocation well under that ceiling (see commandTimeout in
// integration_helpers.go) so this returns in well under a minute instead
// of blocking for up to 15 — the known-incompatibility signature is
// already present in restic's captured retry-log output long before that
// bound is reached, so the skip below still fires correctly.
func TestIntegration_Azure(t *testing.T) {
	restic.RequireResticBinary(t)
	restic.RequireReachable(t, "localhost:10000")

	config := restic.LoadIntegrationConfig(t, "config.restic-azure.json")
	backend := restic.NewIntegrationBackend(t, config)

	stagingDir, testFilePath, testContent := restic.WriteIntegrationTestFile(t)

	err := backend.Backup(stagingDir)
	if err != nil {
		if isKnownAzuriteAddressingIncompatibility(err) {
			t.Skipf("skipping: restic's azure client can't reach Azurite (restic always speaks virtual-hosted-style HTTPS, Azurite serves path-style HTTP, and restic's azure backend has no override for either) — see azure.Storage.EndpointSuffix's doc comment: %v", err)
		}
		t.Fatalf("Backup returned error: %v", err)
	}

	restic.VerifyRestoreRoundTrip(t, backend, stagingDir, testFilePath, testContent)
}

// isKnownAzuriteAddressingIncompatibility reports whether err matches the
// specific, confirmed restic/Azurite incompatibility documented on
// TestIntegration_Azure and azure.Storage.EndpointSuffix — a TLS client
// talking to a plaintext HTTP server, which is what restic's always-HTTPS
// azure backend gets from Azurite's plain-HTTP endpoint.
//
// "tls: first record does not look like a TLS handshake" is Go crypto/tls's
// own RecordHeaderError message (src/crypto/tls/conn.go) for exactly this
// case, and is what actually comes back — confirmed by running the real
// restic 0.19.1 binary against a plain HTTP listener on the configured
// endpoint_suffix port. net/http.ErrSchemeMismatch ("server gave HTTP
// response to HTTPS client", checked here too since it's a strictly more
// specific match for the same underlying condition) only replaces that
// message when the request goes through net/http.Client.Do — see
// src/net/http/client.go's tls.RecordHeaderError handling — which the
// Azure SDK's own pipeline (used by restic's azure backend) doesn't do, so
// that friendlier string never actually appears here in practice; kopia's
// own azure.Storage.StorageDomain doc comment and
// isKnownAzuriteAddressingIncompatibility describe the same assumption and
// are equally worth rechecking against a real kopia binary.
//
// Any other error is treated as a genuine failure, not this known
// limitation, so a real regression still fails loudly instead of being
// silently swallowed.
func isKnownAzuriteAddressingIncompatibility(err error) bool {
	message := err.Error()
	return strings.Contains(message, "first record does not look like a TLS handshake") ||
		strings.Contains(message, "server gave HTTP response to HTTPS client")
}
