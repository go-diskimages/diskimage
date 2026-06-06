package exec

import (
	"fmt"
	"io"

	"github.com/go-diskimages/diskimage"
	filesystem_fat32 "github.com/go-filesystems/fat32"
	filesystem "github.com/go-filesystems/interface"
)

// fat32OpenLabeller opens the image as fat32 and returns the handle as
// the capability interface for label ops, plus a Close hook. Mirrors
// ext4OpenLabeller — same pattern, different driver.
func fat32OpenLabeller(file string, partIndex int) (filesystem.Labeller, io.Closer, error) {
	fsType, err := diskimage.DetectFilesystem(file, partIndex)
	if err != nil {
		return nil, nil, fmt.Errorf("detect filesystem: %w", err)
	}
	if fsType != diskimage.FSFat32 {
		return nil, nil, fmt.Errorf("expected fat32 image, got %s", fsType)
	}
	fs, err := filesystem_fat32.Open(file, partIndex)
	if err != nil {
		return nil, nil, fmt.Errorf("open fat32 image: %w", err)
	}
	l, ok := fs.(filesystem.Labeller)
	if !ok {
		fs.Close()
		return nil, nil, fmt.Errorf("fat32 driver does not implement Labeller")
	}
	return l, fs, nil
}

// Fat32GetLabelOnImage prints the current fat32 volume label. An empty
// label is printed as an empty line.
func Fat32GetLabelOnImage(file string, partIndex int, w io.Writer) error {
	l, closer, err := fat32OpenLabeller(file, partIndex)
	if err != nil {
		return err
	}
	defer closer.Close()
	if w != nil {
		fmt.Fprintln(w, l.Label())
	}
	return nil
}

// Fat32SetLabelOnImage writes a new fat32 volume label. The label is
// capped at 11 bytes (BPB BS_VolLab); the driver rejects longer values
// and space-pads shorter ones.
func Fat32SetLabelOnImage(file string, partIndex int, newLabel string, w io.Writer) error {
	l, closer, err := fat32OpenLabeller(file, partIndex)
	if err != nil {
		return err
	}
	defer closer.Close()
	if err := l.SetLabel(newLabel); err != nil {
		return err
	}
	if w != nil {
		fmt.Fprintf(w, "fat32 label set %q ok\n", newLabel)
	}
	return nil
}
