package diskimage

import (
	"fmt"
	"io"
	"os"

	disk_dmg "github.com/go-diskimages/dmg"
	fde_apfs "github.com/go-fde/apfs"
	filesystem_apfs "github.com/go-filesystems/apfs"
	filesystem_btrfs "github.com/go-filesystems/btrfs"
	filesystem_exfat "github.com/go-filesystems/exfat"
	filesystem_ext4 "github.com/go-filesystems/ext4"
	filesystem_fat32 "github.com/go-filesystems/fat32"
	filesystem_ntfs "github.com/go-filesystems/ntfs"
	filesystem_xfs "github.com/go-filesystems/xfs"
	filesystem_zfs "github.com/go-filesystems/zfs"
)

// ImageFormat is the container format for the disk image.
type ImageFormat string

// PartitionScheme describes the partition table placed in the image.
type PartitionScheme string

// FilesystemType identifies which filesystem to write inside the image.
type FilesystemType string

const (
	FormatRaw ImageFormat = "raw"
	FormatDmg ImageFormat = "dmg"

	PartNone PartitionScheme = "none"
	PartMBR  PartitionScheme = "mbr"
	PartGPT  PartitionScheme = "gpt"

	FSNone  FilesystemType = "none"
	FSExt4  FilesystemType = "ext4"
	FSFat32 FilesystemType = "fat32"
	FSBtrfs FilesystemType = "btrfs"
	FSXfs   FilesystemType = "xfs"
	FSZfs   FilesystemType = "zfs"
	FSExFAT FilesystemType = "exfat"
	FSNTFS  FilesystemType = "ntfs"
	FSApfs  FilesystemType = "apfs"
)

var (
	// ValidFormats lists accepted --format values.
	ValidFormats = []string{"raw", "dmg"}
	// ValidPartSchemes lists accepted --part values.
	ValidPartSchemes = []string{"none", "mbr", "gpt"}
	// ValidFilesystems lists accepted --filesystem values.
	ValidFilesystems = []string{"none", "ext4", "fat32", "btrfs", "xfs", "zfs", "exfat", "ntfs", "apfs"}
	// DmgSupportedFilesystems is the set of filesystems that may be embedded
	// in a DMG image. Only filesystems natively understood by macOS are
	// allowed: APFS (with optional FileVault FDE), FAT32 and exFAT.
	DmgSupportedFilesystems = []string{"none", "apfs", "fat32", "exfat"}
)

// IsDmgSupportedFilesystem reports whether fs is allowed inside a DMG image.
func IsDmgSupportedFilesystem(fs FilesystemType) bool {
	for _, ok := range DmgSupportedFilesystems {
		if string(fs) == ok {
			return true
		}
	}
	return false
}

// CreateOptions configures the disk image to create.
type CreateOptions struct {
	// Path is the output file path.
	Path string
	// SizeBytes is the total image size in bytes.
	SizeBytes int64
	// Format is the image container format (default: raw).
	Format ImageFormat
	// Partition is the partition table scheme (default: none).
	Partition PartitionScheme
	// Filesystem is the filesystem to write in the (first) partition or bare
	// image. Use FSNone to create an image/partition without any filesystem.
	Filesystem FilesystemType
	// Label is passed to the filesystem formatter as volume label / pool name.
	Label string
	// DmgUDIFFormat, when creating a DMG on darwin, selects the UDIF format
	// to use (e.g. "UDRW" or "UDSP"). If empty defaults to "UDRW".
	DmgUDIFFormat string
	// DmgPassphrase, when non-empty and Format=FormatDmg with Filesystem=FSApfs,
	// requests a FileVault-encrypted (FDE) APFS DMG. The passphrase is fed to
	// pkg/go-fde/apfs which writes the APFS NX superblock and key bag using
	// PBKDF2-SHA256 + AES Key Wrap (RFC 3394) over AES-256-XTS payload. It is
	// only used in-process (never persisted in argv).
	DmgPassphrase []byte
}

