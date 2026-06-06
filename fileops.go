package diskimage

import (
	"fmt"
	"os"
	"path"
	"strings"

	disk_dmg "github.com/go-diskimages/dmg"
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

// FileOptions configures ReadFile/WriteFile operations against an image.
type FileOptions struct {
	// Path is the path to the disk image file.
	Path string
	// Filesystem selects which filesystem driver to use. If empty, the
	// filesystem type is auto-detected from the image.
	Filesystem FilesystemType
	// PartIndex selects the 0-based partition index to open.
	PartIndex int
	// FilePath is the absolute path inside the filesystem (e.g. "/etc/hosts").
	FilePath string
	// UnlockKeys provides optional keys used to unlock encrypted APFS images.
	// For bi-key encrypted images provide two keys.
	UnlockKeys []string
}

type dmgwrapper struct {
	filesystem.Filesystem
	tmpPath  string
	origPath string
}

func (w *dmgwrapper) Close() error {
	err := w.Filesystem.Close()
	if packErr := disk_dmg.PackFromTemp(w.tmpPath, w.origPath); packErr != nil {
		if err != nil {
			return fmt.Errorf("close error: %v; pack error: %w", err, packErr)
		}
		return packErr
	}
	_ = os.Remove(w.tmpPath)
	return err
}

// openFS opens the requested filesystem driver for the image.
func openFS(imagePath string, partIndex int, fsType FilesystemType, unlockKeys []string) (filesystem.Filesystem, error) {
	if fsType == "" || fsType == FSNone {
		detected, err := DetectFilesystem(imagePath, partIndex)
		if err != nil {
			return nil, fmt.Errorf("auto-detect filesystem: %w", err)
		}
		fsType = detected
	}

	// If the image is a UDIF, unpack to a temp file and open the underlying
	// filesystem there. The returned filesystem will re-pack the temp file
	// back into the UDIF on Close.
	if disk_dmg.IsUDIF(imagePath) {
		tmpPath, err := disk_dmg.UnpackToTemp(imagePath)
		if err != nil {
			return nil, err
		}
		// wrapper so Close repacks the temp file back into the DMG
		wrap := func(fs filesystem.Filesystem) filesystem.Filesystem {
			return &dmgwrapper{Filesystem: fs, tmpPath: tmpPath, origPath: imagePath}
		}

		switch fsType {
		case FSExt4:
			fs, err := filesystem_ext4.Open(tmpPath, partIndex)
			if err != nil {
				return nil, err
			}
			return wrap(fs), nil
		case FSFat32:
			fs, err := filesystem_fat32.Open(tmpPath, partIndex)
			if err != nil {
				return nil, err
			}
			return wrap(fs), nil
		case FSBtrfs:
			fs, err := filesystem_btrfs.Open(tmpPath, partIndex)
			if err != nil {
				return nil, err
			}
			return wrap(fs), nil
		case FSExFAT:
			fs, err := filesystem_exfat.Open(tmpPath, partIndex)
			if err != nil {
				return nil, err
			}
			return wrap(fs), nil
		case FSNTFS:
			fs, err := filesystem_ntfs.Open(tmpPath, partIndex)
			if err != nil {
				return nil, err
			}
			return wrap(fs), nil

		case FSApfs:
			var fs filesystem.Filesystem
			var err error
			if len(unlockKeys) > 0 {
				fs, err = filesystem_apfs.OpenWithKeys(tmpPath, partIndex, unlockKeys...)
			} else {
				fs, err = filesystem_apfs.Open(tmpPath, partIndex)
			}
			if err != nil {
				return nil, err
			}
			return wrap(fs), nil
		default:
			return nil, fmt.Errorf("unsupported filesystem %q", fsType)
		}
	}

	switch fsType {
	case FSExt4:
		return filesystem_ext4.Open(imagePath, partIndex)
	case FSFat32:
		return filesystem_fat32.Open(imagePath, partIndex)
	case FSBtrfs:
		return filesystem_btrfs.Open(imagePath, partIndex)
	case FSXfs:
		return filesystem_xfs.Open(imagePath, partIndex)
	case FSZfs:
		return filesystem_zfs.Open(imagePath, partIndex)
	case FSExFAT:
		return filesystem_exfat.Open(imagePath, partIndex)
	case FSNTFS:
		return filesystem_ntfs.Open(imagePath, partIndex)
	case FSApfs:
		if len(unlockKeys) > 0 {
			return filesystem_apfs.OpenWithKeys(imagePath, partIndex, unlockKeys...)
		}
		return filesystem_apfs.Open(imagePath, partIndex)
	default:
		return nil, fmt.Errorf("unsupported filesystem %q", fsType)
	}
}

// ReadFile reads and returns the contents of a regular file inside an image.
func ReadFile(opts FileOptions) ([]byte, error) {
	if opts.Path == "" {
		return nil, fmt.Errorf("image path is required")
	}
	fs, err := openFS(opts.Path, opts.PartIndex, opts.Filesystem, opts.UnlockKeys)
	if err != nil {
		return nil, err
	}
	defer fs.Close()
	return fs.ReadFile(opts.FilePath)
}

// WriteFile writes data to a file inside the image, creating parent
// directories as needed.
func WriteFile(opts FileOptions, data []byte, perm os.FileMode) error {
	if opts.Path == "" {
		return fmt.Errorf("image path is required")
	}
	fs, err := openFS(opts.Path, opts.PartIndex, opts.Filesystem, opts.UnlockKeys)
	if err != nil {
		return err
	}
	defer fs.Close()

	if err := fs.WriteFile(opts.FilePath, data, perm); err == nil {
		return nil
	}

	// Try to create parent directories and retry.
	if err := ensureParentDirs(fs, opts.FilePath); err != nil {
		return err
	}
	if err := fs.WriteFile(opts.FilePath, data, perm); err != nil {
		return err
	}
	return nil
}

// ensureParentDirs creates all missing parent directories for filePath.
func ensureParentDirs(fs filesystem.Filesystem, filePath string) error {
	dir := path.Dir(filePath)
	if dir == "" || dir == "." || dir == "/" {
		return nil
	}
	parts := strings.Split(strings.TrimPrefix(dir, "/"), "/")
	cur := "/"
	for _, p := range parts {
		cur = path.Join(cur, p)
		if _, err := fs.Stat(cur); err == nil {
			continue
		}
		if err := fs.MkDir(cur, 0o755); err != nil {
			// If Stat succeeds now, another concurrent mkdir created it.
			if _, sErr := fs.Stat(cur); sErr == nil {
				continue
			}
			return err
		}
	}
	return nil
}

// MkDir creates a directory inside the image.
func MkDir(opts FileOptions, perm os.FileMode) error {
	if opts.Path == "" {
		return fmt.Errorf("image path is required")
	}
	fs, err := openFS(opts.Path, opts.PartIndex, opts.Filesystem, opts.UnlockKeys)
	if err != nil {
		return err
	}
	defer fs.Close()
	return fs.MkDir(opts.FilePath, perm)
}

// DeleteFile removes a file at the given path inside the image.
func DeleteFile(opts FileOptions) error {
	if opts.Path == "" {
		return fmt.Errorf("image path is required")
	}
	fs, err := openFS(opts.Path, opts.PartIndex, opts.Filesystem, opts.UnlockKeys)
	if err != nil {
		return err
	}
	defer fs.Close()
	return fs.DeleteFile(opts.FilePath)
}

// DeleteDir removes a directory (recursively) inside the image.
func DeleteDir(opts FileOptions) error {
	if opts.Path == "" {
		return fmt.Errorf("image path is required")
	}
	fs, err := openFS(opts.Path, opts.PartIndex, opts.Filesystem, opts.UnlockKeys)
	if err != nil {
		return err
	}
	defer fs.Close()
	return fs.DeleteDir(opts.FilePath)
}

// Rename moves or renames a file or directory inside the image.
func Rename(opts FileOptions, newPath string) error {
	if opts.Path == "" {
		return fmt.Errorf("image path is required")
	}
	fs, err := openFS(opts.Path, opts.PartIndex, opts.Filesystem, opts.UnlockKeys)
	if err != nil {
		return err
	}
	defer fs.Close()
	return fs.Rename(opts.FilePath, newPath)
}

// Stat returns filesystem metadata for the given path inside the image.
func Stat(opts FileOptions) (filesystem.Stat, error) {
	if opts.Path == "" {
		return nil, fmt.Errorf("image path is required")
	}
	fs, err := openFS(opts.Path, opts.PartIndex, opts.Filesystem, opts.UnlockKeys)
	if err != nil {
		return nil, err
	}
	defer fs.Close()
	return fs.Stat(opts.FilePath)
}
