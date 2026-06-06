package diskimage

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestSetExt4Label_RoundTrip stamps a label on a freshly-created ext4
// image and re-reads it via the public API. Confirms the partition-
// offset detection (PartMBR here) lines up between Create and the
// SetExt4Label / Ext4Label path.
func TestSetExt4Label_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	img := filepath.Join(tmp, "labelled.img")
	if err := Create(CreateOptions{Path: img, SizeBytes: 16 << 20, Partition: PartMBR, Filesystem: FSExt4}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	want := "cloudimg-rootfs"
	if err := SetExt4Label(img, 0, want); err != nil {
		t.Fatalf("SetExt4Label: %v", err)
	}
	got, err := Ext4Label(img, 0)
	if err != nil {
		t.Fatalf("Ext4Label: %v", err)
	}
	if got != want {
		t.Errorf("Ext4Label = %q, want %q", got, want)
	}
}

// TestSetExt4Label_OverwritePrevious confirms a second SetExt4Label
// fully replaces the prior bytes — important when the new label is
// shorter than the old one (no leftover trailing characters).
func TestSetExt4Label_OverwritePrevious(t *testing.T) {
	tmp := t.TempDir()
	img := filepath.Join(tmp, "relabelled.img")
	if err := Create(CreateOptions{Path: img, SizeBytes: 16 << 20, Partition: PartMBR, Filesystem: FSExt4}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := SetExt4Label(img, 0, "first-fifteen!!"); err != nil {
		t.Fatalf("SetExt4Label #1: %v", err)
	}
	if err := SetExt4Label(img, 0, "tiny"); err != nil {
		t.Fatalf("SetExt4Label #2: %v", err)
	}
	got, err := Ext4Label(img, 0)
	if err != nil {
		t.Fatalf("Ext4Label: %v", err)
	}
	if got != "tiny" {
		t.Errorf("after overwrite, got %q, want \"tiny\"", got)
	}
}

func TestSetExt4Label_TooLong(t *testing.T) {
	tmp := t.TempDir()
	img := filepath.Join(tmp, "long.img")
	if err := Create(CreateOptions{Path: img, SizeBytes: 16 << 20, Partition: PartMBR, Filesystem: FSExt4}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := SetExt4Label(img, 0, strings.Repeat("x", 17)); err == nil {
		t.Fatal("expected error for >16-byte label")
	}
}