// Create creates a disk image according to opts.
//
// For --part none the filesystem is written directly at offset 0 of the image.
// For --part mbr/gpt the partition table is written first, then the filesystem
// is written at the partition offset (first partition, aligned to 1 MiB).
func Create(opts CreateOptions) error {
	if opts.Path == "" {
		return fmt.Errorf("diskimage: path is required")
	}
	if opts.SizeBytes <= 0 {
		return fmt.Errorf("diskimage: size must be positive")
	}
	if opts.Format == "" {
		opts.Format = FormatRaw
	}
	if opts.Partition == "" {
		opts.Partition = PartNone
	}
	if opts.Filesystem == "" {
		opts.Filesystem = FSNone
	}

	if opts.Format == FormatDmg {
		if !IsDmgSupportedFilesystem(opts.Filesystem) {
			return fmt.Errorf("diskimage: filesystem %q is not supported in DMG images (supported: %v)", opts.Filesystem, DmgSupportedFilesystems)
		}
	}
	if len(opts.DmgPassphrase) > 0 {
		if opts.Format != FormatDmg {
			return fmt.Errorf("diskimage: passphrase is only valid for DMG images")
		}
		if opts.Filesystem != FSApfs {
			return fmt.Errorf("diskimage: passphrase is only valid for APFS DMG images")
		}
	}

	switch opts.Format {
	case FormatRaw:
		return createRaw(opts)
	case FormatDmg:
		if opts.Filesystem == FSApfs && len(opts.DmgPassphrase) > 0 {
			if err := createEncryptedApfsDmg(opts); err != nil {
				return err
			}
			return disk_dmg.WrapRaw(opts.Path)
		}
		if err := createRaw(opts); err != nil {
			return err
		}
		// APFS DMGs default to raw layout (no UDIF wrap) — matches
		// what Apple's `hdiutil create -fs APFS` produces, which
		// hdiutil auto-detects as `Class Name: CRawDiskImage` and
		// mounts cleanly. Wrapping in UDIF would just add a koly
		// trailer hdiutil mis-reads as a malformed blkx range.
		// Callers can still opt into explicit UDIF wrapping by
		// setting CreateOptions.DmgUDIFFormat (e.g. "UDRW", "UDZO").
		if opts.Filesystem == FSApfs && opts.DmgUDIFFormat == "" {
			return nil
		}
		return disk_dmg.WrapRaw(opts.Path)
	default:
		return fmt.Errorf("diskimage: unsupported format %q (supported: %v)", opts.Format, ValidFormats)
	}
}

// createEncryptedApfsDmg writes a FileVault-encrypted APFS NX container at
// opts.Path using pkg/go-fde/apfs. The output file is sized to opts.SizeBytes
// and contains: a plaintext NX superblock (block 0), a plaintext key bag
// (block 1) protecting the wrapped KEK/VEK with the supplied passphrase, and
// AES-256-XTS-encrypted payload from block 2 onwards.
func createEncryptedApfsDmg(opts CreateOptions) error {
	const fdeHeaderBytes = int64(8192) // NX superblock (4096) + key bag (4096)
	if opts.SizeBytes <= fdeHeaderBytes {
		return fmt.Errorf("diskimage: image size %d is too small for an encrypted APFS DMG (need > %d bytes)", opts.SizeBytes, fdeHeaderBytes)
	}
	f, err := os.OpenFile(opts.Path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("diskimage: create %s: %w", opts.Path, err)
	}
	if err := f.Truncate(opts.SizeBytes); err != nil {
		f.Close()
		os.Remove(opts.Path)
		return fmt.Errorf("diskimage: truncate: %w", err)
	}
	f.Close()
	dev, err := fde_apfs.Format(opts.Path, opts.DmgPassphrase)
	if err != nil {
		os.Remove(opts.Path)
		return fmt.Errorf("diskimage: apfs fde format: %w", err)
	}
	if err := dev.Close(); err != nil {
		return fmt.Errorf("diskimage: apfs fde close: %w", err)
	}
	return nil
}

