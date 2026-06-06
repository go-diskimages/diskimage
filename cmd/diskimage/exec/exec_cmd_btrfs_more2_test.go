package exec

import (
	"bytes"
	"io"
	"strings"
	"testing"

	filesystem_btrfs "github.com/go-filesystems/btrfs"
)

func TestExecBtrfs_Truncate_GrowsAndShrinks(t *testing.T) {
	img := formatBtrfsAndWrite(t) // creates /target with body "hi"

	// Grow to 4 KiB.
	if err := BtrfsTruncateOnImage(img, -1, "4k", "/target", io.Discard); err != nil {
		t.Fatalf("BtrfsTruncateOnImage grow: %v", err)
	}
	fs, err := filesystem_btrfs.Open(img, -1)
	if err != nil {
		t.Fatalf("Open after grow: %v", err)
	}
	st, _ := fs.ExtendedStat("/target")
	if st.Size != 4096 {
		t.Errorf("after grow: Size = %d, want 4096", st.Size)
	}
	fs.Close()

	// Shrink to 1 byte.
	if err := BtrfsTruncateOnImage(img, -1, "1b", "/target", io.Discard); err != nil {
		t.Fatalf("BtrfsTruncateOnImage shrink: %v", err)
	}
	fs2, err := filesystem_btrfs.Open(img, -1)
	if err != nil {
		t.Fatalf("Open after shrink: %v", err)
	}
	defer fs2.Close()
	st2, _ := fs2.ExtendedStat("/target")
	if st2.Size != 1 {
		t.Errorf("after shrink: Size = %d, want 1", st2.Size)
	}
}

func TestExecBtrfs_Truncate_RejectsBadSize(t *testing.T) {
	img := formatBtrfsAndWrite(t)
	if err := BtrfsTruncateOnImage(img, -1, "not-a-size", "/target", io.Discard); err == nil {
		t.Fatal("expected error for bogus size")
	}
}

func TestExecBtrfs_ExtendedStat_RendersAllFields(t *testing.T) {
	img := formatBtrfsAndWrite(t)
	var buf bytes.Buffer
	if err := BtrfsExtendedStatOnImage(img, -1, "/target", &buf); err != nil {
		t.Fatalf("BtrfsExtendedStatOnImage: %v", err)
	}
	out := buf.String()
	// Spot-check that every field label is rendered.
	for _, want := range []string{
		"Path:", "Inode:", "Type:", "Mode:", "Size:", "Links:",
		"Owner:", "Generation:", "Flags:",
		"ATime:", "MTime:", "CTime:", "OTime:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("extended-stat output missing %q\n--- output ---\n%s", want, out)
		}
	}
	// We wrote a regular file, so the kind must be "regular".
	if !strings.Contains(out, "Type:       regular") {
		t.Errorf("extended-stat: expected Type 'regular' line, got:\n%s", out)
	}
}

func TestExecBtrfs_ExtendedStat_DirectoryKind(t *testing.T) {
	img := formatBtrfsAndWrite(t)
	// Make a dir to stat.
	fs, err := filesystem_btrfs.Open(img, -1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := fs.MkDir("/d", 0o755); err != nil {
		t.Fatalf("MkDir: %v", err)
	}
	fs.Close()

	var buf bytes.Buffer
	if err := BtrfsExtendedStatOnImage(img, -1, "/d", &buf); err != nil {
		t.Fatalf("BtrfsExtendedStatOnImage: %v", err)
	}
	if !strings.Contains(buf.String(), "Type:       directory") {
		t.Errorf("expected directory kind in output:\n%s", buf.String())
	}
}

func TestExecBtrfs_ExtendedStat_MissingPath(t *testing.T) {
	img := formatBtrfsAndWrite(t)
	if err := BtrfsExtendedStatOnImage(img, -1, "/no/such/file", io.Discard); err == nil {
		t.Fatal("expected error for missing path")
	}
}
