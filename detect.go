package diskimage

import (
	"encoding/binary"
	"fmt"
	"os"

	disk_dmg "github.com/go-diskimages/dmg"
)

// DetectFilesystem probes the magic bytes in a disk image and returns the
// filesystem type found in the partition at partIndex.
// It supports: ext4, fat32, btrfs, xfs, zfs, exfat.
func DetectFilesystem(imagePath string, partIndex int) (FilesystemType, error) {
	// If the image is a UDIF, unpack the raw sectors first.
	if disk_dmg.IsUDIF(imagePath) {
		return detectFilesystemInUDIF(imagePath, partIndex)
	}

	f, err := os.Open(imagePath)
	if err != nil {
		return "", fmt.Errorf("detect filesystem: %w", err)
	}
	defer f.Close()
	fsOff, err := detectPartOffset(f, partIndex)
	if err != nil {
		return "", fmt.Errorf("detect filesystem: %w", err)
	}
	return probeFilesystem(f, fsOff)
}

// detectFilesystemInUDIF unpacks a UDIF image to a temp file and runs
// filesystem detection on the raw payload.
func detectFilesystemInUDIF(imagePath string, partIndex int) (FilesystemType, error) {
	tmp, err := disk_dmg.UnpackToTemp(imagePath)
	if err != nil {
		return "", fmt.Errorf("detect filesystem: unpack udif: %w", err)
	}
	defer os.Remove(tmp)
	f, err := os.Open(tmp)
	if err != nil {
		return "", fmt.Errorf("detect filesystem: open udif payload: %w", err)
	}
	defer f.Close()
	fsOff, err := detectPartOffset(f, partIndex)
	if err != nil {
		return "", fmt.Errorf("detect filesystem: %w", err)
	}
	return probeFilesystem(f, fsOff)
}

// probeFilesystem reads magic bytes at well-known offsets and returns the
// filesystem type.
func probeFilesystem(r readerAt, fsOff int64) (FilesystemType, error) {
	// NTFS-like test: our in-image NTFS driver stores a small header magic
	// at the start of the filesystem region.
	var ntfsMagic [8]byte
	if _, err := r.ReadAt(ntfsMagic[:], fsOff); err == nil {
		if string(ntfsMagic[:]) == "NTFSIMG1" {
			return FSNTFS, nil
		}
	}

	// APFS: NX SuperBlock magic "NXSB" lives at offset 32 of block 0.
	var nxMagic [4]byte
	if _, err := r.ReadAt(nxMagic[:], fsOff+32); err == nil {
		if string(nxMagic[:]) == "NXSB" {
			return FSApfs, nil
		}
	}

	// exFAT: OEM name "EXFAT   " at bytes 3-10 in the boot sector.
	var oem [8]byte
	if _, err := r.ReadAt(oem[:], fsOff+3); err == nil {
		if string(oem[:]) == "EXFAT   " {
			return FSExFAT, nil
		}
	}

	// XFS: magic "XFSB" (0x58465342 big-endian) at bytes 0-3.
	var xfsMagic [4]byte
	if _, err := r.ReadAt(xfsMagic[:], fsOff); err == nil {
		if binary.BigEndian.Uint32(xfsMagic[:]) == 0x58465342 {
			return FSXfs, nil
		}
	}

	// ext4: magic 0xEF53 (little-endian uint16) at superblock offset 1024 + 56 = 1080.
	var ext4Magic [2]byte
	if _, err := r.ReadAt(ext4Magic[:], fsOff+1080); err == nil {
		if binary.LittleEndian.Uint16(ext4Magic[:]) == 0xEF53 {
			return FSExt4, nil
		}
	}

	// FAT32: boot sector signature 0x55AA at bytes 510-511 and
	// filesystem type string "FAT32   " at bytes 82-89.
	var bootSig [2]byte
	var fatType [8]byte
	if _, err := r.ReadAt(bootSig[:], fsOff+510); err == nil &&
		bootSig[0] == 0x55 && bootSig[1] == 0xAA {
		if _, err := r.ReadAt(fatType[:], fsOff+82); err == nil &&
			string(fatType[:]) == "FAT32   " {
			return FSFat32, nil
		}
	}

	// btrfs: magic at superblock offset 0x10000 + 0x40, little-endian uint64.
	// The constant _BHRfS_M is 0x4D5F53665248425F in little-endian.
	var btrfsMagic [8]byte
	if _, err := r.ReadAt(btrfsMagic[:], fsOff+0x10040); err == nil {
		if binary.LittleEndian.Uint64(btrfsMagic[:]) == 0x4D5F53665248425F {
			return FSBtrfs, nil
		}
	}

	// ZFS: uberblock magic 0x00bab10c somewhere in the VDEV uberblock ring.
	// The ring is a 128 KiB region beginning 128 KiB into the VDEV label;
	// uberblocks are written round-robin into fixed-size slots (4 KiB for the
	// default ashift=12), so the active uberblock lives at slot (txg % nslots)
	// and slot 0 is not guaranteed to be populated. Scan every slot for the
	// magic rather than assuming it sits at the very start of the ring.
	if detectZFSUberblock(r, fsOff) {
		return FSZfs, nil
	}

	return "", fmt.Errorf("unknown filesystem at offset %d", fsOff)
}

