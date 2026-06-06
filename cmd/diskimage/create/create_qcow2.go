package create

import (
	"fmt"

	image_qcow2 "github.com/go-diskimages/qcow2"
	"github.com/spf13/cobra"
)

// qcow2CreateFunc is the function used by the qcow2 sub-command.
// Tests may replace this to avoid doing real disk operations.
var qcow2CreateFunc = func(path string, sizeBytes int64) error {
	return image_qcow2.Create(path, sizeBytes)
}

// qcow2Cmd returns the "create qcow2" sub-command.
func qcow2Cmd() *cobra.Command {
	var file, sizeStr string
	cmd := &cobra.Command{
		Use:   "qcow2",
		Short: "Create an empty QCOW2 v2 disk image",
		Long: `Create a new empty QCOW2 v2 disk image at the given path.

The image has no allocated data clusters; all reads return zeros until
data is written. The virtual size is set to --size.

Examples:
  diskimage create qcow2 --file disk.qcow2 --size 10G
  diskimage create qcow2 --file disk.qcow2 --size 512M`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQCOW2Create(cmd, file, sizeStr)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "Output QCOW2 file path (required)")
	cmd.Flags().StringVarP(&sizeStr, "size", "s", "", "Virtual image size, e.g. 10G, 512M (required)")
	cmd.MarkFlagRequired("file") //nolint:errcheck
	cmd.MarkFlagRequired("size") //nolint:errcheck
	return cmd
}

// runQCOW2Create parses flags and calls qcow2CreateFunc.
func runQCOW2Create(cmd *cobra.Command, file, sizeStr string) error {
	sizeBytes, err := parseSize(sizeStr)
	if err != nil {
		return fmt.Errorf("invalid --size %q: %w", sizeStr, err)
	}
	if err := qcow2CreateFunc(file, sizeBytes); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(),
		"created %s: size=%s format=qcow2\n",
		file, humanSize(sizeBytes),
	)
	return nil
}
