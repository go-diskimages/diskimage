package diskimage

import (
	"fmt"
	"os"
	"path/filepath"

	disk_dmg "github.com/go-diskimages/dmg"
)

// ConvertImageFormat converts the UDIF image at path to the given format
// (e.g. "UDRW" or "UDSP") in-place. Works on all platforms.
func ConvertImageFormat(path, dstFormat string) error {
	if path == "" {
		return fmt.Errorf("diskimage: path is required")
	}
	if _, err := disk_dmg.DetectUDIFFormat(path); err != nil {
		return fmt.Errorf("ConvertImageFormat: not a UDIF image: %s", path)
	}
	dir := filepath.Dir(path)
	tmpPath := filepath.Join(dir, filepath.Base(path)+".convert.tmp")
	if err := disk_dmg.ConvertUDIF(path, tmpPath, dstFormat); err != nil {
		return fmt.Errorf("convert udif: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("replace original image: %w", err)
	}
	return nil
}

// ResizeImage resizes a disk image:
//   - UDIF images (UDRW, UDSP, …): uses the pure-Go UDIF resize, works on all platforms.
//     UDSP cannot be resized directly; convert to UDRW first.
//   - Raw files: truncates to new size via Grow.
func ResizeImage(path string, newSizeBytes int64) error {
	if path == "" {
		return fmt.Errorf("diskimage: path is required")
	}
	if newSizeBytes <= 0 {
		return fmt.Errorf("diskimage: size must be positive")
	}
	if f, err := disk_dmg.DetectUDIFFormat(path); err == nil {
		if f == "UDSP" {
			return fmt.Errorf("ResizeImage: cannot resize UDSP directly; convert to UDRW first")
		}
		return disk_dmg.ResizeUDRW(path, newSizeBytes)
	}
	// Fallback: raw image file.
	return Grow(path, newSizeBytes)
}
