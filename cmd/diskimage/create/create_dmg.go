package create

import (
	"fmt"
	"strings"

	"github.com/go-diskimages/diskimage"
	"github.com/spf13/cobra"
)

// dmgCmd returns the "create dmg" sub-command.
func dmgCmd() *cobra.Command {
	var file, sizeStr, part, filesystem, label, udif, password string
	cmd := &cobra.Command{
		Use:   "dmg",
		Short: "Create a DMG disk image",
		Long: `Create a DMG disk image. Only filesystems natively supported by macOS may
be embedded inside the DMG: none, apfs, fat32, exfat. APFS DMGs may be
encrypted (FileVault FDE, AES-256-XTS) by passing --password.

  --part:        none (default), mbr, gpt
  --filesystem:  none (default), apfs, fat32, exfat
  --udif-format: UDIF format (default UDRW): UDRW, UDSP, ...
  --password:    Passphrase for FileVault-encrypted APFS DMG (apfs only)

Examples:
  diskimage create dmg --file disk.dmg --size 10G --filesystem apfs
  diskimage create dmg --file disk.dmg --size 10G --filesystem apfs --password 'secret'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fs := diskimage.FilesystemType(strings.ToLower(filesystem))
			if !diskimage.IsDmgSupportedFilesystem(fs) {
				return fmt.Errorf("filesystem %q is not supported in DMG images (supported: %v)",
					filesystem, diskimage.DmgSupportedFilesystems)
			}
			if password != "" && fs != diskimage.FSApfs {
				return fmt.Errorf("--password requires --filesystem apfs")
			}
			return runCreate(cmd, file, sizeStr, diskimage.FormatDmg, part, filesystem, label, udif, password)
		},
	}
	registerImageFlags(cmd, &file, &sizeStr, &part, &filesystem, &label)
	cmd.Flags().StringVar(&udif, "udif-format", "", "UDIF format for DMG (default UDRW): UDRW, UDSP")
	cmd.Flags().StringVar(&password, "password", "", "Passphrase for FileVault-encrypted APFS DMG")
	return cmd
}
