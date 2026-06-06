package diskimage

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"

	filesystem "github.com/go-filesystems/interface"
)

// --- Detect/probe tests ---------------------------------------------------

func TestProbeFilesystem_NTFS(t *testing.T) {
	buf := make([]byte, 512)
	copy(buf[0:], []byte("NTFSIMG1"))
	got, err := probeFilesystem(bytes.NewReader(buf), 0)
	if err != nil || got != FSNTFS {
		t.Fatalf("probe NTFS: got %v, %v", got, err)
	}
}

func TestProbeFilesystem_EXFAT(t *testing.T) {
	buf := make([]byte, 512)
	copy(buf[3:11], []byte("EXFAT   "))
	got, err := probeFilesystem(bytes.NewReader(buf), 0)
	if err != nil || got != FSExFAT {
		t.Fatalf("probe exFAT: got %v, %v", got, err)
	}
}

func TestProbeFilesystem_XFS_Ext4_FAT32_BTRFS_ZFS(t *testing.T) {
	// XFS
	xfsBuf := make([]byte, 1400)
	binary.BigEndian.PutUint32(xfsBuf[0:], 0x58465342)
	if got, _ := probeFilesystem(bytes.NewReader(xfsBuf), 0); got != FSXfs {
		t.Fatalf("expected xfs, got %v", got)
	}

	// ext4 magic at 1080
	extBuf := make([]byte, 1100)
	binary.LittleEndian.PutUint16(extBuf[1080:], 0xEF53)
	if got, _ := probeFilesystem(bytes.NewReader(extBuf), 0); got != FSExt4 {
		t.Fatalf("expected ext4, got %v", got)
	}

	// FAT32: signature + FAT32 string
	fatBuf := make([]byte, 512)
	fatBuf[510] = 0x55
	fatBuf[511] = 0xAA
	copy(fatBuf[82:90], []byte("FAT32   "))
	if got, _ := probeFilesystem(bytes.NewReader(fatBuf), 0); got != FSFat32 {
		t.Fatalf("expected fat32, got %v", got)
	}

	// btrfs magic at 0x10040 little-endian
	bBuf := make([]byte, 0x10080)
	binary.LittleEndian.PutUint64(bBuf[0x10040:], 0x4D5F53665248425F)
	if got, _ := probeFilesystem(bytes.NewReader(bBuf), 0); got != FSBtrfs {
		t.Fatalf("expected btrfs, got %v", got)
	}

	// zfs uberblock magic at 128KiB
	zBuf := make([]byte, 128*1024+8)
	binary.LittleEndian.PutUint64(zBuf[128*1024:], 0x00bab10c)
	if got, _ := probeFilesystem(bytes.NewReader(zBuf), 0); got != FSZfs {
		t.Fatalf("expected zfs, got %v", got)
	}
}

func TestProbeFilesystem_Unknown(t *testing.T) {
	if _, err := probeFilesystem(bytes.NewReader(make([]byte, 512)), 0); err == nil {
		t.Fatalf("expected unknown filesystem error")
	}
}

// --- Utility tests -------------------------------------------------------

func TestAlignDownMbrGptHelpers(t *testing.T) {
	if got := alignDown(10000, 4096); got != 8192 {
		t.Fatalf("alignDown wrong: %d", got)
	}
	if mbrPartType(FSFat32) != 0x0C {
		t.Fatalf("mbrPartType fat32")
	}
	if mbrPartType(FSExFAT) != 0x07 {
		t.Fatalf("mbrPartType exfat")
	}
	// gpt GUIDs: ensure returned length and some bytes match expectations
	g := gptPartTypeGUID(FSFat32)
	if g[0] == 0 && g[15] == 0 {
		t.Fatalf("gpt GUID seems empty")
	}
}

// --- ensureParentDirs tests ----------------------------------------------

type memFS struct{ dirs map[string]bool }

