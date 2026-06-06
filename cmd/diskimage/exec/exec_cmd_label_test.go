package exec

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	filesystem_btrfs "github.com/go-filesystems/btrfs"
	filesystem_exfat "github.com/go-filesystems/exfat"
	filesystem_ext4 "github.com/go-filesystems/ext4"
	filesystem_fat32 "github.com/go-filesystems/fat32"
	ntfs "github.com/go-filesystems/ntfs"
	filesystem_xfs "github.com/go-filesystems/xfs"
)

func formatXfsForLabel(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "disk.img")
	fs, err := filesystem_xfs.Format(p, xfsCliTestSize, filesystem_xfs.FormatConfig{})
	if err != nil {
		t.Fatalf("Format xfs: %v", err)
	}
	fs.Close()
	return p
}

func formatExt4ForLabel(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "disk.img")
	fs, err := filesystem_ext4.Format(p, 4*1024*1024, filesystem_ext4.FormatConfig{})
	if err != nil {
		t.Fatalf("Format ext4: %v", err)
	}
	fs.Close()
	return p
}

// ──────── xfs ────────────────────────────────────────────────────────

func TestExecXfs_LabelGet_DefaultIsEmpty(t *testing.T) {
	img := formatXfsForLabel(t)
	var stdout bytes.Buffer
	if err := XfsGetLabelOnImage(img, -1, &stdout); err != nil {
		t.Fatalf("XfsGetLabelOnImage: %v", err)
	}
	if got := strings.TrimRight(stdout.String(), "\n"); got != "" {
		t.Errorf("default label = %q, want empty", got)
	}
}

func TestExecXfs_LabelSetThenGet_Roundtrip(t *testing.T) {
	img := formatXfsForLabel(t)
	if err := XfsSetLabelOnImage(img, -1, "rootfs", io.Discard); err != nil {
		t.Fatalf("XfsSetLabelOnImage: %v", err)
	}
	var stdout bytes.Buffer
	if err := XfsGetLabelOnImage(img, -1, &stdout); err != nil {
		t.Fatalf("XfsGetLabelOnImage: %v", err)
	}
	if got := strings.TrimRight(stdout.String(), "\n"); got != "rootfs" {
		t.Errorf("label after set = %q, want %q", got, "rootfs")
	}
}

func TestExecXfs_LabelSet_RejectsTooLong(t *testing.T) {
	img := formatXfsForLabel(t)
	// xfs sb_fname caps at 12 bytes — driver must error.
	if err := XfsSetLabelOnImage(img, -1, strings.Repeat("x", 20), io.Discard); err == nil {
		t.Fatal("XfsSetLabelOnImage with oversize label unexpectedly succeeded")
	}
}

// ──────── ext4 ────────────────────────────────────────────────────────

func TestExecExt4_LabelGet_DefaultIsEmpty(t *testing.T) {
	img := formatExt4ForLabel(t)
	var stdout bytes.Buffer
	if err := Ext4GetLabelOnImage(img, -1, &stdout); err != nil {
		t.Fatalf("Ext4GetLabelOnImage: %v", err)
	}
	if got := strings.TrimRight(stdout.String(), "\n"); got != "" {
		t.Errorf("default label = %q, want empty", got)
	}
}

func TestExecExt4_LabelSetThenGet_Roundtrip(t *testing.T) {
	img := formatExt4ForLabel(t)
	if err := Ext4SetLabelOnImage(img, -1, "datafs", io.Discard); err != nil {
		t.Fatalf("Ext4SetLabelOnImage: %v", err)
	}
	var stdout bytes.Buffer
	if err := Ext4GetLabelOnImage(img, -1, &stdout); err != nil {
		t.Fatalf("Ext4GetLabelOnImage: %v", err)
	}
	if got := strings.TrimRight(stdout.String(), "\n"); got != "datafs" {
		t.Errorf("label after set = %q, want %q", got, "datafs")
	}
}

