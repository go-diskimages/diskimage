// Package grow implements the diskimage grow sub-command.
package grow

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-diskimages/diskimage"
	"github.com/spf13/cobra"
)

// growFunc is the underlying implementation used by the command. Tests may
// replace it to avoid doing real disk work.
var growFunc = diskimage.Grow

// Command returns the cobra command for `diskimage grow`.
func Command() *cobra.Command {
	var (
		file    string
		sizeStr string
	)

	cmd := &cobra.Command{
		Use:   "grow",
		Short: "Grow (resize) a disk image file",
		Long: `Grow expands or shrinks the backing image file. The --size
must be an absolute size like the create subcommand (e.g. 20G).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			sizeBytes, err := parseSize(sizeStr)
			if err != nil {
				return fmt.Errorf("invalid --size %q: %w", sizeStr, err)
			}
			if file == "" {
				return fmt.Errorf("--file is required")
			}
			if err := growFunc(file, sizeBytes); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "resized %s -> %s\n", file, humanSize(sizeBytes))
			return nil
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "Image file path (required)")
	cmd.Flags().StringVarP(&sizeStr, "size", "s", "", "Image size, e.g. 10G, 512M (required)")
	cmd.MarkFlagRequired("file")
	cmd.MarkFlagRequired("size")
	return cmd
}

// parseSize converts a human-readable size string to bytes.
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
	// longest suffix first
	keys := make([]string, 0, len(suffixes))
	for k := range suffixes {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && len(keys[j]) > len(keys[j-1]); j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	for _, suf := range keys {
		if strings.HasSuffix(lower, suf) {
			numStr := strings.TrimSpace(strings.TrimSuffix(lower, suf))
			n, err := strconv.ParseInt(numStr, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("cannot parse number %q", numStr)
			}
			if n <= 0 {
				return 0, fmt.Errorf("size must be positive")
			}
			return n * suffixes[suf], nil
		}
	}
	n, err := strconv.ParseInt(lower, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot parse size %q", s)
	}
	if n <= 0 {
		return 0, fmt.Errorf("size must be positive")
	}
	return n, nil
}

// humanSize formats bytes to a human friendly string (KiB/MiB...).
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
