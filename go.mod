module github.com/go-diskimages/diskimage

go 1.25.0

require (
	github.com/go-diskimages/dmg v0.0.0
	github.com/go-diskimages/qcow2 v0.0.0
	github.com/go-fde/apfs v0.0.0
	github.com/go-fde/fde v0.0.0
	github.com/go-filesystems/apfs v0.0.0
	github.com/go-filesystems/btrfs v0.0.0
	github.com/go-filesystems/exfat v0.0.0
	github.com/go-filesystems/ext4 v0.0.0
	github.com/go-filesystems/fat32 v0.0.0
	github.com/go-filesystems/interface v0.0.0
	github.com/go-filesystems/ntfs v0.0.0
	github.com/go-filesystems/uefi v0.0.0
	github.com/go-filesystems/xfs v0.0.0
	github.com/go-filesystems/zfs v0.0.0
	github.com/spf13/cobra v1.10.2
)

require (
	github.com/go-compressions/lzfse v0.0.0 // indirect
	github.com/go-crypto/ccm v0.0.0 // indirect
	github.com/go-crypto/zfscrypt v0.0.0 // indirect
	github.com/go-fde/clear v0.0.0 // indirect
	github.com/go-fde/luks v0.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/sys v0.43.0 // indirect
)

replace github.com/go-filesystems/btrfs => ../../go-filesystems/btrfs

replace github.com/go-filesystems/exfat => ../../go-filesystems/exfat

replace github.com/go-filesystems/ext4 => ../../go-filesystems/ext4

replace github.com/go-filesystems/fat32 => ../../go-filesystems/fat32

replace github.com/go-filesystems/interface => ../../go-filesystems/interface

replace github.com/go-filesystems/xfs => ../../go-filesystems/xfs

replace github.com/go-filesystems/zfs => ../../go-filesystems/zfs

replace github.com/go-filesystems/ntfs => ../../go-filesystems/ntfs

replace github.com/go-filesystems/apfs => ../../go-filesystems/apfs

require (
	github.com/configuration-management-tool/mock/pkg/go-bootloaders/grub v0.0.0
	golang.org/x/crypto v0.50.0
)

replace github.com/configuration-management-tool/mock/pkg/go-bootloaders/grub => ../../go-bootloaders/grub

replace github.com/go-diskimages/qcow2 => ../qcow2

replace github.com/go-fde/fde => ../../go-fde/fde

replace github.com/go-fde/apfs => ../../go-fde/apfs

replace github.com/go-diskimages/dmg => ../dmg

replace github.com/go-filesystems/uefi => ../../go-filesystems/uefi

replace github.com/go-crypto/zfscrypt => ../../go-crypto/zfscrypt

replace github.com/go-crypto/ccm => ../../go-crypto/ccm

replace github.com/go-fde/luks => ../../go-fde/luks

replace github.com/go-fde/clear => ../../go-fde/clear

replace github.com/go-compressions/lzfse => ../../go-compressions/lzfse
