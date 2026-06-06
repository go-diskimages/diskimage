package diskimage

import (
	filesystem_uefi "github.com/go-filesystems/uefi"
)

// UEFIVarsStore is the handle returned by CreateUEFIVarsStore.
// It embeds the full uefi.Store interface so callers can read/write
// variables and enroll Secure Boot keys before handing the file to QEMU.
type UEFIVarsStore = filesystem_uefi.Store

// CreateUEFIVarsStore creates a new UEFI NvVar variable store file at path
// with the given size in bytes.
//
// The file is created (or truncated) and initialised with a valid
// VARIABLE_STORE_HEADER followed by erased-flash bytes (0xFF), matching the
// format expected by QEMU pflash images (OVMF_VARS.fd for x86-64,
// QEMU_VARS.fd for arm64).
//
// Typical sizes:
//
//	x86-64 : UEFIVarsSizeX86_64 (512 KiB)
//	arm64  : UEFIVarsSizeARM64  (64 MiB)
//
// The returned UEFIVarsStore is open and ready for use; call Close when done.
func CreateUEFIVarsStore(path string, sizeBytes int64) (UEFIVarsStore, error) {
	return filesystem_uefi.Format(path, sizeBytes)
}

// UEFIVarsSizeX86_64 is the standard OVMF_VARS.fd size for x86-64 (512 KiB).
const UEFIVarsSizeX86_64 = filesystem_uefi.VarsSizeX86_64

// UEFIVarsSizeARM64 is the standard QEMU_VARS.fd size for arm64 (64 MiB).
const UEFIVarsSizeARM64 = filesystem_uefi.VarsSizeARM64
