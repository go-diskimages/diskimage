package diskimage

import (
	"fmt"
	"os"
	"path/filepath"

	disk_dmg "github.com/go-diskimages/dmg"
)

// ConvertImageFormat converts the image at path to the given format
// (e.g. "UDZO" or "UDSP") in-place. Works on all platforms.
//
// "UDRW" is the raw image, with no UDIF container: that is what hdiutil
// writes for it and the only shape macOS mounts read/write. A raw image is
// accepted as the source too, so the conversion goes both ways.
func ConvertImageFormat(path, dstFormat string) error {
	if path == "" {
		return fmt.Errorf("diskimage: path is required")
	}
	dir := filepath.Dir(path)
	tmpPath := filepath.Join(dir, filepath.Base(path)+".convert.tmp")
	if err := disk_dmg.ConvertUDIF(path, tmpPath, dstFormat); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("convert udif: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("replace original image: %w", err)
	}
	return nil
}

// ResizeImage resizes a disk image:
//   - UDIF images: uses the pure-Go UDIF resize, works on all platforms, and
//     puts the image back in the shape it was found in. A sparse image stays
//     sparse; it used to be refused here because growing one silently
//     rewrote it uncompressed.
//   - Raw files: truncates to new size via Grow.
func ResizeImage(path string, newSizeBytes int64) error {
	if path == "" {
		return fmt.Errorf("diskimage: path is required")
	}
	if newSizeBytes <= 0 {
		return fmt.Errorf("diskimage: size must be positive")
	}
	if disk_dmg.IsUDIF(path) {
		return disk_dmg.ResizeUDRW(path, newSizeBytes)
	}
	// Fallback: raw image file.
	return Grow(path, newSizeBytes)
}