func newMemFS() *memFS                                         { return &memFS{dirs: map[string]bool{"/": true}} }
func (m *memFS) Close() error                                  { return nil }
func (m *memFS) ReadFile(string) ([]byte, error)               { return nil, io.EOF }
func (m *memFS) ListDir(string) ([]filesystem.DirEntry, error) { return nil, io.EOF }
func (m *memFS) Stat(p string) (filesystem.Stat, error) {
	if m.dirs[p] {
		return filesystem.NewStat(0o040755, 0, 1), nil
	}
	return nil, os.ErrNotExist
}
func (m *memFS) WriteFile(string, []byte, os.FileMode) error { return nil }
func (m *memFS) ReadLink(string) (string, error)             { return "", io.EOF }
func (m *memFS) MkDir(p string, _ os.FileMode) error         { m.dirs[p] = true; return nil }
func (m *memFS) DeleteFile(string) error                     { return nil }
func (m *memFS) DeleteDir(string) error                      { return nil }
func (m *memFS) Rename(_, _ string) error                    { return nil }

func TestEnsureParentDirs_CreatesHierarchy(t *testing.T) {
	m := newMemFS()
	if err := ensureParentDirs(m, "/a/b/c/file.txt"); err != nil {
		t.Fatalf("ensureParentDirs: %v", err)
	}
	for _, p := range []string{"/a", "/a/b", "/a/b/c"} {
		if !m.dirs[p] {
			t.Fatalf("expected dir %s created", p)
		}
	}
}

func TestEnsureParentDirs_NoopRoot(t *testing.T) {
	m := newMemFS()
	if err := ensureParentDirs(m, "/file.txt"); err != nil {
		t.Fatalf("ensureParentDirs root: %v", err)
	}
}

// --- writeMBR / writeGPT / Grow tests ------------------------------------

func TestWriteMBRAndGPTAndGrow(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "img.bin")
	// Create a 10 MiB file
	f, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	f.Truncate(10 * 1024 * 1024)
	f.Close()

	if err := writeMBR(tmp, 10*1024*1024, 0x83); err != nil {
		t.Fatalf("writeMBR: %v", err)
	}
	// verify signature
	b, _ := os.ReadFile(tmp)
	if b[510] != 0x55 || b[511] != 0xAA {
		t.Fatalf("mbr signature missing")
	}

	// write GPT
	if err := writeGPT(tmp, 10*1024*1024, gptPartTypeGUID(FSNone)); err != nil {
		t.Fatalf("writeGPT: %v", err)
	}
	b2, _ := os.ReadFile(tmp)
	if string(b2[512:520]) != "EFI PART" {
		t.Fatalf("gpt header not written")
	}

	// Test Grow
	if err := Grow(tmp, 5*1024*1024); err != nil {
		t.Fatalf("Grow shrink should succeed: %v", err)
	}
	if err := Grow(tmp, 2*1024*1024); err != nil {
		t.Fatalf("Grow shrink again: %v", err)
	}
}

// --- List/Create/formatFilesystem error branches -------------------------

func TestListUnsupportedFilesystem(t *testing.T) {
	_, err := List(ListOptions{Path: "", Filesystem: "bogus"})
	if err == nil {
		t.Fatalf("expected unsupported filesystem error")
	}
}

func TestCreateValidationAndFormatUnsupported(t *testing.T) {
	if err := Create(CreateOptions{Path: "", SizeBytes: 0}); err == nil {
		t.Fatalf("expected error when path empty/size invalid")
	}
	// Unsupported partition scheme
	tmp := filepath.Join(t.TempDir(), "c.img")
	if err := Create(CreateOptions{Path: tmp, SizeBytes: 1024, Partition: "bogus"}); err == nil {
		t.Fatalf("expected unsupported partition error")
	}
	// formatFilesystem unsupported FS
	if err := formatFilesystem(tmp, 1024, "bogus", ""); err == nil {
		t.Fatalf("expected unsupported filesystem in formatFilesystem")
	}
}
