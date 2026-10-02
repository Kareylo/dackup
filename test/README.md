# Test fixtures

Everything in this directory is throwaway test material for the local
integration test stack (`test/compose.yml`). None of it protects anything
real.

- `secret.key` decrypts the `encrypted_*` fields in `config.*.json`.
- `restic_sftp_key/id_ed25519` is the SSH key the `test_restic_sftp`
  container accepts.
- `compose.yml` and the `config.*.json` files hold the emulator
  credentials (`minioadmin`, `dackuppass`, the Azurite development key).

These keys and credentials are public, because this repository is public.
Never use them, or a copy of them, outside this test stack: do not use
`secret.key` as a real `~/.config/dackup/secret.key`, and do not authorize
`id_ed25519.pub` on a real host.
