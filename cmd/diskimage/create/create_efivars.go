package create

import (
	"fmt"

	"github.com/go-diskimages/diskimage"
	"github.com/spf13/cobra"
)

// efiVarsCreateFunc is the function used by the efivars sub-command.
// Tests may replace this.
var efiVarsCreateFunc = func(path string, sizeBytes int64) error {
	s, err := diskimage.CreateUEFIVarsStore(path, sizeBytes)
	if err != nil {
		return err
	}
	return s.Close()
}

// efiCmd returns the "create efivars" sub-command.
func efiCmd() *cobra.Command {
	var file, sizeStr string
	cmd := &cobra.Command{
		Use:   "efivars",
		Short: "Create a UEFI variable store file (pflash)",
		Long: `Create a UEFI NvVar variable store file compatible with QEMU pflash images.

Default size is 512K (OVMF_VARS.fd, x86-64). Use 64M for arm64 (QEMU_VARS.fd).

Examples:
  diskimage create efivars --file OVMF_VARS.fd
  diskimage create efivars --file QEMU_VARS.fd --size 64M`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEfi(cmd, file, sizeStr)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "Output file path (required)")
	cmd.Flags().StringVarP(&sizeStr, "size", "s", "512K", "Store size (default 512K for x86-64, 64M for arm64)")
	cmd.MarkFlagRequired("file") //nolint:errcheck
	return cmd
}

// runEfi executes the efivars store creation.
func runEfi(cmd *cobra.Command, file, sizeStr string) error {
	sizeBytes, err := parseSize(sizeStr)
	if err != nil {
		return fmt.Errorf("invalid --size %q: %w", sizeStr, err)
	}
	if err := efiVarsCreateFunc(file, sizeBytes); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "created %s: size=%s format=efivars\n", file, humanSize(sizeBytes))
	return nil
}
