package exec

import (
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/go-diskimages/diskimage"
)

// DUSummaryFromImage computes the total size of pathArg inside image and
// writes a single-line summary similar to `du -sh`.
func DUSummaryFromImage(file string, filesystemArg string, partIndex int, pathArg string, w io.Writer, human bool) error {
	fsType := diskimage.FilesystemType(strings.ToLower(filesystemArg))
	used, err := computeUsedFromPath(file, fsType, partIndex, pathArg)
	if err != nil {
		return err
	}
	if human {
		fmt.Fprintf(w, "%s\t%s\n", humanSize(used), pathArg)
	} else {
		fmt.Fprintf(w, "%d\t%s\n", used, pathArg)
	}
	return nil
}

// computeUsedFromPath walks the tree under startPath and sums sizes using
// the package-local listFunc so tests can override it.
func computeUsedFromPath(file string, fsType diskimage.FilesystemType, partIndex int, startPath string) (uint64, error) {
	visited := make(map[string]bool)
	if startPath == "" {
		startPath = "/"
	}
	return computeUsedFromPathWalk(file, fsType, partIndex, visited, startPath)
}

func computeUsedFromPathWalk(file string, fsType diskimage.FilesystemType, partIndex int, visited map[string]bool, p string) (uint64, error) {
	if visited[p] {
		return 0, nil
	}
	visited[p] = true
	opts := diskimage.ListOptions{Path: file, Filesystem: fsType, PartIndex: partIndex, DirPath: p, FetchStat: true}
	entries, err := listFunc(opts)
	if err != nil {
		return 0, err
	}
	var total uint64
	for _, e := range entries {
		if e.Name == "." || e.Name == ".." {
			continue
		}
		isDir := false
		if e.Mode != 0 {
			if ((e.Mode >> 12) & 0xF) == 0x4 {
				isDir = true
			}
		} else {
			if e.FileType == 2 {
				isDir = true
			}
		}
		var np string
		if p == "/" {
			np = "/" + e.Name
		} else {
			np = path.Join(p, e.Name)
		}
		if isDir {
			sub, err := computeUsedFromPathWalk(file, fsType, partIndex, visited, np)
			if err != nil {
				return 0, err
			}
			total += sub
		} else {
			total += e.Size
		}
	}
	return total, nil
}
