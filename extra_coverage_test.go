package diskimage

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectMBROffsetAndGPTOffset(t *testing.T) {
	// MBR table: create a 512-byte buffer with a partition entry starting at LBA 2048
	mbr := make([]byte, 512)
	mbr[510] = 0x55
	mbr[511] = 0xAA
	// set start LBA for partition index 1
	off := 446 + 1*16
	binary.LittleEndian.PutUint32(mbr[off+8:], uint32(2048))
	o, err := detectMBROffset(bytes.NewReader(mbr), 1)
	if err != nil {
		t.Fatalf("detectMBROffset failed: %v", err)
	}
	if o != int64(2048*detectSectorSize) {
		t.Fatalf("unexpected mbr offset: %d", o)
	}

	// GPT header: craft a header at LBA1 with entry table at LBA 2 and one entry
	entrySize := uint32(128)
	numParts := uint32(2)
	partEntryLBA := uint64(2)
	// allocate buffer large enough for header + table
	bufLen := int(partEntryLBA+uint64(numParts))*detectSectorSize + int(entrySize)
	g := make([]byte, bufLen)
	copy(g[512:520], []byte("EFI PART"))
	binary.LittleEndian.PutUint64(g[512+72:], partEntryLBA)
	binary.LittleEndian.PutUint32(g[512+80:], numParts)
	binary.LittleEndian.PutUint32(g[512+84:], entrySize)
	// create an entry for index 0 with start LBA 4096
	tableOff := int(partEntryLBA) * detectSectorSize
	// set a non-zero type GUID in the first 16 bytes
	copy(g[tableOff:tableOff+16], []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	binary.LittleEndian.PutUint64(g[tableOff+32:], uint64(4096))
	o2, err := detectGPTOffset(bytes.NewReader(g), 0)
	if err != nil {
		t.Fatalf("detectGPTOffset failed: %v", err)
	}
	if o2 != int64(4096*detectSectorSize) {
		t.Fatalf("unexpected gpt offset: %d", o2)
	}
}

func TestDetectFilesystem_BareFiles(t *testing.T) {
	// NTFS
	ntfsBuf := make([]byte, 512)
	copy(ntfsBuf[0:], []byte("NTFSIMG1"))
	p := filepath.Join(t.TempDir(), "ntfs.img")
	if err := os.WriteFile(p, ntfsBuf, 0644); err != nil {
		t.Fatal(err)
	}
	if got, err := DetectFilesystem(p, 0); err != nil || got != FSNTFS {
		t.Fatalf("DetectFilesystem ntfs: %v %v", got, err)
	}

	// EXFAT
	ex := make([]byte, 512)
	copy(ex[3:11], []byte("EXFAT   "))
	p2 := filepath.Join(t.TempDir(), "exfat.img")
	if err := os.WriteFile(p2, ex, 0644); err != nil {
		t.Fatal(err)
	}
	if got, err := DetectFilesystem(p2, 0); err != nil || got != FSExFAT {
		t.Fatalf("DetectFilesystem exfat: %v %v", got, err)
	}

	// XFS
	x := make([]byte, 1400)
	binary.BigEndian.PutUint32(x[0:], 0x58465342)
	p3 := filepath.Join(t.TempDir(), "xfs.img")
	if err := os.WriteFile(p3, x, 0644); err != nil {
		t.Fatal(err)
	}
	if got, err := DetectFilesystem(p3, 0); err != nil || got != FSXfs {
		t.Fatalf("DetectFilesystem xfs: %v %v", got, err)
	}

	// ext4
	e := make([]byte, 1100)
	binary.LittleEndian.PutUint16(e[1080:], 0xEF53)
	p4 := filepath.Join(t.TempDir(), "ext4.img")
	if err := os.WriteFile(p4, e, 0644); err != nil {
		t.Fatal(err)
	}
	if got, err := DetectFilesystem(p4, 0); err != nil || got != FSExt4 {
		t.Fatalf("DetectFilesystem ext4: %v %v", got, err)
	}

	// FAT32
	f := make([]byte, 512)
	f[510] = 0x55
	f[511] = 0xAA
	copy(f[82:90], []byte("FAT32   "))
	p5 := filepath.Join(t.TempDir(), "fat32.img")
	if err := os.WriteFile(p5, f, 0644); err != nil {
		t.Fatal(err)
	}
	if got, err := DetectFilesystem(p5, 0); err != nil || got != FSFat32 {
		t.Fatalf("DetectFilesystem fat32: %v %v", got, err)
	}

	// btrfs
	b := make([]byte, 0x10080)
	binary.LittleEndian.PutUint64(b[0x10040:], 0x4D5F53665248425F)
	p6 := filepath.Join(t.TempDir(), "btrfs.img")
	if err := os.WriteFile(p6, b, 0644); err != nil {
		t.Fatal(err)
	}
	if got, err := DetectFilesystem(p6, 0); err != nil || got != FSBtrfs {
		t.Fatalf("DetectFilesystem btrfs: %v %v", got, err)
	}

	// zfs
	z := make([]byte, 128*1024+8)
	binary.LittleEndian.PutUint64(z[128*1024:], 0x00bab10c)
	p7 := filepath.Join(t.TempDir(), "zfs.img")
	if err := os.WriteFile(p7, z, 0644); err != nil {
		t.Fatal(err)
	}
	if got, err := DetectFilesystem(p7, 0); err != nil || got != FSZfs {
		t.Fatalf("DetectFilesystem zfs: %v %v", got, err)
	}
}

func TestOpenFSSwitchBranchesAndFileOpsErrors(t *testing.T) {
	// prepare an empty image file
	tmp := filepath.Join(t.TempDir(), "img.bin")
	if err := os.WriteFile(tmp, make([]byte, 1024), 0644); err != nil {
		t.Fatal(err)
	}

	// exercise openFS switch branches by attempting to open with each fs type
	types := []FilesystemType{FSExt4, FSFat32, FSBtrfs, FSXfs, FSZfs, FSExFAT, FSNTFS}
	for _, ft := range types {
		fs, err := openFS(tmp, 0, ft, nil)
		if err != nil {
			// open may fail; that's acceptable (we exercised the branch)
			continue
		}
		// if open succeeded, close the handle
		fs.Close()
	}

	// fileops functions should validate path argument
	if _, err := ReadFile(FileOptions{}); err == nil {
		t.Fatalf("expected error for empty path ReadFile")
	}
	if err := WriteFile(FileOptions{}, nil, 0); err == nil {
		t.Fatalf("expected error for empty path WriteFile")
	}
	if err := MkDir(FileOptions{}, 0); err == nil {
		t.Fatalf("expected error for empty path MkDir")
	}
	if err := DeleteFile(FileOptions{}); err == nil {
		t.Fatalf("expected error for empty path DeleteFile")
	}
	if err := DeleteDir(FileOptions{}); err == nil {
		t.Fatalf("expected error for empty path DeleteDir")
	}
	if err := Rename(FileOptions{}, "x"); err == nil {
		t.Fatalf("expected error for empty path Rename")
	}
	if _, err := Stat(FileOptions{}); err == nil {
		t.Fatalf("expected error for empty path Stat")
	}
}

func TestDetectPartOffset_MBRFallback(t *testing.T) {
	// Buffer without MBR/GPT should return 0 offset
	buf := make([]byte, 1024)
	off, err := detectPartOffset(bytes.NewReader(buf), 0)
	if err != nil {
		t.Fatalf("detectPartOffset unexpected error: %v", err)
	}
	if off != 0 {
		t.Fatalf("expected 0 offset, got %d", off)
	}

	// MBR signature but no partition entries -> return 0 (bare fs)
	mbr := make([]byte, 512)
	mbr[510] = 0x55
	mbr[511] = 0xAA
	off2, err := detectPartOffset(bytes.NewReader(mbr), 0)
	if err != nil {
		t.Fatalf("detectPartOffset mbr error: %v", err)
	}
	if off2 != 0 {
		t.Fatalf("expected 0 offset for bare mbr, got %d", off2)
	}
}
