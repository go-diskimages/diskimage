package diskimage

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"os"
)

const (
	sectorSize    = 512
	gptHeaderSize = 92
	gptEntrySize  = 128
	gptMaxEntries = 128
)

// writeMBR writes a minimal MBR partition table into the raw image at path.
// One primary partition starting at sector 2048 is created; partType sets the
// partition type byte (0x83 = Linux, 0x0C = FAT32 LBA, 0x07 = exFAT/NTFS).
func writeMBR(path string, imageSize int64, partType byte) error {
	const firstSector = uint32(2048)

	totalSectors := uint32(imageSize / sectorSize)
	if uint32(totalSectors) < firstSector+1 {
		return fmt.Errorf("diskimage: MBR: image too small (%d sectors)", totalSectors)
	}
	partSectors := totalSectors - firstSector

	mbr := make([]byte, sectorSize)

	// Partition entry 1 at offset 446.
	entry := mbr[446:]
	entry[0] = 0x00 // not bootable
	// CHS start — ignored by LBA, set to safe sentinel
	entry[1] = 0x00
	entry[2] = 0x02 // sector 2
	entry[3] = 0x00
	entry[4] = partType
	// CHS end — use the 0xFE/0xFF/0xFF convention for LBA-addressed partitions
	entry[5] = 0xFE
	entry[6] = 0xFF
	entry[7] = 0xFF
	binary.LittleEndian.PutUint32(entry[8:], firstSector)  // LBA start
	binary.LittleEndian.PutUint32(entry[12:], partSectors) // LBA count

	// Boot signature.
	mbr[510] = 0x55
	mbr[511] = 0xAA

	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("diskimage: writeMBR: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteAt(mbr, 0); err != nil {
		return fmt.Errorf("diskimage: writeMBR: write: %w", err)
	}
	return nil
}

