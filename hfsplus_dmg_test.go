package diskimage

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestHfsPlusIsAcceptedInADmg(t *testing.T) {
	if !IsDmgSupportedFilesystem(FSHfsPlus) {
		t.Error("HFS+ should be allowed inside a DMG: it is what a distribution image uses")
	}
	found := false
	for _, s := range ValidFilesystems {
		if s == string(FSHfsPlus) {
			found = true
		}
	}
	if !found {
		t.Errorf("hfsplus missing from ValidFilesystems: %v", ValidFilesystems)
	}
}

// DmgUDIFFormat documented a choice and made none: WrapRaw always writes
// UDRW, so asking for UDZO returned a raw image the size of the volume.
func TestConvertDmgFormatIsANoOpForRaw(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.dmg")
	if err := os.WriteFile(p, []byte("not a dmg"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"", "UDRW"} {
		if err := convertDmgFormat(p, f); err != nil {
			t.Errorf("convertDmgFormat(%q) = %v, want nil", f, err)
		}
	}
	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("a no-op conversion rewrote the file")
	}
}

func TestConvertDmgFormatRefusesTheImpossible(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.dmg")
	if err := os.WriteFile(p, []byte("not a dmg at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := convertDmgFormat(p, "UDZO"); err == nil {
		t.Error("converting something that is not a UDIF image should fail")
	}
	if _, err := os.Stat(p + ".converting"); !os.IsNotExist(err) {
		t.Error("a failed conversion left its temporary file behind")
	}
}

func TestCreateRejectsAPassphraseOnHfsPlus(t *testing.T) {
	err := Create(CreateOptions{
		Path: filepath.Join(t.TempDir(), "x.dmg"), SizeBytes: 16 << 20,
		Format: FormatDmg, Filesystem: FSHfsPlus, DmgPassphrase: []byte("x"),
	})
	if err == nil || !strings.Contains(err.Error(), "APFS") {
		t.Errorf("err = %v, want a refusal naming APFS", err)
	}
}

var devRe = regexp.MustCompile(`/dev/disk\d+`)

// The whole point of the wiring, judged by macOS: an HFS+ volume in a UDZO
// DMG that hdiutil mounts. Both halves of that sentence were broken until
// today — the filesystem was not allowed in a DMG at all, and the UDIF writer
// produced images macOS refused.
func TestHfsPlusUDZODmgMounts(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("needs macOS")
	}
	hdiutil, err := exec.LookPath("hdiutil")
	if err != nil {
		t.Skip("hdiutil not present")
	}
	p := filepath.Join(t.TempDir(), "app.dmg")
	if err := Create(CreateOptions{
		Path: p, SizeBytes: 16 << 20,
		Format: FormatDmg, Partition: PartNone, Filesystem: FSHfsPlus,
		Label: "HFSPROBE", DmgUDIFFormat: "UDZO",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	// UDZO on a mostly-empty volume is the reason to prefer HFS+ here; if the
	// image is still 16 MiB the format was not applied.
	if fi.Size() > 1<<20 {
		t.Errorf("UDZO image is %d bytes for a 16 MiB volume — it was not compressed", fi.Size())
	}
	out, err := exec.Command(hdiutil, "attach", "-nobrowse", "-readonly", p).CombinedOutput()
	if err != nil {
		t.Fatalf("hdiutil refused an HFS+ UDZO image: %v\n%s", err, out)
	}
	dev := devRe.FindString(string(out))
	if dev == "" {
		t.Fatalf("attached but no device:\n%s", out)
	}
	// Detach only the device this test attached: a broad match once ejected
	// another session's volumes on this machine.
	t.Cleanup(func() {
		if o, err := exec.Command(hdiutil, "detach", dev).CombinedOutput(); err != nil {
			t.Errorf("detach %s: %v\n%s", dev, err, o)
		}
	})
	if !strings.Contains(string(out), "/Volumes/") {
		t.Errorf("attached but did not mount:\n%s", out)
	}
}
