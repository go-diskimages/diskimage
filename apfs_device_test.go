package diskimage

// Tests for OpenAPFSBlockDevice and the private apfsDevice wrapper.
// Coverage requirements (repo rules): 100% statement coverage.
// Every branch in apfs_device.go is exercised here.

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	image_qcow2 "github.com/go-diskimages/qcow2"
	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/xts"
)

// ── APFS image builder ─────────────────────────────────────────────────────
// Builds minimal synthetic APFS containers that go-fde/apfs can parse.
// See pkg/go-fde/apfs for the full specification.

const (
	apfsTestBlockSize = 4096
	apfsSectorSize    = 512
	// apfsNXMagic is the APFS NX container superblock magic. Apple writes
	// the ASCII bytes "NXSB" (LE uint32 0x4253584E). An earlier version of
	// these tests used "BSXN" — self-consistent for round-tripping the
	// reader's matching bug, but rejected by every real Apple parser.
	apfsNXMagic    = "NXSB"
	apfsKBVersion  = uint16(2)
	apfsTagPassPhr = 0x0003
	apfsTagVEK     = 0x0002
	apfsKDFPBKDF2  = uint16(0x0002)
	// nx_keylocker (apfs_prange) at NX SB offset 1296: paddr u64 at +1296,
	// block_count u64 at +1304. Earlier versions of these tests wrote at
	// offsets 64-79 (which is nx_incompatible_features); that was wrong
	// and only worked with the matching reader bug we've also fixed.
	apfsNXKeylockerOff = 1296
	// Media keybag obj_phys.type — APFS_OBJECT_TYPE_MEDIA_KEYBAG, stored
	// at obj_phys offset +24 within the keybag block.
	apfsKBObjType = uint32(0x6b657973)
	// Entry data starts at byte 48 of the keybag block: 32-byte obj_phys
	// + 16-byte apfs_kb_locker header. Entries are 16-byte aligned.
	apfsKBEntryArea  = 48
	apfsKBEntryAlign = 16
)

// apfsMustRandBytes panics if crypto/rand fails.
func apfsMustRandBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

// apfsAESKeyWrap performs RFC 3394 AES Key Wrap.
func apfsAESKeyWrap(kek, plaintext []byte) []byte {
	n := len(plaintext) / 8
	a := [8]byte{0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6}
	r := make([][]byte, n)
	for i := range r {
		r[i] = make([]byte, 8)
		copy(r[i], plaintext[i*8:])
	}
	block := apfsMustNewAES(kek)
	for j := 0; j < 6; j++ {
		for i := 0; i < n; i++ {
			b := make([]byte, 16)
			copy(b[:8], a[:])
			copy(b[8:], r[i])
			block.Encrypt(b, b)
			copy(a[:], b[:8])
			t := uint64(n*j + i + 1)
			xorIntoA(&a, t)
			copy(r[i], b[8:])
		}
	}
	out := make([]byte, 8+len(plaintext))
	copy(out[:8], a[:])
	for i, rb := range r {
		copy(out[8+i*8:], rb)
	}
	return out
}

func xorIntoA(a *[8]byte, v uint64) {
	tmp := make([]byte, 8)
	copy(tmp, a[:])
	tmp[0] ^= byte(v >> 56)
	tmp[1] ^= byte(v >> 48)
	tmp[2] ^= byte(v >> 40)
	tmp[3] ^= byte(v >> 32)
	tmp[4] ^= byte(v >> 24)
	tmp[5] ^= byte(v >> 16)
	tmp[6] ^= byte(v >> 8)
	tmp[7] ^= byte(v)
	copy(a[:], tmp)
}

// writeAPFSKeybag fills the 4096-byte key bag block at buf with the
// Apple-shape layout: 32-byte obj_phys (type=0x6b657973 "syek") followed
// by 16-byte apfs_kb_locker header (version, nkeys, nbytes, padding) and
// 16-byte-aligned entries. Earlier versions used a 4-byte "kbag" magic
// at offset 0 of the block; that was wrong (no Apple container ever has
// it) and only worked because our matching reader checked for the same
// wrong magic.
func writeAPFSKeybag(buf []byte, salt []byte, iter int, wrappedKEK, wrappedVEK []byte) {
	// obj_phys.type at +24 (cksum/oid/xid/subtype left zero — these
	// tests don't validate them).
	binary.LittleEndian.PutUint32(buf[24:28], apfsKBObjType)
	// apfs_kb_locker header at +32: version(2) + nkeys(2) + nbytes(4) + 8 bytes pad.
	binary.LittleEndian.PutUint16(buf[32:34], apfsKBVersion)
	binary.LittleEndian.PutUint16(buf[34:36], 2) // nkeys
	off := apfsKBEntryArea
	off = apfsWriteEntry(buf, off, apfsTagPassPhr, buildAPFSLockerData(salt, iter, wrappedKEK))
	apfsWriteEntry(buf, off, apfsTagVEK, wrappedVEK)
}