func TestExecExt4_LabelSet_RejectsTooLong(t *testing.T) {
	img := formatExt4ForLabel(t)
	// ext4 s_volume_name caps at 16 bytes — driver must error.
	if err := Ext4SetLabelOnImage(img, -1, strings.Repeat("x", 20), io.Discard); err == nil {
		t.Fatal("Ext4SetLabelOnImage with oversize label unexpectedly succeeded")
	}
}

// ──────── btrfs ───────────────────────────────────────────────────────

func formatBtrfsForLabel(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "disk.img")
	fs, err := filesystem_btrfs.Format(p, 2*1024*1024, filesystem_btrfs.FormatConfig{})
	if err != nil {
		t.Fatalf("Format btrfs: %v", err)
	}
	fs.Close()
	return p
}

func TestExecBtrfs_LabelGet_DefaultIsEmpty(t *testing.T) {
	img := formatBtrfsForLabel(t)
	var stdout bytes.Buffer
	if err := BtrfsGetLabelOnImage(img, -1, &stdout); err != nil {
		t.Fatalf("BtrfsGetLabelOnImage: %v", err)
	}
	if got := strings.TrimRight(stdout.String(), "\n"); got != "" {
		t.Errorf("default label = %q, want empty", got)
	}
}

func TestExecBtrfs_LabelSetThenGet_Roundtrip(t *testing.T) {
	img := formatBtrfsForLabel(t)
	if err := BtrfsSetLabelOnImage(img, -1, "btrfsrootfs", io.Discard); err != nil {
		t.Fatalf("BtrfsSetLabelOnImage: %v", err)
	}
	var stdout bytes.Buffer
	if err := BtrfsGetLabelOnImage(img, -1, &stdout); err != nil {
		t.Fatalf("BtrfsGetLabelOnImage: %v", err)
	}
	if got := strings.TrimRight(stdout.String(), "\n"); got != "btrfsrootfs" {
		t.Errorf("label after set = %q, want %q", got, "btrfsrootfs")
	}
}

// ──────── fat32 ───────────────────────────────────────────────────────

func formatFat32ForLabel(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "disk.img")
	fs, err := filesystem_fat32.Format(p, 4*1024*1024, filesystem_fat32.FormatConfig{})
	if err != nil {
		t.Fatalf("Format fat32: %v", err)
	}
	fs.Close()
	return p
}

func TestExecFat32_LabelGet_DefaultIsEmpty(t *testing.T) {
	img := formatFat32ForLabel(t)
	var stdout bytes.Buffer
	if err := Fat32GetLabelOnImage(img, -1, &stdout); err != nil {
		t.Fatalf("Fat32GetLabelOnImage: %v", err)
	}
	if got := strings.TrimRight(stdout.String(), "\n"); got != "" {
		t.Errorf("default label = %q, want empty", got)
	}
}

func TestExecFat32_LabelSetThenGet_Roundtrip(t *testing.T) {
	img := formatFat32ForLabel(t)
	if err := Fat32SetLabelOnImage(img, -1, "USBDISK", io.Discard); err != nil {
		t.Fatalf("Fat32SetLabelOnImage: %v", err)
	}
	var stdout bytes.Buffer
	if err := Fat32GetLabelOnImage(img, -1, &stdout); err != nil {
		t.Fatalf("Fat32GetLabelOnImage: %v", err)
	}
	if got := strings.TrimRight(stdout.String(), "\n"); got != "USBDISK" {
		t.Errorf("label after set = %q, want %q", got, "USBDISK")
	}
}

func TestExecFat32_LabelSet_RejectsTooLong(t *testing.T) {
	img := formatFat32ForLabel(t)
	// fat32 cap = 11 bytes — driver must error.
	if err := Fat32SetLabelOnImage(img, -1, strings.Repeat("X", 20), io.Discard); err == nil {
		t.Fatal("Fat32SetLabelOnImage with oversize label unexpectedly succeeded")
	}
}

