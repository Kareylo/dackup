# Builds dackup and runs its full test suite, including the borg + kopia
# storage "//go:build integration" tests (see AGENTS.md's "Borg integration
# tests" / "Kopia storage integration tests" sections), against real,
# pinned borg/kopia binaries instead of whatever (if anything) happens to
# be on a contributor's or CI runner's PATH. Run via the "test_dackup"
# service in test/compose.yml (`make test-integration-docker`), which
# starts it with network_mode: host so it reaches the emulator containers
# the same way the host-executed `make test-integration` does — through
# their published localhost ports — without touching any of the
# integration tests' hardcoded "localhost:<port>" config.
FROM golang:1.26.2-bookworm

# kopia isn't packaged in Debian's apt repos, and unlike borg its exact
# version matters more here (behavior can vary enough across releases to
# affect the storage integration tests), so it's pinned explicitly rather
# than tracked via a rolling apt repo or "whatever is on PATH" — see the
# `kopia --version` check that motivated this Dockerfile in the first
# place: a locally installed kopia with no reproducible version at all.
ARG KOPIA_VERSION=0.23.1

# borgbackup is left unpinned, tracking whatever version Debian bookworm's
# own apt snapshot carries — consistent with the Makefile's own
# `deps-install` target, which installs it the same unpinned way.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
         borgbackup \
         rsync \
         make \
         curl \
         ca-certificates \
    && curl -fsSLo /tmp/kopia.deb \
         "https://github.com/kopia/kopia/releases/download/v${KOPIA_VERSION}/kopia_${KOPIA_VERSION}_linux_amd64.deb" \
    && apt-get install -y --no-install-recommends /tmp/kopia.deb \
    && rm -f /tmp/kopia.deb \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# Layer the module cache separately from the rest of the source so editing
# application code doesn't invalidate `go mod download`'s layer.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Fails the image build fast on a compile error, before any container ever
# starts and burns time waiting on the emulator services in compose.yml.
RUN make build

# Runs the whole suite (unit + integration — -tags=integration only adds
# the integration-tagged files, it doesn't exclude the regular unit tests)
# and enforces the same coverage threshold the "test" job checks for the
# unit-only suite; see test/scripts/run-tests.sh.
CMD ["test/scripts/run-tests.sh"]
