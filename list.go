package diskimage

import (
	"fmt"
	"path"

	filesystem_apfs "github.com/go-filesystems/apfs"
	filesystem_btrfs "github.com/go-filesystems/btrfs"
	filesystem_exfat "github.com/go-filesystems/exfat"
	filesystem_ext4 "github.com/go-filesystems/ext4"
	filesystem_fat32 "github.com/go-filesystems/fat32"
	filesystem "github.com/go-filesystems/interface"
	filesystem_ntfs "github.com/go-filesystems/ntfs"
	filesystem_xfs "github.com/go-filesystems/xfs"
	filesystem_zfs "github.com/go-filesystems/zfs"
)

// ListEntry is a single entry returned by List.
type ListEntry struct {
	Name     string
	FileType uint8  // raw filesystem-specific type byte
	Mode     uint16 // POSIX mode (0 if not fetched)
	Size     uint64 // file size in bytes (0 if not fetched)
	Inode    uint64 // inode number (0 if not fetched)
}

// ListOptions controls the List operation.
type ListOptions struct {
	// Path is the path to the disk image file.
	Path string
	// Filesystem selects which filesystem driver to use.
	// If empty, the filesystem type is auto-detected from the image.
	Filesystem FilesystemType
	// PartIndex is the 0-based partition index to open (0 for unpartitioned images).
	PartIndex int
	// DirPath is the directory inside the filesystem to list (default "/").
	DirPath string
	// FetchStat populates Mode, Size and Inode on each ListEntry.
	FetchStat bool
	// UnlockKeys provides optional keys used to unlock encrypted APFS images.
	UnlockKeys []string
}

// fslister is satisfied by every FS type returned by each package's Open().
type fslister interface {
	ListDir(p string) ([]filesystem.DirEntry, error)
	Stat(p string) (filesystem.Stat, error)
	Close() error
}

// List opens a disk image and returns the directory entries at DirPath.
// If opts.Filesystem is empty, the filesystem type is auto-detected.
func List(opts ListOptions) ([]ListEntry, error) {
	if opts.DirPath == "" {
		opts.DirPath = "/"
	}
	if opts.Filesystem == "" || opts.Filesystem == FSNone {
		detected, err := DetectFilesystem(opts.Path, opts.PartIndex)
		if err != nil {
			return nil, fmt.Errorf("auto-detect filesystem: %w", err)
		}
		opts.Filesystem = detected
	}

	var fsi fslister
	var err error

	switch opts.Filesystem {
	case FSExt4:
		fsi, err = filesystem_ext4.Open(opts.Path, opts.PartIndex)
	case FSFat32:
		fsi, err = filesystem_fat32.Open(opts.Path, opts.PartIndex)
	case FSBtrfs:
		fsi, err = filesystem_btrfs.Open(opts.Path, opts.PartIndex)
	case FSXfs:
		fsi, err = filesystem_xfs.Open(opts.Path, opts.PartIndex)
	case FSZfs:
		fsi, err = filesystem_zfs.Open(opts.Path, opts.PartIndex)
	case FSExFAT:
		fsi, err = filesystem_exfat.Open(opts.Path, opts.PartIndex)
	case FSNTFS:
		fsi, err = filesystem_ntfs.Open(opts.Path, opts.PartIndex)
	case FSApfs:
		if len(opts.UnlockKeys) > 0 {
			fsi, err = filesystem_apfs.OpenWithKeys(opts.Path, opts.PartIndex, opts.UnlockKeys...)
		} else {
			fsi, err = filesystem_apfs.Open(opts.Path, opts.PartIndex)
		}
	default:
		return nil, fmt.Errorf("unsupported filesystem %q", opts.Filesystem)
	}
	if err != nil {
		return nil, err
	}
	defer fsi.Close()

	raw, err := fsi.ListDir(opts.DirPath)
	if err != nil {
		return nil, err
	}
	entries := make([]ListEntry, len(raw))
	for i, e := range raw {
		entries[i] = ListEntry{Name: e.Name(), FileType: e.FileType()}
		if opts.FetchStat {
			if st, err := fsi.Stat(path.Join(opts.DirPath, e.Name())); err == nil {
				entries[i].Mode = st.Mode()
				entries[i].Size = st.Size()
				entries[i].Inode = st.Inode()
			}
		}
	}
	return entries, nil
}
