package diskimage

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	disk_dmg "github.com/go-diskimages/dmg"
)

// TestOpenBlockDevice_UDRW_RoundTrip stamps an ext4 label inside a
// UDRW DMG via the full public API (OpenBlockDevice → ext4 → SetLabel)
// and confirms it survives a close/reopen. Exercises:
//   - OpenBlockDevice's UDIF detection branch
//   - udrwBlockDevice.WriteAt / Close koly refresh
//   - readAllUDIFSectors validating the new master checksum on reopen
func TestOpenBlockDevice_UDRW_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	rawPath := filepath.Join(tmp, "src.img")
	if err := Create(CreateOptions{Path: rawPath, SizeBytes: 16 << 20, Partition: PartMBR, Filesystem: FSExt4}); err != nil {
		t.Fatalf("Create raw: %v", err)
	}

	// Wrap the raw image into a UDRW DMG; same content, koly trailer
	// appended.
	dmgPath := filepath.Join(tmp, "labelled.dmg")
	if err := copyFile(rawPath, dmgPath); err != nil {
		t.Fatalf("copy raw → dmg: %v", err)
	}
	if err := disk_dmg.WrapRaw(dmgPath); err != nil {
		t.Fatalf("WrapRaw: %v", err)
	}
	// WrapRaw puts the sectors in a container, uncompressed. macOS mounts
	// that read-only and calls it UDRO -- UDRW is the raw image, which has
	// no container at all -- but the sectors are still one-to-one in the
	// file, which is what the block device below needs.
	variant, err := disk_dmg.DetectUDIFFormat(dmgPath)
	if err != nil {
		t.Fatalf("DetectUDIFFormat: %v", err)
	}
	if variant != "UDRO" {
		t.Fatalf("WrapRaw should produce UDRO, got %s", variant)
	}
	if ok, err := disk_dmg.InPlaceWritable(dmgPath); err != nil || !ok {
		t.Fatalf("InPlaceWritable = %v, %v; want true", ok, err)
	}

	// Round-trip the label via the diskimage public API.
	want := "udrw-rootfs"
	if err := SetExt4Label(dmgPath, 0, want); err != nil {
		t.Fatalf("SetExt4Label on UDRW: %v", err)
	}
	got, err := Ext4Label(dmgPath, 0)
	if err != nil {
		t.Fatalf("Ext4Label on UDRW: %v", err)
	}
	if got != want {
		t.Errorf("Ext4Label = %q, want %q", got, want)
	}

	// Also confirm the koly's master checksum is consistent — if it
	// weren't, readAllUDIFSectors would refuse to open the image.
	if _, err := disk_dmg.UnpackToTemp(dmgPath); err != nil {
		t.Fatalf("UnpackToTemp after relabel: %v (master checksum mismatch?)", err)
	}
}

func TestOpenBlockDevice_UDIF_RejectsCompressed(t *testing.T) {
	tmp := t.TempDir()
	rawPath := filepath.Join(tmp, "src.img")
	if err := Create(CreateOptions{Path: rawPath, SizeBytes: 4 << 20, Partition: PartMBR, Filesystem: FSExt4}); err != nil {
		t.Fatalf("Create raw: %v", err)
	}
	// A container first, so the source has one to convert from.
	udrwPath := filepath.Join(tmp, "udrw.dmg")
	if err := copyFile(rawPath, udrwPath); err != nil {
		t.Fatalf("copy raw → udrw: %v", err)
	}
	if err := disk_dmg.WrapRaw(udrwPath); err != nil {
		t.Fatalf("WrapRaw: %v", err)
	}
	dmgPath := filepath.Join(tmp, "udzo.dmg")
	if err := disk_dmg.ConvertUDIF(udrwPath, dmgPath, "UDZO"); err != nil {
		t.Skipf("ConvertUDIF UDZO not available: %v", err)
	}
	_, err := OpenBlockDevice(dmgPath)
	if err == nil {
		t.Fatal("expected error opening compressed UDIF in-place")
	}
	if !errors.Is(err, errUnsupportedDMGVariant) {
		t.Errorf("error should be the unsupported-variant one, got: %v", err)
	}
	if !strings.Contains(err.Error(), "UDZO") {
		t.Errorf("error should name the format it was given, got: %v", err)
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
