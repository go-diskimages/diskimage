package diskimage

import (
	"fmt"
	"os"
	"testing"

	filesystem "github.com/go-filesystems/interface"
)

// fakeFS implements the filesystem.Filesystem interface for unit tests.
type fakeFS2 struct {
	stats    map[string]bool
	writes   map[string]int
	mkErrors map[string]error
}

func newFakeFS2() *fakeFS2 {
	return &fakeFS2{stats: map[string]bool{}, writes: map[string]int{}, mkErrors: map[string]error{}}
}

func (f *fakeFS2) Close() error                      { return nil }
func (f *fakeFS2) ReadFile(p string) ([]byte, error) { return nil, fmt.Errorf("not implemented") }
func (f *fakeFS2) ListDir(p string) ([]filesystem.DirEntry, error) {
	return []filesystem.DirEntry{}, nil
}
func (f *fakeFS2) Stat(p string) (filesystem.Stat, error) {
	if f.stats[p] {
		return filesystem.NewStat(0, 0, 0), nil
	}
	return nil, fmt.Errorf("noent")
}
func (f *fakeFS2) WriteFile(p string, data []byte, perm os.FileMode) error {
	cnt := f.writes[p]
	f.writes[p] = cnt + 1
	if cnt == 0 {
		return fmt.Errorf("write fail")
	}
	return nil
}
func (f *fakeFS2) ReadLink(p string) (string, error) { return "", fmt.Errorf("not implemented") }
func (f *fakeFS2) MkDir(p string, perm os.FileMode) error {
	if err, ok := f.mkErrors[p]; ok {
		// optionally simulate concurrent creation by setting stats before returning error
		if err == errConcurrent {
			f.stats[p] = true
			return fmt.Errorf("concurrent")
		}
		return err
	}
	f.stats[p] = true
	return nil
}
func (f *fakeFS2) DeleteFile(p string) error { return nil }
func (f *fakeFS2) DeleteDir(p string) error  { return nil }
func (f *fakeFS2) Rename(a, b string) error  { return nil }

var errConcurrent = fmt.Errorf("concurrent")

func TestFileOps_RequirePath(t *testing.T) {
	if _, err := ReadFile(FileOptions{}); err == nil {
		t.Fatalf("expected error for empty path")
	}
	if err := MkDir(FileOptions{}, 0); err == nil {
		t.Fatalf("expected error for empty path MkDir")
	}
	if err := DeleteFile(FileOptions{}); err == nil {
		t.Fatalf("expected error for empty path DeleteFile")
	}
	if err := DeleteDir(FileOptions{}); err == nil {
		t.Fatalf("expected error for empty path DeleteDir")
	}
	if err := Rename(FileOptions{}, ""); err == nil {
		t.Fatalf("expected error for empty path Rename")
	}
	if _, err := Stat(FileOptions{}); err == nil {
		t.Fatalf("expected error for empty path Stat")
	}
}

func TestEnsureParentDirs_SuccessAndConcurrent(t *testing.T) {
	fs := newFakeFS2()
	// success path: no existing dirs
	if err := ensureParentDirs(fs, "/a/b/c/file"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if !fs.stats["/a"] || !fs.stats["/a/b"] || !fs.stats["/a/b/c"] {
		t.Fatalf("expected created dirs, got %v", fs.stats)
	}

	// concurrent path: MkDir returns error but Stat then succeeds
	fs2 := newFakeFS2()
	fs2.mkErrors["/x"] = errConcurrent
	if err := ensureParentDirs(fs2, "/x/y/file"); err != nil {
		t.Fatalf("expected nil for concurrent mk error, got %v", err)
	}
	if !fs2.stats["/x"] {
		t.Fatalf("expected /x to be present after concurrent path handling")
	}
}

func TestEnsureParentDirs_MkFail(t *testing.T) {
	fs := newFakeFS2()
	fs.mkErrors["/z"] = fmt.Errorf("perm denied")
	if err := ensureParentDirs(fs, "/z/file"); err == nil {
		t.Fatalf("expected error when MkDir fails and Stat stays missing")
	}
}
