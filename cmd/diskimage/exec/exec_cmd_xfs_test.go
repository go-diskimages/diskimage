package exec

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	filesystem_xfs "github.com/go-filesystems/xfs"
)

const xfsCliTestSize = int64(64 * 1024 * 1024) // 64 MiB: one XFS allocation group (fmtMinSize)

// formatXfsAndWrite creates a fresh xfs image at TempDir + "disk.img" and
// writes a regular file `/target`. Returns the image path.
func formatXfsAndWrite(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "disk.img")
	fs, err := filesystem_xfs.Format(p, xfsCliTestSize, filesystem_xfs.FormatConfig{})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	if err := fs.WriteFile("/target", []byte("hi"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	fs.Close()
	return p
}

func TestExecXfs_Chmod_AppliesPermBits(t *testing.T) {
	img := formatXfsAndWrite(t)
	var stdout bytes.Buffer

	if err := XfsChmodOnImage(img, -1, "0755", "/target", &stdout); err != nil {
		t.Fatalf("XfsChmodOnImage: %v", err)
	}
	if !strings.Contains(stdout.String(), "xfs chmod") {
		t.Errorf("expected success message, got %q", stdout.String())
	}

	fs, err := filesystem_xfs.Open(img, -1)
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

func TestExecXfs_Chown_SetsUIDGID(t *testing.T) {
	img := formatXfsAndWrite(t)
	if err := XfsChownOnImage(img, -1, "1000", "2000", "/target", io.Discard); err != nil {
		t.Fatalf("XfsChownOnImage: %v", err)
	}
	fs, err := filesystem_xfs.Open(img, -1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer fs.Close()
	st, _ := fs.ExtendedStat("/target")
	if st.UID != 1000 || st.GID != 2000 {
		t.Errorf("after chown, uid=%d gid=%d, want 1000/2000", st.UID, st.GID)
	}
}

func TestExecXfs_Symlink_ReadsBackTarget(t *testing.T) {
	img := formatXfsAndWrite(t)
	if err := XfsSymlinkOnImage(img, -1, "/target", "/lnk", io.Discard); err != nil {
		t.Fatalf("XfsSymlinkOnImage: %v", err)
	}
	fs, err := filesystem_xfs.Open(img, -1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer fs.Close()
	got, err := fs.ReadLink("/lnk")
	if err != nil {
		t.Fatalf("ReadLink: %v", err)
	}
	if got != "/target" {
		t.Errorf("ReadLink = %q, want %q", got, "/target")
	}
}

func TestExecXfs_Link_BumpsNlink(t *testing.T) {
	img := formatXfsAndWrite(t)
	if err := XfsLinkOnImage(img, -1, "/target", "/dup", io.Discard); err != nil {
		t.Fatalf("XfsLinkOnImage: %v", err)
	}
	fs, err := filesystem_xfs.Open(img, -1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer fs.Close()
	st, _ := fs.ExtendedStat("/target")
	if st.NLink != 2 {
		t.Errorf("nlink after link = %d, want 2", st.NLink)
	}
}

func TestExecXfs_Truncate_AppliesSize(t *testing.T) {
	img := formatXfsAndWrite(t)
	if err := XfsTruncateOnImage(img, -1, "16b", "/target", io.Discard); err != nil {
		t.Fatalf("XfsTruncateOnImage: %v", err)
	}
	fs, err := filesystem_xfs.Open(img, -1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer fs.Close()
	st, _ := fs.ExtendedStat("/target")
	if st.Size != 16 {
		t.Errorf("size after truncate = %d, want 16", st.Size)
	}
}

func TestExecXfs_ExtendedStat_PrintsFields(t *testing.T) {
	img := formatXfsAndWrite(t)
	var stdout bytes.Buffer
	if err := XfsExtendedStatOnImage(img, -1, "/target", &stdout); err != nil {
		t.Fatalf("XfsExtendedStatOnImage: %v", err)
	}
	s := stdout.String()
	for _, want := range []string{"Path:", "Inode:", "Type:", "Mode:", "Size:", "Links:", "Owner:", "ATime:", "MTime:", "CTime:"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, s)
		}
	}
}

func TestExecXfs_RejectsNonXfs(t *testing.T) {
	p := filepath.Join(t.TempDir(), "blob.img")
	if err := writeAllZerosFile(p, 4096); err != nil {
		t.Fatalf("writeAllZerosFile: %v", err)
	}
	if err := XfsChmodOnImage(p, -1, "0755", "/anywhere", io.Discard); err == nil {
		t.Fatal("XfsChmodOnImage on non-xfs image unexpectedly succeeded")
	}
}

func TestExecXfs_RejectsInvalidArgs(t *testing.T) {
	img := formatXfsAndWrite(t)
	if err := XfsChmodOnImage(img, -1, "not-octal", "/target", io.Discard); err == nil {
		t.Fatal("XfsChmodOnImage with bogus mode unexpectedly succeeded")
	}
	if err := XfsChownOnImage(img, -1, "not-a-uid", "0", "/target", io.Discard); err == nil {
		t.Fatal("XfsChownOnImage with bogus uid unexpectedly succeeded")
	}
	if err := XfsChtimesOnImage(img, -1, "not-an-int", "0", "/target", io.Discard); err == nil {
		t.Fatal("XfsChtimesOnImage with bogus atime unexpectedly succeeded")
	}
	if err := XfsTruncateOnImage(img, -1, "not-a-size", "/target", io.Discard); err == nil {
		t.Fatal("XfsTruncateOnImage with bogus size unexpectedly succeeded")
	}
}
