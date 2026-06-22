package exec

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/go-diskimages/diskimage"
	"github.com/spf13/cobra"
)

// TestExec_Subcommands_Combinations creates small images for each partition
// scheme and filesystem and exercises the exec subcommands to ensure they
// operate without unexpected errors. Tests skip write-dependent checks when
// writing into the image fails for a given driver.
func TestExec_Subcommands_Combinations(t *testing.T) {
	parts := []diskimage.PartitionScheme{diskimage.PartNone, diskimage.PartMBR, diskimage.PartGPT}
	fss := []diskimage.FilesystemType{diskimage.FSExt4, diskimage.FSFat32, diskimage.FSBtrfs, diskimage.FSXfs, diskimage.FSZfs, diskimage.FSExFAT, diskimage.FSNTFS}
	for _, part := range parts {
		for _, fs := range fss {
			name := fmt.Sprintf("part=%s/fs=%s", part, fs)
			t.Run(name, func(t *testing.T) {
				img, partIdx := createTestImage(t, part, fs)
				writeErr := tryWriteHello(t, img, partIdx, fs)
				runCoreChecks(t, img, partIdx, fs, writeErr)
				if fs == diskimage.FSZfs {
					runZFSChecks(t, img, partIdx)
				}
			})
		}
	}
}

func createTestImage(t *testing.T, part diskimage.PartitionScheme, fs diskimage.FilesystemType) (string, int) {
	tmp, err := os.CreateTemp("", "diskimage-test-*.img")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	img := tmp.Name()
	tmp.Close()
	t.Cleanup(func() { os.Remove(img) })

	// XFS needs a full allocation group (1 AG = 64 MiB) to format. Partitioned
	// schemes (MBR/GPT) consume some space for the table/alignment ahead of the
	// filesystem, so size the image at 80 MiB to keep at least 64 MiB usable in
	// every scheme; the other drivers all fit comfortably within it.
	if err := diskimage.Create(diskimage.CreateOptions{Path: img, SizeBytes: 80 * 1024 * 1024, Partition: part, Filesystem: fs}); err != nil {
		t.Fatalf("create image failed: %v", err)
	}
	partIdx := 0
	if part == diskimage.PartNone {
		partIdx = -1
	}
	return img, partIdx
}

func tryWriteHello(t *testing.T, img string, partIdx int, fs diskimage.FilesystemType) error {
	content := []byte("hello-exec\n")
	return diskimage.WriteFile(diskimage.FileOptions{Path: img, PartIndex: partIdx, Filesystem: fs, FilePath: "/hello.txt"}, content, 0o644)
}

func runCoreChecks(t *testing.T, img string, partIdx int, fs diskimage.FilesystemType, writeErr error) {
	var out bytes.Buffer
	if err := PartPrintFromImage(img, &out); err != nil {
		t.Fatalf("part print failed: %v", err)
	}
	out.Reset()
	if err := DFFromImage(img, "", partIdx, &out, true); err != nil {
		t.Fatalf("df -h failed: %v", err)
	}
	out.Reset()
	if err := DUSummaryFromImage(img, "", partIdx, "/", &out, true); err != nil {
		t.Fatalf("du -sh failed: %v", err)
	}
	out.Reset()
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := handleLS(cmd, []string{"ls"}, img, "", partIdx, "/"); err != nil {
		t.Fatalf("ls failed: %v", err)
	}
	if writeErr == nil {
		out.Reset()
		cmd.SetOut(&out)
		if err := handleCat(cmd, []string{"cat", "/hello.txt"}, img, "", partIdx); err != nil {
			t.Fatalf("cat failed: %v", err)
		}
		got := out.Bytes()
		if !bytes.Contains(got, []byte("hello-exec\n")) {
			t.Fatalf("cat content mismatch: got %q", string(got))
		}
	}
}

func runZFSChecks(t *testing.T, img string, partIdx int) {
	var out bytes.Buffer
	if err := ZpoolStatusFromImage(img, partIdx, &out); err != nil {
		t.Fatalf("zpool status failed: %v", err)
	}
	out.Reset()
	if err := ZfsListFromImage(img, partIdx, &out); err != nil {
		t.Fatalf("zfs list failed: %v", err)
	}
}
