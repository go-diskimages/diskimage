package create

import (
	"github.com/go-diskimages/diskimage"
	"github.com/spf13/cobra"
)

// rawCreateFunc is the function used by the raw and dmg sub-commands.
// Tests may replace this to avoid doing real disk operations.
var rawCreateFunc = diskimage.Create

// rawCmd returns the "create raw" sub-command.
func rawCmd() *cobra.Command {
	var file, sizeStr, part, filesystem, label string
	cmd := &cobra.Command{
		Use:   "raw",
		Short: "Create a raw disk image",
		Long: `Create a raw sparse image with an optional partition table and filesystem.

  --part:       none (default), mbr, gpt
  --filesystem: none (default), ext4, fat32, btrfs, xfs, zfs, exfat, ntfs, apfs

Examples:
  diskimage create raw --file disk.img --size 10G
  diskimage create raw --file disk.img --size 20G --part gpt --filesystem ext4
  diskimage create raw --file disk.img --size 5G  --part mbr --filesystem fat32 --label DATA`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd, file, sizeStr, diskimage.FormatRaw, part, filesystem, label, "", "")
		},
	}
	registerImageFlags(cmd, &file, &sizeStr, &part, &filesystem, &label)
	return cmd
}