// buildAPFSLockerData serialises PBKDF2 params + wrappedKEK.
func buildAPFSLockerData(salt []byte, iter int, wrappedKEK []byte) []byte {
	size := 2 + 2 + 4 + 2 + len(salt) + len(wrappedKEK)
	b := make([]byte, size)
	binary.LittleEndian.PutUint16(b[0:2], uint16(apfsKDFPBKDF2))
	binary.LittleEndian.PutUint32(b[4:8], uint32(iter))
	binary.LittleEndian.PutUint16(b[8:10], uint16(len(salt)))
	copy(b[10:], salt)
	copy(b[10+len(salt):], wrappedKEK)
	return b
}

// apfsWriteEntry writes a keybag entry and returns the new offset.
// Entries are 16-byte aligned per Apple's apfs-fuse reference; earlier
// versions used 8-byte alignment, which Apple's parser tolerated only by
// accident.
func apfsWriteEntry(buf []byte, off, tag int, data []byte) int {
	const hLen = 24
	copy(buf[off:off+16], "test-uuid-000000")
	binary.LittleEndian.PutUint16(buf[off+16:], uint16(tag))
	binary.LittleEndian.PutUint16(buf[off+18:], uint16(len(data)))
	off += hLen
	copy(buf[off:], data)
	off += len(data)
	if rem := off % apfsKBEntryAlign; rem != 0 {
		off += apfsKBEntryAlign - rem
	}
	return off
}

// ── Tests ──────────────────────────────────────────────────────────────────

