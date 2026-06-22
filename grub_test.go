//go:build darwin

package diskimage

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	grubpkg "github.com/go-bootloaders/grub"
)

// ─── rawReplaceAll ────────────────────────────────────────────────────────────

func TestRawReplaceAll_ReplacesAllOccurrences(t *testing.T) {
	content := []byte("aaa bbb aaa ccc aaa")
	path := filepath.Join(t.TempDir(), "test.raw")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	grubpkg.RawReplaceAll(f, []byte("aaa"), []byte("xxx"))
	f.Close()

	got, _ := os.ReadFile(path)
	want := []byte("xxx bbb xxx ccc xxx")
	if !bytes.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRawReplaceAll_NoMatch(t *testing.T) {
	content := []byte("hello world")
	path := filepath.Join(t.TempDir(), "test.raw")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	grubpkg.RawReplaceAll(f, []byte("xyz"), []byte("abc"))
	f.Close()

	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, content) {
		t.Errorf("content unexpectedly changed: %q", got)
	}
}

func TestRawReplaceAll_MismatchedLengths(t *testing.T) {
	content := []byte("aaabbb")
	path := filepath.Join(t.TempDir(), "test.raw")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	grubpkg.RawReplaceAll(f, []byte("aaa"), []byte("zz"))
	f.Close()

	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, content) {
		t.Errorf("content changed despite mismatched lengths: %q", got)
	}
}

// ─── PatchGrubQuiet ───────────────────────────────────────────────────────────

