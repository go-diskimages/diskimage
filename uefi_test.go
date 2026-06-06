package diskimage

import (
	"path/filepath"
	"testing"
)

// TestCreateUEFIVarsStore_Success verifies that CreateUEFIVarsStore creates a
// valid UEFI NvVar variable store file of the requested size.
func TestCreateUEFIVarsStore_Success(t *testing.T) {
	path := filepath.Join(t.TempDir(), "OVMF_VARS.fd")
	s, err := CreateUEFIVarsStore(path, 4096)
	if err != nil {
		t.Fatalf("CreateUEFIVarsStore: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify exported size constants are sane.
	if UEFIVarsSizeX86_64 <= 0 {
		t.Error("UEFIVarsSizeX86_64 must be positive")
	}
	if UEFIVarsSizeARM64 <= 0 {
		t.Error("UEFIVarsSizeARM64 must be positive")
	}
}

// TestCreateUEFIVarsStore_ErrorPropagation verifies that errors from the
// underlying Format call are propagated to the caller.
func TestCreateUEFIVarsStore_ErrorPropagation(t *testing.T) {
	if _, err := CreateUEFIVarsStore("/no/such/dir/vars.bin", 4096); err == nil {
		t.Fatal("expected error for bad path, got nil")
	}
}
