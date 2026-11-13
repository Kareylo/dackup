# Builds dackup and runs its full test suite, including the borg + kopia +
# restic "//go:build integration" tests (see AGENTS.md's "Borg integration
# tests" / "Kopia storage integration tests" / "Restic integration tests"
# sections), against real, pinned borg/kopia/restic binaries instead of
# whatever (if anything) happens to be on a contributor's or CI runner's
# PATH. Run via the "test_dackup" service in test/compose.yml
# (`make test-integration-docker`), which starts it with network_mode: host
# so it reaches the emulator containers the same way the host-executed
# `make test-integration` does — through their published localhost ports —
# without touching any of the integration tests' hardcoded
# "localhost:<port>" config.
FROM golang:1.26.11-bookworm

# kopia isn't packaged in Debian's apt repos, and unlike borg its exact
# version matters more here (behavior can vary enough across releases to
# affect the storage integration tests), so it's pinned explicitly rather
# than tracked via a rolling apt repo or "whatever is on PATH" — see the
# `kopia --version` check that motivated this Dockerfile in the first
# place: a locally installed kopia with no reproducible version at all.
ARG KOPIA_VERSION=0.23.1
# SHA-256 of kopia_${KOPIA_VERSION}_linux_<arch>.deb for each supported
# architecture, from that release's checksums.txt. Update them together
# with KOPIA_VERSION.
ARG KOPIA_SHA256_AMD64=3998c96b2db3410880ec8f6723f3c127248915896c22e6b882b352693230253a
ARG KOPIA_SHA256_ARM64=9503237e323062136273b2e8e133b5e459ab60d762be14c5df2e09276bc8bea3

# restic is pinned the same way and for the same reason as kopia above.
# Debian bookworm does package a restic, but at a version this repo doesn't
# control the freshness of; restic's own release process publishes
# prebuilt binaries as a single bzip2-compressed file per platform (no
# .deb), unlike kopia's GitHub-released .deb — confirmed against the
# restic/restic GitHub releases API, whose per-platform assets are named
# "restic_<version>_linux_<arch>.bz2".
ARG RESTIC_VERSION=0.19.1
# SHA-256 of restic_${RESTIC_VERSION}_linux_<arch>.bz2 for each supported
# architecture, from that release's SHA256SUMS. Update them together with
# RESTIC_VERSION.
ARG RESTIC_SHA256_AMD64=f415415624dcc452f2a02b8c33641791a8c6d6d3b65bbb3543fcf9a25151585c
ARG RESTIC_SHA256_ARM64=a5f64aaab53d51e311fa3829124c5b703f2d14cf187d8640b6be3b2b49376465

# borgbackup is left unpinned, tracking whatever version Debian bookworm's
# own apt snapshot carries — consistent with the Makefile's own
# `deps-install` target, which installs it the same unpinned way.
#
# The kopia/restic architecture comes from dpkg inside the image rather
# than from BuildKit's TARGETARCH, so it is right under any builder
# (docker, podman/buildah) and on amd64 and arm64 hosts alike. Debian's
# architecture names match the ones kopia and restic use in their asset
# names.
RUN arch="$(dpkg --print-architecture)" \
    && case "$arch" in \
         amd64) kopia_sha256="${KOPIA_SHA256_AMD64}"; restic_sha256="${RESTIC_SHA256_AMD64}" ;; \
         arm64) kopia_sha256="${KOPIA_SHA256_ARM64}"; restic_sha256="${RESTIC_SHA256_ARM64}" ;; \
         *) echo "test/Dockerfile: unsupported architecture ${arch} (only amd64 and arm64 are pinned)" >&2; exit 1 ;; \
       esac \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
         borgbackup \
         rsync \
         make \
         curl \
         bzip2 \
         ca-certificates \
    && curl -fsSLo /tmp/kopia.deb \
         "https://github.com/kopia/kopia/releases/download/v${KOPIA_VERSION}/kopia_${KOPIA_VERSION}_linux_${arch}.deb" \
    && echo "${kopia_sha256}  /tmp/kopia.deb" | sha256sum -c - \
    && apt-get install -y --no-install-recommends /tmp/kopia.deb \
    && rm -f /tmp/kopia.deb \
    && curl -fsSLo /tmp/restic.bz2 \
         "https://github.com/restic/restic/releases/download/v${RESTIC_VERSION}/restic_${RESTIC_VERSION}_linux_${arch}.bz2" \
    && echo "${restic_sha256}  /tmp/restic.bz2" | sha256sum -c - \
    && bunzip2 /tmp/restic.bz2 \
    && install -m 0755 /tmp/restic /usr/local/bin/restic \
    && rm -f /tmp/restic \
    && rm -rf /var/lib/apt/lists/*

# The suite runs as an unprivileged user, not root: none of the tests need
# root (dackup's integration runs never chown), and a test that only
# passes as root would hide a real permission bug. GOPATH (/go) is 1777 in
# the official golang image, so the module cache below is created by this
# user; /src is created up front because WORKDIR would make it root-owned,
# and the suite writes coverage.out and build/ into it.
RUN useradd --create-home --uid 1000 tester \
    && install -d -o tester -g tester /src

WORKDIR /src
USER tester

# Layer the module cache separately from the rest of the source so editing
# application code doesn't invalidate `go mod download`'s layer.
COPY --chown=tester:tester go.mod go.sum ./
RUN go mod download

COPY --chown=tester:tester . .

# Fails the image build fast on a compile error, before any container ever
# starts and burns time waiting on the emulator services in compose.yml.
RUN make build

# Runs the whole suite (unit + integration — -tags=integration only adds
# the integration-tagged files, it doesn't exclude the regular unit tests)
# and enforces the same coverage threshold the "test" job checks for the
# unit-only suite; see test/scripts/run-tests.sh.
CMD ["test/scripts/run-tests.sh"]
