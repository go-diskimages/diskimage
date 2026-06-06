// diskimage is a command-line tool for creating and managing VM disk images.
package main

import (
	"fmt"
	"os"

	"github.com/go-diskimages/diskimage/cmd/diskimage/copy"
	"github.com/go-diskimages/diskimage/cmd/diskimage/create"
	"github.com/go-diskimages/diskimage/cmd/diskimage/exec"
	"github.com/go-diskimages/diskimage/cmd/diskimage/grow"
	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diskimage",
		Short: "Create and manage VM disk images",
		Long: `diskimage creates raw disk images with optional partition tables
	(MBR, GPT) and filesystems (ext4, fat32, btrfs, xfs, zfs, exfat).`,
	}
	cmd.AddCommand(create.Command())
	cmd.AddCommand(grow.Command())
	cmd.AddCommand(exec.Command())
	cmd.AddCommand(copy.Command())
	return cmd
}
