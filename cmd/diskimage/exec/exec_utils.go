package exec

import (
	"fmt"

	"github.com/go-diskimages/diskimage"
)

func indicator(e diskimage.ListEntry, classify bool) string {
	if !classify {
		return ""
	}
	switch (e.Mode >> 12) & 0xF {
	case 0x4:
		return "/"
	case 0xA:
		return "@"
	case 0x1:
		return "|"
	case 0xC:
		return "="
	case 0x8:
		if e.Mode&0o111 != 0 {
			return "*"
		}
	}
	if e.Mode == 0 && e.FileType == 2 {
		return "/"
	}
	return ""
}

func modeString(mode uint16) string {
	if mode == 0 {
		return "??????????"
	}
	buf := [10]byte{}
	switch (mode >> 12) & 0xF {
	case 0x4:
		buf[0] = 'd'
	case 0xA:
		buf[0] = 'l'
	case 0x6:
		buf[0] = 'b'
	case 0x2:
		buf[0] = 'c'
	case 0x1:
		buf[0] = 'p'
	case 0xC:
		buf[0] = 's'
	default:
		buf[0] = '-'
	}
	owner := (mode >> 6) & 0o7
	buf[1] = byteIf(owner&4 != 0, 'r', '-')
	buf[2] = byteIf(owner&2 != 0, 'w', '-')
	if mode&0o4000 != 0 {
		buf[3] = byteIf(owner&1 != 0, 's', 'S')
	} else {
		buf[3] = byteIf(owner&1 != 0, 'x', '-')
	}
	group := (mode >> 3) & 0o7
	buf[4] = byteIf(group&4 != 0, 'r', '-')
	buf[5] = byteIf(group&2 != 0, 'w', '-')
	if mode&0o2000 != 0 {
		buf[6] = byteIf(group&1 != 0, 's', 'S')
	} else {
		buf[6] = byteIf(group&1 != 0, 'x', '-')
	}
	other := mode & 0o7
	buf[7] = byteIf(other&4 != 0, 'r', '-')
	buf[8] = byteIf(other&2 != 0, 'w', '-')
	if mode&0o1000 != 0 {
		buf[9] = byteIf(other&1 != 0, 't', 'T')
	} else {
		buf[9] = byteIf(other&1 != 0, 'x', '-')
	}
	return string(buf[:])
}

func byteIf(cond bool, t, f byte) byte {
	if cond {
		return t
	}
	return f
}

func humanSize(b uint64) string {
	const (
		kib = 1024
		mib = 1024 * kib
		gib = 1024 * mib
		tib = 1024 * gib
	)
	switch {
	case b >= tib:
		return fmt.Sprintf("%.1fT", float64(b)/float64(tib))
	case b >= gib:
		return fmt.Sprintf("%.1fG", float64(b)/float64(gib))
	case b >= mib:
		return fmt.Sprintf("%.1fM", float64(b)/float64(mib))
	case b >= kib:
		return fmt.Sprintf("%.1fK", float64(b)/float64(kib))
	default:
		return fmt.Sprintf("%d", b)
	}
}
