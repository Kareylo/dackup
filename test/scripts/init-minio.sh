#!/bin/sh
# Creates the "dackup-test" bucket kopia's s3 storage type expects
# (test/config.s3.json). MinIO doesn't auto-create buckets on first write,
# unlike a local filesystem directory. Retries until test_minio is actually
# accepting connections — "depends_on" alone only waits for the container
# to start, not for the server inside it to be ready — but bounded, so a
# real, persistent failure (bad host, bad credentials) exits with a clear
# error instead of hanging test_minio_init (and anything waiting on it)
# forever.
#
# Addressed as "minio" (a networks.default.aliases entry on test_minio in
# compose.yml), not "test_minio": the latter has an underscore, which isn't
# a valid DNS hostname per RFC 1123 — Docker's own resolver is lenient
# about it, but mc strictly validates hostnames and rejects it ("Invalid
# Request (invalid hostname)"), a real failure discovered via live testing
# that looked identical to "not ready yet" until the retry loop's bound was
# hit and its actual error text inspected.
#
# "mc alias set" is retried together with "mc mb" (not run once up front)
# because it may itself fail while test_minio isn't reachable yet — under
# `set -e` a failure there would have exited the script before the retry
# loop even started, defeating the whole point of retrying.
set -u

attempt=0
until mc alias set local http://minio:9000 minioadmin minioadmin && mc mb --ignore-existing local/dackup-test; do
	attempt=$((attempt + 1))
	if [ "$attempt" -ge 30 ]; then
		echo "timed out waiting for test_minio to accept connections" >&2
		exit 1
	fi
	sleep 1
done

echo "dackup-test bucket ready"
