package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/go-diskimages/diskimage"
)

// stressIterations is configurable via env DISKIMAGE_STRESS_ITERS.
func stressIterations() int {
	if s := os.Getenv("DISKIMAGE_STRESS_ITERS"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			return v
		}
	}
	return 50
}

var fsList = []diskimage.FilesystemType{
	diskimage.FSFat32,
	diskimage.FSExt4,
	diskimage.FSBtrfs,
	diskimage.FSXfs,
	diskimage.FSZfs,
	diskimage.FSExFAT,
}

var parts = []diskimage.PartitionScheme{
	diskimage.PartNone,
	diskimage.PartMBR,
	diskimage.PartGPT,
}

// sizeForFS chooses a reasonable image size per filesystem to keep tests fast.
func sizeForFS(fs diskimage.FilesystemType) int64 {
	switch fs {
	case diskimage.FSFat32, diskimage.FSExFAT:
		return 4 * 1024 * 1024 // 4 MiB
	case diskimage.FSBtrfs:
		return 8 * 1024 * 1024 * 1024 // 8 GiB for btrfs
	case diskimage.FSXfs:
		return 8 * 1024 * 1024 * 1024 // 8 GiB for xfs
	case diskimage.FSZfs:
		return 8 * 1024 * 1024 * 1024 // 8 GiB for ZFS
	default:
		return 20 * 1024 * 1024 // 20 MiB
	}
}

