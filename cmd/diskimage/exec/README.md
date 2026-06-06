# exec package

This package implements the `diskimage exec` command, which runs coreutils-like operations directly inside disk images (no host shell-out required).

## Invocation

The entire command string — including its own arguments — must be passed as a single quoted argument so that the command's flags are not confused with the `diskimage exec` flags (`--file`, `--filesystem`, `--part`, `--path`):

```sh
diskimage exec "COMMAND [args...]" --file IMAGE [--filesystem TYPE] [--part N] [--path PATH]
```

## Supported commands

- `ls` — list directory contents.

  ```sh
  diskimage exec "ls"               --file disk.img
  diskimage exec "ls -alFh /etc"    --file disk.img
  diskimage exec "ls -a /var"       --file disk.img
  ```

- `cat` — print a file from inside the image.

  ```sh
  diskimage exec "cat /etc/hostname" --file disk.img
  ```

- `uefi` — inspect and modify UEFI variable stores (`.fd` files).

  ```sh
  diskimage exec "uefi list"                        --file OVMF_VARS.fd
  diskimage exec "uefi get BootOrder"               --file OVMF_VARS.fd
  diskimage exec "uefi set BootOrder --data 0100"   --file OVMF_VARS.fd
  diskimage exec "uefi delete BootOrder"            --file OVMF_VARS.fd
  ```

- `part print` — display the partition table (GPT/MBR).

  ```sh
  diskimage exec "part print" --file disk.img
  ```

- `df` — show filesystem usage.

  ```sh
  diskimage exec "df"    --file disk.img
  diskimage exec "df -h" --file disk.img
  ```

- `du` — show directory size summary.

  ```sh
  diskimage exec "du -sh" --file disk.img
  diskimage exec "du -sh /var" --file disk.img
  ```

- `zfs list` / `zfs get` / `zfs set` / `zfs create` — ZFS dataset operations.

  ```sh
  diskimage exec "zfs list"                   --file pool.img
  diskimage exec "zfs get quota pool/data"    --file pool.img
  diskimage exec "zfs set quota pool/data 1G" --file pool.img
  diskimage exec "zfs create pool/data"       --file pool.img
  ```

- `zpool status` — show ZFS pool health.

  ```sh
  diskimage exec "zpool status" --file pool.img
  ```

- `ntfs compact` — compact an NTFS image.

  ```sh
  diskimage exec "ntfs compact" --file disk.ntfs
  ```

- `xfs info` — show XFS geometry.

  ```sh
  diskimage exec "xfs info" --file disk.xfs
  ```

## Flags

| Flag | Description |
|---|---|
| `--file` | Path to the disk image (required for all commands) |
| `--filesystem` | Filesystem type: `ext4`, `fat32`, `btrfs`, `xfs`, `zfs`, `exfat`, `ntfs` (auto-detected if omitted) |
| `--part` | 0-based partition index (default `0` / whole image) |
| `--path` | Default directory path inside the filesystem (overridden by a path in the command string; default `/`) |

## Architecture

Dispatch is handled entirely through `execDispatch` in `exec_handlers.go`, which switches on `argv[0]` (the first token of the quoted command string). Each command is implemented in a dedicated `exec_cmd_<name>.go` file.

Files overview:

| File | Purpose |
|---|---|
| `exec.go` | `Command()` entry point — registers cobra command and flags |
| `exec_handlers.go` | `execRunE`, `execDispatch` — parses the quoted string and dispatches to per-command handlers |
| `exec_cmd_ls.go` | `ls` — directory listing |
| `exec_cmd_uefi.go` | `uefi` — UEFI variable store read/write |
| `exec_cmd_zfs.go` | `zfs` / `zpool` — ZFS dataset and pool operations |
| `exec_cmd_part.go` | `part` — partition table printing |
| `exec_cmd_df.go` | `df` — filesystem usage |
| `exec_cmd_du.go` | `du` — directory size summary |
| `exec_cmd_ntfs.go` | `ntfs` — NTFS compact |
| `exec_helpers.go` | Shared test hooks and utilities |

## Naming conventions

- Command implementations: `exec_cmd_<name>.go`.
- Dispatcher and shared helpers: `exec_<name>.go` (e.g. `exec_handlers.go`, `exec_helpers.go`).

## Adding a new command

1. Create `exec_cmd_<name>.go` with the command logic. Keep each function ≤ 50 lines.
2. Add a `case "<name>":` branch in `execDispatch` in `exec_handlers.go` that parses `argv` and calls your handler.
3. Parse command-level flags from `argv` (as existing handlers do) — do **not** add new global cobra flags.
4. Add unit tests in `exec_cmd_<name>_test.go`. Use the test hook pattern (package-level `var xxxFunc = realFunc`) to avoid hitting real disks.
5. Update this README with usage examples and a row in the files table.
6. Run formatting and tests locally:

```sh
gofmt -w pkg/go-diskimages/diskimage/cmd/diskimage/exec
go test ./pkg/go-diskimages/diskimage/cmd/diskimage/exec/...
```

## Notes

- `--filesystem` accepts: `ext4`, `fat32`, `btrfs`, `xfs`, `zfs`, `exfat`, `ntfs`. Auto-detected when omitted where supported.
- All commands operate on image internals; they do not invoke host `zfs`, `zpool`, or other binaries.
- Update this README when adding or changing commands so the repository doc-check pre-commit hook passes.
