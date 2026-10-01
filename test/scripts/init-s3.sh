#!/bin/sh
# Creates the "dackup-test" bucket kopia's and restic's s3 storage types
# expect (test/config.s3.json, test/config.restic-s3.json). versitygw
# doesn't auto-create buckets on first write, unlike a local filesystem
# directory. Retries until test_s3 is actually accepting connections —
# "depends_on" alone only waits for the container to start, not for the
# server inside it to be ready — but bounded, so a real, persistent failure
# (bad host, bad credentials) exits with a clear error instead of hanging
# test_s3_init (and anything waiting on it) forever.
#
# Addressed as "s3" (a networks.default.aliases entry on test_s3 in
# compose.yml), not "test_s3": the latter has an underscore, which isn't a
# valid DNS hostname per RFC 1123 — Docker's own resolver is lenient about
# it, but S3 clients strictly validate hostnames and reject it ("Invalid
# Request (invalid hostname)"), a real failure discovered via live testing
# that looked identical to "not ready yet" until the retry loop's bound was
# hit and its actual error text inspected.
#
# "head-bucket" makes the script idempotent: re-running it against an
# existing bucket succeeds instead of failing on "BucketAlreadyOwnedByYou".
set -u

attempt=0
until aws --endpoint-url http://s3:9000 s3api head-bucket --bucket dackup-test 2>/dev/null \
	|| aws --endpoint-url http://s3:9000 s3 mb s3://dackup-test; do
	attempt=$((attempt + 1))
	if [ "$attempt" -ge 30 ]; then
		echo "timed out waiting for test_s3 to accept connections" >&2
		exit 1
	fi
	sleep 1
done

echo "dackup-test bucket ready"
