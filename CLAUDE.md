# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`dackup` is a Go CLI (Cobra-based) that backs up and restores Docker application data with `rsync`. It stops configured containers, transfers configured paths between a source root and a backup root, fixes ownership, and restarts only the containers it stopped.

The project was refactored toward SOLID principles (see `AGENTS.md` for the full rationale and decisions log — read it for anything not covered here, especially before adding new files or abstractions). Business logic lives in `internal/shared` behind small interfaces; `cmd/*` packages are thin Cobra wiring.

## Commands

```bash
go build -o build/dackup .   # build locally
make build                    # build via Makefile (runs `deps` first, strips symbols)
make test                     # go test ./... (runs `deps` first)
go fmt ./...                  # required after any refactor, per AGENTS.md
make install / make uninstall # install/remove binary at /usr/local/sbin/dackup (uses sudo if not root)
```

There is no separate lint target; `go vet ./...` is the natural check if needed. Go 1.26.2+ is required (go.mod).

## Architecture

```text
main.go -> cmd (root.go)
cmd/root.go -> cmd/backend, cmd/backup, cmd/config, cmd/restore
cmd/backend, cmd/backup, cmd/config, cmd/restore -> internal/shared
cmd/backend, cmd/backup, cmd/restore -> internal/backend
internal/backend -> internal/shared
```

Do not create import cycles, and avoid subcommand packages importing `cmd` directly.

- **cmd/root.go** — builds a single `*shared.Options{Verbose, DryRun}` from the persistent `--verbose`/`-v` and `--dry-run`/`-d` flags, then wires subcommands via `backend.NewCommand(options)`, `backup.NewCommand(options)`, `config.NewCommand(options)`, `restore.NewCommand(options)`. Each subcommand package exposes a `NewCommand(*shared.Options) *cobra.Command` constructor rather than registering itself from `init()`.
- **cmd/backup**, **cmd/restore** — each builds a `commandService` (fs, command runner, logger, path resolver, `TransferService`) via a local `newCommandService()`, then runs: resolve effective config → resolve the configured `Backend` via `resolveBackend` (fails fast on an unknown backend name, before touching containers) → filter requested containers → preflight checks → stop containers → transfer paths → fix ownership → restart containers → (backup only) `Backend.Backup(stagingDir)`. `restore` calls `Backend.Restore(stagingDir)` right after preflight instead, *before* stopping containers, so the potentially slow restore-from-backend-storage step happens before the downtime-critical stop/stage/start sequence — see `AGENTS.md`'s "Backend interface" section for the exact call sites. `restore.go` is a near-mirror of `backup.go` with its own `restore*`-prefixed functions rather than shared code at the `cmd` layer — when fixing a bug in one, check whether the other needs the same fix. Restore swaps direction: it defaults `restoreSrcDir`/`restoreDstDir` to the config's `staging_dir`/`data_dir` (i.e. it reads from the staging location by default) unless `--src-dir`/`--dst-dir` are explicitly passed.
- **cmd/config** — the `config` subcommands (`init`, `add`, `update`, `remove`, `list`, `use-file`). `init`/`add`/`update`/`remove`/`use-file` are interactive via `shared.PromptService` over `bufio.Reader`; `list` is the exception — read-only and non-interactive, it just prints the effective config path and containers (via the shared `printContainers` helper) and never prompts or writes. `remove` lists containers, prompts for one, confirms (defaults to no), then rewrites the config without it, printing a warning if any remaining container's `contains` still references the removed name — it does not auto-clean those references. `config_helper.go` in this package is a thin compatibility shim (`dackupConfig` type alias plus wrapper functions) over `internal/shared`'s config I/O — new code should call `internal/shared` directly rather than adding to this shim.
- **cmd/backend** — the `backend` subcommands (`create`, `show`, `update`, `remove`), mirroring `cmd/config`'s interactive style for a singleton value (the `Backend`/`BackendSettings` fields on the main config) rather than a list. `show` is read-only like `config list`. Selecting a backend name and prompting for its settings goes through `internal/backend.AvailableBackends()` and a local per-backend settings-prompt switch; since no concrete backend is registered yet, `create`/`update` currently just report that nothing is implemented.
- **internal/shared** — the actual business logic, split one-concern-per-file (see file list below). Command packages depend on the interfaces here (`CommandRunner`, `FileSystem`, `Logger`) so services are testable and implementations are replaceable.
- **internal/backend** — the `Backend` interface, `Factory.GetBackend`, `AvailableBackends()`, and `ParseSettings`. Concrete implementations live in their own subpackage (`internal/backend/defaultbackend/` today, `internal/backend/borg/` once that exists) — `internal/backend` itself never holds one. See "TransferService vs. a backup backend" below and `AGENTS.md`'s "Backend interface" section for the full design.

### internal/shared files

