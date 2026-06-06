package copy_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-diskimages/diskimage"
	cpkg "github.com/go-diskimages/diskimage/cmd/diskimage/copy"
)

// TestCopyToFromImage performs a real round-trip copy using a temporary
// disk image formatted as FAT32 (small footprint). This is an integration
// style test that exercises the real formatters and filesystem drivers —
// no stubs are used so the behavior corresponds to reality.
func TestCopyToFromImage(t *testing.T) {
	tmp := t.TempDir()

	// Create a local source file.
	src := filepath.Join(tmp, "src.txt")
	content := []byte("hello from host\n")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatalf("write src file: %v", err)
	}

	// Create a small FAT32 image (4 MiB) to keep test time and memory small.
	img := filepath.Join(tmp, "disk.img")
	if err := diskimage.Create(diskimage.CreateOptions{
		Path:       img,
		SizeBytes:  4 * 1024 * 1024,
		Filesystem: diskimage.FSFat32,
	}); err != nil {
		t.Fatalf("create image: %v", err)
	}

	// Copy local -> image
	cmd := cpkg.Command()
	cmd.SetArgs([]string{"--file", img, "--to-image", src, "/hello.txt"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cp --to-image failed: %v", err)
	}

	// Verify file exists inside image
	got, err := diskimage.ReadFile(diskimage.FileOptions{Path: img, PartIndex: -1, FilePath: "/hello.txt"})
	if err != nil {
		t.Fatalf("read inside image: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("content mismatch inside image: got %q want %q", string(got), string(content))
	}

	// Copy image -> local
	dst := filepath.Join(tmp, "dst.txt")
	cmd = cpkg.Command()
	cmd.SetArgs([]string{"--file", img, "--from-image", "/hello.txt", dst})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cp --from-image failed: %v", err)
	}
	out, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst file: %v", err)
	}
	if string(out) != string(content) {
		t.Fatalf("content mismatch roundtrip: got %q want %q", string(out), string(content))
	}
}
