package exec

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

const partSectorSize = 512

type partitionEntry struct {
	Index int
	Start int64 // bytes
	Size  int64 // bytes
	Type  string
}

// PartPrintFromImage lists partitions found in raw image 'file' and writes a
// simple table to w. It supports GPT and MBR.
func PartPrintFromImage(file string, w io.Writer) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	// Check GPT signature at LBA1.
	sig := make([]byte, 8)
	if n, _ := f.ReadAt(sig, 512); n == 8 && string(sig) == "EFI PART" {
		parts, err := parseGPT(f)
		if err != nil {
			return err
		}
		if len(parts) == 0 {
			fmt.Fprintln(w, "no partitions found")
			return nil
		}
		fmt.Fprintln(w, "Index\tStart(LBA)\tSize\tType")
		for _, p := range parts {
			fmt.Fprintf(w, "%d\t%d\t%s\t%s\n", p.Index, p.Start/partSectorSize, humanSize(uint64(p.Size)), p.Type)
		}
		return nil
	}

	// Fallback: check MBR signature at 510.
	sig2 := make([]byte, 2)
	if n, _ := f.ReadAt(sig2, 510); n == 2 && sig2[0] == 0x55 && sig2[1] == 0xAA {
		parts, err := parseMBR(f)
		if err != nil {
			return err
		}
		if len(parts) == 0 {
			fmt.Fprintln(w, "no partitions found")
			return nil
		}
		fmt.Fprintln(w, "Index\tStart(LBA)\tSize\tType")
		for _, p := range parts {
			fmt.Fprintf(w, "%d\t%d\t%s\t%s\n", p.Index, p.Start/partSectorSize, humanSize(uint64(p.Size)), p.Type)
		}
		return nil
	}

	fmt.Fprintln(w, "no partition table")
	return nil
}

func parseMBR(f *os.File) ([]partitionEntry, error) {
	table := make([]byte, 64)
	if _, err := f.ReadAt(table, 446); err != nil {
		return nil, fmt.Errorf("read MBR table: %w", err)
	}
	var parts []partitionEntry
	for i := 0; i < 4; i++ {
		e := table[i*16 : i*16+16]
		start := binary.LittleEndian.Uint32(e[8:12])
		count := binary.LittleEndian.Uint32(e[12:16])
		if start == 0 || count == 0 {
			continue
		}
		parts = append(parts, partitionEntry{
			Index: i,
			Start: int64(start) * partSectorSize,
			Size:  int64(count) * partSectorSize,
			Type:  fmt.Sprintf("0x%02X", e[4]),
		})
	}
	return parts, nil
}

func parseGPT(f *os.File) ([]partitionEntry, error) {
	hdr := make([]byte, 92)
	if _, err := f.ReadAt(hdr, 512); err != nil {
		return nil, fmt.Errorf("read GPT header: %w", err)
	}
	partEntryLBA := binary.LittleEndian.Uint64(hdr[72:])
	numParts := binary.LittleEndian.Uint32(hdr[80:])
	entrySize := binary.LittleEndian.Uint32(hdr[84:])
	tableOff := int64(partEntryLBA) * partSectorSize

	buf := make([]byte, entrySize)
	var parts []partitionEntry
	for i := uint32(0); i < numParts; i++ {
		if _, err := f.ReadAt(buf, tableOff+int64(i)*int64(entrySize)); err != nil {
			break
		}
		var typeGUID [16]byte
		copy(typeGUID[:], buf[:16])
		if typeGUID == ([16]byte{}) {
			continue
		}
		start := binary.LittleEndian.Uint64(buf[32:])
		end := binary.LittleEndian.Uint64(buf[40:])
		if start == 0 && end == 0 {
			continue
		}
		sectors := end - start + 1
		parts = append(parts, partitionEntry{
			Index: int(i),
			Start: int64(start) * partSectorSize,
			Size:  int64(sectors) * partSectorSize,
			Type:  fmt.Sprintf("%x", typeGUID),
		})
	}
	return parts, nil
}
