package exec

import (
	"fmt"
	"io"

	filesystem_zfs "github.com/go-filesystems/zfs"
)

// ZfsGrowFromImage opens the image's ZFS pool and grows it to an absolute size.
func ZfsGrowFromImage(file string, partIndex int, sizeStr string, w io.Writer) error {
	sizeBytes, err := parseSize(sizeStr)
	if err != nil {
		return fmt.Errorf("invalid size %q: %w", sizeStr, err)
	}

	fs, err := filesystem_zfs.Open(file, partIndex)
	if err != nil {
		return fmt.Errorf("open zfs image: %w", err)
	}
	defer fs.Close()

	if err := fs.GrowTo(sizeBytes); err != nil {
		return fmt.Errorf("zfs grow failed: %w", err)
	}
	if w != nil {
		fmt.Fprintf(w, "zfs grown to %s\n", humanSize(uint64(sizeBytes)))
	}
	return nil
}
