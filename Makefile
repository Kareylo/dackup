APP_NAME := dackup
BUILD_DIR := build
BIN := $(BUILD_DIR)/$(APP_NAME)
INSTALL_DIR := /usr/local/sbin
INSTALL_BIN := $(INSTALL_DIR)/$(APP_NAME)

GO ?= go
GOFLAGS ?=
LDFLAGS ?= -s -w

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  make deps       Install required dependencies when possible"
	@echo "  make build      Build $(APP_NAME)"
	@echo "  make test       Run tests"
	@echo "  make install    Build and install $(APP_NAME) to $(INSTALL_BIN)"
	@echo "  make uninstall  Remove $(INSTALL_BIN)"
	@echo "  make clean      Remove build artifacts"

.PHONY: deps
deps:
	@echo "Installing dependencies..."
	@if command -v go >/dev/null 2>&1; then \
		echo "Go is already installed: $$(go version)"; \
	else \
		echo "Go is not installed."; \
		if command -v apt-get >/dev/null 2>&1; then \
			echo "Using apt-get..."; \
			sudo apt-get update; \
			sudo apt-get install -y golang-go rsync; \
		elif command -v dnf >/dev/null 2>&1; then \
			echo "Using dnf..."; \
			sudo dnf install -y golang rsync; \
		elif command -v yum >/dev/null 2>&1; then \
			echo "Using yum..."; \
			sudo yum install -y golang rsync; \
		elif command -v pacman >/dev/null 2>&1; then \
			echo "Using pacman..."; \
			sudo pacman -Sy --needed go rsync; \
		elif command -v zypper >/dev/null 2>&1; then \
			echo "Using zypper..."; \
			sudo zypper install -y go rsync; \
		elif command -v apk >/dev/null 2>&1; then \
			echo "Using apk..."; \
			sudo apk add go rsync; \
		elif command -v pkg >/dev/null 2>&1; then \
			echo "Using pkg..."; \
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
	@echo "Downloading Go module dependencies..."
	$(GO) mod download

.PHONY: build
build: deps
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BIN) .
	@echo "Built: $(BIN)"

.PHONY: test
test: deps
	$(GO) test ./...

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
