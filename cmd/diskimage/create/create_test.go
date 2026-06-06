package create

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/go-diskimages/diskimage"
)

func TestParseSize(t *testing.T) {
	cases := map[string]int64{
		"1K":   1024,
		"1KiB": 1024,
		"2M":   2 * 1024 * 1024,
		"3MiB": 3 * 1024 * 1024,
		"1G":   1024 * 1024 * 1024,
		"512":  512,
	}
	for s, want := range cases {
		t.Run(s, func(t *testing.T) {
			n, err := parseSize(s)
			if err != nil {
				t.Fatalf("parseSize(%q) error: %v", s, err)
			}
			if n != want {
				t.Fatalf("parseSize(%q) = %d, want %d", s, n, want)
			}
		})
	}
}

func TestParseSizeErrors(t *testing.T) {
	if _, err := parseSize(""); err == nil {
		t.Fatal("expected error for empty size")
	}
	if _, err := parseSize("abc"); err == nil {
		t.Fatal("expected error for invalid size")
	}
	if _, err := parseSize("0"); err == nil {
		t.Fatal("expected error for zero size")
	}
}

func TestHumanSize(t *testing.T) {
	if got := humanSize(1024); got != "1KiB" {
		t.Fatalf("humanSize(1024) = %q", got)
	}
	if got := humanSize(1024 * 1024); got != "1MiB" {
		t.Fatalf("humanSize(1MiB) = %q", got)
	}
	if got := humanSize(123); got != "123B" {
		t.Fatalf("humanSize(123) = %q", got)
	}
}

// TestRawSubcommand_Success verifies the happy path for "diskimage create raw".
func TestRawSubcommand_Success(t *testing.T) {
	orig := rawCreateFunc
	defer func() { rawCreateFunc = orig }()
	rawCreateFunc = func(opts diskimage.CreateOptions) error { return nil }

	buf := &bytes.Buffer{}
	cmd := Command()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"raw", "--file", "/tmp/disk.img", "--size", "1K"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !strings.Contains(buf.String(), "created /tmp/disk.img") {
		t.Fatalf("unexpected output: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "format=raw") {
		t.Fatalf("format not in output: %q", buf.String())
	}
}

// TestRawSubcommand_Failure propagates errors from rawCreateFunc.
func TestRawSubcommand_Failure(t *testing.T) {
	orig := rawCreateFunc
	defer func() { rawCreateFunc = orig }()
	rawCreateFunc = func(opts diskimage.CreateOptions) error { return errors.New("boom") }

	cmd := Command()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"raw", "--file", "/tmp/disk.img", "--size", "1K"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error from rawCreateFunc")
	}
}

// TestDmgSubcommand_Success verifies the happy path for "diskimage create dmg".
func TestDmgSubcommand_Success(t *testing.T) {
	orig := rawCreateFunc
	defer func() { rawCreateFunc = orig }()
	rawCreateFunc = func(opts diskimage.CreateOptions) error {
		if opts.Format != diskimage.FormatDmg {
			return errors.New("wrong format")
		}
		return nil
	}

	buf := &bytes.Buffer{}
	cmd := Command()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"dmg", "--file", "/tmp/disk.dmg", "--size", "1K"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !strings.Contains(buf.String(), "format=dmg") {
		t.Fatalf("format not in output: %q", buf.String())
	}
}

// TestDmgSubcommand_RejectsUnsupportedFilesystem verifies that DMG creation
// fails for filesystems not natively supported on macOS.
func TestDmgSubcommand_RejectsUnsupportedFilesystem(t *testing.T) {
	orig := rawCreateFunc
	defer func() { rawCreateFunc = orig }()
	called := false
	rawCreateFunc = func(opts diskimage.CreateOptions) error {
		called = true
		return nil
	}

	cmd := Command()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"dmg", "--file", "/tmp/disk.dmg", "--size", "1K", "--filesystem", "ext4"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for unsupported DMG filesystem")
	}
	if called {
		t.Fatal("rawCreateFunc must not be called when validation fails")
	}
}

// TestDmgSubcommand_PasswordRequiresApfs verifies that --password is rejected
// when the filesystem is not APFS.
func TestDmgSubcommand_PasswordRequiresApfs(t *testing.T) {
	orig := rawCreateFunc
	defer func() { rawCreateFunc = orig }()
	rawCreateFunc = func(opts diskimage.CreateOptions) error { return nil }

	cmd := Command()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"dmg", "--file", "/tmp/disk.dmg", "--size", "1K", "--filesystem", "fat32", "--password", "secret"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error: --password without --filesystem apfs")
	}
}

