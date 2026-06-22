package exec

import (
	"fmt"
	"io"
	"strconv"

	"github.com/go-diskimages/diskimage"
	filesystem "github.com/go-filesystems/interface"
	ntfs "github.com/go-filesystems/ntfs"
)

// ntfsOpenLabeller opens the image as ntfs and returns the handle as
// the Labeller capability, plus a Close hook. Mirrors fat32OpenLabeller
// — ntfs.Open returns filesystem.Filesystem so the capability is
// reached via type-assertion.
func ntfsOpenLabeller(file string, partIndex int) (filesystem.Labeller, io.Closer, error) {
	fsType, err := diskimage.DetectFilesystem(file, partIndex)
	if err != nil {
		return nil, nil, fmt.Errorf("detect filesystem: %w", err)
	}
	if fsType != diskimage.FSNTFS {
		return nil, nil, fmt.Errorf("expected ntfs image, got %s", fsType)
	}
	fs, err := ntfs.Open(file, partIndex)
	if err != nil {
		return nil, nil, fmt.Errorf("open ntfs image: %w", err)
	}
	l, ok := fs.(filesystem.Labeller)
	if !ok {
		fs.Close()
		return nil, nil, fmt.Errorf("ntfs driver does not implement Labeller")
	}
	return l, fs, nil
}

// NtfsGetLabelOnImage prints the current ntfs volume label.
func NtfsGetLabelOnImage(file string, partIndex int, w io.Writer) error {
	l, closer, err := ntfsOpenLabeller(file, partIndex)
	if err != nil {
		return err
	}
	defer closer.Close()
	if w != nil {
		fmt.Fprintln(w, l.Label())
	}
	return nil
}

// NtfsSetLabelOnImage writes a new ntfs volume label.
func NtfsSetLabelOnImage(file string, partIndex int, newLabel string, w io.Writer) error {
	l, closer, err := ntfsOpenLabeller(file, partIndex)
	if err != nil {
		return err
	}
	defer closer.Close()
	if err := l.SetLabel(newLabel); err != nil {
		return err
	}
	if w != nil {
		fmt.Fprintf(w, "ntfs label set %q ok\n", newLabel)
	}
	return nil
}

// NtfsCompactFromImage opens the image's NTFS filesystem and runs compaction.
// The booleans control formatting and whether to list file layouts.
func NtfsCompactFromImage(file string, partIndex int, human, listFiles bool, w io.Writer) error {
	fs, err := ntfs.Open(file, partIndex)
	if err != nil {
		return fmt.Errorf("open ntfs image: %w", err)
	}
	defer fs.Close()

	// report before stats
	bf, bu, bfe, btf, blf := fs.FragmentationStats()
	if w != nil {
		if human {
			fmt.Fprintf(w, "Before: files=%d used=%s free_extents=%d total_free=%s largest_free=%s\n", bf, humanSize(bu), bfe, humanSize(btf), humanSize(blf))
		} else {
			fmt.Fprintf(w, "Before: files=%d used=%d free_extents=%d total_free=%d largest_free=%d\n", bf, bu, bfe, btf, blf)
		}
		if listFiles {
			layout := fs.Layout()
			// compute column widths
			pathW := len("Path")
			offW := len("Offset")
			sizeW := len("Size")
			rows := make([][3]string, 0, len(layout))
			for _, l := range layout {
				var offStr, sizeStr string
				if human {
					offStr = humanSize(l.Offset)
					sizeStr = humanSize(l.Size)
				} else {
					offStr = strconv.FormatUint(l.Offset, 10)
					sizeStr = strconv.FormatUint(l.Size, 10)
				}
				if len(l.Path) > pathW {
					pathW = len(l.Path)
				}
				if len(offStr) > offW {
					offW = len(offStr)
				}
				if len(sizeStr) > sizeW {
					sizeW = len(sizeStr)
				}
				rows = append(rows, [3]string{l.Path, offStr, sizeStr})
			}
			fmt.Fprintf(w, "%-*s  %*s  %*s\n", pathW, "Path", offW, "Offset", sizeW, "Size")
			for _, r := range rows {
				fmt.Fprintf(w, "%-*s  %*s  %*s\n", pathW, r[0], offW, r[1], sizeW, r[2])
			}
		}
	}

	if err := fs.Compact(); err != nil {
		return fmt.Errorf("compact ntfs image: %w", err)
	}

	// report after stats
	af, au, afe, atf, alf := fs.FragmentationStats()
	if w != nil {
		if human {
			fmt.Fprintf(w, "After: files=%d used=%s free_extents=%d total_free=%s largest_free=%s\n", af, humanSize(au), afe, humanSize(atf), humanSize(alf))
		} else {
			fmt.Fprintf(w, "After: files=%d used=%d free_extents=%d total_free=%d largest_free=%d\n", af, au, afe, atf, alf)
		}
		if listFiles {
			layout := fs.Layout()
			// compute column widths
			pathW := len("Path")
			offW := len("Offset")
			sizeW := len("Size")
			rows := make([][3]string, 0, len(layout))
			for _, l := range layout {
				var offStr, sizeStr string
				if human {
					offStr = humanSize(l.Offset)
					sizeStr = humanSize(l.Size)
				} else {
					offStr = strconv.FormatUint(l.Offset, 10)
					sizeStr = strconv.FormatUint(l.Size, 10)
				}
				if len(l.Path) > pathW {
					pathW = len(l.Path)
				}
				if len(offStr) > offW {
					offW = len(offStr)
				}
				if len(sizeStr) > sizeW {
					sizeW = len(sizeStr)
				}
				rows = append(rows, [3]string{l.Path, offStr, sizeStr})
			}
			fmt.Fprintf(w, "%-*s  %*s  %*s\n", pathW, "Path", offW, "Offset", sizeW, "Size")
			for _, r := range rows {
				fmt.Fprintf(w, "%-*s  %*s  %*s\n", pathW, r[0], offW, r[1], sizeW, r[2])
			}
		}
		fmt.Fprintln(w, "compacted")
	}
	return nil
}
