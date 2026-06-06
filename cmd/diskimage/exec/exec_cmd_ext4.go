package exec

import (
	"fmt"
	"io"

	"github.com/go-diskimages/diskimage"
	filesystem "github.com/go-filesystems/interface"
	filesystem_ext4 "github.com/go-filesystems/ext4"
)

// ext4OpenLabeller opens the image as ext4 and returns the handle as the
// capability interface for label ops, plus a Close hook. The ext4 driver
// returns filesystem.Filesystem from Open; we type-assert to Labeller
// (compile-time-verified upstream) to access Label/SetLabel.
func ext4OpenLabeller(file string, partIndex int) (filesystem.Labeller, io.Closer, error) {
	fsType, err := diskimage.DetectFilesystem(file, partIndex)
	if err != nil {
		return nil, nil, fmt.Errorf("detect filesystem: %w", err)
	}
	if fsType != diskimage.FSExt4 {
		return nil, nil, fmt.Errorf("expected ext4 image, got %s", fsType)
	}
	fs, err := filesystem_ext4.Open(file, partIndex)
	if err != nil {
		return nil, nil, fmt.Errorf("open ext4 image: %w", err)
	}
	l, ok := fs.(filesystem.Labeller)
	if !ok {
		fs.Close()
		return nil, nil, fmt.Errorf("ext4 driver does not implement Labeller")
	}
	return l, fs, nil
}

// Ext4GetLabelOnImage prints the current ext4 volume label. An empty
// label is printed as an empty line.
func Ext4GetLabelOnImage(file string, partIndex int, w io.Writer) error {
	l, closer, err := ext4OpenLabeller(file, partIndex)
	if err != nil {
		return err
	}
	defer closer.Close()
	if w != nil {
		fmt.Fprintln(w, l.Label())
	}
	return nil
}

// Ext4SetLabelOnImage writes a new ext4 volume label. The label is capped
// at 16 bytes (ext4 s_volume_name); the driver returns an error if longer.
func Ext4SetLabelOnImage(file string, partIndex int, newLabel string, w io.Writer) error {
	l, closer, err := ext4OpenLabeller(file, partIndex)
	if err != nil {
		return err
	}
	defer closer.Close()
	if err := l.SetLabel(newLabel); err != nil {
		return err
	}
	if w != nil {
		fmt.Fprintf(w, "ext4 label set %q ok\n", newLabel)
	}
	return nil
}