// ──────── ntfs ────────────────────────────────────────────────────────

func formatNtfsForLabel(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "disk.img")
	fs, err := ntfs.Format(p, 4*1024*1024, ntfs.FormatConfig{})
	if err != nil {
		t.Fatalf("Format ntfs: %v", err)
	}
	fs.Close()
	return p
}

func TestExecNtfs_LabelGet_DefaultIsEmpty(t *testing.T) {
	img := formatNtfsForLabel(t)
	var stdout bytes.Buffer
	if err := NtfsGetLabelOnImage(img, -1, &stdout); err != nil {
		t.Fatalf("NtfsGetLabelOnImage: %v", err)
	}
	if got := strings.TrimRight(stdout.String(), "\n"); got != "" {
		t.Errorf("default label = %q, want empty", got)
	}
}

func TestExecNtfs_LabelSetThenGet_Roundtrip(t *testing.T) {
	img := formatNtfsForLabel(t)
	if err := NtfsSetLabelOnImage(img, -1, "WinDrive", io.Discard); err != nil {
		t.Fatalf("NtfsSetLabelOnImage: %v", err)
	}
	var stdout bytes.Buffer
	if err := NtfsGetLabelOnImage(img, -1, &stdout); err != nil {
		t.Fatalf("NtfsGetLabelOnImage: %v", err)
	}
	if got := strings.TrimRight(stdout.String(), "\n"); got != "WinDrive" {
		t.Errorf("label after set = %q, want %q", got, "WinDrive")
	}
}

// ──────── exfat ───────────────────────────────────────────────────────

func formatExfatForLabel(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "disk.img")
	fs, err := filesystem_exfat.Format(p, 4*1024*1024, filesystem_exfat.FormatConfig{})
	if err != nil {
		t.Fatalf("Format exfat: %v", err)
	}
	fs.Close()
	return p
}

func TestExecExfat_LabelGet_DefaultIsEmpty(t *testing.T) {
	img := formatExfatForLabel(t)
	var stdout bytes.Buffer
	if err := ExfatGetLabelOnImage(img, -1, &stdout); err != nil {
		t.Fatalf("ExfatGetLabelOnImage: %v", err)
	}
	if got := strings.TrimRight(stdout.String(), "\n"); got != "" {
		t.Errorf("default label = %q, want empty", got)
	}
}

func TestExecExfat_LabelSetThenGet_Roundtrip(t *testing.T) {
	img := formatExfatForLabel(t)
	if err := ExfatSetLabelOnImage(img, -1, "USBSTICK", io.Discard); err != nil {
		t.Fatalf("ExfatSetLabelOnImage: %v", err)
	}
	var stdout bytes.Buffer
	if err := ExfatGetLabelOnImage(img, -1, &stdout); err != nil {
		t.Fatalf("ExfatGetLabelOnImage: %v", err)
	}
	if got := strings.TrimRight(stdout.String(), "\n"); got != "USBSTICK" {
		t.Errorf("label after set = %q, want %q", got, "USBSTICK")
	}
}

func TestExecExfat_LabelSet_RejectsTooLong(t *testing.T) {
	img := formatExfatForLabel(t)
	if err := ExfatSetLabelOnImage(img, -1, strings.Repeat("X", 20), io.Discard); err == nil {
		t.Fatal("ExfatSetLabelOnImage with oversize label unexpectedly succeeded")
	}
}

func TestExecExt4_RejectsNonExt4(t *testing.T) {
	p := filepath.Join(t.TempDir(), "blob.img")
	if err := writeAllZerosFile(p, 4096); err != nil {
		t.Fatalf("writeAllZerosFile: %v", err)
	}
	if err := Ext4GetLabelOnImage(p, -1, io.Discard); err == nil {
		t.Fatal("Ext4GetLabelOnImage on non-ext4 image unexpectedly succeeded")
	}
}