// TestDmgSubcommand_PasswordPropagated verifies that --password reaches
// CreateOptions.DmgPassphrase when --filesystem apfs is used.
func TestDmgSubcommand_PasswordPropagated(t *testing.T) {
	orig := rawCreateFunc
	defer func() { rawCreateFunc = orig }()
	var seen []byte
	rawCreateFunc = func(opts diskimage.CreateOptions) error {
		seen = opts.DmgPassphrase
		return nil
	}

	cmd := Command()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"dmg", "--file", "/tmp/disk.dmg", "--size", "1K", "--filesystem", "apfs", "--password", "s3cret"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if string(seen) != "s3cret" {
		t.Fatalf("DmgPassphrase = %q, want %q", string(seen), "s3cret")
	}
}

// TestEfiSubcommand_Success verifies the happy path for "diskimage create efivars".
func TestEfiSubcommand_Success(t *testing.T) {
	orig := efiVarsCreateFunc
	defer func() { efiVarsCreateFunc = orig }()
	efiVarsCreateFunc = func(path string, sizeBytes int64) error { return nil }

	buf := &bytes.Buffer{}
	cmd := Command()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"efivars", "--file", "/tmp/OVMF_VARS.fd"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !strings.Contains(buf.String(), "format=efivars") {
		t.Fatalf("format not in output: %q", buf.String())
	}
}

// TestEfiSubcommand_Failure propagates errors from efiVarsCreateFunc.
func TestEfiSubcommand_Failure(t *testing.T) {
	orig := efiVarsCreateFunc
	defer func() { efiVarsCreateFunc = orig }()
	efiVarsCreateFunc = func(path string, sizeBytes int64) error { return errors.New("no space") }

	cmd := Command()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"efivars", "--file", "/tmp/OVMF_VARS.fd"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error from efiVarsCreateFunc")
	}
}

// TestEfiSubcommand_InvalidSize verifies that a bad --size returns an error.
func TestEfiSubcommand_InvalidSize(t *testing.T) {
	cmd := Command()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"efivars", "--file", "/tmp/OVMF_VARS.fd", "--size", "bad"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for invalid --size")
	}
}

// TestCreateHasSubcommands verifies that "create" registers raw, dmg, efivars, qcow2.
func TestCreateHasSubcommands(t *testing.T) {
	cmd := Command()
	names := map[string]bool{}
	for _, sub := range cmd.Commands() {
		names[sub.Name()] = true
	}
	for _, want := range []string{"raw", "dmg", "efivars", "qcow2"} {
		if !names[want] {
			t.Errorf("sub-command %q not registered", want)
		}
	}
}

// TestQCOW2Subcommand_Success verifies the happy path for "diskimage create qcow2".
func TestQCOW2Subcommand_Success(t *testing.T) {
	orig := qcow2CreateFunc
	defer func() { qcow2CreateFunc = orig }()
	qcow2CreateFunc = func(path string, sizeBytes int64) error { return nil }

	buf := &bytes.Buffer{}
	cmd := Command()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"qcow2", "--file", "/tmp/disk.qcow2", "--size", "1G"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !strings.Contains(buf.String(), "created /tmp/disk.qcow2") {
		t.Fatalf("unexpected output: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "format=qcow2") {
		t.Fatalf("format not in output: %q", buf.String())
	}
}

// TestQCOW2Subcommand_Failure propagates errors from qcow2CreateFunc.
func TestQCOW2Subcommand_Failure(t *testing.T) {
	orig := qcow2CreateFunc
	defer func() { qcow2CreateFunc = orig }()
	qcow2CreateFunc = func(path string, sizeBytes int64) error { return errors.New("no space") }

	cmd := Command()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"qcow2", "--file", "/tmp/disk.qcow2", "--size", "1G"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error from qcow2CreateFunc")
	}
}

// TestQCOW2Subcommand_InvalidSize verifies that a bad --size returns an error.
func TestQCOW2Subcommand_InvalidSize(t *testing.T) {
	cmd := Command()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"qcow2", "--file", "/tmp/disk.qcow2", "--size", "badsize"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for invalid --size")
	}
}
