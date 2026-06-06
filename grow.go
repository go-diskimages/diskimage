package diskimage

import (
	"fmt"
	"os"
)

// Grow resizes the backing image file at path to sizeBytes.
// It behaves like truncate: the file is grown or shrunk to the given size.
func Grow(path string, sizeBytes int64) error {
	if path == "" {
		return fmt.Errorf("diskimage: path is required")
	}
	if sizeBytes <= 0 {
		return fmt.Errorf("diskimage: size must be positive")
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("diskimage: open %s: %w", path, err)
	}
	defer f.Close()
	if err := f.Truncate(sizeBytes); err != nil {
		return fmt.Errorf("diskimage: truncate: %w", err)
	}
	return nil
}
