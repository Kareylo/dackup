# AGENTS.md

## Project Overview

`dackup` is a Go CLI application for backing up and restoring Docker application data.

The application:

- Stops configured containers before staging data.
- Uses `TransferService` (currently implemented with `rsync`) to copy configured paths into a staging directory.
- Restarts containers immediately after staging completes to minimize downtime.
- Performs backups from the staged data using a backup backend.
- Restores staged data back to the live filesystem before restarting services.
- Fixes ownership with `chown` where appropriate.
- Restarts only containers that were stopped by the app.
- Supports interactive configuration management.
- Supports dry-run and verbose modes.

## Go Version

Use:

```bash
Go 1.26.0+
```

## Important Commands

Run formatting:

```bash
go fmt ./...
```

Run tests:

```bash
go test ./...
```

Build locally:

```bash
go build -o build/dackup .
```

## Current Architecture

The project is organized around top-level Cobra commands:

```text
.
├── cmd/
│ ├── backup/
│ │ ├── backup.go
│ │ └── backup_test.go
│ ├── config/
│ │ ├── config.go
│ │ ├── config_helper.go
│ │ ├── config_helper_test.go
│ │ └── config_test.go
│ ├── restore/
│ │ ├── restore.go
│ │ └── restore_test.go
│ ├── fixtures_test.go
│ ├── root.go
│ └── root_test.go
├── internal/
│ └── shared/
│   ├── command_runner.go
│   ├── container_lifecycle.go
│   ├── container_selection.go
│   ├── docker.go
│   ├── filesystem.go
│   ├── logger.go
│   ├── paths.go
│   ├── preflight.go
│   ├── prompts.go
│   ├── shared.go
│   └── transfer.go
└── main.go
```

Top-level command packages should expose constructors:

```go
func NewCommand(options *shared.Options) *cobra.Command
```

The root command wires commands together:

```go
rootCmd.AddCommand(backup.NewCommand(options))
rootCmd.AddCommand(config.NewCommand(options))
rootCmd.AddCommand(restore.NewCommand(options))
```

Avoid subcommand packages importing \`cmd\` directly. The dependency direction should be:

```text
main.go -> cmd
cmd/root.go -> cmd/backup -> cmd/config -> cmd/restore -> internal/shared
cmd/backup cmd/config cmd/restore -> internal/shared
```

Do not create import cycles.

## SOLID Refactor Decisions

The app was refactored toward SOLID principles.

### Single Responsibility

Shared infrastructure is split by concern:

- `command_runner.go` — command execution abstraction.
- `filesystem.go` — filesystem abstraction.
- `logger.go` — logging abstraction.
- `docker.go` — Docker CLI integration.
- `container_lifecycle.go` — stop/start orchestration.
- `container_selection.go` — container filtering and dependency selection.
- `paths.go` — path normalization and path resolution.
- `preflight.go` — prerequisite validation.
- `prompts.go` — interactive terminal prompting.
- `transfer.go` — staging data transfer and ownership operations.
- `shared.go` — shared config types and config file read/write helpers.

### Dependency Inversion

Command packages should depend on abstractions where practical:

```go
type CommandRunner interface {
    Run(name string, args ...string) error
    Output(name string, args ...string) ([]byte, error)
    LookPath(file string) (string, error)
}
```

```go
type FileSystem interface {
    Stat(name string) (os.FileInfo, error)
    MkdirAll(path string, perm os.FileMode) error
    OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)
}
```

```go
type Logger interface {
    Log(level string, message string)
}
```

This makes services easier to test and keeps implementation details replaceable.

### Open/Closed

The command code should not directly call `exec.Command` for business operations.

Instead, use:

```go
shared.CommandRunner
shared.LoggedCommandRunner
shared.DockerService
shared.TransferService
shared.ContainerLifecycleService
```

This enables future changes such as replacing `rsync` with another staging implementation without rewriting the backup/restore workflow.

## TransferService and rsync

The project deliberately separates **staging** from **backup**.

`TransferService` is **not** the backup engine. Its responsibility is to create an isolated staging copy of the application data while minimizing container downtime.

The current implementation uses `rsync`, but `rsync` is considered an implementation detail of `TransferService`, not the backup mechanism itself.

The backup workflow is:

```text
Stop containers
        ↓
TransferService (currently rsync)
        ↓
Start containers
        ↓
BackupBackend
```

The restore workflow is the inverse:

```text
RestoreBackend
        ↓
Stop containers
        ↓
TransferService (currently rsync)
        ↓
Fix ownership (if needed)
        ↓