// writeGPT writes a protective MBR and a GPT with one data partition into the
// raw image at path.  partTypeGUID must be in the 16-byte wire format (little-
// endian for the first three GUID groups).
func writeGPT(path string, imageSize int64, partTypeGUID [16]byte) error {
	const (
		firstUsable = int64(2048) // sectors
		partStart   = int64(2048) // sectors — first partition
		entryLBA    = int64(2)    // LBA where partition entries start
		numEntries  = uint32(gptMaxEntries)
		entriesSize = int64(numEntries) * gptEntrySize // 16 KiB
	)

	totalSectors := imageSize / sectorSize
	// Backup GPT header is at the last sector; 32 sectors of backup entries precede it.
	backupHeaderLBA := totalSectors - 1
	backupEntriesLBA := backupHeaderLBA - int64(entriesSize/sectorSize)
	lastUsable := backupEntriesLBA - 1
	partEnd := lastUsable // single partition spanning all usable space

	// Random disk GUID and partition GUID.
	diskGUID := randomGUID()
	partGUID := randomGUID()

	// ─── Build partition entry table ────────────────────────────────────────
	entries := make([]byte, numEntries*gptEntrySize)
	p := entries[0:]
	copy(p[0:16], partTypeGUID[:]) // type GUID
	copy(p[16:32], partGUID[:])    // unique GUID
	binary.LittleEndian.PutUint64(p[32:], uint64(partStart))
	binary.LittleEndian.PutUint64(p[40:], uint64(partEnd))
	// attributes = 0 (no special flags)
	// Partition name (UTF-16LE, 36 chars max). Apple's `hdiutil create
	// -fs APFS` sets this to "disk image"; macOS's apfs.kext appears
	// to use the partition name when binding the synthesized container
	// to its physical store. We mirror Apple here so APFS DMGs go all
	// the way to Finder-visible mount.
	const partName = "disk image"
	for i, r := range partName {
		off := 56 + i*2
		if off+2 > 128 {
			break
		}
		p[off] = byte(r)
		p[off+1] = byte(r >> 8)
	}

	entriesCRC := crc32.ChecksumIEEE(entries)

	// ─── Build primary GPT header ────────────────────────────────────────────
	buildHeader := func(currentLBA, backupLBA, entryStartLBA int64) []byte {
		h := make([]byte, sectorSize) // one full sector; header occupies first 92 bytes
		copy(h[0:8], []byte("EFI PART"))
		binary.LittleEndian.PutUint32(h[8:], 0x00010000) // revision 1.0
		binary.LittleEndian.PutUint32(h[12:], gptHeaderSize)
		// h[16:20] = header CRC32, filled in below
		// h[20:24] = reserved, zero
		binary.LittleEndian.PutUint64(h[24:], uint64(currentLBA))
		binary.LittleEndian.PutUint64(h[32:], uint64(backupLBA))
		binary.LittleEndian.PutUint64(h[40:], uint64(firstUsable))
		binary.LittleEndian.PutUint64(h[48:], uint64(lastUsable))
		copy(h[56:72], diskGUID[:])
		binary.LittleEndian.PutUint64(h[72:], uint64(entryStartLBA))
		binary.LittleEndian.PutUint32(h[80:], numEntries)
		binary.LittleEndian.PutUint32(h[84:], gptEntrySize)
		binary.LittleEndian.PutUint32(h[88:], entriesCRC)
		// Compute header CRC over first 92 bytes with the CRC field = 0.
		headerCRC := crc32.ChecksumIEEE(h[:gptHeaderSize])
		binary.LittleEndian.PutUint32(h[16:], headerCRC)
		return h
	}

	primaryHeader := buildHeader(1, backupHeaderLBA, entryLBA)
	backupHeader := buildHeader(backupHeaderLBA, 1, backupEntriesLBA)

	// ─── Protective MBR ─────────────────────────────────────────────────────
	pmbr := make([]byte, sectorSize)
	pe := pmbr[446:]
	pe[0] = 0x00 // not bootable
	pe[4] = 0xEE // protective MBR partition type
	pe[5] = 0xFF
	pe[6] = 0xFF
	pe[7] = 0xFF
	binary.LittleEndian.PutUint32(pe[8:], 1) // LBA start
	sz := uint32(totalSectors - 1)
	if sz > 0xFFFFFFFF {
		sz = 0xFFFFFFFF
	}
	binary.LittleEndian.PutUint32(pe[12:], sz) // LBA count
	pmbr[510] = 0x55
	pmbr[511] = 0xAA

	// ─── Write everything to the file ───────────────────────────────────────
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("diskimage: writeGPT: %w", err)
	}
	defer file.Close()

	type region struct {
		off  int64
		data []byte
	}
	writes := []region{
		{0, pmbr},
		{1 * sectorSize, primaryHeader},
		{entryLBA * sectorSize, entries},
		{backupEntriesLBA * sectorSize, entries},
		{backupHeaderLBA * sectorSize, backupHeader},
	}
	for _, w := range writes {
		if _, err := file.WriteAt(w.data, w.off); err != nil {
			return fmt.Errorf("diskimage: writeGPT: write at 0x%X: %w", w.off, err)
		}
	}
	return nil
}

// randomGUID returns a random RFC 4122 v4 GUID stored in wire format (the
// first three groups are stored little-endian as required by the UEFI spec).
func randomGUID() [16]byte {
	var b [16]byte
	rand.Read(b[:])
	// Set version 4 and variant bits.
	b[6] = (b[6] & 0x0F) | 0x40
	b[8] = (b[8] & 0x3F) | 0x80
	// Convert first three groups to little-endian wire format.
	// Group 1: bytes 0-3 (uint32) — swap to LE
	b[0], b[3] = b[3], b[0]
	b[1], b[2] = b[2], b[1]
	// Group 2: bytes 4-5 (uint16) — swap to LE
	b[4], b[5] = b[5], b[4]
	// Group 3: bytes 6-7 (uint16) — swap to LE
	b[6], b[7] = b[7], b[6]
	return b
}
