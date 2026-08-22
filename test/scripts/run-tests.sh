#!/bin/sh
# Runs dackup's full test suite (unit + "//go:build integration") with
# coverage credited across package boundaries (-coverpkg=./...,  same
# reasoning as the "test" job in .github/workflows/test.yml: a plain
# `go test -cover ./...` undercounts internal/shared logic that's only
# exercised indirectly through cmd/*'s thin wrappers), then enforces the
# same coverage threshold that job checks — kept in sync so
# `make test-integration-docker` gates coverage the same way locally and
# in CI as the faster, hermetic unit-only job does, rather than only the
# latter being checked.
set -eu

threshold="${COVERAGE_THRESHOLD:-80}"

go test -tags=integration -coverpkg=./... -coverprofile=coverage.out ./...

total=$(go tool cover -func=coverage.out | tail -1 | awk '{print $NF}' | tr -d '%')
echo "Total coverage (credited for cross-package calls, tags=integration): ${total}%"

awk -v total="$total" -v threshold="$threshold" 'BEGIN {
	if (total + 0 < threshold + 0) {
		printf "FAIL: coverage %.1f%% is below the %s%% threshold\n", total, threshold
		exit 1
	}
	printf "OK: coverage %.1f%% meets the %s%% threshold\n", total, threshold
}'