Start containers
```

This separation exists because:

- Container downtime should only include the time required to stage data.
- Long-running backup operations should never operate on live application data.
- Backup implementations should operate exclusively on the staging directory.
- `TransferService` and the backup backend solve different problems and should remain independent.

Future backup backends may include:

- Kopia
- Restic
- Borg
- Rsync

These are **backup implementations**, not replacements for `TransferService`.

Likewise, `TransferService` may eventually support alternative staging implementations (filesystem snapshots, reflinks, etc.) without affecting backup backends.

## Docker Integration Decision

Use the Docker CLI for now, not the Docker socket/API.

Reasons:

- Smaller compiled binary.
- No Docker SDK dependency tree.
- Easier distribution.
- Consistent with existing external command usage.
- Current Docker operations are simple:
    - `docker ps`
    - `docker ps -a`
    - `docker stop`
    - `docker start`

Keep Docker access behind `DockerService`.

Future Podman support should be added by abstracting the engine command, for example:

```text
--container-engine docker
--container-engine podman
```

or via config:

```json
{ "container_engine": "podman" }
```

Do not switch to socket integration unless there is a strong reason.

## Shared Options

Global CLI flags are stored in:

```go
type Options struct {
    Verbose bool
    DryRun bool
}
```

Pass `*shared.Options` into command constructors.

Avoid direct cross-package globals.

## Command Registration

Do not register subcommands from package `init()` inside command packages.

Preferred:

```go
func NewCommand(options *shared.Options) *cobra.Command {
    cmd := &cobra.Command{ Use: "..." }
    return cmd
}
```

Then register in `cmd/root.go`.

## Backup/Restore Deduplication

Backup and restore share most mechanics:

- filtering selected containers,
- recursively including contained containers,
- stop/start lifecycle,
- preflight checks,
- path cleaning,
- staging via `TransferService`,
- logging.

Use shared services for these:

```go
shared.FilterContainerConfigs(...)
shared.ContainersToStopFromConfig(...)
shared.ContainerLifecycleService
shared.PreflightChecks(...)
shared.TransferService
```

Avoid duplicating equivalent logic between backup and restore packages.

## Config Command Notes

The config command currently keeps its subcommands in one package:

```text
cmd/config/
```

Possible future split:

```text
cmd/config/init/
cmd/config/add/
cmd/config/update/
cmd/config/usefile/
```

But do not split these prematurely. They share prompt and config-writing logic. Prefer extracting shared services first.

The prompt logic lives in:

```text
internal/shared/prompts.go
```

It contains:

```go
PromptService
ParseStringList
```

## File Naming Guidelines

Use specific file names that describe one responsibility.

Good examples:

```text
command_runner.go
filesystem.go
logger.go
docker.go
container_lifecycle.go
container_selection.go
paths.go
preflight.go
prompts.go
transfer.go
```

Avoid vague “bucket” files like:

```text
runtime.go
helpers.go
utils.go
common.go
```

unless there is a strong reason.

If creating a new file, explain why the file name was chosen.

## Tests

Keep tests close to the package they test:

```text
cmd/backup/backup_test.go
cmd/restore/restore_test.go
cmd/config/config_test.go
cmd/config/config_helper_test.go
cmd/root_test.go
```

For command package tests, prefer local test helpers instead of importing unexported helpers from another package.

Avoid tests depending on package-private symbols from unrelated packages.

## Compatibility Wrappers

Some unexported wrapper functions remain for tests and package compatibility, for example:

```go
filterConfigsForBackup(...)
containersToStopFromConfig(...)
cleanConfiguredPath(...)
```

These should delegate to shared services rather than contain duplicated logic.

Example:

```go
func filterConfigsForBackup(configs []shared.ContainerConfig, requestedContainers []string) ([]shared.ContainerConfig, error) {
    return shared.FilterContainerConfigs(configs, requestedContainers, "backup")
}
```

## Dry Run Behavior

Dry-run should not:

- write config files,
- create directories,
- perform staging transfers,
- invoke backup backends,
- stop containers,
- start containers,
- run `chown`.

It should log/print what would happen.

Always check:

```go
if options != nil && options.DryRun {
    // preview action
}
```

## Verbose Behavior

Verbose should print commands before running them.

Use `LoggedCommandRunner` for command execution that needs logging and verbose behavior.

## Root Privileges

Backup and restore require root:

```go
if os.Geteuid() != 0 {
    return fmt.Errorf("this command requires root privileges; run it with sudo")
}
```

Config commands should not require root.

## Config Files

The default config path is:

```text
~/.config/dackup/config.json
```

Shared config helpers live in \`internal/shared/shared.go\`.

Primary types:

```go
type ContainerConfig struct {
    Container string `json:"container"`
    ToStop bool `json:"to_stop"`
    Paths []string `json:"paths,omitempty"`
    Contains []string `json:"contains,omitempty"`
}
```

```go
type DackupConfig struct {
    User string `json:"user,omitempty"`
    Group string `json:"group,omitempty"`
    ConfigFile string `json:"config_file,omitempty"`
    BackupSrcDir string `json:"backup_src_dir,omitempty"`
    BackupDstDir string `json:"backup_dst_dir,omitempty"`
    Containers []ContainerConfig `json:"containers,omitempty"`
}
```

## Safety Notes

Be careful with restore behavior. Restoring staged data back to the live filesystem may overwrite or delete existing files depending on the transfer implementation.

Preserve dry-run functionality for destructive operations.

## After Any Refactor

Always run:

```bash
go fmt ./...
go test ./...
```

If available, also run the Makefile test target:

```bash
make test
```