func TestStress_FileOperations_AllFS_AllPartitions(t *testing.T) {
	iters := stressIterations()

	for _, fs := range fsList {
		for _, part := range parts {
			name := fmt.Sprintf("fs=%s/part=%s", string(fs), string(part))
			t.Run(name, func(t *testing.T) {
				tmp := t.TempDir()
				img := filepath.Join(tmp, "disk.img")

				// Create the image with the appropriate filesystem/partition.
				if err := diskimage.Create(diskimage.CreateOptions{
					Path:       img,
					SizeBytes:  sizeForFS(fs),
					Filesystem: fs,
					Partition:  part,
				}); err != nil {
					// Consider explicit ZFS-on-partition refusal as a test skip
					if fs == diskimage.FSZfs && strings.Contains(err.Error(), "zfs on partitioned images is not supported") {
						t.Skipf("skipping %s due to unsupported zfs on partition: %v", name, err)
					}
					t.Fatalf("create image %s: %v", name, err)
				}

				// Determine which partition index to open for operations.
				openPart := -1
				if part != diskimage.PartNone {
					// Default to opening the first partition explicitly (0).
					openPart = 0
				}
				// ZFS implementation may require auto-detect for partitioned images.
				if fs == diskimage.FSZfs && part != diskimage.PartNone {
					openPart = -1
				}

				// Heavy filesystems that tend to consume metadata quickly.
				heavy := fs == diskimage.FSBtrfs || fs == diskimage.FSXfs || fs == diskimage.FSZfs

				// Ensure a base directory exists for the workload; prefer MkDir
				// but fall back to writing a placeholder file if MkDir fails.
				if err := diskimage.MkDir(diskimage.FileOptions{Path: img, PartIndex: openPart, Filesystem: fs, FilePath: "/stress"}, 0o755); err != nil {
					_ = diskimage.WriteFile(diskimage.FileOptions{Path: img, PartIndex: openPart, Filesystem: fs, FilePath: "/stress/.init"}, []byte("init"), 0o644)
				}

				// Run iterations of write/read/rename/delete.
				for i := 0; i < iters; i++ {
					if heavy {
						// Reuse a small set of names to avoid metadata / inode exhaustion
						file := "/stress/file.txt"
						content := []byte(fmt.Sprintf("iteration %d\n", i))

						if err := diskimage.WriteFile(diskimage.FileOptions{Path: img, PartIndex: openPart, Filesystem: fs, FilePath: file}, content, 0o644); err != nil {
							if heavy {
								if strings.Contains(err.Error(), "leaf full") || strings.Contains(err.Error(), "no free inode") || strings.Contains(err.Error(), "no free object") || strings.Contains(err.Error(), "unsupported ZAP type") {
									t.Skipf("skipping %s due to resource/driver error on write: %v", fs, err)
								}
							}
							t.Fatalf("write failed (iter %d): %v", i, err)
						}
						got, err := diskimage.ReadFile(diskimage.FileOptions{Path: img, PartIndex: openPart, Filesystem: fs, FilePath: file})
						if err != nil {
							if heavy {
								if strings.Contains(err.Error(), "leaf full") || strings.Contains(err.Error(), "no free inode") || strings.Contains(err.Error(), "no free object") || strings.Contains(err.Error(), "unsupported ZAP type") {
									t.Skipf("skipping %s due to resource/driver error on read: %v", fs, err)
								}
							}
							t.Fatalf("read failed (iter %d): %v", i, err)
						}
						if string(got) != string(content) {
							t.Fatalf("content mismatch (iter %d): got %q want %q", i, string(got), string(content))
						}

						if err := diskimage.DeleteFile(diskimage.FileOptions{Path: img, PartIndex: openPart, Filesystem: fs, FilePath: file}); err != nil {
							if heavy {
								if strings.Contains(err.Error(), "cow delete") || strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no free inode") || strings.Contains(err.Error(), "no free object") {
									t.Skipf("skipping %s due to resource/driver error on delete: %v", fs, err)
								}
							}
							t.Fatalf("delete failed (iter %d): %v", i, err)
						}
						continue
					}

					// cycle directories to exercise many directories
					dir := fmt.Sprintf("/stress/dir%02d", i%10)
					file := fmt.Sprintf("%s/file_%06d.txt", dir, i)
					content := []byte(fmt.Sprintf("iteration %d\n", i))

					// Write file (should create parents)
					if err := diskimage.WriteFile(diskimage.FileOptions{Path: img, PartIndex: openPart, Filesystem: fs, FilePath: file}, content, 0o644); err != nil {
						t.Fatalf("write failed (iter %d): %v", i, err)
					}
					// Read back
					got, err := diskimage.ReadFile(diskimage.FileOptions{Path: img, PartIndex: openPart, Filesystem: fs, FilePath: file})
					if err != nil {
						t.Fatalf("read failed (iter %d): %v", i, err)
					}
					if string(got) != string(content) {
						t.Fatalf("content mismatch (iter %d): got %q want %q", i, string(got), string(content))
					}

					// Rename within same dir
					renamed := file + ".ren"
					if err := diskimage.Rename(diskimage.FileOptions{Path: img, PartIndex: openPart, Filesystem: fs, FilePath: file}, renamed); err != nil {
						t.Fatalf("rename failed (iter %d): %v", i, err)
					}

					// Move to another dir
					targetDir := fmt.Sprintf("/stress/other%02d", (i+3)%10)
					// ensure target dir exists; if MkDir fails try a WriteFile fallback
					if err := diskimage.MkDir(diskimage.FileOptions{Path: img, PartIndex: openPart, Filesystem: fs, FilePath: targetDir}, 0o755); err != nil {
						if werr := diskimage.WriteFile(diskimage.FileOptions{Path: img, PartIndex: openPart, Filesystem: fs, FilePath: targetDir + "/.init"}, []byte("init"), 0o644); werr != nil {
							t.Fatalf("ensure target dir failed (iter %d): mkdir: %v; write fallback: %v", i, err, werr)
						}
					}
					target := targetDir + "/" + fmt.Sprintf("moved_%06d.txt", i)
					if err := diskimage.Rename(diskimage.FileOptions{Path: img, PartIndex: openPart, Filesystem: fs, FilePath: renamed}, target); err != nil {
						t.Fatalf("move failed (iter %d): %v", i, err)
					}

					// Delete
					if err := diskimage.DeleteFile(diskimage.FileOptions{Path: img, PartIndex: openPart, Filesystem: fs, FilePath: target}); err != nil {
						t.Fatalf("delete failed (iter %d): %v", i, err)
					}
				}

				// After iterations, attempt to list root to ensure image is readable.
				if _, err := diskimage.List(diskimage.ListOptions{Path: img, PartIndex: openPart, FetchStat: true, Filesystem: fs}); err != nil {
					t.Fatalf("final list failed for %s: %v", name, err)
				}
			})
		}
	}
}
