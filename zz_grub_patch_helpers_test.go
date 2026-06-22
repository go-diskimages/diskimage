package diskimage

import (
	"strings"
	"testing"

	grubpkg "github.com/go-bootloaders/grub"
)

func TestAlignAndPartTypes(t *testing.T) {
	if got := alignDown(1000, 512); got != 512 {
		t.Fatalf("alignDown: want 512 got %d", got)
	}
	if got := mbrPartType(FSFat32); got != 0x0C {
		t.Fatalf("mbrPartType fat32: want 0x0C got 0x%X", got)
	}
	if got := mbrPartType(FSExFAT); got != 0x07 {
		t.Fatalf("mbrPartType exfat: want 0x07 got 0x%X", got)
	}
	if got := mbrPartType(FSExt4); got != 0x83 {
		t.Fatalf("mbrPartType default: want 0x83 got 0x%X", got)
	}
	gfat := gptPartTypeGUID(FSFat32)
	if gfat[0] != 0xA2 {
		t.Fatalf("gptPartTypeGUID fat32: unexpected guid start: 0x%X", gfat[0])
	}
	gdef := gptPartTypeGUID(FSExt4)
	if gdef[0] != 0xAF {
		t.Fatalf("gptPartTypeGUID default: unexpected guid start: 0x%X", gdef[0])
	}
}

func TestGrubCmdlineRemoveWord(t *testing.T) {
	// Quick path: word not present
	in := "console=tty0 root=/dev/rd0"
	if out := grubpkg.CmdlineRemoveWord(in, "quiet"); out != in {
		t.Fatalf("expected unchanged, got %q", out)
	}

	// Remove one of multiple tokens
	in2 := "quiet splash"
	if out := grubpkg.CmdlineRemoveWord(in2, "quiet"); out != "splash" {
		t.Fatalf("expected 'splash', got %q", out)
	}

	// Remove the only token -> empty string
	if out := grubpkg.CmdlineRemoveWord("quiet", "quiet"); out != "" {
		t.Fatalf("expected empty string, got %q", out)
	}
}

func TestPatchGrubCfgContent_Basic(t *testing.T) {
	input := "menuentry 'test' {\n    linux /vmlinuz root=/dev/rd0 quiet splash\n}"
	out := grubpkg.PatchGrubCfgContent(input)
	if out == input {
		t.Fatalf("expected modified output, got unchanged")
	}
	// ensures consoles were added and quiet/splash removed
	if !strings.Contains(out, "console=tty0") || !strings.Contains(out, "console=hvc0") {
		t.Fatalf("expected consoles added; got %q", out)
	}
	if strings.Contains(out, "quiet") || strings.Contains(out, "splash") {
		t.Fatalf("quiet/splash should be removed; got %q", out)
	}
}

func TestPatchGrubCfgContent_Linux16_NoDup(t *testing.T) {
	input := "    linux16 /vmlinuz console=hvc0 quiet"
	out := grubpkg.PatchGrubCfgContent(input)
	// Should preserve existing console and not duplicate
	if strings.Count(out, "console=hvc0") != 1 {
		t.Fatalf("expected single console=hvc0, got %q", out)
	}
	if strings.Contains(out, "quiet") {
		t.Fatalf("quiet should be removed, got %q", out)
	}
}

func TestPatchGrubCfgContent_NoChange(t *testing.T) {
	input := "# no linux lines here\nfoo=bar"
	out := grubpkg.PatchGrubCfgContent(input)
	if out != input {
		t.Fatalf("expected unchanged input, got %q", out)
	}
}

func TestPatchGrubDefaultsContent_InsertAndReplace(t *testing.T) {
	wantTerminal := `GRUB_TERMINAL_OUTPUT="console"`
	wantCmdline := `GRUB_CMDLINE_LINUX_DEFAULT="console=tty0 console=hvc0"`

	// Empty input -> should append both
	out := grubpkg.PatchGrubDefaultsContent("")
	if !strings.Contains(out, wantTerminal) || !strings.Contains(out, wantCmdline) {
		t.Fatalf("expected defaults present, got %q", out)
	}

	// Replace existing values
	inp := "GRUB_TERMINAL_OUTPUT=screen\nGRUB_CMDLINE_LINUX_DEFAULT=\"quiet\""
	out2 := grubpkg.PatchGrubDefaultsContent(inp)
	if !strings.Contains(out2, wantTerminal) || !strings.Contains(out2, wantCmdline) {
		t.Fatalf("expected replaced values, got %q", out2)
	}
}