// createRaw creates a raw sparse image and optionally writes a partition table
// and/or formats a filesystem.
func createRaw(opts CreateOptions) error {
	const sectorSize = int64(512)
	const alignedStart = int64(2048) // sectors — 1 MiB boundary (conventional)

	// Create or truncate the output file.
	f, err := os.OpenFile(opts.Path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("diskimage: create %s: %w", opts.Path, err)
	}
	if err := f.Truncate(opts.SizeBytes); err != nil {
		f.Close()
		os.Remove(opts.Path)
		return fmt.Errorf("diskimage: truncate: %w", err)
	}
	f.Close()

	// Determine the filesystem region.
	var partOffset, partSize int64
	switch opts.Partition {
	case PartNone, "":
		partOffset = 0
		partSize = opts.SizeBytes

	case PartMBR:
		partOffset = alignedStart * sectorSize
		partSize = alignDown(opts.SizeBytes-partOffset, 4096)
		if partSize <= 0 {
			return fmt.Errorf("diskimage: image too small for MBR + 1 MiB alignment")
		}
		if err := writeMBR(opts.Path, opts.SizeBytes, mbrPartType(opts.Filesystem)); err != nil {
			return err
		}

	case PartGPT:
		// 33 sectors at the end are reserved for backup GPT entries + backup header.
		partOffset = alignedStart * sectorSize
		partSize = alignDown(opts.SizeBytes-partOffset-33*sectorSize, 4096)
		if partSize <= 0 {
			return fmt.Errorf("diskimage: image too small for GPT + 1 MiB alignment")
		}
		if err := writeGPT(opts.Path, opts.SizeBytes, gptPartTypeGUID(opts.Filesystem)); err != nil {
			return err
		}

	default:
		return fmt.Errorf("diskimage: unsupported partition scheme %q (supported: %v)", opts.Partition, ValidPartSchemes)
	}

	if opts.Filesystem == FSNone || opts.Filesystem == "" {
		return nil
	}

	return formatFilesystemAt(opts.Path, partOffset, partSize, opts.Filesystem, opts.Label)
}