func TestOpenAPFSBlockDevice_NotExist(t *testing.T) {
	_, err := OpenAPFSBlockDevice(filepath.Join(t.TempDir(), "nofile"), []byte("x"))
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestOpenAPFSBlockDevice_WrongPassphrase(t *testing.T) {
	passphrase := []byte("correct")
	img := buildAPFSTestImageSimple(t, passphrase)
	path := filepath.Join(t.TempDir(), "apfs.img")
	if err := os.WriteFile(path, img, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := OpenAPFSBlockDevice(path, []byte("wrong"))
	if err == nil {
		t.Fatal("expected error for wrong passphrase")
	}
}

func TestOpenAPFSBlockDevice_Raw_ReadWrite(t *testing.T) {
	passphrase := []byte("test passphrase")
	payload := make([]byte, apfsSectorSize)
	copy(payload, []byte("apfs payload test"))
	img := buildRawAPFSImage(t, passphrase, payload)

	path := filepath.Join(t.TempDir(), "apfs.img")
	if err := os.WriteFile(path, img, 0o600); err != nil {
		t.Fatal(err)
	}
	dev, err := OpenAPFSBlockDevice(path, passphrase)
	if err != nil {
		t.Fatalf("OpenAPFSBlockDevice: %v", err)
	}
	defer dev.Close()

	// Size returns 0 for APFS.
	if dev.Size() != 0 {
		t.Fatalf("Size: want 0, got %d", dev.Size())
	}

	// ReadAt then WriteAt.
	buf := make([]byte, apfsSectorSize)
	if _, err := dev.ReadAt(buf, int64(2*apfsTestBlockSize)); err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if !bytes.Equal(buf[:len(payload)], payload) {
		t.Fatalf("ReadAt content mismatch")
	}

	newData := make([]byte, apfsSectorSize)
	copy(newData, []byte("updated data"))
	if _, err := dev.WriteAt(newData, int64(2*apfsTestBlockSize)); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
}

func TestOpenAPFSBlockDevice_QCOW2_OpenError(t *testing.T) {
	// File has QCOW2 magic but invalid content → OpenDevice fails.
	tmp := filepath.Join(t.TempDir(), "bad.qcow2")
	magic := []byte{0x51, 0x46, 0x49, 0xfb, 0x00, 0x00, 0x00, 0x00}
	if err := os.WriteFile(tmp, magic, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := OpenAPFSBlockDevice(tmp, []byte("pass"))
	if err == nil {
		t.Fatal("expected error opening invalid qcow2 for APFS")
	}
}

func TestOpenAPFSBlockDevice_QCOW2_APFSError(t *testing.T) {
	// Valid QCOW2 but virtual disk is all zeros — no APFS magic.
	qcow2Path := filepath.Join(t.TempDir(), "empty.qcow2")
	if err := image_qcow2.Create(qcow2Path, 4*1024*1024); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err := OpenAPFSBlockDevice(qcow2Path, []byte("pass"))
	if err == nil {
		t.Fatal("expected error when virtual disk has no APFS container")
	}
}

func TestOpenAPFSBlockDevice_QCOW2_Success(t *testing.T) {
	passphrase := []byte("qcow2 apfs test")
	payload := make([]byte, apfsSectorSize)
	copy(payload, []byte("qcow2+apfs"))
	img := buildRawAPFSImage(t, passphrase, payload)

	qcow2Path := filepath.Join(t.TempDir(), "apfs.qcow2")
	if err := image_qcow2.Create(qcow2Path, int64(len(img))*2); err != nil {
		t.Fatalf("Create qcow2: %v", err)
	}
	qdev, err := image_qcow2.OpenDevice(qcow2Path)
	if err != nil {
		t.Fatalf("OpenDevice: %v", err)
	}
	if _, err := qdev.WriteAt(img, 0); err != nil {
		qdev.Close()
		t.Fatalf("WriteAt APFS into qcow2: %v", err)
	}
	qdev.Close()

	dev, err := OpenAPFSBlockDevice(qcow2Path, passphrase)
	if err != nil {
		t.Fatalf("OpenAPFSBlockDevice qcow2+apfs: %v", err)
	}
	defer dev.Close()

	buf := make([]byte, apfsSectorSize)
	if _, err := dev.ReadAt(buf, int64(2*apfsTestBlockSize)); err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if !bytes.Equal(buf[:len(payload)], payload) {
		t.Fatalf("round-trip content mismatch via qcow2+apfs")
	}
}

func TestAPFSDevice_Methods(t *testing.T) {
	passphrase := []byte("device methods test")
	img := buildRawAPFSImage(t, passphrase, make([]byte, apfsSectorSize))

	path := filepath.Join(t.TempDir(), "methods.img")
	if err := os.WriteFile(path, img, 0o600); err != nil {
		t.Fatal(err)
	}
	dev, err := OpenAPFSBlockDevice(path, passphrase)
	if err != nil {
		t.Fatalf("OpenAPFSBlockDevice: %v", err)
	}
	buf := make([]byte, apfsSectorSize)
	if _, err := dev.ReadAt(buf, int64(2*apfsTestBlockSize)); err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if _, err := dev.WriteAt(buf, int64(2*apfsTestBlockSize)); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
	_ = dev.Size()
	if err := dev.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// ── Helpers ────────────────────────────────────────────────────────────────

// buildAPFSTestImageSimple builds an image with a valid APFS container but
// without writing any encrypted payload (enough for passphrase error testing).
func buildAPFSTestImageSimple(t *testing.T, passphrase []byte) []byte {
	t.Helper()
	return buildRawAPFSImage(t, passphrase, make([]byte, apfsSectorSize))
}

// buildRawAPFSImage builds a complete APFS image with encrypted payload at
// block 2 using the go-fde/apfs test image format.
func buildRawAPFSImage(t *testing.T, passphrase, payload []byte) []byte {
	t.Helper()
	vek := apfsMustRandBytes(32)
	salt := apfsMustRandBytes(16)
	const iter = 1000
	kek := pbkdf2.Key(passphrase, salt, iter, 32, sha256.New)

	wrappedVEK := apfsAESKeyWrap(kek, vek)
	wrappedKEK := apfsAESKeyWrap(kek, kek)

	const blocks = 4
	img := make([]byte, blocks*apfsTestBlockSize)

	// Block 0: NX superblock
	copy(img[32:36], apfsNXMagic)
	binary.LittleEndian.PutUint32(img[36:40], apfsTestBlockSize)
	// nx_keylocker at offset 1296 (paddr=1, count=1).
	binary.LittleEndian.PutUint64(img[apfsNXKeylockerOff:apfsNXKeylockerOff+8], 1)
	binary.LittleEndian.PutUint64(img[apfsNXKeylockerOff+8:apfsNXKeylockerOff+16], 1)

	// Block 1: key bag
	writeAPFSKeybag(img[apfsTestBlockSize:], salt, iter, wrappedKEK, wrappedVEK)

	// Block 2: payload encrypted with AES-XTS(vek, sectorNum=2)
	payloadBlock := make([]byte, apfsTestBlockSize)
	if len(payload) > apfsTestBlockSize {
		payload = payload[:apfsTestBlockSize]
	}
	copy(payloadBlock, payload)
	apfsEncryptBlock(payloadBlock, vek, 2)
	copy(img[2*apfsTestBlockSize:], payloadBlock)

	return img
}

// apfsMustNewAES creates an AES cipher or panics.
func apfsMustNewAES(key []byte) cipher.Block {
	b, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	return b
}

// apfsEncryptBlock encrypts buf in-place using AES-128-XTS.
// The XTS tweak is the absolute sector number (byte_offset / 512).
// For a block at blockNum * blockSize bytes the first sector is
// (blockNum * blockSize / sectorSize).
func apfsEncryptBlock(buf, vek []byte, blockNum uint64) {
	c, err := xts.NewCipher(aes.NewCipher, vek)
	if err != nil {
		panic(err)
	}
	sectorNum := blockNum * (apfsTestBlockSize / apfsSectorSize)
	for i := 0; i < len(buf)/apfsSectorSize; i++ {
		s := buf[i*apfsSectorSize : (i+1)*apfsSectorSize]
		c.Encrypt(s, s, sectorNum+uint64(i))
	}
}
