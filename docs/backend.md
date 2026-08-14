As a User, I want to be able to connect my `backup` and `restore` to a Backup Backend like `Borg`, `Kopia` or `rsync`.

## Status

Implemented: `internal/backend/borg` and `internal/backend/kopia`, both wired into `dackup backup`/`dackup restore` via `internal/backend.Factory`, plus the `dackup backend` CRUD command for configuring them. `AvailableBackends()` returns `["borg", "kopia"]`.

## Where this is documented

This design (the `Backend`/`GroupedBackend` interfaces, config file shape, backend resolution/dispatch, the `dackup backend` CRUD command, the borg and kopia implementations including kopia's per-storage-type subpackages, and the exact call sites in `cmd/backup`/`cmd/restore`) is documented in full in `../AGENTS.md`'s "Backend interface" section.

This file previously duplicated that content and drifted out of sync with it across the Borg and Kopia implementations. Rather than maintain two descriptions of the same design, this file now just points at `AGENTS.md` as the single source of truth — update that file when the backend design changes, not this one.
