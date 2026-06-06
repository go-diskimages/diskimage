package diskimage

import (
	"path/filepath"
	"testing"
)

func TestCreate_ValidationAndPartitionErrors(t *testing.T) {
	// Missing path
	if err := Create(CreateOptions{Path: "", SizeBytes: 1024}); err == nil {
		t.Fatalf("expected error for missing path")
	}

	// Unsupported format
	p := filepath.Join(t.TempDir(), "out.img")
	if err := Create(CreateOptions{Path: p, SizeBytes: 2 * 1024, Format: ImageFormat("bogus")}); err == nil {
		t.Fatalf("expected unsupported format error")
	}

	// Unsupported partition scheme
	if err := Create(CreateOptions{Path: p, SizeBytes: 2 * 1024, Partition: PartitionScheme("bogus")}); err == nil {
		t.Fatalf("expected unsupported partition scheme error")
	}

	// Too small for MBR
	small := int64(1 << 20) // 1 MiB
	if err := Create(CreateOptions{Path: filepath.Join(t.TempDir(), "m.img"), SizeBytes: small, Partition: PartMBR}); err == nil {
		t.Fatalf("expected error for image too small for MBR")
	}

	// Too small for GPT
	if err := Create(CreateOptions{Path: filepath.Join(t.TempDir(), "g.img"), SizeBytes: small, Partition: PartGPT}); err == nil {
		t.Fatalf("expected error for image too small for GPT")
	}
}
