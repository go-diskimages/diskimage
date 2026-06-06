package exec

import (
	"fmt"
	"io"

	"github.com/go-diskimages/diskimage"
	filesystem_exfat "github.com/go-filesystems/exfat"
	filesystem "github.com/go-filesystems/interface"
)

// exfatOpenLabeller opens the image as exfat and returns the handle as
// the Labeller capability, plus a Close hook. Same pattern as
// ext4OpenLabeller / fat32OpenLabeller / ntfsOpenLabeller.
func exfatOpenLabeller(file string, partIndex int) (filesystem.Labeller, io.Closer, error) {
	fsType, err := diskimage.DetectFilesystem(file, partIndex)
	if err != nil {
		return nil, nil, fmt.Errorf("detect filesystem: %w", err)
	}
	if fsType != diskimage.FSExFAT {
		return nil, nil, fmt.Errorf("expected exfat image, got %s", fsType)
	}
	fs, err := filesystem_exfat.Open(file, partIndex)
	if err != nil {
		return nil, nil, fmt.Errorf("open exfat image: %w", err)
	}
	l, ok := fs.(filesystem.Labeller)
	if !ok {
		fs.Close()
		return nil, nil, fmt.Errorf("exfat driver does not implement Labeller")
	}
	return l, fs, nil
}

// ExfatGetLabelOnImage prints the current exfat volume label.
func ExfatGetLabelOnImage(file string, partIndex int, w io.Writer) error {
	l, closer, err := exfatOpenLabeller(file, partIndex)
	if err != nil {
		return err
	}
	defer closer.Close()
	if w != nil {
		fmt.Fprintln(w, l.Label())
	}
	return nil
}

// ExfatSetLabelOnImage writes a new exfat volume label. The label is
// capped at 11 UTF-16 code units (Volume Label directory entry).
func ExfatSetLabelOnImage(file string, partIndex int, newLabel string, w io.Writer) error {
	l, closer, err := exfatOpenLabeller(file, partIndex)
	if err != nil {
		return err
	}
	defer closer.Close()
	if err := l.SetLabel(newLabel); err != nil {
		return err
	}
	if w != nil {
		fmt.Fprintf(w, "exfat label set %q ok\n", newLabel)
	}
	return nil
}
