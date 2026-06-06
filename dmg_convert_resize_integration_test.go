//go:build darwin

package diskimage

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	disk_dmg "github.com/go-diskimages/dmg"
)

// Integration test: create a native APFS UDRW DMG, convert to UDSP, back
// to UDRW, and then resize it.
func TestCreateConvertAndResizeUDIF(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only")
	}
	if _, err := exec.LookPath("hdiutil"); err != nil {
		t.Skip("hdiutil not available")
	}

	tmp := t.TempDir()
	img := filepath.Join(tmp, "img.dmg")

	// Create initial UDRW APFS image via diskimage Create (uses hdiutil)
	if err := Create(CreateOptions{Path: img, SizeBytes: 10 << 20, Format: FormatDmg, Filesystem: FSApfs, Label: "Test", DmgUDIFFormat: "UDRW"}); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Ensure it's UDRW
	f, err := disk_dmg.DetectUDIFFormat(img)
	if err != nil || f != "UDRW" {
		t.Fatalf("expected UDRW, detect returned %q err %v", f, err)
	}

	// Convert to UDSP
	if err := ConvertImageFormat(img, "UDSP"); err != nil {
		t.Fatalf("Convert to UDSP failed: %v", err)
	}
	f2, err := disk_dmg.DetectUDIFFormat(img)
	if err != nil || f2 != "UDSP" {
		t.Fatalf("expected UDSP, detect returned %q err %v", f2, err)
	}

	// Convert back to UDRW
	if err := ConvertImageFormat(img, "UDRW"); err != nil {
		t.Fatalf("Convert to UDRW failed: %v", err)
	}
	f3, err := disk_dmg.DetectUDIFFormat(img)
	if err != nil || f3 != "UDRW" {
		t.Fatalf("expected UDRW, detect returned %q err %v", f3, err)
	}

	// Resize to 20MB
	if err := ResizeImage(img, 20*1024*1024); err != nil {
		t.Fatalf("ResizeImage failed: %v", err)
	}
	st, err := os.Stat(img)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st.Size() < 20*1024*1024 {
		t.Fatalf("file size %d smaller than expected", st.Size())
	}
}
