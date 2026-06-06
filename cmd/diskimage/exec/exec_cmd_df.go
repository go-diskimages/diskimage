package exec

import (
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/go-diskimages/diskimage"
)

// DFFromImage writes a minimal 'df' output for the image. When human is
// true, sizes are formatted with human-readable units.
func DFFromImage(file string, filesystemArg string, partIndex int, w io.Writer, human bool) error {
	// Compute used space by walking the image tree via listFunc.
	fsType := diskimage.FilesystemType(strings.ToLower(filesystemArg))
	used, err := computeUsedListing(file, fsType, partIndex)
	if err != nil {
		// Non-fatal: continue with used=0
		used = 0
	}

	fi, err := os.Stat(file)
	if err != nil {
		return err
	}
	total := uint64(fi.Size())
	var avail uint64
	if total > used {
		avail = total - used
	} else {
		avail = 0
	}
	var pct int
	if total > 0 {
		pct = int((used * 100) / total)
	}

	if human {
		fmt.Fprintln(w, "Filesystem\tSize\tUsed\tAvail\tUse%\tMounted on")
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d%%\t%s\n",
			path.Base(file), humanSize(used), humanSize(total-used), humanSize(avail), pct, "/")
	} else {
		fmt.Fprintln(w, "Filesystem\tSize\tUsed\tAvail\tUse%\tMounted on")
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d%%\t%s\n",
			path.Base(file), total, used, avail, pct, "/")
	}
	return nil
}

// computeUsedListing recursively sums file sizes using the package listFunc
// so tests can override it.
func computeUsedListing(file string, fsType diskimage.FilesystemType, partIndex int) (uint64, error) {
	visited := make(map[string]bool)
	return computeUsedListingWalk(file, fsType, partIndex, visited, "/")
}

func computeUsedListingWalk(file string, fsType diskimage.FilesystemType, partIndex int, visited map[string]bool, p string) (uint64, error) {
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
			sub, err := computeUsedListingWalk(file, fsType, partIndex, visited, np)
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
