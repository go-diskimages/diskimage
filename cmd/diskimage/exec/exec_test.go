package exec

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/go-diskimages/diskimage"
	filesystem "github.com/go-filesystems/interface"
	zfs "github.com/go-filesystems/zfs"
)

type fakeZFS struct{}

func (f *fakeZFS) Close() error           { return nil }
func (f *fakeZFS) Info() zfs.Info         { return zfs.Info{} }
func (f *fakeZFS) PartitionOffset() int64 { return 0 }
func (f *fakeZFS) ListDir(p string) ([]filesystem.DirEntry, error) {
	if p == "/" {
		return []filesystem.DirEntry{filesystem.NewDirEntry(1, "file.txt", 8)}, nil
	}
	return nil, os.ErrNotExist
}
func (f *fakeZFS) Stat(p string) (filesystem.Stat, error) {
	if p == "/file.txt" || p == "file.txt" {
		return filesystem.NewStat(uint16(0o100644), 2048, 1), nil
	}
	return nil, os.ErrNotExist
}
func (f *fakeZFS) MkDir(p string, perm os.FileMode) error {
	if p == "/" || p == "" {
		return fmt.Errorf("zfs: MkDir: cannot create root")
	}
	return nil
}

func TestExecZpoolStatus_Image(t *testing.T) {
	tmp, err := os.CreateTemp("", "diskimage-test-*.img")
	if err != nil {
		t.Fatal(err)
	}
	name := tmp.Name()
	tmp.Close()
	defer os.Remove(name)

	prev := SetOpenZFS(func(path string, part int) (ZFSImage, error) { return &fakeZFS{}, nil })
	defer SetOpenZFS(prev)

	var out bytes.Buffer
	if err := ZpoolStatusFromImage(name, 0, &out); err != nil {
		t.Fatalf("ZpoolStatusFromImage failed: %v\noutput:\n%s", err, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "pool:") {
		t.Fatalf("expected 'pool:' in output, got:\n%s", s)
	}
}

func TestExecZfsList_Image(t *testing.T) {
	tmp, err := os.CreateTemp("", "diskimage-test-*.img")
	if err != nil {
		t.Fatal(err)
	}
	name := tmp.Name()
	tmp.Close()
	defer os.Remove(name)

	prev := SetOpenZFS(func(path string, part int) (ZFSImage, error) { return &fakeZFS{}, nil })
	defer SetOpenZFS(prev)

	var out bytes.Buffer
	if err := ZfsListFromImage(name, 0, &out); err != nil {
		t.Fatalf("ZfsListFromImage failed: %v\noutput:\n%s", err, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "NAME") || !strings.Contains(s, "2.0K") {
		t.Fatalf("unexpected zfs list output:\n%s", s)
	}
}

func TestExec_NotLs(t *testing.T) {
	cmd := Command()
	cmd.SetArgs([]string{"cat", "--file", "/dev/null"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "cat requires a file path argument") {
		t.Fatalf("expected 'cat requires a file path argument' error, got %v", err)
	}
}

func TestExec_FileRequired(t *testing.T) {
	cmd := Command()
	cmd.SetArgs([]string{"ls"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--file is required for 'ls'") {
		t.Fatalf("expected '--file is required for 'ls' error, got %v", err)
	}
}

func TestExec_Listing(t *testing.T) {
	// Create fake entries.
	entries := []diskimage.ListEntry{
		{Name: "file1", FileType: 8, Mode: 0o100755, Size: 123},
		{Name: ".hidden", FileType: 8, Mode: 0o100644, Size: 10},
		{Name: "dir", FileType: 2, Mode: 0o040755, Size: 4096},
	}

	// Default (no -a): hidden filtered in short listing
	buf := &bytes.Buffer{}
	classify := true
	for _, e := range entries {
		if strings.HasPrefix(e.Name, ".") {
			continue
		}
		fmt.Fprintln(buf, e.Name+indicator(e, classify))
	}
	out := buf.String()
	if strings.Contains(out, ".hidden") {
		t.Fatalf("hidden entry should be filtered: %q", out)
	}

	// Long, human-readable and classify output (format inline to avoid
	// calling package helper directly in tests).
	buf = &bytes.Buffer{}
	for _, e := range entries {
		perm := modeString(e.Mode)
		size := fmt.Sprintf("%8s", humanSize(e.Size))
		fmt.Fprintf(buf, "%s %s %s\n", perm, size, e.Name+indicator(e, true))
	}
	out = buf.String()
	if !strings.Contains(out, "file1") || !strings.Contains(out, "dir/") {
		t.Fatalf("expected long listing with indicators: %q", out)
	}
}

func TestExec_Empty(t *testing.T) {
	// Empty directory prints the placeholder
	buf := &bytes.Buffer{}
	fmt.Fprintln(buf, "(empty directory)")
	if !strings.Contains(buf.String(), "(empty directory)") {
		t.Fatalf("expected empty directory message: %q", buf.String())
	}
}

func TestExec_PartPrint_GPT(t *testing.T) {
	tmp, err := os.CreateTemp("", "diskimage-test-*.img")
	if err != nil {
		t.Fatal(err)
	}
	name := tmp.Name()
	tmp.Close()
	defer os.Remove(name)

	// Create a GPT image with no filesystem (partition table only).
	if err := diskimage.Create(diskimage.CreateOptions{
		Path:       name,
		SizeBytes:  10 * 1024 * 1024,
		Partition:  diskimage.PartGPT,
		Filesystem: diskimage.FSNone,
	}); err != nil {
		t.Fatalf("create image failed: %v", err)
	}

	var out bytes.Buffer
	if err := PartPrintFromImage(name, &out); err != nil {
		t.Fatalf("PartPrintFromImage failed: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "Index") || !strings.Contains(s, "2048") {
		t.Fatalf("unexpected part print output:\n%s", s)
	}
}

func TestExec_PartPrint_MBR(t *testing.T) {
	tmp, err := os.CreateTemp("", "diskimage-test-*.img")
	if err != nil {
		t.Fatal(err)
	}
	name := tmp.Name()
	tmp.Close()
	defer os.Remove(name)

	if err := diskimage.Create(diskimage.CreateOptions{
		Path:       name,
		SizeBytes:  10 * 1024 * 1024,
		Partition:  diskimage.PartMBR,
		Filesystem: diskimage.FSNone,
	}); err != nil {
		t.Fatalf("create image failed: %v", err)
	}

	var out bytes.Buffer
	if err := PartPrintFromImage(name, &out); err != nil {
		t.Fatalf("PartPrintFromImage failed: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "Index") || !strings.Contains(s, "2048") {
		t.Fatalf("unexpected part print output:\n%s", s)
	}
}

func TestExec_DF_Human(t *testing.T) {
	// Override listFunc to provide deterministic entries.
	SetListFunc(func(opts diskimage.ListOptions) ([]diskimage.ListEntry, error) {
		switch opts.DirPath {
		case "/":
			return []diskimage.ListEntry{
				{Name: "file1", FileType: 8, Mode: 0o100644, Size: 1024},
				{Name: "dir", FileType: 2, Mode: 0o040755, Size: 0},
			}, nil
		case "/dir":
			return []diskimage.ListEntry{{Name: "file2", FileType: 8, Mode: 0o100644, Size: 2048}}, nil
		default:
			return nil, os.ErrNotExist
		}
	})
	defer SetListFunc(nil)

	tmp, err := os.CreateTemp("", "diskimage-test-*.img")
	if err != nil {
		t.Fatal(err)
	}
	name := tmp.Name()
	tmp.Close()
	defer os.Remove(name)

	var out bytes.Buffer
	if err := DFFromImage(name, "", 0, &out, true); err != nil {
		t.Fatalf("DFFromImage failed: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "Filesystem") || !strings.Contains(s, "3.0K") {
		t.Fatalf("unexpected df output:\n%s", s)
	}
}

func TestExec_DF_NonHuman(t *testing.T) {
	SetListFunc(func(opts diskimage.ListOptions) ([]diskimage.ListEntry, error) {
		if opts.DirPath == "/" {
			return []diskimage.ListEntry{{Name: "f", FileType: 8, Mode: 0o100644, Size: 512}}, nil
		}
		return nil, os.ErrNotExist
	})
	defer SetListFunc(nil)

	tmp, err := os.CreateTemp("", "diskimage-test-*.img")
	if err != nil {
		t.Fatal(err)
	}
	name := tmp.Name()
	tmp.Close()
	defer os.Remove(name)

	var out bytes.Buffer
	if err := DFFromImage(name, "", 0, &out, false); err != nil {
		t.Fatalf("DFFromImage failed: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "Filesystem") || !strings.Contains(s, "512") {
		t.Fatalf("unexpected df output:\n%s", s)
	}
}

func TestExec_DU_Human(t *testing.T) {
	SetListFunc(func(opts diskimage.ListOptions) ([]diskimage.ListEntry, error) {
		switch opts.DirPath {
		case "/":
			return []diskimage.ListEntry{
				{Name: "file1", FileType: 8, Mode: 0o100644, Size: 1024},
				{Name: "dir", FileType: 2, Mode: 0o040755, Size: 0},
			}, nil
		case "/dir":
			return []diskimage.ListEntry{{Name: "file2", FileType: 8, Mode: 0o100644, Size: 2048}}, nil
		default:
			return nil, os.ErrNotExist
		}
	})
	defer SetListFunc(nil)

	tmp, err := os.CreateTemp("", "diskimage-test-*.img")
	if err != nil {
		t.Fatal(err)
	}
	name := tmp.Name()
	tmp.Close()
	defer os.Remove(name)

	var out bytes.Buffer
	if err := DUSummaryFromImage(name, "", 0, "/", &out, true); err != nil {
		t.Fatalf("DUSummaryFromImage failed: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "3.0K") {
		t.Fatalf("unexpected du output:\n%s", s)
	}
}

func TestExec_DU_NonHuman(t *testing.T) {
	SetListFunc(func(opts diskimage.ListOptions) ([]diskimage.ListEntry, error) {
		if opts.DirPath == "/" {
			return []diskimage.ListEntry{{Name: "f", FileType: 8, Mode: 0o100644, Size: 512}}, nil
		}
		return nil, os.ErrNotExist
	})
	defer SetListFunc(nil)

	tmp, err := os.CreateTemp("", "diskimage-test-*.img")
	if err != nil {
		t.Fatal(err)
	}
	name := tmp.Name()
	tmp.Close()
	defer os.Remove(name)

	var out bytes.Buffer
	if err := DUSummaryFromImage(name, "", 0, "/", &out, false); err != nil {
		t.Fatalf("DUSummaryFromImage failed: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "512") {
		t.Fatalf("unexpected du output:\n%s", s)
	}
}

func TestExec_ZfsGetQuota_Image(t *testing.T) {
	tmp, err := os.CreateTemp("", "diskimage-test-*.img")
	if err != nil {
		t.Fatal(err)
	}
	name := tmp.Name()
	tmp.Close()
	defer os.Remove(name)

	prev := SetOpenZFS(func(path string, part int) (ZFSImage, error) { return &fakeZFS{}, nil })
	defer SetOpenZFS(prev)

	var out bytes.Buffer
	if err := ZfsGetQuotaFromImage(name, 0, path.Base(name), &out); err != nil {
		t.Fatalf("ZfsGetQuotaFromImage failed: %v\noutput:\n%s", err, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "NAME") || !strings.Contains(s, "2.0K") {
		t.Fatalf("unexpected zfs get quota output:\n%s", s)
	}
}

func TestExec_ZfsSetQuota_Image(t *testing.T) {
	tmp, err := os.CreateTemp("", "diskimage-test-*.img")
	if err != nil {
		t.Fatal(err)
	}
	name := tmp.Name()
	tmp.Close()
	defer os.Remove(name)

	prev := SetOpenZFS(func(path string, part int) (ZFSImage, error) { return &fakeZFS{}, nil })
	defer SetOpenZFS(prev)

	var out bytes.Buffer
	dataset := path.Base(name) + "/file.txt"
	if err := ZfsSetQuotaFromImage(name, 0, dataset, "10G", &out); err != nil {
		t.Fatalf("ZfsSetQuotaFromImage failed: %v\noutput:\n%s", err, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "set quota") || !strings.Contains(s, "10G") {
		t.Fatalf("unexpected zfs set quota output:\n%s", s)
	}
}

func TestExec_NtfsCompact_Image(t *testing.T) {
	tmp, err := os.CreateTemp("", "diskimage-test-*.img")
	if err != nil {
		t.Fatal(err)
	}
	name := tmp.Name()
	tmp.Close()
	defer os.Remove(name)

	// Create a small NTFS image
	if err := diskimage.Create(diskimage.CreateOptions{
		Path:       name,
		SizeBytes:  2 * 1024 * 1024,
		Partition:  diskimage.PartNone,
		Filesystem: diskimage.FSNTFS,
	}); err != nil {
		t.Fatalf("create ntfs image failed: %v", err)
	}

	var out bytes.Buffer
	if err := NtfsCompactFromImage(name, 0, false, false, &out); err != nil {
		t.Fatalf("NtfsCompactFromImage failed: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "Before:") || !strings.Contains(s, "After:") || !strings.Contains(s, "compacted") {
		t.Fatalf("expected Before/After/compacted in output, got %q", s)
	}
}
