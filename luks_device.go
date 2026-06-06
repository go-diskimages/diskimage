package diskimage

import (
	"fmt"
	"io"
	"os"

	disk_dmg "github.com/go-diskimages/dmg"
	image_qcow2 "github.com/go-diskimages/qcow2"
	fde "github.com/go-fde/fde"
)

// BlockDevice is a read-write block device backed by a disk image file.
// It is the common interface returned by OpenBlockDevice and
// OpenLUKSBlockDevice.
type BlockDevice interface {
	io.ReaderAt
	WriteAt(p []byte, off int64) (int, error)
	// Size returns the usable byte count of the device.
	Size() int64
	// Close releases all resources associated with the device.
	Close() error
}

// OpenBlockDevice opens a disk image as a read-write block device.
// The format is detected automatically from the file header:
//
//   - QCOW2 → reads/writes go through the QCOW2 copy-on-write layer.
//   - UDIF DMG (UDRW only) → reads/writes hit the uncompressed data
//     fork in place; the koly trailer's CRC-32 checksums are
//     refreshed on Close. Non-UDRW subformats are refused — the
//     compressed variants need an unpack-pack strategy that has a
//     surprise Close cost (see dmg.UnpackToTemp / PackFromTemp if
//     you want it explicitly).
//   - everything else → treated as a raw image; the device reads
//     and writes the file directly.
func OpenBlockDevice(path string) (BlockDevice, error) {
	if image_qcow2.IsQCOW2File(path) {
		dev, err := image_qcow2.OpenDevice(path)
		if err != nil {
			return nil, fmt.Errorf("diskimage: open qcow2 block device %s: %w", path, err)
		}
		return &qcow2BlockDevice{dev: dev}, nil
	}
	if disk_dmg.IsUDIF(path) {
		return openDMGBlockDevice(path)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("diskimage: open raw block device %s: %w", path, err)
	}
	return &rawFileDevice{f: f}, nil
}

// OpenLUKSBlockDevice opens a raw or QCOW2 disk image that contains a LUKS
// encrypted payload. The image format is detected automatically; the LUKS
// container embedded in the virtual disk is unlocked with passphrase.
//
// The returned BlockDevice transparently encrypts/decrypts all I/O.
//
// Supported container formats:
//   - raw   — the file itself is a LUKS device (LUKS header at byte 0)
//   - qcow2 — the virtual disk exported by the QCOW2 layer begins with a
//     LUKS header; I/O is translated through both the QCOW2 and LUKS layers
func OpenLUKSBlockDevice(path string, passphrase []byte) (BlockDevice, error) {
	if image_qcow2.IsQCOW2File(path) {
		qdev, err := image_qcow2.OpenDevice(path)
		if err != nil {
			return nil, fmt.Errorf("diskimage: open qcow2 for LUKS %s: %w", path, err)
		}
		dev, err := fde.OpenLUKSFrom(qdev, passphrase)
		if err != nil {
			qdev.Close()
			return nil, fmt.Errorf("diskimage: unlock LUKS over qcow2 %s: %w", path, err)
		}
		return dev, nil
	}
	dev, err := fde.OpenLUKS(path, passphrase)
	if err != nil {
		return nil, fmt.Errorf("diskimage: unlock LUKS raw device %s: %w", path, err)
	}
	return dev, nil
}

// ─── concrete block device implementations ───────────────────────────────────

// rawFileDevice wraps *os.File as a BlockDevice.
type rawFileDevice struct{ f *os.File }

func (d *rawFileDevice) ReadAt(p []byte, off int64) (int, error) { return d.f.ReadAt(p, off) }
func (d *rawFileDevice) WriteAt(p []byte, off int64) (int, error) {
	return d.f.WriteAt(p, off)
}
func (d *rawFileDevice) Size() int64 {
	fi, err := d.f.Stat()
	if err != nil {
		return 0
	}
	return fi.Size()
}
func (d *rawFileDevice) Close() error { return d.f.Close() }

// Sync flushes the wrapped *os.File. Not part of the BlockDevice
// interface — exposed so adapters (e.g. ext4Adapter in label.go) can
// pick it up via a type assertion when forwarding fsync semantics.
func (d *rawFileDevice) Sync() error { return d.f.Sync() }

// qcow2BlockDevice wraps a *image_qcow2.Device as a BlockDevice.
// The QCOW2 Device.Size() returns (int64, error); we adapt it to the
// single-return signature required by BlockDevice.
type qcow2BlockDevice struct{ dev *image_qcow2.Device }

func (d *qcow2BlockDevice) ReadAt(p []byte, off int64) (int, error) { return d.dev.ReadAt(p, off) }
func (d *qcow2BlockDevice) WriteAt(p []byte, off int64) (int, error) {
	return d.dev.WriteAt(p, off)
}
func (d *qcow2BlockDevice) Size() int64 {
	sz, _ := d.dev.Size()
	return sz
}
func (d *qcow2BlockDevice) Close() error { return d.dev.Close() }

// Sync flushes the underlying QCOW2 device (and its backing file).
// Same rationale as rawFileDevice.Sync — surfaced for adapter-based
// fsync forwarding.
func (d *qcow2BlockDevice) Sync() error { return d.dev.Sync() }
