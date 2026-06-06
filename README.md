# diskimage

Unified toolkit for creating and converting VM disk images (raw / QCOW2 / OCI-Tart) and patching GRUB configurations in-place. Used to prepare images for Apple Virtualization.framework VMs.

Note: the CLI command was renamed from `diskimagec` to `diskimage`.

## Module

```
github.com/openweft/diskimage
```

Depends on [`ext4`](../go-filesystems/ext4) and [`lzfse`](../go-compressions/lzfse).

## API

### Raw images

```go
func CreateRaw(path string, sizeBytes int64) error
```

### QCOW2 → raw conversion

```go
func IsQCOW2File(path string) bool
func ConvertQCOW2ToRaw(src, dst string, w io.Writer) error
```

### OCI / Tart disk extraction *(darwin)*

```go
const TartDiskMediaType = "application/vnd.cirruslabs.tart.disk.v2"

func BlobPath(cacheDir, digest string) string
func ExtractOCIDisk(cacheDir, dst string, w io.Writer) error
```

### GRUB patching *(darwin)*

```go
func PatchGrubQuiet(diskPath string)   // removes `quiet` from GRUB cmdline
func PatchGrubConsole(diskPath string) // enables VGA + VirtIO console
```

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

macOS only for OCI and GRUB features (build tag `darwin`).

## Used by

- [`pkg/openweft/weft`](../apple-vz/vzd) — extracts and prepares VM disk images

## Notes

 - ZFS images: ZFS is supported on whole-disk and partitioned images. The ZFS driver will attempt to handle fat‑ZAP directory blocks by converting them to micro‑ZAP when possible. Conversion behaviour:
	 - If the ZAP header block is zeroed, a micro‑ZAP is written in-place (no new allocation).
	 - Otherwise the driver allocates a new block for the micro‑ZAP and updates the directory dnode to point to it.
	 - This is a pragmatic compatibility path (avoids implementing full fat‑ZAP write support) and has limitations — very large directories or long names may exceed micro‑ZAP capacity and cause write errors.
 - Partitioned images: allocation offsets are partition-aware; writes are performed at `partition base + allocator offset` to ensure correct physical placement inside partitioned images.
 - Debugging: enable verbose driver traces with environment variables to assist diagnosis:
	 - `DISKIMAGE_DEBUG` — global debug output
	 - `DISKIMAGE_ZFS_DEBUG` — ZFS-specific traces
	 - `DISKIMAGE_BTRFS_DEBUG` — Btrfs-specific traces

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
DISKIMAGE_ZFS_DEBUG=1 DISKIMAGE_BTRFS_DEBUG=1 DISKIMAGE_STRESS_ITERS=1 \
	go test ./integration -run TestStress_FileOperations_AllFS_AllPartitions -v
```

See OpenZFS docs for low-level format details: https://openzfs.github.io/openzfs-docs/

## Developer hooks

Install the local pre-commit hook (recommended) to enable automatic
documentation verification before commits:

```bash
mkdir -p .githooks
cp scripts/verify-docs.sh .githooks/pre-commit
chmod +x .githooks/pre-commit
git config core.hooksPath .githooks
```

To bypass the check for an individual commit (not recommended):

```bash
SKIP_DOC_CHECK=1 git commit -m "..."
# or
git commit --no-verify -m "..."
```

## UDIF / DMG (macOS)

- DMG is an Apple UDIF container which can be created in several forms:
	- **UDRW** (read-write): fixed-size at creation but resizable with `hdiutil`.
	- **UDSP** (sparseimage / sparsebundle): grows automatically as files are added.

- This module exposes helpers to detect, convert and resize UDIF images on macOS:
	- **CreateOptions.DmgUDIFFormat**: when creating a DMG with `Create`, set this
		field to `UDRW` or `UDSP` to request the underlying UDIF format (defaults to `UDRW`).
	- `ConvertImageFormat(path, dstFormat string) error` — convert an image in-place
		between UDIF formats (e.g. `UDRW` ↔ `UDSP`). Uses `hdiutil convert` on macOS.
	- `ResizeImage(path string, newSizeBytes int64) error` — resize an image:
		- For **UDRW** images on macOS the function uses `hdiutil resize`.
		- For **UDSP** images the image must first be converted to `UDRW`.
		- For raw files the function falls back to truncation.
		- For the repository's mock DMG container the payload is unpacked, grown, and repacked.

Notes and caveats:

- All UDIF operations (detect/convert/resize) call macOS `hdiutil`/`plutil` and
	are only supported when running on `darwin` and with `hdiutil` available.
- `hdiutil convert` may create files with different suffixes; `ConvertImageFormat`
	will choose the generated file and atomically replace the original.
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
GOWORK=off go test ./pkg/go-diskimages/diskimage -run TestCreateConvertAndResizeUDIF -v
```
