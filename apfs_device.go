package diskimage

import (
	"fmt"

	image_qcow2 "github.com/go-diskimages/qcow2"
	fde "github.com/go-fde/fde"
)

// OpenAPFSBlockDevice opens a raw or QCOW2 disk image that contains a
// FileVault-encrypted APFS container. The image format is detected
// automatically; the APFS key bag embedded in the container is unlocked with
// passphrase.
//
// The returned BlockDevice transparently decrypts / encrypts all I/O with the
// Volume Encryption Key recovered from the container key bag.
//
// Supported container formats:
//   - raw   — the file itself is an APFS container (NX superblock at byte 0)
//   - qcow2 — the virtual disk exported by the QCOW2 layer begins with an
//     APFS container; I/O is translated through both the QCOW2 and APFS layers
func OpenAPFSBlockDevice(path string, passphrase []byte) (BlockDevice, error) {
	if image_qcow2.IsQCOW2File(path) {
		return openAPFSQCOW2BlockDevice(path, passphrase)
	}
	return openAPFSRawBlockDevice(path, passphrase)
}

func openAPFSRawBlockDevice(path string, passphrase []byte) (BlockDevice, error) {
	dev, err := fde.OpenAPFS(path, passphrase)
	if err != nil {
		return nil, fmt.Errorf("diskimage: unlock APFS raw device %s: %w", path, err)
	}
	return dev, nil
}

func openAPFSQCOW2BlockDevice(path string, passphrase []byte) (BlockDevice, error) {
	qdev, err := image_qcow2.OpenDevice(path)
	if err != nil {
		return nil, fmt.Errorf("diskimage: open qcow2 for APFS %s: %w", path, err)
	}
	dev, err := fde.OpenAPFSFrom(qdev, passphrase)
	if err != nil {
		qdev.Close()
		return nil, fmt.Errorf("diskimage: unlock APFS over qcow2 %s: %w", path, err)
	}
	return dev, nil
}
