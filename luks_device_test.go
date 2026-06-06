package diskimage

// Tests for OpenBlockDevice, OpenLUKSBlockDevice, and the private device
// wrapper types defined in luks_device.go.
//
// Coverage requirements (repo rules): 100% statement coverage for the
// diskimage package. Every branch in luks_device.go is exercised here.

import (
	"bytes"
	"crypto/aes"
	"crypto/hmac"
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

// ── LUKS1 image builder ────────────────────────────────────────────────────
// These helpers are self-contained: they do not import the luks package
// internals, but produce images that the luks package can parse.

const luks1Magic = "LUKS\xba\xbe"
const luks1KeySlotActive = 0x00AC71F3

func mustRandBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

// xorBytes computes dst ^= src in place (equal-length slices).
func testXorBytes(dst, src []byte) {
	for i := range dst {
		dst[i] ^= src[i]
	}
}

// afDiffuse applies the LUKS diffusion function to d in-place using HMAC-SHA256.
// The counter is a big-endian 4-byte integer used as the HMAC key.
// This matches hashDiffuse("sha256") in the luks package.
func afDiffuse(block []byte) {
	const digestLen = 32 // SHA-256
	counter := make([]byte, 4)
	pos := 0
	for i := 0; pos < len(block); i++ {
		binary.BigEndian.PutUint32(counter, uint32(i))
		chunk := len(block) - pos
		if chunk > digestLen {
			chunk = digestLen
		}
		h := hmac.New(sha256.New, counter)
		h.Write(block[pos : pos+chunk])
		sum := h.Sum(nil)
		copy(block[pos:pos+chunk], sum[:chunk])
		pos += chunk
	}
}

// afSplitKey produces an AF-split payload of key using sha256 diffusion.
// It is the inverse of afMerge used by the luks package.
func afSplitKey(key []byte, stripes int) []byte {
	klen := len(key)
	out := make([]byte, klen*stripes)
	d := make([]byte, klen)
	for i := 0; i < stripes-1; i++ {
		stripe := mustRandBytes(klen)
		copy(out[i*klen:], stripe)
		testXorBytes(d, stripe)
		afDiffuse(d)
	}
	last := out[(stripes-1)*klen : stripes*klen]
	copy(last, d)
	testXorBytes(last, key)
	return out
}

// encryptAFBlocks encrypts the AF payload using AES-XTS-plain64 with key.
// Sector size is 512; the IV equals the sector number.
func encryptAFBlocks(afData []byte, key []byte) []byte {
	const ss = 512
	c, err := xts.NewCipher(aes.NewCipher, key)
	if err != nil {
		panic("encryptAFBlocks: " + err.Error())
	}
	out := make([]byte, len(afData))
	copy(out, afData)
	for i := 0; i*ss < len(out); i++ {
		end := (i + 1) * ss
		if end > len(out) {
			end = len(out)
		}
		c.Encrypt(out[i*ss:end], out[i*ss:end], uint64(i))
	}
	return out
}

type luks1ImageParams struct {
	passphrase []byte
	volumeKey  []byte // 32 bytes (256-bit AES-XTS)
}

// buildLUKS1ImageFile creates a minimal valid LUKS1 image at path using
// passphrase to protect volumeKey.
func buildLUKS1ImageFile(t *testing.T, path string, p luks1ImageParams) {
	t.Helper()
	const (
		cipherName = "aes"
		cipherMode = "xts-plain64"
		hashSpec   = "sha256"
		stripes    = 4000
		sectorSize = 512
		keyBytes   = 32
		slotIter   = 1000
		mkIter     = 1000
		kmOffset   = uint32(8)
	)

	slotSalt := mustRandBytes(32)
	slotKey := pbkdf2.Key(p.passphrase, slotSalt, slotIter, keyBytes, sha256.New)
	afData := afSplitKey(p.volumeKey, stripes)
	encAF := encryptAFBlocks(afData, slotKey)

	mkSalt := mustRandBytes(32)
	mkDigest := pbkdf2.Key(p.volumeKey, mkSalt, mkIter, 20, sha256.New)

	payloadOffset := uint32(8 + uint32(stripes*keyBytes)/sectorSize + 8)
	imgSize := int(payloadOffset)*sectorSize + sectorSize

	img := make([]byte, imgSize)

	// Header
	copy(img[0:6], luks1Magic)
	binary.BigEndian.PutUint16(img[6:8], 1)
	writePaddedStr(img[8:40], cipherName)
	writePaddedStr(img[40:72], cipherMode)
	writePaddedStr(img[72:104], hashSpec)
	binary.BigEndian.PutUint32(img[104:108], payloadOffset)
	binary.BigEndian.PutUint32(img[108:112], keyBytes)
	copy(img[112:132], mkDigest)
	copy(img[132:164], mkSalt)
	binary.BigEndian.PutUint32(img[164:168], mkIter)
	copy(img[168:208], "test-uuid-0000000000000000000000")

	// Slot 0 active
	base := 208
	binary.BigEndian.PutUint32(img[base:], luks1KeySlotActive)
	binary.BigEndian.PutUint32(img[base+4:], slotIter)
	copy(img[base+8:base+40], slotSalt)
	binary.BigEndian.PutUint32(img[base+40:], kmOffset)
	binary.BigEndian.PutUint32(img[base+44:], stripes)

	// Slots 1-7 inactive
	for i := 1; i < 8; i++ {
		b := 208 + i*48
		binary.BigEndian.PutUint32(img[b:], 0xDEAD0000)
	}

	// Key material
	copy(img[int(kmOffset)*sectorSize:], encAF)

	if err := os.WriteFile(path, img, 0o600); err != nil {
		t.Fatalf("buildLUKS1ImageFile: write: %v", err)
	}
}

// writePaddedStr writes s null-terminated into buf (buf is zeroed beyond s).
func writePaddedStr(buf []byte, s string) {
	copy(buf, s)
	for i := len(s); i < len(buf); i++ {
		buf[i] = 0
	}
}

// ── rawFileDevice ──────────────────────────────────────────────────────────

func TestRawFileDevice_ReadWriteSize(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "raw.img")
	if err := os.WriteFile(tmp, make([]byte, 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(tmp, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	dev := &rawFileDevice{f: f}
	defer dev.Close()

	// Size
	if got := dev.Size(); got != 4096 {
		t.Fatalf("Size: want 4096, got %d", got)
	}

	// WriteAt
	data := []byte("hello diskimage")
	if _, err := dev.WriteAt(data, 0); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}

	// ReadAt
	buf := make([]byte, len(data))
	if _, err := dev.ReadAt(buf, 0); err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if !bytes.Equal(buf, data) {
		t.Fatalf("ReadAt: want %q got %q", data, buf)
	}
}

func TestRawFileDevice_Size_AfterClose(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "raw.img")
	if err := os.WriteFile(tmp, make([]byte, 512), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(tmp, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	dev := &rawFileDevice{f: f}
	// Close the file to force Stat to fail.
	_ = f.Close()
	// Size should return 0 when Stat fails.
	if got := dev.Size(); got != 0 {
		t.Fatalf("Size after close: want 0, got %d", got)
	}
}

// ── qcow2BlockDevice ──────────────────────────────────────────────────────

func TestQCOW2BlockDevice_ReadWriteSizeClose(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "disk.qcow2")
	const vSize = int64(4 * 1024 * 1024)
	if err := image_qcow2.Create(tmp, vSize); err != nil {
		t.Fatalf("Create qcow2: %v", err)
	}
	qdev, err := image_qcow2.OpenDevice(tmp)
	if err != nil {
		t.Fatalf("OpenDevice: %v", err)
	}
	dev := &qcow2BlockDevice{dev: qdev}
	defer dev.Close()

	// Size
	if got := dev.Size(); got != vSize {
		t.Fatalf("Size: want %d, got %d", vSize, got)
	}

	// WriteAt + ReadAt
	data := []byte("qcow2 block device test")
	if _, err := dev.WriteAt(data, 0); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
	buf := make([]byte, len(data))
	if _, err := dev.ReadAt(buf, 0); err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if !bytes.Equal(buf, data) {
		t.Fatalf("ReadAt: want %q got %q", data, buf)
	}
}

// ── OpenBlockDevice ────────────────────────────────────────────────────────

func TestOpenBlockDevice_Raw_Success(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "disk.img")
	if err := os.WriteFile(tmp, make([]byte, 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	dev, err := OpenBlockDevice(tmp)
	if err != nil {
		t.Fatalf("OpenBlockDevice raw: %v", err)
	}
	defer dev.Close()
	if dev.Size() != 4096 {
		t.Fatalf("Size: want 4096, got %d", dev.Size())
	}
}

func TestOpenBlockDevice_Raw_NotExist(t *testing.T) {
	_, err := OpenBlockDevice(filepath.Join(t.TempDir(), "nofile.img"))
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestOpenBlockDevice_QCOW2_Success(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "disk.qcow2")
	if err := image_qcow2.Create(tmp, 4*1024*1024); err != nil {
		t.Fatalf("Create: %v", err)
	}
	dev, err := OpenBlockDevice(tmp)
	if err != nil {
		t.Fatalf("OpenBlockDevice qcow2: %v", err)
	}
	defer dev.Close()
	if dev.Size() <= 0 {
		t.Fatalf("Size: want > 0, got %d", dev.Size())
	}
}

func TestOpenBlockDevice_QCOW2_InvalidFile(t *testing.T) {
	// Create a file with QCOW2 magic bytes but invalid content so
	// OpenDevice fails.
	tmp := filepath.Join(t.TempDir(), "bad.qcow2")
	magic := []byte{0x51, 0x46, 0x49, 0xfb, 0x00, 0x00, 0x00, 0x00}
	if err := os.WriteFile(tmp, magic, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := OpenBlockDevice(tmp)
	if err == nil {
		t.Fatal("expected error opening invalid qcow2")
	}
}

// ── OpenLUKSBlockDevice ────────────────────────────────────────────────────

func TestOpenLUKSBlockDevice_Raw_Success(t *testing.T) {
	passphrase := []byte("test passphrase")
	volumeKey := mustRandBytes(32)
	path := filepath.Join(t.TempDir(), "luks.img")
	buildLUKS1ImageFile(t, path, luks1ImageParams{
		passphrase: passphrase,
		volumeKey:  volumeKey,
	})

	dev, err := OpenLUKSBlockDevice(path, passphrase)
	if err != nil {
		t.Fatalf("OpenLUKSBlockDevice raw: %v", err)
	}
	defer dev.Close()

	// Size() returns 0 for LUKS1 (payload length is dynamic/unknown)
	_ = dev.Size()

	// Write and read back through LUKS encryption.
	payload := []byte("luks plaintext payload")
	if _, err := dev.WriteAt(payload, 0); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
	buf := make([]byte, len(payload))
	if _, err := dev.ReadAt(buf, 0); err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if !bytes.Equal(buf, payload) {
		t.Fatalf("round-trip mismatch: want %q got %q", payload, buf)
	}
}

func TestOpenLUKSBlockDevice_Raw_WrongPassphrase(t *testing.T) {
	passphrase := []byte("correct passphrase")
	volumeKey := mustRandBytes(32)
	path := filepath.Join(t.TempDir(), "luks.img")
	buildLUKS1ImageFile(t, path, luks1ImageParams{
		passphrase: passphrase,
		volumeKey:  volumeKey,
	})

	_, err := OpenLUKSBlockDevice(path, []byte("wrong passphrase"))
	if err == nil {
		t.Fatal("expected error for wrong passphrase")
	}
}

func TestOpenLUKSBlockDevice_Raw_NotExist(t *testing.T) {
	_, err := OpenLUKSBlockDevice(filepath.Join(t.TempDir(), "nofile"), []byte("x"))
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestOpenLUKSBlockDevice_QCOW2_OpenError(t *testing.T) {
	// File has QCOW2 magic but invalid content → OpenDevice fails.
	tmp := filepath.Join(t.TempDir(), "bad.qcow2")
	magic := []byte{0x51, 0x46, 0x49, 0xfb, 0x00, 0x00, 0x00, 0x00}
	if err := os.WriteFile(tmp, magic, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := OpenLUKSBlockDevice(tmp, []byte("pass"))
	if err == nil {
		t.Fatal("expected error opening invalid qcow2 for LUKS")
	}
}

func TestOpenLUKSBlockDevice_QCOW2_Success(t *testing.T) {
	// Build a LUKS1 image in memory, then write it into the virtual disk
	// of a QCOW2 container so that OpenLUKSBlockDevice can find the LUKS
	// header at virtual offset 0.
	passphrase := []byte("qcow2 luks test")
	volumeKey := mustRandBytes(32)

	// Build the raw LUKS1 image bytes.
	const (
		sectorSize = 512
		stripes    = 4000
		slotIter   = 1000
		mkIter     = 1000
		kmOffset   = uint32(8)
		keyBytes   = 32
		cipherName = "aes"
		cipherMode = "xts-plain64"
		hashSpec   = "sha256"
	)
	slotSalt := mustRandBytes(32)
	slotKey := pbkdf2.Key(passphrase, slotSalt, slotIter, keyBytes, sha256.New)
	afData := afSplitKey(volumeKey, stripes)
	encAF := encryptAFBlocks(afData, slotKey)
	mkSalt := mustRandBytes(32)
	mkDigest := pbkdf2.Key(volumeKey, mkSalt, mkIter, 20, sha256.New)
	payloadOffset := uint32(8 + uint32(stripes*keyBytes)/sectorSize + 8)
	luksBytes := makeLUKS1Bytes(payloadOffset, encAF, mkDigest, mkSalt, slotSalt, slotIter)

	// Create a QCOW2 virtual disk large enough to hold the LUKS image.
	qcow2Path := filepath.Join(t.TempDir(), "luks.qcow2")
	vSize := int64(len(luksBytes)+sectorSize) * 2
	if err := image_qcow2.Create(qcow2Path, vSize); err != nil {
		t.Fatalf("Create qcow2: %v", err)
	}
	qdev, err := image_qcow2.OpenDevice(qcow2Path)
	if err != nil {
		t.Fatalf("OpenDevice: %v", err)
	}
	if _, err := qdev.WriteAt(luksBytes, 0); err != nil {
		qdev.Close()
		t.Fatalf("WriteAt LUKS into qcow2: %v", err)
	}
	qdev.Close()

	// Now open it via OpenLUKSBlockDevice — should traverse QCOW2 + LUKS.
	dev, err := OpenLUKSBlockDevice(qcow2Path, passphrase)
	if err != nil {
		t.Fatalf("OpenLUKSBlockDevice qcow2+luks: %v", err)
	}
	defer dev.Close()

	// Write and read back a payload to confirm round-trip works.
	payload := []byte("qcow2+luks round-trip")
	if _, err := dev.WriteAt(payload, 0); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
	buf := make([]byte, len(payload))
	if _, err := dev.ReadAt(buf, 0); err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if !bytes.Equal(buf, payload) {
		t.Fatalf("round-trip mismatch: want %q got %q", payload, buf)
	}
}

// makeLUKS1Bytes builds the raw binary LUKS1 header + key material area.
func makeLUKS1Bytes(payloadOffset uint32, encAF, mkDigest, mkSalt, slotSalt []byte, slotIter int) []byte {
	const (
		sectorSize = 512
		kmOffset   = uint32(8)
		stripes    = 4000
		keyBytes   = 32
	)
	imgSize := int(payloadOffset)*sectorSize + sectorSize
	img := make([]byte, imgSize)
	copy(img[0:6], luks1Magic)
	binary.BigEndian.PutUint16(img[6:8], 1)
	writePaddedStr(img[8:40], "aes")
	writePaddedStr(img[40:72], "xts-plain64")
	writePaddedStr(img[72:104], "sha256")
	binary.BigEndian.PutUint32(img[104:108], payloadOffset)
	binary.BigEndian.PutUint32(img[108:112], keyBytes)
	copy(img[112:132], mkDigest)
	copy(img[132:164], mkSalt)
	binary.BigEndian.PutUint32(img[164:168], 1000)
	copy(img[168:208], "test-uuid-0000000000000000000000")
	base := 208
	binary.BigEndian.PutUint32(img[base:], luks1KeySlotActive)
	binary.BigEndian.PutUint32(img[base+4:], uint32(slotIter))
	copy(img[base+8:base+40], slotSalt)
	binary.BigEndian.PutUint32(img[base+40:], kmOffset)
	binary.BigEndian.PutUint32(img[base+44:], stripes)
	for i := 1; i < 8; i++ {
		b := 208 + i*48
		binary.BigEndian.PutUint32(img[b:], 0xDEAD0000)
	}
	copy(img[int(kmOffset)*sectorSize:], encAF)
	return img
}

// TestOpenLUKSBlockDevice_QCOW2_LUKSError tests that a valid QCOW2 containing
// a bad LUKS image returns an error and cleans up the QCOW2 device.
func TestOpenLUKSBlockDevice_QCOW2_LUKSError(t *testing.T) {
	qcow2Path := filepath.Join(t.TempDir(), "notluks.qcow2")
	if err := image_qcow2.Create(qcow2Path, 4*1024*1024); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// The QCOW2 virtual disk is all zeros — no LUKS magic.
	_, err := OpenLUKSBlockDevice(qcow2Path, []byte("pass"))
	if err == nil {
		t.Fatal("expected error when virtual disk has no LUKS header")
	}
}

// ── luksDevice methods ─────────────────────────────────────────────────────

func TestLUKSDevice_Methods(t *testing.T) {
	passphrase := []byte("luks device methods test")
	volumeKey := mustRandBytes(32)
	path := filepath.Join(t.TempDir(), "luks_methods.img")
	buildLUKS1ImageFile(t, path, luks1ImageParams{
		passphrase: passphrase,
		volumeKey:  volumeKey,
	})

	dev, err := OpenLUKSBlockDevice(path, passphrase)
	if err != nil {
		t.Fatalf("OpenLUKSBlockDevice: %v", err)
	}

	// ReadAt
	buf := make([]byte, 16)
	if _, err := dev.ReadAt(buf, 0); err != nil {
		t.Fatalf("ReadAt: %v", err)
	}

	// WriteAt
	if _, err := dev.WriteAt(buf, 0); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}

	// Size: returns 0 for LUKS1 (dynamic payload length)
	_ = dev.Size()

	// Close
	if err := dev.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
