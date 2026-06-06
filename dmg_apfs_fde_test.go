package diskimage

import (
	"path/filepath"
	"testing"

	disk_dmg "github.com/go-diskimages/dmg"
	fde_apfs "github.com/go-fde/apfs"
)

// TestCreate_DmgApfsFDE_RoundTrip creates an encrypted APFS DMG via Create and
// then unwraps the UDIF container and re-opens it via go-fde/apfs to verify
// the embedded FileVault container is well-formed and unlocks with the same
// passphrase.
func TestCreate_DmgApfsFDE_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	img := filepath.Join(tmp, "enc.dmg")
	pass := []byte("correct horse battery staple")

	if err := Create(CreateOptions{
		Path:          img,
		SizeBytes:     1 << 20,
		Format:        FormatDmg,
		Filesystem:    FSApfs,
		DmgPassphrase: pass,
	}); err != nil {
		t.Fatalf("Create encrypted APFS DMG: %v", err)
	}

	// Resulting file is a UDIF container; unwrap to a raw temp.
	raw, err := disk_dmg.UnpackToTemp(img)
	if err != nil {
		t.Fatalf("UnpackToTemp: %v", err)
	}

	// Wrong passphrase must fail.
	if _, err := fde_apfs.Open(raw, []byte("wrong")); err == nil {
		t.Fatal("Open with wrong passphrase unexpectedly succeeded")
	}

	// Correct passphrase must unlock.
	dev, err := fde_apfs.Open(raw, pass)
	if err != nil {
		t.Fatalf("Open with correct passphrase failed: %v", err)
	}
	if err := dev.Close(); err != nil {
		t.Fatalf("dev.Close: %v", err)
	}
}

// TestCreate_DmgRejectsUnsupportedFilesystem verifies the diskimage layer
// (not just the CLI layer) refuses non-mac filesystems for DMG.
func TestCreate_DmgRejectsUnsupportedFilesystem(t *testing.T) {
	tmp := t.TempDir()
	img := filepath.Join(tmp, "bad.dmg")
	err := Create(CreateOptions{
		Path:       img,
		SizeBytes:  1 << 20,
		Format:     FormatDmg,
		Filesystem: FSExt4,
	})
	if err == nil {
		t.Fatal("expected error for ext4 in a DMG image")
	}
}

// TestCreate_PassphraseRequiresApfs verifies a passphrase is rejected unless
// --filesystem apfs is selected.
func TestCreate_PassphraseRequiresApfs(t *testing.T) {
	tmp := t.TempDir()
	img := filepath.Join(tmp, "bad.dmg")
	err := Create(CreateOptions{
		Path:          img,
		SizeBytes:     1 << 20,
		Format:        FormatDmg,
		Filesystem:    FSFat32,
		DmgPassphrase: []byte("nope"),
	})
	if err == nil {
		t.Fatal("expected error: passphrase with non-apfs filesystem")
	}
}
