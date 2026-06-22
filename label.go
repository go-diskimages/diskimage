package diskimage

import (
	"errors"
	"fmt"

	ext4 "github.com/go-filesystems/ext4"
	filesystem "github.com/go-filesystems/interface"
)

// SetExt4Label opens the ext4 filesystem at imagePath/partIndex and
// writes a new s_volume_name. Works on both raw and QCOW2 images:
// OpenBlockDevice handles format detection, and the ext4 driver does
// the on-disk superblock write directly (no journal). When the
// filesystem has metadata_csum enabled the superblock CRC32c is
// recomputed automatically.
//
// The label is capped at filesystem_ext4.MaxLabelLen (16 bytes); a
// longer label is rejected rather than silently truncated. Pass an
// empty string to clear the label.
//
// SetExt4Label is intended for offline image manipulation (e.g.
// stamping a label on a cloud image before booting it under cloud-
// boot's `device = "LABEL=…"` plan target). Don't call it
// concurrently with a process actively mutating the same image — see
// filesystem_ext4.(*ext4FS).SetLabel for the locking story.
func SetExt4Label(imagePath string, partIndex int, label string) error {
	dev, err := OpenBlockDevice(imagePath)
	if err != nil {
		return fmt.Errorf("diskimage: open block device: %w", err)
	}
	defer dev.Close()

	fs, err := ext4.OpenFromDevice(ext4Adapter{dev: dev}, partIndex)
	if err != nil {
		return fmt.Errorf("diskimage: open ext4 at part %d: %w", partIndex, err)
	}
	defer fs.Close()

	labeller, ok := fs.(filesystem.Labeller)
	if !ok {
		return errors.New("diskimage: ext4 filesystem does not implement Labeller (driver too old?)")
	}
	if err := labeller.SetLabel(label); err != nil {
		return fmt.Errorf("diskimage: set label: %w", err)
	}
	return nil
}

// Ext4Label reads the current volume label from the ext4 filesystem
// at imagePath/partIndex. Convenience helper for callers that want to
// confirm an image's label without round-tripping through e2label.
// Returns the empty string when no label has been set.
func Ext4Label(imagePath string, partIndex int) (string, error) {
	dev, err := OpenBlockDevice(imagePath)
	if err != nil {
		return "", fmt.Errorf("diskimage: open block device: %w", err)
	}
	defer dev.Close()
	fs, err := ext4.OpenFromDevice(ext4Adapter{dev: dev}, partIndex)
	if err != nil {
		return "", fmt.Errorf("diskimage: open ext4 at part %d: %w", partIndex, err)
	}
	defer fs.Close()
	labeller, ok := fs.(filesystem.Labeller)
	if !ok {
		return "", errors.New("diskimage: ext4 filesystem does not implement Labeller")
	}
	return labeller.Label(), nil
}

// ext4Adapter bridges diskimage's BlockDevice
// (ReadAt/WriteAt/Size→int64/Close) to ext4's blockDevice interface
// (additionally requires Sync, Truncate, Size→(int64, error)). The
// shape difference is intentional — diskimage's BlockDevice
// deliberately keeps a narrow surface — so we adapt locally rather
// than extending it.
//
//   - Sync   : forwarded when the underlying device exposes one
//     (qcow2.Device and *os.File both do), no-op otherwise.
//   - Truncate: stubbed as not-supported. SetExt4Label never grows
//     the image, so the ext4 driver shouldn't call it.
//   - Close  : intentionally a no-op. The caller owns dev and closes
//     it via defer; closing twice would double-free.
type ext4Adapter struct {
	dev BlockDevice
}

func (a ext4Adapter) ReadAt(p []byte, off int64) (int, error)  { return a.dev.ReadAt(p, off) }
func (a ext4Adapter) WriteAt(p []byte, off int64) (int, error) { return a.dev.WriteAt(p, off) }

func (a ext4Adapter) Sync() error {
	if s, ok := a.dev.(interface{ Sync() error }); ok {
		return s.Sync()
	}
	return nil
}

func (a ext4Adapter) Size() (int64, error) { return a.dev.Size(), nil }
func (a ext4Adapter) Truncate(size int64) error {
	return errors.New("ext4Adapter: Truncate not supported")
}
func (a ext4Adapter) Close() error { return nil }
