#!/bin/sh
# Creates the "dackup-test" container kopia's azure storage type expects
# (test/config.azure.json). Azurite doesn't auto-create containers any more
# than real Azure Blob Storage does. AZURE_STORAGE_CONNECTION_STRING (set
# on this container in compose.yml) carries Azurite's well-known local-dev
# account/key — the same ones test/config.azure.json's
# storage_account/encrypted_storage_key encode — and addresses test_azurite
# via its "azurite" network alias rather than the service name itself,
# which has an underscore and isn't a valid DNS hostname per RFC 1123 (see
# scripts/init-minio.sh's comment — the same class of failure showed up
# there first). Retries until test_azurite is actually accepting
# connections, bounded so a real, persistent failure exits with a clear
# error instead of hanging forever — "az" itself is slow to start per
# invocation, so each attempt already costs a few seconds before it even
# gets to fail or succeed.
set -eu

attempt=0
until az storage container create --name dackup-test; do
	attempt=$((attempt + 1))
	if [ "$attempt" -ge 30 ]; then
		echo "timed out waiting for test_azurite to accept connections" >&2
		exit 1
	fi
	sleep 2
done

echo "dackup-test container ready"
