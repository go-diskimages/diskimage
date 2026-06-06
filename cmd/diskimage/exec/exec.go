package exec

import "github.com/spf13/cobra"

// Command returns the exec cobra sub-command.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec \"command [args...]\" [flags]",
		Short: "Run a coreutils-like command on a disk image",
		Long: `exec runs a single quoted command string against a disk image filesystem.

The entire command (with all its arguments) must be passed as one quoted
string so that the command's own arguments are not confused with diskimage
flags such as --file or --filesystem.

  Supported commands: ls, cat, uefi, zpool, zfs, ntfs, xfs, btrfs, ext4, fat32, exfat, part, df, du

Examples:
  diskimage exec "ls"                          --file disk.img
  diskimage exec "ls -alFh /etc"               --file disk.img
  diskimage exec "cat /etc/hostname"           --file disk.img
  diskimage exec "uefi list"                   --file OVMF_VARS.fd
  diskimage exec "uefi get BootOrder"          --file OVMF_VARS.fd
  diskimage exec "uefi set BootOrder --data 0100" --file OVMF_VARS.fd
  diskimage exec "uefi delete BootOrder"       --file OVMF_VARS.fd
  diskimage exec "zfs list"                    --file pool.img
  diskimage exec "df -h"                       --file disk.img
  diskimage exec "xfs chmod 0755 /etc/init"    --file disk.img
  diskimage exec "btrfs symlink ../lib /lib64" --file disk.img
  diskimage exec "ext4 label set rootfs"       --file disk.img
  diskimage exec "fat32 label"                 --file disk.img
  diskimage exec "ntfs label set WinDrive"     --file disk.img
  diskimage exec "exfat label set USBSTICK"    --file disk.img
`,
		Args: cobra.ExactArgs(1),
		RunE: execRunE,
	}

	cmd.Flags().String("file", "", "path to the disk image (required for image operations)")
	cmd.Flags().String("filesystem", "", "filesystem type: ext4, fat32, btrfs, xfs, zfs, exfat, ntfs (auto-detected if omitted)")
	cmd.Flags().Int("part", 0, "0-based partition index to open (0 for unpartitioned images)")
	cmd.Flags().String("path", "/", "default directory path inside the filesystem (overridden by a path in the command string)")
	return cmd
}
