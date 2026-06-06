// Package create implements the diskimage create sub-commands.
package create

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-diskimages/diskimage"
	"github.com/spf13/cobra"
)

// Command returns the create cobra command with format-specific sub-commands.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a disk image",
		Long: `Create a new disk image file.

Choose the image format as a sub-command:
  diskimage create raw     --file disk.img   --size 10G [--part gpt] [--filesystem ext4]
  diskimage create dmg     --file disk.dmg   --size 10G [--part gpt] [--filesystem apfs]
  diskimage create efivars --file OVMF_VARS.fd [--size 512K]
  diskimage create qcow2   --file disk.qcow2 --size 10G`,
	}
	cmd.AddCommand(rawCmd())
	cmd.AddCommand(dmgCmd())
	cmd.AddCommand(efiCmd())
	cmd.AddCommand(qcow2Cmd())
	return cmd
}

// registerImageFlags adds the common --file, --size, --part, --filesystem,
// and --label flags to cmd.
func registerImageFlags(cmd *cobra.Command, file, sizeStr, part, filesystem, label *string) {
	cmd.Flags().StringVarP(file, "file", "f", "", "Output image file path (required)")
	cmd.Flags().StringVarP(sizeStr, "size", "s", "", "Image size, e.g. 10G, 512M, 2T (required)")
	cmd.Flags().StringVar(part, "part", "none", "Partition table: none, mbr, gpt")
	cmd.Flags().StringVar(filesystem, "filesystem", "none", "Filesystem: none, ext4, fat32, btrfs, xfs, zfs, exfat, ntfs, apfs")
	cmd.Flags().StringVar(label, "label", "", "Volume label / pool name")
	cmd.MarkFlagRequired("file") //nolint:errcheck
	cmd.MarkFlagRequired("size") //nolint:errcheck
}

// runCreate executes the raw/dmg image creation.
func runCreate(cmd *cobra.Command, file, sizeStr string, format diskimage.ImageFormat, part, filesystem, label, udif, passphrase string) error {
	sizeBytes, err := parseSize(sizeStr)
	if err != nil {
		return fmt.Errorf("invalid --size %q: %w", sizeStr, err)
	}
	opts := diskimage.CreateOptions{
		Path:          file,
		SizeBytes:     sizeBytes,
		Format:        format,
		Partition:     diskimage.PartitionScheme(strings.ToLower(part)),
		Filesystem:    diskimage.FilesystemType(strings.ToLower(filesystem)),
		Label:         label,
		DmgUDIFFormat: udif,
	}
	if passphrase != "" {
		opts.DmgPassphrase = []byte(passphrase)
	}
	if err := rawCreateFunc(opts); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(),
		"created %s: size=%s format=%s part=%s filesystem=%s\n",
		file, humanSize(sizeBytes), format, part, filesystem,
	)
	return nil
}

// parseSize converts a human-readable size string to bytes.
// Supported suffixes: B, K/KB/KiB, M/MB/MiB, G/GB/GiB, T/TB/TiB (case-insensitive).
// A bare number is interpreted as bytes.
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size string")
	}

	suffixes := map[string]int64{
		"b": 1,
		"k": 1024, "kb": 1024, "kib": 1024,
		"m": 1024 * 1024, "mb": 1024 * 1024, "mib": 1024 * 1024,
		"g": 1024 * 1024 * 1024, "gb": 1024 * 1024 * 1024, "gib": 1024 * 1024 * 1024,
		"t": 1024 * 1024 * 1024 * 1024, "tb": 1024 * 1024 * 1024 * 1024, "tib": 1024 * 1024 * 1024 * 1024,
	}

	lower := strings.ToLower(s)
	// Iterate suffixes longest-first so that "kib" matches before "k".
	keys := make([]string, 0, len(suffixes))
	for k := range suffixes {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && len(keys[j]) > len(keys[j-1]); j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	for _, suffix := range keys {
		mult := suffixes[suffix]
		if strings.HasSuffix(lower, suffix) {
			numStr := strings.TrimSpace(strings.TrimSuffix(lower, suffix))
			n, err := strconv.ParseInt(numStr, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("cannot parse number %q", numStr)
			}
			if n <= 0 {
				return 0, fmt.Errorf("size must be positive")
			}
			return n * mult, nil
		}
	}

	// Plain number → bytes.
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot parse size %q", s)
	}
	if n <= 0 {
		return 0, fmt.Errorf("size must be positive")
	}
	return n, nil
}

// humanSize formats bytes as a human-readable string.
func humanSize(b int64) string {
	const (
		kib = 1024
		mib = 1024 * kib
		gib = 1024 * mib
		tib = 1024 * gib
	)
	switch {
	case b >= tib && b%tib == 0:
		return fmt.Sprintf("%dTiB", b/tib)
	case b >= gib && b%gib == 0:
		return fmt.Sprintf("%dGiB", b/gib)
	case b >= mib && b%mib == 0:
		return fmt.Sprintf("%dMiB", b/mib)
	case b >= kib && b%kib == 0:
		return fmt.Sprintf("%dKiB", b/kib)
	default:
		return fmt.Sprintf("%dB", b)
	}
}
