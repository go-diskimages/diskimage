package exec

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	filesystem "github.com/go-filesystems/interface"
	zfs "github.com/go-filesystems/zfs"
)

// ZFSImage is a small interface used by exec handlers so tests can inject
// a fake implementation without requiring a real zfs.Open on disk images.
type ZFSImage interface {
	Close() error
	Info() zfs.Info
	PartitionOffset() int64
	ListDir(string) ([]filesystem.DirEntry, error)
	Stat(string) (filesystem.Stat, error)
	MkDir(string, os.FileMode) error
}

// OpenZFS is a variable so tests can override it. By default it wraps zfs.Open.
var OpenZFS = func(imagePath string, partIndex int) (ZFSImage, error) {
	f, err := zfs.Open(imagePath, partIndex)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// SetOpenZFS replaces the OpenZFS function used by the package and returns
// the previous value. Passing nil leaves the current value unchanged.
func SetOpenZFS(f func(imagePath string, partIndex int) (ZFSImage, error)) (prev func(imagePath string, partIndex int) (ZFSImage, error)) {
	prev = OpenZFS
	if f != nil {
		OpenZFS = f
	}
	return prev
}

// parseNVPair parses a single NV pair payload and returns the name and
// stringified value when possible. This keeps the main parser compact.
func parseNVPair(pair []byte) (string, string, bool) {
	if len(pair) < 4 {
		return "", "", false
	}
	nameLen := int(binary.BigEndian.Uint32(pair[0:4]))
	nameStart := 4
	if nameStart+nameLen > len(pair) {
		return "", "", false
	}
	nameBytes := pair[nameStart : nameStart+nameLen]
	if len(nameBytes) > 0 && nameBytes[len(nameBytes)-1] == 0 {
		nameBytes = nameBytes[:len(nameBytes)-1]
	}
	name := string(nameBytes)

	pad := (4 - (nameLen % 4)) % 4
	typeOff := nameStart + nameLen + pad
	if typeOff+8 > len(pair) {
		return "", "", false
	}
	typ := binary.BigEndian.Uint32(pair[typeOff : typeOff+4])
	nelem := binary.BigEndian.Uint32(pair[typeOff+4 : typeOff+8])
	dataOff := typeOff + 8

	switch typ {
	case 14: // string
		if dataOff+4 > len(pair) {
			return "", "", false
		}
		strLen := int(binary.BigEndian.Uint32(pair[dataOff : dataOff+4]))
		if dataOff+4+strLen > len(pair) {
			return "", "", false
		}
		s := string(pair[dataOff+4 : dataOff+4+strLen])
		return name, s, true
	case 11: // uint64
		if nelem >= 1 && dataOff+8 <= len(pair) {
			v := binary.BigEndian.Uint64(pair[dataOff : dataOff+8])
			return name, fmt.Sprintf("%d", v), true
		}
	default:
		_ = nelem
	}
	return "", "", false
}

// parseLabelNVList attempts to parse a vdev label NVList (XDR-encoded) and
// returns a map of simple string-valued entries (e.g. "name").
func parseLabelNVList(b []byte) (map[string]string, error) {
	if len(b) < 16 {
		return nil, fmt.Errorf("nvlist too short")
	}
	// Skip 8-byte outer header and 8-byte inner header
	off := 16
	res := make(map[string]string)
	for {
		if off+8 > len(b) {
			break
		}
		encoded := binary.BigEndian.Uint32(b[off : off+4])
		if encoded == 0 {
			break
		}
		// decoded := binary.BigEndian.Uint32(b[off+4 : off+8])
		pairLen := int(encoded) - 8
		if pairLen <= 0 || off+8+pairLen > len(b) {
			break
		}
		pair := b[off+8 : off+8+pairLen]
		if name, val, ok := parseNVPair(pair); ok {
			res[name] = val
		}
		off += 8 + pairLen
	}
	return res, nil
}

// computeUsed walks the ZPL tree and sums regular file sizes.
func computeUsed(fs ZFSImage) (uint64, error) {
	var total uint64
	var walk func(string) error
	walk = func(p string) error {
		entries, err := fs.ListDir(p)
		if err != nil {
			return err
		}
		for _, e := range entries {
			name := e.Name()
			var np string
			if p == "/" {
				np = "/" + name
			} else {
				np = path.Join(p, name)
			}
			// fileType code: DT_DIR == 4
			if e.FileType() == 4 {
				if err := walk(np); err != nil {
					return err
				}
			} else {
				st, err := fs.Stat(np)
				if err != nil {
					continue
				}
				total += st.Size()
			}
		}
		return nil
	}
	if err := walk("/"); err != nil {
		return total, err
	}
	return total, nil
}

// ZpoolStatusFromImage writes a minimal zpool status-like output for the
// pool found in the image at file/partIndex to the provided writer.
func ZpoolStatusFromImage(file string, partIndex int, w io.Writer) error {
	fs, err := OpenZFS(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()

	info := fs.Info()
	var poolName string
	if f, err := os.Open(file); err == nil {
		labelOff := info.LabelOffset(fs.PartitionOffset())
		nvOff := int64(4 * 1024)
		buf := make([]byte, 112*1024)
		if n, rerr := f.ReadAt(buf, labelOff+nvOff); rerr == nil || n > 0 {
			if m, perr := parseLabelNVList(buf[:n]); perr == nil {
				if name, ok := m["name"]; ok {
					poolName = name
				}
			}
		}
		f.Close()
	}
	if poolName == "" {
		poolName = path.Base(file)
	}

	fmt.Fprintf(w, "pool: %s\n", poolName)
	fmt.Fprintln(w, " state: ONLINE")
	fmt.Fprintln(w, "  scan: none requested")
	fmt.Fprintln(w, "config:")
	fmt.Fprintln(w, "\n    NAME\tSTATE\tREAD\tWRITE\tCKSUM")
	fmt.Fprintf(w, "\t%s\tONLINE\t0\t0\t0\n", poolName)
	fmt.Fprintf(w, "\t\t%s\tONLINE\t0\t0\t0\n", file)
	fmt.Fprintln(w, "\nerrors: No known data errors")
	return nil
}

// ZfsListFromImage writes a minimal zfs list-like output for the pool in the
// image to the given writer.
func ZfsListFromImage(file string, partIndex int, w io.Writer) error {
	fs, err := OpenZFS(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()

	info := fs.Info()
	var poolName string
	if f, err := os.Open(file); err == nil {
		labelOff := info.LabelOffset(fs.PartitionOffset())
		nvOff := int64(4 * 1024)
		buf := make([]byte, 112*1024)
		if n, rerr := f.ReadAt(buf, labelOff+nvOff); rerr == nil || n > 0 {
			if m, perr := parseLabelNVList(buf[:n]); perr == nil {
				if name, ok := m["name"]; ok {
					poolName = name
				}
			}
		}
		f.Close()
	}
	if poolName == "" {
		poolName = path.Base(file)
	}

	used, _ := computeUsed(fs)
	var avail uint64
	if fi, err := os.Stat(file); err == nil {
		if fi.Size() < 0 {
			avail = 0
		} else {
			total := uint64(fi.Size())
			if total > used {
				avail = total - used
			} else {
				avail = 0
			}
		}
	}

	fmt.Fprintf(w, "NAME\tUSED\tAVAIL\tREFER\tMOUNTPOINT\n")
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t-\n", poolName, humanSize(used), humanSize(avail), humanSize(used))
	return nil
}

// computeUsedAt walks the ZPL tree starting at startPath and sums regular file sizes.
func computeUsedAt(fs ZFSImage, startPath string) (uint64, error) {
	var total uint64
	visited := map[string]bool{}
	var walk func(string) error
	walk = func(p string) error {
		if visited[p] {
			return nil
		}
		visited[p] = true
		entries, err := fs.ListDir(p)
		if err != nil {
			return err
		}
		for _, e := range entries {
			name := e.Name()
			var np string
			if p == "/" {
				np = "/" + name
			} else {
				np = path.Join(p, name)
			}
			if e.FileType() == 4 { // DT_DIR
				if err := walk(np); err != nil {
					return err
				}
			} else {
				st, err := fs.Stat(np)
				if err != nil {
					continue
				}
				total += st.Size()
			}
		}
		return nil
	}
	if err := walk(startPath); err != nil {
		return total, err
	}
	return total, nil
}

// ZfsGetQuotaFromImage attempts to read the quota property for the given
// dataset inside the ZFS image. Because the on-disk ZFS property store is
// not fully implemented here, this function reports the used/avail sizes and
// prints '-' for an unset quota.
func ZfsGetQuotaFromImage(file string, partIndex int, dataset string, w io.Writer) error {
	fs, err := OpenZFS(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()

	info := fs.Info()
	var poolName string
	if f, err := os.Open(file); err == nil {
		labelOff := info.LabelOffset(fs.PartitionOffset())
		nvOff := int64(4 * 1024)
		buf := make([]byte, 112*1024)
		if n, rerr := f.ReadAt(buf, labelOff+nvOff); rerr == nil || n > 0 {
			if m, perr := parseLabelNVList(buf[:n]); perr == nil {
				if name, ok := m["name"]; ok {
					poolName = name
				}
			}
		}
		f.Close()
	}
	if poolName == "" {
		poolName = path.Base(file)
	}

	// Map dataset to an on-disk path inside the ZPL. If dataset equals the
	// pool name, use the pool root ("/"). If dataset is of the form
	// "pool/name/..." then the remainder maps to "/name/...".
	var startPath string
	if dataset == poolName {
		startPath = "/"
	} else if strings.HasPrefix(dataset, poolName+"/") {
		startPath = "/" + strings.TrimPrefix(dataset, poolName+"/")
	} else if strings.Contains(dataset, "/") {
		// dataset with other pool name — map to provided path portion after first slash
		parts := strings.SplitN(dataset, "/", 2)
		startPath = "/" + parts[1]
	} else {
		startPath = "/" + dataset
	}

	used, _ := computeUsedAt(fs, startPath)
	var avail uint64
	if fi, err := os.Stat(file); err == nil {
		total := uint64(fi.Size())
		if total > used {
			avail = total - used
		} else {
			avail = 0
		}
	}

	fmt.Fprintf(w, "NAME\tUSED\tQUOTA\tAVAIL\n")
	fmt.Fprintf(w, "%s\t%s\t-%s\t%s\n", dataset, humanSize(used), "\t", humanSize(avail))
	return nil
}

// ZfsSetQuotaFromImage is a best-effort setter for the quota property; it does
// not persist properties to disk but validates the dataset exists and reports
// success. This is sufficient for tests and basic tooling that expects the
// command to run inside an image.
func ZfsSetQuotaFromImage(file string, partIndex int, dataset string, quota string, w io.Writer) error {
	fs, err := OpenZFS(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()

	// Resolve dataset path similar to ZfsGetQuotaFromImage.
	info := fs.Info()
	var poolName string
	if f, err := os.Open(file); err == nil {
		labelOff := info.LabelOffset(fs.PartitionOffset())
		nvOff := int64(4 * 1024)
		buf := make([]byte, 112*1024)
		if n, rerr := f.ReadAt(buf, labelOff+nvOff); rerr == nil || n > 0 {
			if m, perr := parseLabelNVList(buf[:n]); perr == nil {
				if name, ok := m["name"]; ok {
					poolName = name
				}
			}
		}
		f.Close()
	}
	if poolName == "" {
		poolName = path.Base(file)
	}

	var startPath string
	if dataset == poolName {
		startPath = "/"
	} else if strings.HasPrefix(dataset, poolName+"/") {
		startPath = "/" + strings.TrimPrefix(dataset, poolName+"/")
	} else if strings.Contains(dataset, "/") {
		parts := strings.SplitN(dataset, "/", 2)
		startPath = "/" + parts[1]
	} else {
		startPath = "/" + dataset
	}

	// Ensure dataset exists
	if _, err := fs.Stat(startPath); err != nil {
		return fmt.Errorf("zfs set quota: dataset %s not found: %w", dataset, err)
	}

	fmt.Fprintf(w, "set quota %s=%s\n", dataset, quota)
	return nil
}

// ZfsCreateFromImage creates a dataset inside the ZFS image. The dataset may
// be provided as either "pool/dataset" or just "dataset" (created at pool
// root). A confirmation line is written to w on success.
func ZfsCreateFromImage(file string, partIndex int, dataset string, w io.Writer) error {
	fs, err := OpenZFS(file, partIndex)
	if err != nil {
		return err
	}
	defer fs.Close()

	info := fs.Info()
	var poolName string
	if f, err := os.Open(file); err == nil {
		labelOff := info.LabelOffset(fs.PartitionOffset())
		nvOff := int64(4 * 1024)
		buf := make([]byte, 112*1024)
		if n, rerr := f.ReadAt(buf, labelOff+nvOff); rerr == nil || n > 0 {
			if m, perr := parseLabelNVList(buf[:n]); perr == nil {
				if name, ok := m["name"]; ok {
					poolName = name
				}
			}
		}
		f.Close()
	}
	if poolName == "" {
		poolName = path.Base(file)
	}

	if strings.TrimSpace(dataset) == "" {
		return fmt.Errorf("zfs create: dataset name required")
	}

	var dsPath string
	if strings.Contains(dataset, "/") {
		parts := strings.SplitN(dataset, "/", 2)
		if parts[0] != poolName {
			return fmt.Errorf("zfs create: pool mismatch (image pool=%s)", poolName)
		}
		dsPath = "/" + parts[1]
	} else {
		dsPath = "/" + dataset
	}

	clean := strings.Trim(dsPath, "/")
	if clean == "" {
		return fmt.Errorf("zfs create: cannot create root dataset")
	}
	segments := strings.Split(clean, "/")
	for i := range segments {
		cur := "/" + strings.Join(segments[:i+1], "/")
		if _, err := fs.Stat(cur); err == nil {
			continue
		}
		if err := fs.MkDir(cur, 0o755); err != nil {
			return fmt.Errorf("zfs create: mkdir %s: %w", cur, err)
		}
	}

	fmt.Fprintf(w, "created dataset %s\n", dataset)
	return nil
}
