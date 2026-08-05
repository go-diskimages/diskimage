<p align="center"><img src="https://raw.githubusercontent.com/go-diskimages/brand/main/social/go-diskimages.png" alt="go-diskimages/diskimage" width="720"></p>

# diskimage

Unified toolkit (library + CLI) for creating, converting, resizing, and
reading/writing files inside VM disk images (raw / DMG-UDIF, plus QCOW2 via
the CLI), across ext4/fat32/btrfs/xfs/zfs/exfat/apfs and MBR/GPT partition
tables, with optional LUKS and APFS FileVault encryption. Used to prepare
images for Apple Virtualization.framework VMs. GRUB-config patching lives in
the sibling [`go-bootloaders/grub`](https://github.com/go-bootloaders/grub)
package; OCI/Tart image extraction lives in the sibling
[`tart-oci`](https://github.com/go-diskimages/tart-oci) package.

Note: the CLI command was renamed from `diskimagec` to `diskimage`.

## Module

```
github.com/go-diskimages/diskimage
```

The orchestrator of the `go-diskimages` family: it composes
[`dmg`](https://github.com/go-diskimages/dmg) and
[`qcow2`](https://github.com/go-diskimages/qcow2) (image containers),
the [`go-filesystems`](https://github.com/go-filesystems) drivers (ext4,
fat32, btrfs, xfs, zfs, exfat, apfs, uefi), [`go-fde`](https://github.com/go-fde)
(APFS/LUKS full-disk encryption), and [`go-bootloaders/grub`](https://github.com/go-bootloaders/grub)
into one CLI + library. See `go.mod` for exact versions.

For raw→OCI/Tart image extraction, see the sibling
[`tart-oci`](https://github.com/go-diskimages/tart-oci) package.

## API

### Creating images

```go
func Create(opts CreateOptions) error
```

`CreateOptions` selects the container format (`FormatRaw` or `FormatDmg`),
partition scheme (`PartNone`, `PartMBR`, `PartGPT`), filesystem
(`FSNone`, `FSExt4`, `FSFat32`, `FSBtrfs`, `FSXfs`, `FSZfs`, `FSExFAT`,
`FSNTFS`, `FSApfs`), volume label, and — for DMG images — the UDIF
sub-format (`DmgUDIFFormat`) and an optional FileVault passphrase
(`DmgPassphrase`, APFS only). QCOW2 image creation is exposed by the CLI
(`diskimage create qcow2`) via the sibling `qcow2` package directly, not
through `CreateOptions`.

### Growing, resizing, converting

```go
func Grow(path string, sizeBytes int64) error
func ResizeImage(path string, newSizeBytes int64) error
func ConvertImageFormat(path, dstFormat string) error
```

`ResizeImage` and `ConvertImageFormat` are pure Go (via the `dmg`
package's UDIF codec) and work on all platforms — see
[UDIF / DMG](#udif--dmg) below.

### File operations inside an image

```go
func ReadFile(opts FileOptions) ([]byte, error)
func WriteFile(opts FileOptions, data []byte, perm os.FileMode) error
func Stat(opts FileOptions) (filesystem.Stat, error)
func Rename(opts FileOptions, newPath string) error
func DeleteFile(opts FileOptions) error
func DeleteDir(opts FileOptions) error
func MkDir(opts FileOptions, perm os.FileMode) error
func List(opts ListOptions) ([]ListEntry, error)
```

`FileOptions`/`ListOptions` name the image path, optional partition
index, and (auto-detected if empty) filesystem type; `ListOptions`
can also request populated `Mode`/`Size`/`Inode` via `FetchStat`.

### Filesystem detection

```go
func DetectFilesystem(imagePath string, partIndex int) (FilesystemType, error)
```

Probes magic bytes for ext4, fat32, btrfs, xfs, zfs, exfat.

### UEFI variable store

```go
func CreateUEFIVarsStore(path string, sizeBytes int64) (UEFIVarsStore, error)
```

`UEFIVarsSizeX86_64` (512 KiB, OVMF_VARS.fd-compatible) and
`UEFIVarsSizeARM64` (64 MiB, QEMU_VARS.fd-compatible) give the
conventional store sizes.

### Volume label (ext4)

```go
func SetExt4Label(imagePath string, partIndex int, label string) error
func Ext4Label(imagePath string, partIndex int) (string, error)
```

Reads or writes the ext4 `s_volume_name` of the partition at
`partIndex`. Works on raw, QCOW2, and UDIF-UDRW DMG inputs via the
unified `OpenBlockDevice` dispatcher, bridged into the ext4 driver
via a small in-package `ext4Adapter`. Label capped at 16 bytes; the
metadata-csum is refreshed using the kernel-canonical CRC-32C
convention (`crc32c(~0, sb[:0x3FC])`, no final XOR). Use it
**offline** — concurrent writers may produce a torn superblock.

### Block device access

`OpenBlockDevice` opens a raw, QCOW2, or UDIF-UDRW DMG image as a
uniform `BlockDevice`:

```go
dev, err := diskimage.OpenBlockDevice("disk.qcow2")
if err != nil { log.Fatal(err) }
defer dev.Close()

buf := make([]byte, 512)
dev.ReadAt(buf, 0)
```

The `BlockDevice` interface exposes `ReadAt`, `WriteAt`, `Size`, and
`Close`. The implementation auto-detects QCOW2 by magic bytes; UDIF
DMG by the `koly` trailer; everything else is treated as a raw file.

UDIF support is **UDRW-only in-place** — Read/Write are bounded to
the data fork (sector area at file offset 0), and `Close` refreshes
the koly trailer's `dataForkChecksum` + `masterChecksum` if anything
was written so the next `readAllUDIFSectors` open passes its master
checksum verification. Compressed UDIF subformats (UDRO / UDZO /
UDBZ / UDSP) are rejected with an error pointing at `dmg.UnpackToTemp`
+ `PackFromTemp` for the explicit unpack-edit-repack path.

### LUKS block devices

`OpenLUKSBlockDevice` layers LUKS decryption on top of a raw or QCOW2 image:

```go
dev, err := diskimage.OpenLUKSBlockDevice("disk.luks.qcow2", []byte("pass"))
if err != nil { log.Fatal(err) }
defer dev.Close()   // closes LUKS and the underlying QCOW2 device

buf := make([]byte, 4096)
dev.ReadAt(buf, 0)          // reads from the decrypted LUKS payload
dev.WriteAt(buf, 0)         // encrypts and writes back
```

Supported backing formats:

| Format | How detected |
|--------|-------------|
| Raw file | fallback (any path not recognized as QCOW2) |
| QCOW2 | magic bytes `QFI\xfb` at offset 0 |

Filesystem detection inside a backing image (see `detect.go`):

| Filesystem | How detected                                              |
| ---------- | --------------------------------------------------------- |
| APFS       | NX SuperBlock magic `"NXSB"` at offset 32 of block 0      |
| ext4       | superblock magic `0xEF53` at offset 0x438                 |
| FAT32      | OEM "MSWIN4.1" / FAT32 BPB signature in the boot sector   |
| NTFS       | OEM "NTFSIMG1" (test driver) or `"NTFS    "` (real NTFS)  |
| exFAT      | OEM `"EXFAT   "` at bytes 3-10 of the boot sector         |
| btrfs      | superblock magic `_BHRfS_M` at offset 0x10040             |
| XFS        | superblock magic `XFSB` at offset 0                       |
| ZFS        | vdev label magic across blocks 0/256K/end                 |

For raw files the LUKS header is opened directly from the file path.  
For QCOW2 files, the QCOW2 container is opened first and LUKS reads/writes
through it. `dev.Close()` closes both layers.

`dev.Size()` returns the size of the decrypted payload reported by the LUKS
device. For LUKS1 this is `0` (the LUKS1 format does not encode the payload
length in its header); for LUKS2 it reflects the configured payload length.


## Build requirements

Pure Go (`CGO_ENABLED=0`), cross-platform, all six supported 64-bit
architectures. A handful of darwin-only integration tests
(`dmg_convert_resize_integration_test.go`, `grub_test.go`) additionally
cross-check the pure-Go UDIF codec against real `hdiutil` output and the
`go-bootloaders/grub` patch helpers against real GRUB behavior on macOS;
they build under the `darwin` tag but are not part of the public API and
are not required to build or use the library on any platform.

## Used by

- `weft` (`github.com/openweft`) — extracts and prepares VM disk images
  for Apple Virtualization.framework VMs

## Notes

 - ZFS images: ZFS is supported on whole-disk and partitioned images. The ZFS driver will attempt to handle fat‑ZAP directory blocks by converting them to micro‑ZAP when possible. Conversion behaviour:
	 - If the ZAP header block is zeroed, a micro‑ZAP is written in-place (no new allocation).
	 - Otherwise the driver allocates a new block for the micro‑ZAP and updates the directory dnode to point to it.
	 - This is a pragmatic compatibility path (avoids implementing full fat‑ZAP write support) and has limitations — very large directories or long names may exceed micro‑ZAP capacity and cause write errors.
 - Partitioned images: allocation offsets are partition-aware; writes are performed at `partition base + allocator offset` to ensure correct physical placement inside partitioned images.
 - Debugging: the `integration` stress test's iteration count is configurable via
	 the `DISKIMAGE_STRESS_ITERS` environment variable (see `integration/RESOURCES.md`).

ZFS diagnostic commands
------------------------

Use these commands to inspect ZFS state and relevant files on the host:

```bash
# show pool health and errors
zpool status

# list datasets and properties
zfs list

# print a file's contents (replace <file_path> with the real path)
cat <file_path>
```

Example (run integration stress with ZFS/Btrfs debug enabled):

```bash
DISKIMAGE_STRESS_ITERS=1 \
	go test ./integration -run TestStress_FileOperations_AllFS_AllPartitions -v
```

See OpenZFS docs for low-level format details: https://openzfs.github.io/openzfs-docs/

## UDIF / DMG

- DMG is an Apple UDIF container which can be created in several forms:
	- **UDRW** (read-write): fixed-size at creation but resizable in-place.
	- **UDSP** (sparseimage / sparsebundle): grows automatically as files are added.

- This module exposes helpers, backed by the pure-Go
	[`dmg`](https://github.com/go-diskimages/dmg) codec, to detect, convert and
	resize UDIF images on **any platform** — no `hdiutil`/`plutil` required:
	- **CreateOptions.DmgUDIFFormat**: when creating a DMG with `Create`, set this
		field to `UDRW` or `UDSP` to request the underlying UDIF format (defaults to `UDRW`).
	- `ConvertImageFormat(path, dstFormat string) error` — convert an image in-place
		between UDIF formats (e.g. `UDRW` ↔ `UDSP`).
	- `ResizeImage(path string, newSizeBytes int64) error` — resize an image:
		- For **UDIF** images, uses the pure-Go UDIF resize (`UDRW` only).
		- For **UDSP** images the image must first be converted to `UDRW`.
		- For raw files the function falls back to truncation via `Grow`.

Notes and caveats:

- Converting `UDSP` → `UDRW` is supported, but resizing `UDSP` in-place is not —
	convert to `UDRW` first if you need to resize.

Example: create a UDRW APFS DMG and then convert to sparseimage and resize

```go
opts := diskimage.CreateOptions{
		Path: "/tmp/test.dmg",
		SizeBytes: 10 << 20,
		Format: diskimage.FormatDmg,
		Filesystem: diskimage.FSApfs,
		Label: "Example",
		DmgUDIFFormat: "UDRW",
}
if err := diskimage.Create(opts); err != nil {
		log.Fatal(err)
}

if err := diskimage.ConvertImageFormat(opts.Path, "UDSP"); err != nil {
		log.Fatal(err)
}

if err := diskimage.ConvertImageFormat(opts.Path, "UDRW"); err != nil {
		log.Fatal(err)
}

if err := diskimage.ResizeImage(opts.Path, 20*1024*1024); err != nil {
		log.Fatal(err)
}
```

Run the darwin-only integration tests to exercise these features locally:

```bash
GOWORK=off go test . -run TestCreateConvertAndResizeUDIF -v
```
