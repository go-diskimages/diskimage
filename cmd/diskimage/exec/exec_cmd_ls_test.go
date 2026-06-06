package exec

import (
	"bytes"
	"strings"
	"testing"

	"github.com/go-diskimages/diskimage"
)

func TestExecLsCmd_FileRequired(t *testing.T) {
	// Exercises the cobra sub-command path (not the legacy dispatch).
	cmd := Command()
	cmd.SetArgs([]string{"ls"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--file is required for 'ls'") {
		t.Fatalf("expected '--file is required for 'ls'' error, got: %v", err)
	}
}

func TestExecLsCmd_ShortListing(t *testing.T) {
	entries := []diskimage.ListEntry{
		{Name: "file1", FileType: 8, Mode: 0o100755, Size: 100},
		{Name: ".hidden", FileType: 8, Mode: 0o100644, Size: 10},
		{Name: "dir1", FileType: 2, Mode: 0o040755, Size: 4096},
	}
	testEntries = entries
	defer func() { testEntries = nil }()

	cmd := Command()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"ls", "--file", "fake.img"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "file1") || !strings.Contains(s, "dir1") {
		t.Fatalf("expected file1 and dir1 in output: %q", s)
	}
	if strings.Contains(s, ".hidden") {
		t.Fatalf("hidden entry should be filtered without -a: %q", s)
	}
}

func TestExecLsCmd_ShowHidden(t *testing.T) {
	entries := []diskimage.ListEntry{
		{Name: ".hidden", FileType: 8, Mode: 0o100644, Size: 10},
		{Name: "visible", FileType: 8, Mode: 0o100644, Size: 10},
	}
	testEntries = entries
	defer func() { testEntries = nil }()

	cmd := Command()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"ls -a", "--file", "fake.img"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, ".hidden") {
		t.Fatalf("expected .hidden with -a: %q", s)
	}
}

func TestExecLsCmd_LongHuman(t *testing.T) {
	entries := []diskimage.ListEntry{
		{Name: "file1", FileType: 8, Mode: 0o100644, Size: 2048},
		{Name: "dir1", FileType: 2, Mode: 0o040755, Size: 4096},
	}
	testEntries = entries
	defer func() { testEntries = nil }()

	cmd := Command()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"ls -lFh", "--file", "fake.img"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "dir1/") {
		t.Fatalf("expected 'dir1/' (classify) in long output: %q", s)
	}
	if !strings.Contains(s, "2.0K") {
		t.Fatalf("expected human-readable size in output: %q", s)
	}
}

func TestExecLsCmd_Empty(t *testing.T) {
	testEntries = []diskimage.ListEntry{}
	defer func() { testEntries = nil }()

	cmd := Command()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"ls", "--file", "fake.img"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "(empty directory)") {
		t.Fatalf("expected empty directory message: %q", out.String())
	}
}

func TestExecLsCmd_DirPositional(t *testing.T) {
	// Verify that a positional arg overrides the --path flag.
	entries := []diskimage.ListEntry{{Name: "etc_file", FileType: 8, Mode: 0o100644, Size: 1}}
	testEntries = entries
	defer func() { testEntries = nil }()

	cmd := Command()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"ls /etc", "--file", "fake.img"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "etc_file") {
		t.Fatalf("expected etc_file in output: %q", out.String())
	}
}