| File | Responsibility |
| --- | --- |
| `shared.go` | `DackupConfig`/`ContainerConfig` types, config JSON read/write, `EffectiveDackupConfig`/`EffectiveContainersConfigPath` (see config layering below) |
| `command_runner.go` | `CommandRunner` interface + `OSCommandRunner`; `LoggedCommandRunner` wraps a runner to log stdout/stderr to a file and print the command when `--verbose` |
| `filesystem.go` | `FileSystem` interface + `OSFileSystem`, for testable `Stat`/`MkdirAll`/`OpenFile` |
| `logger.go` | `Logger` interface + `FileLogger`, timestamped `[LEVEL] message` lines to stdout and a log file |
| `docker.go` | `DockerService` — `docker ps` based `ContainerRunning`/`ContainerExists` checks |
| `container_lifecycle.go` | `ContainerLifecycleService.StopRunningContainers`/`StartStoppedContainers` — only restarts containers that were actually stopped |
| `container_selection.go` | `FilterContainerConfigs`/`SelectContainerAndContained` — recursive, cycle-safe `contains` dependency resolution; `ContainersToStopFromConfig` |
| `paths.go` | `PathResolver` — resolves a configured path under a source/destination root; `CleanConfiguredPath` strips the leading separator |
| `preflight.go` | `PreflightChecks` — validates config fields, source/dest dirs, `docker`/`rsync` on `PATH`, and that every configured path exists |
| `prompts.go` | `PromptService` — interactive prompt helpers (`RequiredString`, `Bool`, `StringList`, ...) used by `cmd/config` |
| `transfer.go` | `TransferService` — the staging copy (`rsync -a --delete`) in either direction, plus `FixBackupOwnership`/`FixRestoreOwnership` (`chown -R`) |

### Key domain concepts

- **TransferService vs. a backup backend**: `TransferService` (currently `rsync`) is deliberately treated as a *staging* mechanism, not the backup mechanism itself — the intent is that container downtime should only cover the staging copy, and a separate backup backend (rsync/restic/kopia/borg, per the README TODO) operates on the staged data without touching live app data. A single, merged `Backend` interface (`Name()`/`Backup(stagingDir)`/`Restore(stagingDir)`) exists in `internal/backend/backend.go` (config-agnostic by design — see `AGENTS.md`'s "Backend interface" section), along with `internal/backend/registry.go`'s `AvailableBackends()`, `internal/backend/settings.go`'s `ParseSettings`, and `internal/backend/factory.go`'s `Factory.GetBackend`. `internal/backend/defaultbackend.Backend` is a no-op loaded whenever no backend is configured, kept in its own subpackage rather than in `internal/backend` itself, matching where a real backend (e.g. `internal/backend/borg/`) will live — `backend.DefaultBackendName` re-exports its `Name` constant for convenience. `DackupConfig` carries optional `Backend`/`BackendSettings` fields (empty/absent means the default backend), managed via the `dackup backend` CRUD command (`create`/`show`/`update`/`remove`, mirroring `cmd/config`'s interactive style). `cmd/backup`/`cmd/restore` are wired to it (see the `cmd/backup`, `cmd/restore` bullet above for the exact call sites) — but **there is still no concrete backend implementation**: `AvailableBackends()` currently returns nothing, so `Factory.GetBackend` always resolves to `defaultbackend.Backend` in practice, and every `Backup()`/`Restore()` call today is a no-op. Check `AGENTS.md` before adding a concrete backend (e.g. rsync-as-backend, Borg).
- **Container selection & `contains`**: when specific containers are requested on the CLI, `shared.FilterContainerConfigs` selects them plus their `contains` dependents, recursively (cycle-safe — see `SelectContainerAndContained`). Requesting an unconfigured container is an error.
- **Path resolution**: each container's `paths` entries are cleaned (`filepath.Clean`, leading separator stripped via `CleanConfiguredPath`) and joined under the effective source/destination root via `PathResolver`. Duplicate paths across containers are only transferred once per run.
- **Backup vs restore direction**: backup defaults to `data_dir` → `staging_dir`; restore defaults to `staging_dir` → `data_dir` (restore just reverses the two roots unless overridden by flags). A third root, `backend_dir`, holds the configured backend's durable storage (e.g. Borg repositories) and is only used by backends that need local storage — the default no-op backend ignores it.
- **Config file layering**: a main config file (default `~/.config/dackup/config.json`) holds `user`, `group`, dir defaults, and either an inline `containers` list or a `config_file` pointer to a separate JSON file holding `containers`. Always resolve through `shared.EffectiveDackupConfig`/`shared.EffectiveContainersConfigPath` rather than reading `containers` off a raw parsed config, since the containers may live in a different file.
- **`--dry-run`**: every mutating action (stopping/starting containers, transfer, chown, writing config files) checks `Options.DryRun` and logs what it *would* do instead of executing. Follow this pattern for any new mutating operation, and never let dry-run write config files, create directories, transfer data, stop/start containers, or run `chown`. `cmd/backup`/`cmd/restore` call `Backend.Backup()`/`Restore()` unconditionally regardless of `--dry-run` — a concrete backend must self-guard with its own `Options.DryRun` check, same as `TransferService`'s methods do, rather than the call site skipping the call.
- **No root requirement**: `backup`/`restore` do not check `os.Geteuid()` — they run as whatever user invokes them. Permission problems (stopping/starting containers, `chown` to the configured `user`/`group`) surface as ordinary errors at the point they occur, not as an upfront gate. This was deliberately removed; don't reintroduce it without discussing first.
- **Docker via CLI, not the socket/API**: deliberate choice for binary size/distribution simplicity (see AGENTS.md). Keep Docker access behind `DockerService`; if adding Podman support, abstract the engine command rather than switching to socket integration.
- **Compatibility wrapper functions**: `cmd/backup` and `cmd/restore` retain small unexported functions (e.g. `filterConfigsForBackup`, `containersToStopFromConfig`, `cleanConfiguredPath`) that just delegate to `internal/shared`, kept for existing tests. New logic should go in `internal/shared`, not these wrappers.

## Testing conventions

Tests live alongside their package, plus a shared `cmd/fixtures_test.go` providing test helpers (`touchFile`, `assertPathEqual`, etc.). Per AGENTS.md, prefer local test helpers over importing unexported symbols from another package, and always generate unit tests for new code as part of the same change.
