package exec

import (
	"bytes"
	"io"
	"testing"
	"time"

	filesystem_btrfs "github.com/go-filesystems/btrfs"
)

func TestExecBtrfs_Chtimes_AppliesAtimeMtime(t *testing.T) {
	img := formatBtrfsAndWrite(t)
	const atimeSec, mtimeSec = 1700000000, 1700001234
	if err := BtrfsChtimesOnImage(img, -1,
		toBase10(atimeSec), toBase10(mtimeSec), "/target", io.Discard); err != nil {
		t.Fatalf("BtrfsChtimesOnImage: %v", err)
	}
	fs, err := filesystem_btrfs.Open(img, -1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer fs.Close()
	st, _ := fs.ExtendedStat("/target")
	if !st.ATime.Equal(time.Unix(atimeSec, 0).UTC()) {
		t.Errorf("atime = %v, want %v", st.ATime, time.Unix(atimeSec, 0).UTC())
	}
	if !st.MTime.Equal(time.Unix(mtimeSec, 0).UTC()) {
		t.Errorf("mtime = %v, want %v", st.MTime, time.Unix(mtimeSec, 0).UTC())
	}
}

func TestExecBtrfs_Link_AndSymlink(t *testing.T) {
	img := formatBtrfsAndWrite(t)
	if err := BtrfsLinkOnImage(img, -1, "/target", "/aliased", io.Discard); err != nil {
		t.Fatalf("BtrfsLinkOnImage: %v", err)
	}
	if err := BtrfsSymlinkOnImage(img, -1, "/target", "/pointer", io.Discard); err != nil {
		t.Fatalf("BtrfsSymlinkOnImage: %v", err)
	}
	fs, err := filesystem_btrfs.Open(img, -1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer fs.Close()
	// Hardlink: same inode as /target.
	stT, _ := fs.Stat("/target")
	stA, _ := fs.Stat("/aliased")
	if stT.Inode() != stA.Inode() {
		t.Errorf("aliased and target should share inode, got %d vs %d", stA.Inode(), stT.Inode())
	}
	// Symlink: ReadLink returns the target string.
	got, err := fs.ReadLink("/pointer")
	if err != nil {
		t.Fatalf("ReadLink: %v", err)
	}
	if got != "/target" {
		t.Errorf("ReadLink(/pointer) = %q, want %q", got, "/target")
	}
}

func TestExecBtrfs_SetGetRemoveXattr(t *testing.T) {
	img := formatBtrfsAndWrite(t)
	const name, value = "user.tag", "v=1"
	if err := BtrfsSetXattrOnImage(img, -1, "/target", name, value, io.Discard); err != nil {
		t.Fatalf("BtrfsSetXattrOnImage: %v", err)
	}
	fs, err := filesystem_btrfs.Open(img, -1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := fs.GetXattr("/target", name)
	if err != nil {
		t.Fatalf("GetXattr after Set: %v", err)
	}
	if !bytes.Equal(got, []byte(value)) {
		t.Errorf("GetXattr = %q, want %q", got, value)
	}
	fs.Close()

	if err := BtrfsRemoveXattrOnImage(img, -1, "/target", name, io.Discard); err != nil {
		t.Fatalf("BtrfsRemoveXattrOnImage: %v", err)
	}
	fs2, err := filesystem_btrfs.Open(img, -1)
	if err != nil {
		t.Fatalf("Open after Remove: %v", err)
	}
	defer fs2.Close()
	if _, err := fs2.GetXattr("/target", name); err == nil {
		t.Errorf("GetXattr after RemoveXattr unexpectedly succeeded")
	}
}

func TestExecBtrfs_NewSubcommands_RejectMalformedArgs(t *testing.T) {
	img := formatBtrfsAndWrite(t)
	cases := []struct {
		name string
		run  func() error
	}{
		{"chtimes bad atime", func() error {
			return BtrfsChtimesOnImage(img, -1, "nope", "0", "/target", io.Discard)
		}},
		{"chtimes bad mtime", func() error {
			return BtrfsChtimesOnImage(img, -1, "0", "nope", "/target", io.Discard)
		}},
		{"link missing target", func() error {
			return BtrfsLinkOnImage(img, -1, "/no/such", "/never", io.Discard)
		}},
		{"symlink empty target", func() error {
			return BtrfsSymlinkOnImage(img, -1, "", "/some", io.Discard)
		}},
		{"removexattr missing", func() error {
			return BtrfsRemoveXattrOnImage(img, -1, "/target", "user.never-set", io.Discard)
		}},
	}
	for _, c := range cases {
		if err := c.run(); err == nil {
			t.Errorf("%s: expected error, got nil", c.name)
		}
	}
}

// toBase10 keeps the test self-contained without dragging strconv into the
// helper file for one-liner conversions.
func toBase10(n int64) string {
	const neg = "-"
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	digits := make([]byte, 0, 20)
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}
	if negative {
		digits = append(digits, neg[0])
	}
	// Reverse.
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}
