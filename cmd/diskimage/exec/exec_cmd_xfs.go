package exec

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/go-diskimages/diskimage"
	filesystem_xfs "github.com/go-filesystems/xfs"
)

// xfsOpen opens the image as an XFS filesystem, verifying the detected
// on-disk filesystem really is XFS. Mirrors btrfsOpen.
func xfsOpen(file string, partIndex int) (filesystem_xfs.FS, error) {
	fsType, err := diskimage.DetectFilesystem(file, partIndex)
	if err != nil {
		return nil, fmt.Errorf("detect filesystem: %w", err)
	}
	if fsType != diskimage.FSXfs {
		return nil, fmt.Errorf("expected xfs image, got %s", fsType)
	}
	fs, err := filesystem_xfs.Open(file, partIndex)
	if err != nil {
		return nil, fmt.Errorf("open xfs image: %w", err)
	}
	return fs, nil
}

// XfsChmodOnImage applies an octal mode (e.g. "0755") to path inside the
// xfs image. Mirrors BtrfsChmodOnImage.
func XfsChmodOnImage(file string, partIndex int, modeStr, path string, w io.Writer) error {
	mode, err := parseOctalMode(modeStr)
	if err != nil {
		return fmt.Errorf("invalid mode %q: %w", modeStr, err)
	}
	fs, err := xfsOpen(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()
	if err := fs.Chmod(path, mode); err != nil {
		return err
	}
	if w != nil {
		fmt.Fprintf(w, "xfs chmod %s %s ok\n", modeStr, path)
	}
	return nil
}

// XfsChownOnImage applies uid:gid to path inside the xfs image.
func XfsChownOnImage(file string, partIndex int, uidStr, gidStr, path string, w io.Writer) error {
	uid64, err := strconv.ParseUint(uidStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid uid %q: %w", uidStr, err)
	}
	gid64, err := strconv.ParseUint(gidStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid gid %q: %w", gidStr, err)
	}
	fs, err := xfsOpen(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()
	if err := fs.Chown(path, uint32(uid64), uint32(gid64)); err != nil {
		return err
	}
	if w != nil {
		fmt.Fprintf(w, "xfs chown %s:%s %s ok\n", uidStr, gidStr, path)
	}
	return nil
}

// XfsChtimesOnImage sets atime + mtime (both Unix seconds) on path. ctime
// is refreshed by the driver per POSIX.
func XfsChtimesOnImage(file string, partIndex int, atimeStr, mtimeStr, path string, w io.Writer) error {
	atimeSec, err := strconv.ParseInt(atimeStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid atime %q: %w", atimeStr, err)
	}
	mtimeSec, err := strconv.ParseInt(mtimeStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid mtime %q: %w", mtimeStr, err)
	}
	fs, err := xfsOpen(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()
	if err := fs.Chtimes(path, time.Unix(atimeSec, 0).UTC(), time.Unix(mtimeSec, 0).UTC()); err != nil {
		return err
	}
	if w != nil {
		fmt.Fprintf(w, "xfs chtimes atime=%s mtime=%s %s ok\n", atimeStr, mtimeStr, path)
	}
	return nil
}

// XfsLinkOnImage creates a hardlink newPath pointing at the same inode as
// oldPath. Directories are rejected by the driver (POSIX rule).
func XfsLinkOnImage(file string, partIndex int, oldPath, newPath string, w io.Writer) error {
	fs, err := xfsOpen(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()
	if err := fs.Link(oldPath, newPath); err != nil {
		return err
	}
	if w != nil {
		fmt.Fprintf(w, "xfs link %s → %s ok\n", oldPath, newPath)
	}
	return nil
}

// XfsSymlinkOnImage creates a symbolic link at linkPath whose target is
// the literal string `target`.
func XfsSymlinkOnImage(file string, partIndex int, target, linkPath string, w io.Writer) error {
	fs, err := xfsOpen(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()
	if err := fs.Symlink(target, linkPath); err != nil {
		return err
	}
	if w != nil {
		fmt.Fprintf(w, "xfs symlink %s → %s ok\n", linkPath, target)
	}
	return nil
}

// XfsTruncateOnImage resizes the regular file at path to the given size.
// Accepts the same human-readable size suffixes as `xfs grow`.
func XfsTruncateOnImage(file string, partIndex int, sizeStr, path string, w io.Writer) error {
	sizeBytes, err := parseSize(sizeStr)
	if err != nil {
		return fmt.Errorf("invalid size %q: %w", sizeStr, err)
	}
	fs, err := xfsOpen(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()
	if err := fs.Truncate(path, sizeBytes); err != nil {
		return err
	}
	if w != nil {
		fmt.Fprintf(w, "xfs truncate %s → %s ok\n", path, sizeStr)
	}
	return nil
}

// XfsGetLabelOnImage prints the current volume label of the xfs image.
// An empty label is printed as an empty line.
func XfsGetLabelOnImage(file string, partIndex int, w io.Writer) error {
	fs, err := xfsOpen(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()
	if w != nil {
		fmt.Fprintln(w, fs.Label())
	}
	return nil
}

// XfsSetLabelOnImage writes a new volume label into the primary + all AG
// secondary superblocks. The label is capped at 12 bytes (xfs sb_fname).
func XfsSetLabelOnImage(file string, partIndex int, newLabel string, w io.Writer) error {
	fs, err := xfsOpen(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()
	if err := fs.SetLabel(newLabel); err != nil {
		return err
	}
	if w != nil {
		fmt.Fprintf(w, "xfs label set %q ok\n", newLabel)
	}
	return nil
}

// XfsExtendedStatOnImage prints the rich inode metadata of path in a
// stat(1)-like human-readable format.
func XfsExtendedStatOnImage(file string, partIndex int, path string, w io.Writer) error {
	fs, err := xfsOpen(file, partIndex)
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
	fmt.Fprintf(w, "Size:       %d bytes (blocks: %d)\n", st.Size, st.NBlocks)
	fmt.Fprintf(w, "Links:      %d\n", st.NLink)
	fmt.Fprintf(w, "Owner:      uid=%d gid=%d\n", st.UID, st.GID)
	fmt.Fprintf(w, "Generation: %d\n", st.Generation)
	fmt.Fprintf(w, "ATime:      %s\n", st.ATime.Format(time.RFC3339Nano))
	fmt.Fprintf(w, "MTime:      %s\n", st.MTime.Format(time.RFC3339Nano))
	fmt.Fprintf(w, "CTime:      %s\n", st.CTime.Format(time.RFC3339Nano))
	fmt.Fprintf(w, "CRTime:     %s\n", st.CRTime.Format(time.RFC3339Nano))
	return nil
}

// XfsGrowFromImage opens the image's XFS filesystem and grows it to the
// requested absolute size (e.g. 20G). The size string uses the same
// suffixes accepted by `diskimage grow`.
func XfsGrowFromImage(file string, partIndex int, sizeStr string, w io.Writer) error {
	sizeBytes, err := parseSize(sizeStr)
	if err != nil {
		return fmt.Errorf("invalid size %q: %w", sizeStr, err)
	}

	fs, err := filesystem_xfs.Open(file, partIndex)
	if err != nil {
		return fmt.Errorf("open xfs image: %w", err)
	}
	defer fs.Close()

	if err := fs.GrowTo(sizeBytes); err != nil {
		return fmt.Errorf("xfs grow failed: %w", err)
	}
	if w != nil {
		fmt.Fprintf(w, "xfs grown to %s\n", humanSize(uint64(sizeBytes)))
	}
	return nil
}

// parseSize converts a human-readable size string to bytes.
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size string")
	}
	suffixes := map[string]int64{
		"b": 1,
		"k": 1024, "kb": 1024, "kib": 1024,
		"m": 1024 * 1024, "mb": 1024 * 1024, "mib": 1024 * 1024,
		"g": 1024 * 1024 * 1024, "gb": 1024 * 1024 * 1024, "gib": 1024 * 1024 * 1024,
		"t": 1024 * 1024 * 1024 * 1024, "tb": 1024 * 1024 * 1024 * 1024, "tib": 1024 * 1024 * 1024 * 1024,
	}
	lower := strings.ToLower(s)
	// Try suffixes by longest first
	keys := make([]string, 0, len(suffixes))
	for k := range suffixes {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && len(keys[j]) > len(keys[j-1]); j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	for _, suf := range keys {
		if strings.HasSuffix(lower, suf) {
			numStr := strings.TrimSpace(strings.TrimSuffix(lower, suf))
			n, err := strconv.ParseInt(numStr, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("cannot parse number %q", numStr)
			}
			if n <= 0 {
				return 0, fmt.Errorf("size must be positive")
			}
			return n * suffixes[suf], nil
		}
	}
	n, err := strconv.ParseInt(lower, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot parse size %q", s)
	}
	if n <= 0 {
		return 0, fmt.Errorf("size must be positive")
	}
	return n, nil
}
