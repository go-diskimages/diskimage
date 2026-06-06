package exec

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/go-diskimages/diskimage"
	filesystem_btrfs "github.com/go-filesystems/btrfs"
)

// btrfsOpen opens the image as a btrfs filesystem, verifying that the
// detected on-disk filesystem really is btrfs. Returns the concrete
// *btrfsFS handle so callers can use the driver-specific API.
func btrfsOpen(file string, partIndex int) (filesystem_btrfs.FS, error) {
	fsType, err := diskimage.DetectFilesystem(file, partIndex)
	if err != nil {
		return nil, fmt.Errorf("detect filesystem: %w", err)
	}
	if fsType != diskimage.FSBtrfs {
		return nil, fmt.Errorf("expected btrfs image, got %s", fsType)
	}
	fs, err := filesystem_btrfs.Open(file, partIndex)
	if err != nil {
		return nil, fmt.Errorf("open btrfs image: %w", err)
	}
	return fs, nil
}

// BtrfsChmodOnImage parses an octal permission string (e.g. "0755" or
// "755") and applies it to path inside the btrfs image. Only the
// permission bits change — the file-type bits (regular/dir/symlink) are
// preserved.
func BtrfsChmodOnImage(file string, partIndex int, modeStr, path string, w io.Writer) error {
	mode, err := parseOctalMode(modeStr)
	if err != nil {
		return fmt.Errorf("invalid mode %q: %w", modeStr, err)
	}
	fs, err := btrfsOpen(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()
	if err := fs.Chmod(path, mode); err != nil {
		return err
	}
	if w != nil {
		fmt.Fprintf(w, "btrfs chmod %s %s ok\n", modeStr, path)
	}
	return nil
}

// BtrfsChownOnImage applies an owner uid:gid pair to path inside the
// btrfs image. Both must be base-10 unsigned integers.
func BtrfsChownOnImage(file string, partIndex int, uidStr, gidStr, path string, w io.Writer) error {
	uid64, err := strconv.ParseUint(uidStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid uid %q: %w", uidStr, err)
	}
	gid64, err := strconv.ParseUint(gidStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid gid %q: %w", gidStr, err)
	}
	fs, err := btrfsOpen(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()
	if err := fs.Chown(path, uint32(uid64), uint32(gid64)); err != nil {
		return err
	}
	if w != nil {
		fmt.Fprintf(w, "btrfs chown %s:%s %s ok\n", uidStr, gidStr, path)
	}
	return nil
}

// BtrfsChtimesOnImage applies atime and mtime to path. Both inputs are
// parsed as Unix seconds (signed int64) — keeping the CLI surface simple
// and avoiding timezone ambiguity. ctime is bumped to now by the
// underlying Chtimes (POSIX).
func BtrfsChtimesOnImage(file string, partIndex int, atimeStr, mtimeStr, path string, w io.Writer) error {
	atimeSec, err := strconv.ParseInt(atimeStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid atime %q: %w", atimeStr, err)
	}
	mtimeSec, err := strconv.ParseInt(mtimeStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid mtime %q: %w", mtimeStr, err)
	}
	fs, err := btrfsOpen(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()
	if err := fs.Chtimes(path, time.Unix(atimeSec, 0).UTC(), time.Unix(mtimeSec, 0).UTC()); err != nil {
		return err
	}
	if w != nil {
		fmt.Fprintf(w, "btrfs chtimes atime=%s mtime=%s %s ok\n", atimeStr, mtimeStr, path)
	}
	return nil
}

// BtrfsLinkOnImage creates a hard link newPath pointing at the same inode
// as oldPath inside the btrfs image.
func BtrfsLinkOnImage(file string, partIndex int, oldPath, newPath string, w io.Writer) error {
	fs, err := btrfsOpen(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()
	if err := fs.Link(oldPath, newPath); err != nil {
		return err
	}
	if w != nil {
		fmt.Fprintf(w, "btrfs link %s → %s ok\n", oldPath, newPath)
	}
	return nil
}

// BtrfsSymlinkOnImage creates a symbolic link at linkPath whose target is
// the given string. The link's parent directory must already exist.
func BtrfsSymlinkOnImage(file string, partIndex int, target, linkPath string, w io.Writer) error {
	fs, err := btrfsOpen(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()
	if err := fs.Symlink(target, linkPath); err != nil {
		return err
	}
	if w != nil {
		fmt.Fprintf(w, "btrfs symlink %s → %s ok\n", linkPath, target)
	}
	return nil
}

// BtrfsSetXattrOnImage attaches an xattr (name, value) to path. The value
// is taken verbatim as a UTF-8 byte slice — callers that need binary can
// preprocess at the shell level.
func BtrfsSetXattrOnImage(file string, partIndex int, path, name, value string, w io.Writer) error {
	fs, err := btrfsOpen(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()
	if err := fs.SetXattr(path, name, []byte(value)); err != nil {
		return err
	}
	if w != nil {
		fmt.Fprintf(w, "btrfs setxattr %s = %q on %s ok\n", name, value, path)
	}
	return nil
}

// BtrfsRemoveXattrOnImage drops the xattr `name` from path.
func BtrfsRemoveXattrOnImage(file string, partIndex int, path, name string, w io.Writer) error {
	fs, err := btrfsOpen(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()
	if err := fs.RemoveXattr(path, name); err != nil {
		return err
	}
	if w != nil {
		fmt.Fprintf(w, "btrfs removexattr %s on %s ok\n", name, path)
	}
	return nil
}

// BtrfsTruncateOnImage resizes the file at path inside the btrfs image to
// the given size. The size accepts the same human-readable suffixes as
// `diskimage grow` (b/k/m/g/t).
func BtrfsTruncateOnImage(file string, partIndex int, sizeStr, path string, w io.Writer) error {
	sizeBytes, err := parseSize(sizeStr)
	if err != nil {
		return fmt.Errorf("invalid size %q: %w", sizeStr, err)
	}
	fs, err := btrfsOpen(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()
	if err := fs.Truncate(path, sizeBytes); err != nil {
		return err
	}
	if w != nil {
		fmt.Fprintf(w, "btrfs truncate %s → %s ok\n", path, sizeStr)
	}
	return nil
}

// BtrfsExtendedStatOnImage prints the full INODE_ITEM metadata of path —
// the field set returned by the driver's ExtendedStat — in a stat(1)-like
// human-readable format.
func BtrfsExtendedStatOnImage(file string, partIndex int, path string, w io.Writer) error {
	fs, err := btrfsOpen(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()
	st, err := fs.ExtendedStat(path)
	if err != nil {
		return err
	}
	if w == nil {
		return nil
	}
	kind := "regular"
	switch {
	case st.IsDir():
		kind = "directory"
	case st.IsSymlink():
		kind = "symlink"
	}
	fmt.Fprintf(w, "Path:       %s\n", path)
	fmt.Fprintf(w, "Inode:      %d\n", st.Inode)
	fmt.Fprintf(w, "Type:       %s\n", kind)
	fmt.Fprintf(w, "Mode:       0o%04o (perm 0o%03o)\n", st.Mode, st.Mode&0o7777)
	fmt.Fprintf(w, "Size:       %d bytes (on disk: %d)\n", st.Size, st.NBytes)
	fmt.Fprintf(w, "Links:      %d\n", st.NLink)
	fmt.Fprintf(w, "Owner:      uid=%d gid=%d\n", st.UID, st.GID)
	fmt.Fprintf(w, "Generation: %d (transid %d, seq %d)\n", st.Generation, st.TransID, st.Sequence)
	fmt.Fprintf(w, "Flags:      0x%x\n", st.Flags)
	fmt.Fprintf(w, "ATime:      %s\n", st.ATime.Format(time.RFC3339Nano))
	fmt.Fprintf(w, "MTime:      %s\n", st.MTime.Format(time.RFC3339Nano))
	fmt.Fprintf(w, "CTime:      %s\n", st.CTime.Format(time.RFC3339Nano))
	fmt.Fprintf(w, "OTime:      %s\n", st.OTime.Format(time.RFC3339Nano))
	return nil
}

// BtrfsGetLabelOnImage prints the current btrfs volume label. An empty
// label is printed as an empty line.
func BtrfsGetLabelOnImage(file string, partIndex int, w io.Writer) error {
	fs, err := btrfsOpen(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()
	if w != nil {
		fmt.Fprintln(w, fs.Label())
	}
	return nil
}

// BtrfsSetLabelOnImage writes a new volume label into the primary +
// any populated mirror superblock. Capped at 255 bytes by the driver.
func BtrfsSetLabelOnImage(file string, partIndex int, newLabel string, w io.Writer) error {
	fs, err := btrfsOpen(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()
	if err := fs.SetLabel(newLabel); err != nil {
		return err
	}
	if w != nil {
		fmt.Fprintf(w, "btrfs label set %q ok\n", newLabel)
	}
	return nil
}

// parseOctalMode accepts strings like "0755", "755", or "0o755" and
// returns the corresponding os.FileMode containing the permission bits.
func parseOctalMode(s string) (os.FileMode, error) {
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	// Strip optional "0o" prefix or single leading "0".
	stripped := s
	switch {
	case len(stripped) >= 2 && (stripped[:2] == "0o" || stripped[:2] == "0O"):
		stripped = stripped[2:]
	case len(stripped) > 1 && stripped[0] == '0':
		stripped = stripped[1:]
	}
	n, err := strconv.ParseUint(stripped, 8, 32)
	if err != nil {
		return 0, err
	}
	return os.FileMode(n & 0o7777), nil
}
