package exec

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	filesystem_btrfs "github.com/go-filesystems/btrfs"
)

// formatBtrfsAndWrite creates a fresh btrfs image at `path` and writes a
// regular file `/target` so the chmod / chown helpers have something to
// act on. Returns the image path.
func formatBtrfsAndWrite(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "disk.img")
	fs, err := filesystem_btrfs.Format(p, 4*1024*1024, filesystem_btrfs.FormatConfig{})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	if err := fs.WriteFile("/target", []byte("hi"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	fs.Close()
	return p
}

func TestExecBtrfs_Chmod_AppliesPermBits(t *testing.T) {
	img := formatBtrfsAndWrite(t)
	var stdout bytes.Buffer

	if err := BtrfsChmodOnImage(img, -1, "0755", "/target", &stdout); err != nil {
		t.Fatalf("BtrfsChmodOnImage: %v", err)
	}
	if !strings.Contains(stdout.String(), "btrfs chmod") {
		t.Errorf("expected success message, got %q", stdout.String())
	}

	// Reopen and verify via the btrfs FS interface.
	fs, err := filesystem_btrfs.Open(img, -1)
	if err != nil {
		t.Fatalf("Open after chmod: %v", err)
	}
	defer fs.Close()
	st, err := fs.ExtendedStat("/target")
	if err != nil {
		t.Fatalf("ExtendedStat: %v", err)
	}
	if st.Mode&0o7777 != 0o755 {
		t.Errorf("perm bits after chmod = 0o%o, want 0o755", st.Mode&0o7777)
	}
}

func TestExecBtrfs_Chown_SetsUIDGID(t *testing.T) {
	img := formatBtrfsAndWrite(t)
	if err := BtrfsChownOnImage(img, -1, "1000", "2000", "/target", io.Discard); err != nil {
		t.Fatalf("BtrfsChownOnImage: %v", err)
	}
	fs, err := filesystem_btrfs.Open(img, -1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer fs.Close()
	st, _ := fs.ExtendedStat("/target")
	if st.UID != 1000 || st.GID != 2000 {
		t.Errorf("after chown, uid=%d gid=%d, want 1000/2000", st.UID, st.GID)
	}
}

func TestExecBtrfs_Chmod_RejectsNonBtrfs(t *testing.T) {
	// A 4 KiB file with no filesystem header at all → DetectFilesystem will
	// either fail or return something other than FSBtrfs. Either way, the
	// chmod helper must refuse to proceed.
	p := filepath.Join(t.TempDir(), "blob.img")
	if err := writeAllZerosFile(p, 4096); err != nil {
		t.Fatalf("writeAllZerosFile: %v", err)
	}
	if err := BtrfsChmodOnImage(p, -1, "0755", "/anywhere", io.Discard); err == nil {
		t.Fatal("BtrfsChmodOnImage on non-btrfs image unexpectedly succeeded")
	}
}

func TestExecBtrfs_Chmod_RejectsInvalidMode(t *testing.T) {
	img := formatBtrfsAndWrite(t)
	if err := BtrfsChmodOnImage(img, -1, "not-octal", "/target", io.Discard); err == nil {
		t.Fatal("BtrfsChmodOnImage with bogus mode unexpectedly succeeded")
	}
}

func TestExecBtrfs_Chown_RejectsInvalidIDs(t *testing.T) {
	img := formatBtrfsAndWrite(t)
	if err := BtrfsChownOnImage(img, -1, "not-a-uid", "0", "/target", io.Discard); err == nil {
		t.Fatal("BtrfsChownOnImage with bogus uid unexpectedly succeeded")
	}
	if err := BtrfsChownOnImage(img, -1, "0", "not-a-gid", "/target", io.Discard); err == nil {
		t.Fatal("BtrfsChownOnImage with bogus gid unexpectedly succeeded")
	}
}

// writeAllZerosFile creates a file at p containing `size` zero bytes.
func writeAllZerosFile(p string, size int) error {
	return os.WriteFile(p, make([]byte, size), 0o600)
}