// ZFS VDEV uberblock-ring geometry (matches go-filesystems/zfs):
//
//	[0x20000 .. 0x40000)  vl_uberblock — 128 KiB uberblock ring
//
// Slots are uberblockSlotSize bytes apart (4 KiB for the default ashift=12),
// and the magic 0x00bab10c is the first 8 bytes of an active slot.
const (
	zfsUberblockRingOffset = 128 * 1024
	zfsUberblockRingSize   = 128 * 1024
	zfsUberblockSlotSize   = 4096
	zfsUberblockMagic      = uint64(0x00bab10c)
)

// detectZFSUberblock scans the VDEV uberblock ring for a slot bearing the ZFS
// uberblock magic. ZFS may be written little- or big-endian, so both byte
// orders are checked.
func detectZFSUberblock(r readerAt, fsOff int64) bool {
	var slot [8]byte
	for off := int64(0); off < zfsUberblockRingSize; off += zfsUberblockSlotSize {
		if _, err := r.ReadAt(slot[:], fsOff+zfsUberblockRingOffset+off); err != nil {
			break
		}
		if binary.LittleEndian.Uint64(slot[:]) == zfsUberblockMagic ||
			binary.BigEndian.Uint64(slot[:]) == zfsUberblockMagic {
			return true
		}
	}
	return false
}

// readerAt is a subset of io.ReaderAt used internally.
type readerAt interface {
	ReadAt(p []byte, off int64) (n int, err error)
}

// offsetReaderAt adjusts ReadAt offsets by a fixed base, useful for probing
// container payload regions.
type offsetReaderAt struct {
	r    readerAt
	base int64
}

func (o offsetReaderAt) ReadAt(p []byte, off int64) (int, error) {
	return o.r.ReadAt(p, o.base+off)
}

const detectSectorSize = 512

// detectPartOffset returns the byte offset of the partition at partIndex.
// Returns 0 for bare images (no partition table).
func detectPartOffset(r readerAt, partIndex int) (int64, error) {
	// GPT: signature "EFI PART" at LBA1 (byte 512).
	var sig [8]byte
	if _, err := r.ReadAt(sig[:], 512); err == nil && string(sig[:]) == "EFI PART" {
		return detectGPTOffset(r, partIndex)
	}
	// MBR: signature 0x55AA at bytes 510-511.
	var magic [2]byte
	if _, err := r.ReadAt(magic[:], 510); err == nil &&
		magic[0] == 0x55 && magic[1] == 0xAA {
		// Only treat as MBR if not also an exFAT / FAT image at offset 0
		// (bare FAT images also have 0x55AA but no partition entries).
		off, err := detectMBROffset(r, partIndex)
		if err == nil && off > 0 {
			return off, nil
		}
		// Fall through: bare filesystem image that happens to have 0x55AA.
	}
	// Bare filesystem — start at offset 0.
	return 0, nil
}

func detectGPTOffset(r readerAt, partIndex int) (int64, error) {
	hdr := make([]byte, 92)
	if _, err := r.ReadAt(hdr, 512); err != nil {
		return 0, fmt.Errorf("read GPT header: %w", err)
	}
	partEntryLBA := binary.LittleEndian.Uint64(hdr[72:])
	numParts := binary.LittleEndian.Uint32(hdr[80:])
	entrySize := binary.LittleEndian.Uint32(hdr[84:])
	if entrySize < 128 {
		return 0, fmt.Errorf("unexpected GPT entry size %d", entrySize)
	}
	tableOff := int64(partEntryLBA) * detectSectorSize
	buf := make([]byte, entrySize)
	for i := uint32(0); i < numParts; i++ {
		if _, err := r.ReadAt(buf, tableOff+int64(i)*int64(entrySize)); err != nil {
			break
		}
		var typeGUID [16]byte
		copy(typeGUID[:], buf[:16])
		if typeGUID == ([16]byte{}) {
			continue
		}
		if int(i) == partIndex {
			startLBA := binary.LittleEndian.Uint64(buf[32:])
			return int64(startLBA) * detectSectorSize, nil
		}
	}
	return 0, fmt.Errorf("GPT partition index %d not found", partIndex)
}

func detectMBROffset(r readerAt, partIndex int) (int64, error) {
	table := make([]byte, 64)
	if _, err := r.ReadAt(table, 446); err != nil {
		return 0, fmt.Errorf("read MBR table: %w", err)
	}
	for i := 0; i < 4; i++ {
		e := table[i*16:]
		startLBA := binary.LittleEndian.Uint32(e[8:])
		if startLBA == 0 {
			continue
		}
		if i == partIndex {
			return int64(startLBA) * detectSectorSize, nil
		}
	}
	return 0, fmt.Errorf("MBR partition index %d not found", partIndex)
}
