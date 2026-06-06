package copy

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-diskimages/diskimage"
	"github.com/spf13/cobra"
)

// copyReadFunc and copyWriteFunc are indirection points for tests.
var copyReadFunc = diskimage.ReadFile
var copyWriteFunc = diskimage.WriteFile

// Command returns the cp cobra sub-command.
func Command() *cobra.Command {
	var (
		file       string
		filesystem string
		part       int
		toImage    bool
		fromImage  bool
	)

	cmd := &cobra.Command{
		Use:   "copy <src> <dst>",
		Short: "Copy files to/from a disk image",
		Long: `Copy a regular file between the host and a filesystem inside a
disk image. Use --to-image to copy host->image, --from-image to copy image->host.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if toImage && fromImage {
				return fmt.Errorf("cannot set both --to-image and --from-image")
			}
			if !toImage && !fromImage {
				return fmt.Errorf("must set either --to-image or --from-image")
			}
			imgPath := file
			fsType := diskimage.FilesystemType(strings.ToLower(filesystem))

			if toImage {
				srcLocal := args[0]
				dstImage := args[1]
				data, err := os.ReadFile(srcLocal)
				if err != nil {
					return err
				}
				// preserve basic permission bits when possible
				perm := os.FileMode(0o644)
				if st, err := os.Stat(srcLocal); err == nil {
					perm = st.Mode() & os.ModePerm
				}
				opts := diskimage.FileOptions{Path: imgPath, Filesystem: fsType, PartIndex: part, FilePath: dstImage}
				if err := copyWriteFunc(opts, data, perm); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "copied to image")
				return nil
			}

			// fromImage
			srcImage := args[0]
			dstLocal := args[1]
			opts := diskimage.FileOptions{Path: imgPath, Filesystem: fsType, PartIndex: part, FilePath: srcImage}
			data, err := copyReadFunc(opts)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstLocal, data, 0o644); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "copied from image")
			return nil
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "disk image file path (required)")
	cmd.Flags().StringVar(&filesystem, "filesystem", "", "filesystem type: ext4, fat32, btrfs, xfs, zfs, exfat (auto-detected if omitted)")
	cmd.Flags().IntVar(&part, "part", -1, "0-based partition index to open (-1 to auto-detect or 0 for first partition)")
	cmd.Flags().BoolVar(&toImage, "to-image", false, "copy local file into image")
	cmd.Flags().BoolVar(&fromImage, "from-image", false, "copy file from image to local host")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}