// patchQuietFile applies the modern content-level grub patch to a flat file:
// it reads the file, transforms its content with the supplied transform, and
// writes the result back. This mirrors what the disk-image PatchQuietImage path
// does internally, but against a plain file so the quiet-removal intent can be
// exercised without a full disk image.
func patchQuietFile(t *testing.T, path string, transform func(string) string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(transform(string(b))), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPatchGrubQuiet_DefaultGrub(t *testing.T) {
	content := []byte(`GRUB_CMDLINE_LINUX_DEFAULT="quiet"` + "\n")
	path := filepath.Join(t.TempDir(), "disk.raw")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	patchQuietFile(t, path, grubpkg.PatchGrubDefaultsContent)
	got, _ := os.ReadFile(path)
	if bytes.Contains(got, []byte("quiet")) {
		t.Errorf("expected quiet to be removed, got: %s", got)
	}
}

func TestPatchGrubQuiet_GrubCfgDoubleSpace(t *testing.T) {
	content := []byte("linux /vmlinuz root=/dev/sda ro  quiet splash\n")
	path := filepath.Join(t.TempDir(), "disk.raw")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	patchQuietFile(t, path, grubpkg.PatchGrubCfgContent)
	got, _ := os.ReadFile(path)
	if bytes.Contains(got, []byte("quiet")) {
		t.Errorf("expected quiet to be removed, got: %s", got)
	}
}

func TestPatchGrubQuiet_GrubCfgSingleSpace(t *testing.T) {
	content := []byte("linux /vmlinuz root=/dev/sda ro quiet splash\n")
	path := filepath.Join(t.TempDir(), "disk.raw")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	patchQuietFile(t, path, grubpkg.PatchGrubCfgContent)
	got, _ := os.ReadFile(path)
	if bytes.Contains(got, []byte("quiet")) {
		t.Errorf("expected quiet to be removed, got: %s", got)
	}
}

func TestPatchGrubQuiet_MissingFile(t *testing.T) {
	// The modern disk-image entry point must fail gracefully (error, no panic)
	// when the image path does not exist.
	if _, err := grubpkg.PatchQuietImage("/non/existent/disk.raw"); err == nil {
		t.Error("expected error for missing image, got nil")
	}
}

// ─── patchGrubCfgContent ──────────────────────────────────────────────────────

func TestPatchGrubCfgContent_AddConsoles(t *testing.T) {
	got := grubpkg.PatchGrubCfgContent("  linux /vmlinuz root=/dev/sda ro\n")
	if !strings.Contains(got, "console=tty0") {
		t.Errorf("expected console=tty0 to be added, got: %s", got)
	}
	if !strings.Contains(got, "console=hvc0") {
		t.Errorf("expected console=hvc0 to be added, got: %s", got)
	}
}

func TestPatchGrubCfgContent_RemovesQuietSplash(t *testing.T) {
	got := grubpkg.PatchGrubCfgContent("  linux /vmlinuz root=/dev/sda ro quiet splash\n")
	if strings.Contains(got, "quiet") {
		t.Errorf("expected quiet to be removed, got: %s", got)
	}
	if strings.Contains(got, "splash") {
		t.Errorf("expected splash to be removed, got: %s", got)
	}
}

func TestPatchGrubCfgContent_PreservesIndent(t *testing.T) {
	got := grubpkg.PatchGrubCfgContent("\t  linux /vmlinuz root=/dev/sda ro\n")
	if !strings.HasPrefix(got, "\t  linux") {
		t.Errorf("expected indent to be preserved, got: %q", got)
	}
}

func TestPatchGrubCfgContent_NoLinuxLine(t *testing.T) {
	input := "set root=(hd0,1)\ninitrd /initramfs.img\n"
	if got := grubpkg.PatchGrubCfgContent(input); got != input {
		t.Errorf("expected no change for non-linux lines, got: %s", got)
	}
}

func TestPatchGrubCfgContent_DoesNotDuplicateConsoles(t *testing.T) {
	got := grubpkg.PatchGrubCfgContent("  linux /vmlinuz root=/dev/sda ro console=tty0 console=hvc0\n")
	if count := strings.Count(got, "console=tty0"); count != 1 {
		t.Errorf("expected exactly 1 console=tty0, got %d in: %s", count, got)
	}
	if count := strings.Count(got, "console=hvc0"); count != 1 {
		t.Errorf("expected exactly 1 console=hvc0, got %d in: %s", count, got)
	}
}

func TestPatchGrubCfgContent_Linux16Prefix(t *testing.T) {
	got := grubpkg.PatchGrubCfgContent("\tlinux16 /vmlinuz root=/dev/sda quiet\n")
	if strings.Contains(got, "quiet") {
		t.Errorf("expected quiet removed from linux16 line, got: %s", got)
	}
	if !strings.Contains(got, "console=hvc0") {
		t.Errorf("expected console=hvc0 added to linux16 line, got: %s", got)
	}
}

// ─── grubCmdlineRemoveWord ────────────────────────────────────────────────────

func TestGrubCmdlineRemoveWord_MiddleWord(t *testing.T) {
	got := grubpkg.CmdlineRemoveWord("linux /vmlinuz quiet splash", "quiet")
	if strings.Contains(got, "quiet") {
		t.Errorf("expected quiet removed, got: %s", got)
	}
	if !strings.Contains(got, "splash") {
		t.Errorf("expected splash preserved, got: %s", got)
	}
}

func TestGrubCmdlineRemoveWord_TrailingWord(t *testing.T) {
	got := grubpkg.CmdlineRemoveWord("linux /vmlinuz quiet", "quiet")
	if strings.Contains(got, "quiet") {
		t.Errorf("expected trailing quiet removed, got: %s", got)
	}
}

func TestGrubCmdlineRemoveWord_WordAbsent(t *testing.T) {
	input := "linux /vmlinuz root=/dev/sda"
	if got := grubpkg.CmdlineRemoveWord(input, "quiet"); got != input {
		t.Errorf("no-op: got %q, want %q", got, input)
	}
}

// ─── patchGrubDefaultsContent ────────────────────────────────────────────────

func TestPatchGrubDefaultsContent_OverridesBothKeys(t *testing.T) {
	input := "GRUB_TERMINAL_OUTPUT=\"gfxterm\"\nGRUB_CMDLINE_LINUX_DEFAULT=\"quiet splash\"\n"
	got := grubpkg.PatchGrubDefaultsContent(input)
	if !strings.Contains(got, `GRUB_TERMINAL_OUTPUT="console"`) {
		t.Errorf("expected GRUB_TERMINAL_OUTPUT override, got: %s", got)
	}
	if !strings.Contains(got, `GRUB_CMDLINE_LINUX_DEFAULT="console=tty0 console=hvc0"`) {
		t.Errorf("expected GRUB_CMDLINE_LINUX_DEFAULT override, got: %s", got)
	}
}

func TestPatchGrubDefaultsContent_AppendsMissingKeys(t *testing.T) {
	got := grubpkg.PatchGrubDefaultsContent("# empty\n")
	if !strings.Contains(got, `GRUB_TERMINAL_OUTPUT="console"`) {
		t.Errorf("expected GRUB_TERMINAL_OUTPUT appended, got: %s", got)
	}
	if !strings.Contains(got, `GRUB_CMDLINE_LINUX_DEFAULT="console=tty0 console=hvc0"`) {
		t.Errorf("expected GRUB_CMDLINE_LINUX_DEFAULT appended, got: %s", got)
	}
}

func TestPatchGrubDefaultsContent_EmptyFile(t *testing.T) {
	got := grubpkg.PatchGrubDefaultsContent("")
	if !strings.Contains(got, `GRUB_TERMINAL_OUTPUT="console"`) {
		t.Errorf("expected GRUB_TERMINAL_OUTPUT in output, got: %s", got)
	}
	if !strings.Contains(got, `GRUB_CMDLINE_LINUX_DEFAULT="console=tty0 console=hvc0"`) {
		t.Errorf("expected GRUB_CMDLINE_LINUX_DEFAULT in output, got: %s", got)
	}
}
