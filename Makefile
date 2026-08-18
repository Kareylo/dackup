APP_NAME := dackup
BUILD_DIR := build
BIN := $(BUILD_DIR)/$(APP_NAME)
INSTALL_DIR := /usr/local/sbin
INSTALL_BIN := $(INSTALL_DIR)/$(APP_NAME)

GO ?= go
GOFLAGS ?=
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS ?= -s -w -X dackup/cmd/version.Version=$(VERSION)

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  make deps                Check required dependencies are installed"
	@echo "  make deps-install        Install missing dependencies (uses sudo)"
	@echo "  make build               Build $(APP_NAME)"
	@echo "  make test                Run tests"
	@echo "  make test-integration    Start test/compose.yml and run kopia/restic storage + borg integration tests"
	@echo "  make test-integration-docker  Same, but build+run inside test/Dockerfile (pinned borg/kopia/restic, no host install needed)"
	@echo "  make test-integration-down  Stop the containers started by test-integration"
	@echo "  make install             Build and install $(APP_NAME) to $(INSTALL_BIN)"
	@echo "  make uninstall           Remove $(INSTALL_BIN)"
	@echo "  make clean               Remove build artifacts"

.PHONY: deps
deps:
	@echo "Checking dependencies..."
	@if command -v go >/dev/null 2>&1; then \
		echo "Go is already installed: $$(go version)"; \
	else \
		echo "Go is not installed."; \
		echo "Run 'make deps-install' to install it (uses sudo), or install Go manually."; \
		exit 1; \
	fi
	@if command -v rsync >/dev/null 2>&1; then \
		echo "rsync is installed."; \
	else \
		echo "WARNING: rsync is not installed. Run 'make deps-install' or install it manually."; \
	fi
	@if command -v docker >/dev/null 2>&1; then \
		echo "Docker CLI is installed."; \
	else \
		echo "WARNING: Docker CLI is not installed."; \
		echo "Install Docker manually for backup/restore commands to work."; \
	fi
	@echo "Downloading Go module dependencies..."
	$(GO) mod download

.PHONY: deps-install
deps-install:
	@echo "Installing dependencies..."
	@if command -v go >/dev/null 2>&1; then \
		echo "Go is already installed: $$(go version)"; \
	else \
		echo "Go is not installed."; \
		if command -v apt-get >/dev/null 2>&1; then \
			echo "Using apt-get (sudo)..."; \
			sudo apt-get update; \
			sudo apt-get install -y golang-go rsync; \
		elif command -v dnf >/dev/null 2>&1; then \
			echo "Using dnf (sudo)..."; \
			sudo dnf install -y golang rsync; \
		elif command -v yum >/dev/null 2>&1; then \
			echo "Using yum (sudo)..."; \
			sudo yum install -y golang rsync; \
		elif command -v pacman >/dev/null 2>&1; then \
			echo "Using pacman (sudo)..."; \
			sudo pacman -Sy --needed go rsync; \
		elif command -v zypper >/dev/null 2>&1; then \
			echo "Using zypper (sudo)..."; \
			sudo zypper install -y go rsync; \
		elif command -v apk >/dev/null 2>&1; then \
			echo "Using apk (sudo)..."; \
			sudo apk add go rsync; \
		elif command -v pkg >/dev/null 2>&1; then \
			echo "Using pkg (sudo)..."; \
			sudo pkg install -y go rsync; \
		elif command -v brew >/dev/null 2>&1; then \
			echo "Using Homebrew..."; \
			brew install go rsync; \
		else \
			echo "No supported package manager found."; \
			echo "Please install Go and rsync manually."; \
			exit 1; \
		fi; \
	fi
	@if command -v rsync >/dev/null 2>&1; then \
		echo "rsync is installed."; \
	else \
		echo "WARNING: rsync is not installed. Please install it manually if dependency installation did not do it."; \
	fi
	@if command -v docker >/dev/null 2>&1; then \
		echo "Docker CLI is installed."; \
	else \
		echo "WARNING: Docker CLI is not installed."; \
		echo "Install Docker manually for backup/restore commands to work."; \
	fi

.PHONY: build
build: deps
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BIN) .
	@echo "Built: $(BIN)"

.PHONY: test
test: deps
	$(GO) test -cover ./...

.PHONY: test-integration
test-integration: deps
	@if ! command -v docker >/dev/null 2>&1; then \
		echo "Docker is required for integration tests; see 'make deps'."; \
		exit 1; \
	fi
	@echo "Starting test/compose.yml storage emulator containers..."
	docker compose -f test/compose.yml up -d
	@echo "Waiting for bucket/container setup to finish..."
	@for svc in test_minio_init test_azurite_init test_gcs_init; do \
		code=$$(docker wait $$svc); \
		if [ "$$code" != "0" ]; then \
			echo "$$svc failed (exit $$code); see: docker logs $$svc" >&2; \
			exit 1; \
		fi; \
	done
	@echo "Running kopia/restic storage and borg integration tests..."
	$(GO) test -tags=integration -cover ./...

.PHONY: test-integration-docker
test-integration-docker:
	@if ! command -v docker >/dev/null 2>&1; then \
		echo "Docker is required for integration tests; see 'make deps'."; \
		exit 1; \
	fi
	@echo "Starting test/compose.yml storage emulator containers..."
	docker compose -f test/compose.yml up -d
	@echo "Waiting for bucket/container setup to finish..."
	@for svc in test_minio_init test_azurite_init test_gcs_init; do \
		code=$$(docker wait $$svc); \
		if [ "$$code" != "0" ]; then \
			echo "$$svc failed (exit $$code); see: docker logs $$svc" >&2; \
			exit 1; \
		fi; \
	done
	@echo "Building dackup and running kopia/restic storage + borg integration tests inside test/Dockerfile..."
	docker compose -f test/compose.yml --profile docker-tests run --rm test_dackup

.PHONY: test-integration-down
test-integration-down:
	@echo "Stopping test/compose.yml containers..."
	docker compose -f test/compose.yml down -v

.PHONY: install
install: build
	@echo "Installing $(APP_NAME) to $(INSTALL_BIN)..."
	@if [ "$$(id -u)" -ne 0 ]; then \
		echo "Root privileges are required to install to $(INSTALL_DIR)."; \
		echo "Re-running install with sudo..."; \
		sudo install -d -m 0755 $(INSTALL_DIR); \
		sudo install -m 0755 $(BIN) $(INSTALL_BIN); \
	else \
		install -d -m 0755 $(INSTALL_DIR); \
		install -m 0755 $(BIN) $(INSTALL_BIN); \
	fi
	@echo "Installed: $(INSTALL_BIN)"

.PHONY: uninstall
uninstall:
	@echo "Uninstalling $(APP_NAME)..."
	@if [ "$$(id -u)" -ne 0 ]; then \
		echo "Root privileges are required to remove $(INSTALL_BIN)."; \
		echo "Re-running uninstall with sudo..."; \
		sudo rm -f $(INSTALL_BIN); \
	else \
		rm -f $(INSTALL_BIN); \
	fi
	@echo "Removed: $(INSTALL_BIN)"

.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