// formatFilesystemAt formats opts.Filesystem into rawPath at partOffset.
// When partOffset > 0 a temp file is used to get an offset-0 image which is
// then spliced into the raw image at the partition boundary.
func formatFilesystemAt(rawPath string, partOffset, partSize int64, fs FilesystemType, label string) error {
	// ZFS is supported on partitioned images; the driver handles fat-ZAP by
	// converting fat-ZAP directories to micro-ZAP when possible.
	// The allocator must be initialized by the ZFS open path before writes.
	if partOffset == 0 {
		return formatFilesystem(rawPath, partSize, fs, label)
	}

	// Format filesystem into a temporary file, then copy into the raw image.
	tmp, err := os.CreateTemp("", "diskimage-*")
	if err != nil {
		return fmt.Errorf("diskimage: temp file: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	if err := formatFilesystem(tmpPath, partSize, fs, label); err != nil {
		return err
	}

	src, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("diskimage: open temp: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(rawPath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("diskimage: open image for write: %w", err)
	}
	defer dst.Close()

	if _, err := dst.Seek(partOffset, io.SeekStart); err != nil {
		return fmt.Errorf("diskimage: seek to partition offset: %w", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("diskimage: splice filesystem into image: %w", err)
	}
	return nil
}

// formatFilesystem creates a bare filesystem image at path.
func formatFilesystem(path string, sizeBytes int64, fs FilesystemType, label string) error {
	switch fs {
	case FSExt4:
		_, err := filesystem_ext4.Format(path, sizeBytes, filesystem_ext4.FormatConfig{Label: label})
		return err
	case FSFat32:
		_, err := filesystem_fat32.Format(path, sizeBytes, filesystem_fat32.FormatConfig{Label: label})
		return err
	case FSBtrfs:
		_, err := filesystem_btrfs.Format(path, sizeBytes, filesystem_btrfs.FormatConfig{Label: label})
		return err
	case FSXfs:
		_, err := filesystem_xfs.Format(path, sizeBytes, filesystem_xfs.FormatConfig{Label: label})
		return err
	case FSZfs:
		poolName := label
		if poolName == "" {
			poolName = "data"
		}
		_, err := filesystem_zfs.Format(path, sizeBytes, filesystem_zfs.FormatConfig{PoolName: poolName})
		return err
	case FSExFAT:
		_, err := filesystem_exfat.Format(path, sizeBytes, filesystem_exfat.FormatConfig{Label: label})
		return err
	case FSNTFS:
		_, err := filesystem_ntfs.Format(path, sizeBytes, filesystem_ntfs.FormatConfig{Label: label})
		return err
	case FSApfs:
		// Use the spec-compliant FormatContainer path (D-6/D-7). The
		// resulting bytes are a real APFS container that fsck_apfs and
		// macOS's apfs.kext validate and mount as `Class Name:
		// CRawDiskImage / Format: UDRW` — exactly the same shape Apple's
		// own `hdiutil create -size N -fs APFS file.dmg` emits (no koly
		// trailer; hdiutil auto-detects the raw layout).
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			return fmt.Errorf("diskimage: apfs init: %w", err)
		}
		return filesystem_apfs.FormatContainer(path, sizeBytes, label)
	default:
		return fmt.Errorf("diskimage: unsupported filesystem %q (supported: %v)", fs, ValidFilesystems)
	}
}

// alignDown rounds n down to the nearest multiple of align.
func alignDown(n, align int64) int64 { return n - n%align }

// mbrPartType returns the MBR partition type byte for a given filesystem.
func mbrPartType(fs FilesystemType) byte {
	switch fs {
	case FSFat32:
		return 0x0C // FAT32 with LBA
	case FSExFAT:
		return 0x07 // exFAT / NTFS
	default:
		return 0x83 // Linux
	}
}

// gptPartTypeGUID returns the GPT partition type GUID for a given filesystem.
// All returned values are already in little-endian wire format.
func gptPartTypeGUID(fs FilesystemType) [16]byte {
	switch fs {
	case FSFat32, FSExFAT:
		// Microsoft Basic Data: EBD0A0A2-B9E5-4433-87C0-68B6B72699C7
		return [16]byte{
			0xA2, 0xA0, 0xD0, 0xEB,
			0xE5, 0xB9,
			0x33, 0x44,
			0x87, 0xC0,
			0x68, 0xB6, 0xB7, 0x26, 0x99, 0xC7,
		}
	case FSApfs:
		// Apple_APFS: 7C3457EF-0000-11AA-AA11-00306543ECAC.
		// macOS's apfs.kext recognises this GUID in the GPT and binds
		// the partition as the APFS container's physical store; without
		// it, hdiutil attaches the raw image but the synthesized
		// container shows up with size +0 B and no inner volumes.
		return [16]byte{
			0xEF, 0x57, 0x34, 0x7C,
			0x00, 0x00,
			0xAA, 0x11,
			0xAA, 0x11,
			0x00, 0x30, 0x65, 0x43, 0xEC, 0xAC,
		}
	default:
		// Linux Filesystem Data: 0FC63DAF-8483-4772-8E79-3D69D8477DE4
		return [16]byte{
			0xAF, 0x3D, 0xC6, 0x0F,
			0x83, 0x84,
			0x72, 0x47,
			0x8E, 0x79,
			0x3D, 0x69, 0xD8, 0x47, 0x7D, 0xE4,
		}
	}
}
