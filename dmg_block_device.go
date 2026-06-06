package diskimage

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"

	disk_dmg "github.com/go-diskimages/dmg"
)

// openDMGBlockDevice exposes an Apple UDIF/UDRW DMG as a read-write
// BlockDevice. Only the UDRW (Universal Disk Read-Write) subformat is
// supported in-place — its data fork is uncompressed contiguous
// sectors at file offset 0, so ReadAt/WriteAt translate one-to-one
// onto the backing file.
//
// For compressed subformats (UDRO / UDZO / UDBZ / UDSP) the only
// safe write path is unpack-to-temp + repack-on-Close, which has a
// surprise multi-second cost. We don't ship that strategy yet; those
// images return an explicit error so callers can fall back to
// dmg.UnpackToTemp + raw editing if they actually want it.
//
// On Close, if any WriteAt happened, the koly trailer's
// dataForkChecksum and masterChecksum are recomputed over the new
// data fork bytes and rewritten. Without that fix-up,
// readAllUDIFSectors rejects the image with a master-checksum
// mismatch the next time anyone (including hdiutil) opens it.
func openDMGBlockDevice(path string) (BlockDevice, error) {
	variant, err := disk_dmg.DetectUDIFFormat(path)
	if err != nil {
		return nil, fmt.Errorf("dmg: detect format: %w", err)
	}
	if variant != "UDRW" {
		return nil, fmt.Errorf("dmg: in-place block device requires UDRW; got %s (use disk_dmg.UnpackToTemp + PackFromTemp for the others)", variant)
	}

	f, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("dmg: open %s: %w", path, err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("dmg: stat %s: %w", path, err)
	}
	if info.Size() < dmgKolyBlockSize {
		f.Close()
		return nil, fmt.Errorf("dmg: file too small for koly block")
	}

	kolyOff := info.Size() - dmgKolyBlockSize
	kolyBuf := make([]byte, dmgKolyBlockSize)
	if _, err := f.ReadAt(kolyBuf, kolyOff); err != nil {
		f.Close()
		return nil, fmt.Errorf("dmg: read koly: %w", err)
	}
	if binary.BigEndian.Uint32(kolyBuf[0:4]) != dmgKolyMagic {
		f.Close()
		return nil, fmt.Errorf("dmg: bad koly magic (corrupt or non-UDIF file)")
	}

	dataForkOffset := binary.BigEndian.Uint64(kolyBuf[24:32])
	dataForkLength := binary.BigEndian.Uint64(kolyBuf[32:40])
	sectorCount := binary.BigEndian.Uint64(kolyBuf[492:500])

	// UDRW invariants — refuse anything that looks unusual rather
	// than silently mis-mapping offsets.
	if dataForkOffset != 0 {
		f.Close()
		return nil, fmt.Errorf("dmg: UDRW with non-zero dataForkOffset=%d not supported", dataForkOffset)
	}
	if dataForkLength != sectorCount*dmgSectorSize {
		f.Close()
		return nil, fmt.Errorf("dmg: UDRW dataForkLength=%d disagrees with sectorCount*512=%d", dataForkLength, sectorCount*dmgSectorSize)
	}

	return &udrwBlockDevice{
		f:              f,
		kolyOffset:     kolyOff,
		kolyBuf:        kolyBuf,
		dataForkLength: int64(dataForkLength),
	}, nil
}

const (
	dmgKolyMagic     = uint32(0x6B6F6C79) // 'koly'
	dmgKolyBlockSize = 512
	dmgSectorSize    = 512
)

// udrwBlockDevice exposes an UDRW DMG's data fork (sectors at file
// offset 0) as a BlockDevice. Bounded reads/writes; on Close, the
// koly trailer's CRC-32 checksums are refreshed if anything was
// written.
type udrwBlockDevice struct {
	f              *os.File
	kolyOffset     int64
	kolyBuf        []byte // raw koly trailer bytes (512)
	dataForkLength int64
	dirty          bool
}

func (d *udrwBlockDevice) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= d.dataForkLength {
		return 0, io.EOF
	}
	// Bound at the data-fork edge so callers never see the XML plist
	// or koly trailer bytes that follow.
	if max := d.dataForkLength - off; int64(len(p)) > max {
		p = p[:max]
	}
	return d.f.ReadAt(p, off)
}

func (d *udrwBlockDevice) WriteAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fmt.Errorf("udrw: negative offset %d", off)
	}
	if off+int64(len(p)) > d.dataForkLength {
		return 0, fmt.Errorf("udrw: write past data fork (off=%d len=%d, data fork ends at %d)", off, len(p), d.dataForkLength)
	}
	n, err := d.f.WriteAt(p, off)
	if n > 0 {
		d.dirty = true
	}
	return n, err
}

func (d *udrwBlockDevice) Size() int64 { return d.dataForkLength }

// Sync flushes the underlying file. Used by ext4Adapter so SetLabel's
// post-write fsync reaches the disk.
func (d *udrwBlockDevice) Sync() error { return d.f.Sync() }

// Close refreshes the koly trailer's CRC-32 checksums when the device
// was written to, then closes the file. The plist's embedded blkx
// table also holds a checksum but readAllUDIFSectors does not verify
// it — only the koly's masterChecksum gates re-opens — so we leave
// the plist alone. Hand-rolled hdiutil mounts may complain about the
// blkx checksum; that's an acceptable trade-off for not having to
// re-encode the XML on every Close.
func (d *udrwBlockDevice) Close() error {
	if d.dirty {
		if err := d.refreshChecksums(); err != nil {
			d.f.Close()
			return err
		}
		if err := d.f.Sync(); err != nil {
			d.f.Close()
			return err
		}
	}
	return d.f.Close()
}

func (d *udrwBlockDevice) refreshChecksums() error {
	crc := crc32.NewIEEE()
	const bufSize = 1 << 20
	buf := make([]byte, bufSize)
	off := int64(0)
	for off < d.dataForkLength {
		want := int64(bufSize)
		if rem := d.dataForkLength - off; rem < want {
			want = rem
		}
		n, err := d.f.ReadAt(buf[:want], off)
		if err != nil && err != io.EOF {
			return fmt.Errorf("udrw: read for checksum: %w", err)
		}
		crc.Write(buf[:n])
		off += int64(n)
		if int64(n) < want {
			return fmt.Errorf("udrw: short read at off=%d", off)
		}
	}
	newCRC := crc.Sum32()
	// dataForkChecksum @ koly[88:92] (type=2 CRC-32 already set by
	// writeUDIF); masterChecksum @ koly[360:364]. Both equal the
	// data-fork CRC for UDRW since the data fork IS the sectors.
	binary.BigEndian.PutUint32(d.kolyBuf[88:92], newCRC)
	binary.BigEndian.PutUint32(d.kolyBuf[360:364], newCRC)
	if _, err := d.f.WriteAt(d.kolyBuf, d.kolyOffset); err != nil {
		return fmt.Errorf("udrw: rewrite koly: %w", err)
	}
	return nil
}

// errUnsupportedDMGVariant is returned when openDMGBlockDevice is
// asked for a non-UDRW image. Exposed (but unexported by name) so
// tests can predicate on it via errors.Is — useful when we add the
// unpack-to-temp strategy and want to opt callers into the slow path.
var errUnsupportedDMGVariant = errors.New("dmg: only UDRW supported in-place")
