package diskimage

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	disk_dmg "github.com/go-diskimages/dmg"
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
func TestConvertDmgFormatIsANoOpWithoutAFormat(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.dmg")
	if err := os.WriteFile(p, []byte("not a dmg"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := convertDmgFormat(p, ""); err != nil {
		t.Errorf("convertDmgFormat(%q) = %v, want nil", "", err)
	}
	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("a no-op conversion rewrote the file")
	}
	// "UDRW" no longer reaches here: it is the raw image, so Create stops
	// before wrapping. Asked to convert TO it, this function tries, and
	// says what it could not read rather than pretending it worked.
	if err := convertDmgFormat(p, "UDRW"); err == nil {
		t.Error("convertDmgFormat accepted a file that is not an image")
	}
}

// A DMG asked for as UDRW is the raw volume: no container, and the size of
// the volume exactly. Anything with a koly trailer is mounted read-only.
func TestCreateUDRWLeavesTheRawImage(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rw.dmg")
	const size = 8 << 20
	if err := Create(CreateOptions{Path: p, SizeBytes: size, Format: FormatDmg,
		Filesystem: FSHfsPlus, Label: "RW", DmgUDIFFormat: "UDRW"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if disk_dmg.IsUDIF(p) {
		t.Error("a UDRW image was wrapped in a container")
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != size {
		t.Errorf("the image is %d bytes, want %d", info.Size(), size)
	}
	if ok, err := disk_dmg.InPlaceWritable(p); err != nil || !ok {
		t.Errorf("InPlaceWritable = %v, %v; want true", ok, err)
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

// The claim this vocabulary change rests on, put to Apple's own tool: an
// image asked for as UDRW mounts READ/WRITE. It is the only check here that
// would catch a return to wrapping one in a container, which macOS mounts
// read-only whatever the trailer says.
//
// The volume name is unique per run and the detach names the mount point it
// attached, because a broad match once ejected volumes belonging to another
// session on this machine.
func TestDarwinUDRWMountsWritable(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("hdiutil is macOS-only")
	}
	if _, err := exec.LookPath("hdiutil"); err != nil {
		t.Skip("hdiutil is not installed")
	}
	label := fmt.Sprintf("DIRW%d", os.Getpid())
	p := filepath.Join(t.TempDir(), "rw.dmg")
	if err := Create(CreateOptions{Path: p, SizeBytes: 8 << 20, Format: FormatDmg,
		Filesystem: FSHfsPlus, Label: label, DmgUDIFFormat: "UDRW"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	info, err := exec.Command("hdiutil", "imageinfo", p).CombinedOutput()
	if err != nil {
		t.Fatalf("hdiutil imageinfo: %v\n%s", err, info)
	}
	if !strings.Contains(string(info), "Format: UDRW") {
		t.Errorf("hdiutil does not call this UDRW:\n%s", info)
	}

	mount := "/Volumes/" + label
	if out, err := exec.Command("hdiutil", "attach", p, "-noverify").CombinedOutput(); err != nil {
		t.Fatalf("hdiutil attach: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		if out, err := exec.Command("hdiutil", "detach", mount).CombinedOutput(); err != nil {
			t.Logf("detaching %s: %v\n%s", mount, err, out)
		}
	})
	mounted, err := exec.Command("mount").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(mounted), "\n") {
		if strings.Contains(line, mount) && strings.Contains(line, "read-only") {
			t.Errorf("mounted read-only: %s", line)
		}
	}
	if err := os.WriteFile(filepath.Join(mount, "written-by-the-test"), []byte("ok"), 0o644); err != nil {
		t.Errorf("writing to the mounted volume: %v", err)
	}
}
