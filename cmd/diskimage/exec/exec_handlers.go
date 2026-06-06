package exec

import (
	"fmt"
	"strings"

	"github.com/go-diskimages/diskimage"
	"github.com/spf13/cobra"
)

// execRunE is the RunE handler for the legacy `exec "command string"` form.
// The single argument must be a fully-quoted command string so that its
// internal arguments are not confused with diskimage flags.
func execRunE(cmd *cobra.Command, args []string) error {
	file, _ := cmd.Flags().GetString("file")
	filesystemArg, _ := cmd.Flags().GetString("filesystem")
	partIndex, _ := cmd.Flags().GetInt("part")
	dirPath, _ := cmd.Flags().GetString("path")

	// args[0] is the single quoted command string, e.g. "ls -alF /etc".
	argv := strings.Fields(args[0])
	if len(argv) == 0 {
		return fmt.Errorf("no command provided")
	}
	return execDispatch(cmd, argv, file, filesystemArg, partIndex, dirPath)
}

func execDispatch(cmd *cobra.Command, argv []string, file, filesystemArg string, partIndex int, dirPath string) error {
	switch argv[0] {
	case "ls":
		return handleLS(cmd, argv, file, filesystemArg, partIndex, dirPath)
	case "cat":
		return handleCat(cmd, argv, file, filesystemArg, partIndex)
	case "uefi":
		return handleUefi(cmd, argv, file)
	case "zpool":
		return handleZpool(cmd, argv, file, partIndex)
	case "zfs":
		return handleZfs(cmd, argv, file, partIndex)
	case "ntfs":
		return handleNtfs(cmd, argv, file, partIndex)
	case "xfs":
		return handleXfs(cmd, argv, file, partIndex)
	case "btrfs":
		return handleBtrfs(cmd, argv, file, partIndex)
	case "ext4":
		return handleExt4(cmd, argv, file, partIndex)
	case "fat32":
		return handleFat32(cmd, argv, file, partIndex)
	case "exfat":
		return handleExfat(cmd, argv, file, partIndex)
	case "part":
		return handlePart(cmd, argv, file)
	case "df":
		return handleDF(cmd, argv, file, filesystemArg, partIndex)
	case "du":
		return handleDU(cmd, argv, file, filesystemArg, partIndex)
	default:
		return fmt.Errorf("unsupported command: %q", argv[0])
	}
}

func handleNtfs(cmd *cobra.Command, argv []string, file string, partIndex int) error {
	if len(argv) >= 2 && argv[1] == "label" {
		if file == "" {
			return fmt.Errorf("--file is required for 'ntfs label'")
		}
		if len(argv) < 3 || argv[2] == "get" {
			return NtfsGetLabelOnImage(file, partIndex, cmd.OutOrStdout())
		}
		if argv[2] == "set" {
			if len(argv) < 4 {
				return fmt.Errorf("usage: ntfs label set <new-label>")
			}
			return NtfsSetLabelOnImage(file, partIndex, argv[3], cmd.OutOrStdout())
		}
		return fmt.Errorf("usage: ntfs label [get | set <new-label>]")
	}
	if len(argv) >= 2 && argv[1] == "compact" {
		if file == "" {
			return fmt.Errorf("--file is required for 'ntfs compact' on image")
		}
		// Ensure the image is NTFS
		if fsType, err := diskimage.DetectFilesystem(file, partIndex); err != nil {
			return fmt.Errorf("detect filesystem for 'ntfs compact': %w", err)
		} else if fsType != diskimage.FSNTFS {
			return fmt.Errorf("'ntfs compact' requires a NTFS image (detected: %s)", fsType)
		}
		// parse flags from the command string (argv) rather than cobra flags
		human := false
		listFiles := false
		for _, a := range argv[2:] {
			if strings.HasPrefix(a, "--") {
				switch a {
				case "--human":
					human = true
				case "--list-files":
					listFiles = true
				}
				continue
			}
			if strings.HasPrefix(a, "-") {
				// short flags can be combined, e.g. -hl
				for _, c := range a[1:] {
					switch c {
					case 'h':
						human = true
					case 'l':
						// -l => list-files
						listFiles = true
					}
				}
				continue
			}
		}
		return NtfsCompactFromImage(file, partIndex, human, listFiles, cmd.OutOrStdout())
	}
	return fmt.Errorf("unsupported ntfs subcommand")
}

func handleZpool(cmd *cobra.Command, argv []string, file string, partIndex int) error {
	if len(argv) >= 2 && argv[1] == "status" {
		if file == "" {
			return fmt.Errorf("--file is required for 'zpool status' on image")
		}
		if fsType, err := diskimage.DetectFilesystem(file, partIndex); err != nil {
			return fmt.Errorf("detect filesystem for 'zpool status': %w", err)
		} else if fsType != diskimage.FSZfs {
			return fmt.Errorf("'zpool status' requires a ZFS image (detected: %s)", fsType)
		}
		return ZpoolStatusFromImage(file, partIndex, cmd.OutOrStdout())
	}
	return fmt.Errorf("unsupported zpool subcommand")
}

// handleBtrfs dispatches the `btrfs <subcommand> ...` subcommands. Supported
// subcommands operate on the btrfs-specific FS interface.
//
// Examples:
//
//	diskimage exec "btrfs chmod 0755 /etc/script.sh"       --file disk.img
//	diskimage exec "btrfs chown 1000 1000 /home/user/data" --file disk.img
//	diskimage exec "btrfs chtimes 0 0 /etc/old.cfg"        --file disk.img
//	diskimage exec "btrfs link /a /b"                       --file disk.img
//	diskimage exec "btrfs symlink ../lib /usr/lib64"        --file disk.img
//	diskimage exec "btrfs setxattr /f user.note hello"      --file disk.img
//	diskimage exec "btrfs removexattr /f user.note"         --file disk.img
func handleBtrfs(cmd *cobra.Command, argv []string, file string, partIndex int) error {
	if len(argv) < 2 {
		return fmt.Errorf("btrfs: missing subcommand (chmod|chown|chtimes|link|symlink|setxattr|removexattr)")
	}
	if file == "" {
		return fmt.Errorf("--file is required for 'btrfs %s'", argv[1])
	}
	switch argv[1] {
	case "chmod":
		if len(argv) < 4 {
			return fmt.Errorf("usage: btrfs chmod <mode> <path>")
		}
		return BtrfsChmodOnImage(file, partIndex, argv[2], argv[3], cmd.OutOrStdout())
	case "chown":
		if len(argv) < 5 {
			return fmt.Errorf("usage: btrfs chown <uid> <gid> <path>")
		}
		return BtrfsChownOnImage(file, partIndex, argv[2], argv[3], argv[4], cmd.OutOrStdout())
	case "chtimes":
		if len(argv) < 5 {
			return fmt.Errorf("usage: btrfs chtimes <atime-unixsec> <mtime-unixsec> <path>")
		}
		return BtrfsChtimesOnImage(file, partIndex, argv[2], argv[3], argv[4], cmd.OutOrStdout())
	case "link":
		if len(argv) < 4 {
			return fmt.Errorf("usage: btrfs link <oldPath> <newPath>")
		}
		return BtrfsLinkOnImage(file, partIndex, argv[2], argv[3], cmd.OutOrStdout())
	case "symlink":
		if len(argv) < 4 {
			return fmt.Errorf("usage: btrfs symlink <target> <linkPath>")
		}
		return BtrfsSymlinkOnImage(file, partIndex, argv[2], argv[3], cmd.OutOrStdout())
	case "setxattr":
		if len(argv) < 5 {
			return fmt.Errorf("usage: btrfs setxattr <path> <name> <value>")
		}
		return BtrfsSetXattrOnImage(file, partIndex, argv[2], argv[3], argv[4], cmd.OutOrStdout())
	case "removexattr":
		if len(argv) < 4 {
			return fmt.Errorf("usage: btrfs removexattr <path> <name>")
		}
		return BtrfsRemoveXattrOnImage(file, partIndex, argv[2], argv[3], cmd.OutOrStdout())
	case "truncate":
		if len(argv) < 4 {
			return fmt.Errorf("usage: btrfs truncate <size> <path>")
		}
		return BtrfsTruncateOnImage(file, partIndex, argv[2], argv[3], cmd.OutOrStdout())
	case "extended-stat", "stat":
		if len(argv) < 3 {
			return fmt.Errorf("usage: btrfs extended-stat <path>")
		}
		return BtrfsExtendedStatOnImage(file, partIndex, argv[2], cmd.OutOrStdout())
	case "label":
		if len(argv) < 3 || argv[2] == "get" {
			return BtrfsGetLabelOnImage(file, partIndex, cmd.OutOrStdout())
		}
		if argv[2] == "set" {
			if len(argv) < 4 {
				return fmt.Errorf("usage: btrfs label set <new-label>")
			}
			return BtrfsSetLabelOnImage(file, partIndex, argv[3], cmd.OutOrStdout())
		}
		return fmt.Errorf("usage: btrfs label [get | set <new-label>]")
	default:
		return fmt.Errorf("unsupported btrfs subcommand %q (expected chmod|chown|chtimes|link|symlink|setxattr|removexattr|truncate|extended-stat|label)", argv[1])
	}
}

// handleExfat dispatches the `exfat <subcommand> ...` subcommands.
// Currently just the Labeller capability (label get / label set).
func handleExfat(cmd *cobra.Command, argv []string, file string, partIndex int) error {
	if len(argv) < 2 {
		return fmt.Errorf("exfat: missing subcommand (label)")
	}
	if file == "" {
		return fmt.Errorf("--file is required for 'exfat %s'", argv[1])
	}
	switch argv[1] {
	case "label":
		if len(argv) < 3 || argv[2] == "get" {
			return ExfatGetLabelOnImage(file, partIndex, cmd.OutOrStdout())
		}
		if argv[2] == "set" {
			if len(argv) < 4 {
				return fmt.Errorf("usage: exfat label set <new-label>")
			}
			return ExfatSetLabelOnImage(file, partIndex, argv[3], cmd.OutOrStdout())
		}
		return fmt.Errorf("usage: exfat label [get | set <new-label>]")
	default:
		return fmt.Errorf("unsupported exfat subcommand %q (expected label)", argv[1])
	}
}

// handleFat32 dispatches the `fat32 <subcommand> ...` subcommands.
// Currently just the Labeller capability (label get / label set).
func handleFat32(cmd *cobra.Command, argv []string, file string, partIndex int) error {
	if len(argv) < 2 {
		return fmt.Errorf("fat32: missing subcommand (label)")
	}
	if file == "" {
		return fmt.Errorf("--file is required for 'fat32 %s'", argv[1])
	}
	switch argv[1] {
	case "label":
		if len(argv) < 3 || argv[2] == "get" {
			return Fat32GetLabelOnImage(file, partIndex, cmd.OutOrStdout())
		}
		if argv[2] == "set" {
			if len(argv) < 4 {
				return fmt.Errorf("usage: fat32 label set <new-label>")
			}
			return Fat32SetLabelOnImage(file, partIndex, argv[3], cmd.OutOrStdout())
		}
		return fmt.Errorf("usage: fat32 label [get | set <new-label>]")
	default:
		return fmt.Errorf("unsupported fat32 subcommand %q (expected label)", argv[1])
	}
}

// handleExt4 dispatches the `ext4 <subcommand> ...` subcommands. Currently
// just the Labeller capability (label get / label set).
func handleExt4(cmd *cobra.Command, argv []string, file string, partIndex int) error {
	if len(argv) < 2 {
		return fmt.Errorf("ext4: missing subcommand (label)")
	}
	if file == "" {
		return fmt.Errorf("--file is required for 'ext4 %s'", argv[1])
	}
	switch argv[1] {
	case "label":
		if len(argv) < 3 || argv[2] == "get" {
			return Ext4GetLabelOnImage(file, partIndex, cmd.OutOrStdout())
		}
		if argv[2] == "set" {
			if len(argv) < 4 {
				return fmt.Errorf("usage: ext4 label set <new-label>")
			}
			return Ext4SetLabelOnImage(file, partIndex, argv[3], cmd.OutOrStdout())
		}
		return fmt.Errorf("usage: ext4 label [get | set <new-label>]")
	default:
		return fmt.Errorf("unsupported ext4 subcommand %q (expected label)", argv[1])
	}
}

func handleXfs(cmd *cobra.Command, argv []string, file string, partIndex int) error {
	if len(argv) < 2 {
		return fmt.Errorf("xfs: missing subcommand (grow|chmod|chown|chtimes|link|symlink|truncate|extended-stat|label)")
	}
	if file == "" {
		return fmt.Errorf("--file is required for 'xfs %s'", argv[1])
	}
	switch argv[1] {
	case "chmod":
		if len(argv) < 4 {
			return fmt.Errorf("usage: xfs chmod <mode> <path>")
		}
		return XfsChmodOnImage(file, partIndex, argv[2], argv[3], cmd.OutOrStdout())
	case "chown":
		if len(argv) < 5 {
			return fmt.Errorf("usage: xfs chown <uid> <gid> <path>")
		}
		return XfsChownOnImage(file, partIndex, argv[2], argv[3], argv[4], cmd.OutOrStdout())
	case "chtimes":
		if len(argv) < 5 {
			return fmt.Errorf("usage: xfs chtimes <atime-unixsec> <mtime-unixsec> <path>")
		}
		return XfsChtimesOnImage(file, partIndex, argv[2], argv[3], argv[4], cmd.OutOrStdout())
	case "link":
		if len(argv) < 4 {
			return fmt.Errorf("usage: xfs link <oldPath> <newPath>")
		}
		return XfsLinkOnImage(file, partIndex, argv[2], argv[3], cmd.OutOrStdout())
	case "symlink":
		if len(argv) < 4 {
			return fmt.Errorf("usage: xfs symlink <target> <linkPath>")
		}
		return XfsSymlinkOnImage(file, partIndex, argv[2], argv[3], cmd.OutOrStdout())
	case "truncate":
		if len(argv) < 4 {
			return fmt.Errorf("usage: xfs truncate <size> <path>")
		}
		return XfsTruncateOnImage(file, partIndex, argv[2], argv[3], cmd.OutOrStdout())
	case "extended-stat", "stat":
		if len(argv) < 3 {
			return fmt.Errorf("usage: xfs extended-stat <path>")
		}
		return XfsExtendedStatOnImage(file, partIndex, argv[2], cmd.OutOrStdout())
	case "label":
		if len(argv) < 3 || argv[2] == "get" {
			return XfsGetLabelOnImage(file, partIndex, cmd.OutOrStdout())
		}
		if argv[2] == "set" {
			if len(argv) < 4 {
				return fmt.Errorf("usage: xfs label set <new-label>")
			}
			return XfsSetLabelOnImage(file, partIndex, argv[3], cmd.OutOrStdout())
		}
		return fmt.Errorf("usage: xfs label [get | set <new-label>]")
	}
	if argv[1] == "grow" {
		if fsType, err := diskimage.DetectFilesystem(file, partIndex); err != nil {
			return fmt.Errorf("detect filesystem for 'xfs grow': %w", err)
		} else if fsType != diskimage.FSXfs {
			return fmt.Errorf("'xfs grow' requires an XFS image (detected: %s)", fsType)
		}
		// Parse size from argv (supports --size=SIZE, --size SIZE, -sSIZE, -s SIZE, or positional size).
		sizeStr := ""
		for i := 2; i < len(argv); i++ {
			a := argv[i]
			if strings.HasPrefix(a, "--") {
				if strings.HasPrefix(a, "--size=") {
					sizeStr = strings.SplitN(a, "=", 2)[1]
				} else if a == "--size" && i+1 < len(argv) {
					sizeStr = argv[i+1]
					i++
				}
				continue
			}
			if strings.HasPrefix(a, "-") {
				// support -sVALUE or -s VALUE
				if len(a) > 2 && a[1] == 's' {
					sizeStr = a[2:]
					continue
				}
				if a == "-s" && i+1 < len(argv) {
					sizeStr = argv[i+1]
					i++
					continue
				}
				continue
			}
			if sizeStr == "" {
				sizeStr = a
			}
		}
		if sizeStr == "" {
			return fmt.Errorf("xfs grow requires a --size argument")
		}
		return XfsGrowFromImage(file, partIndex, sizeStr, cmd.OutOrStdout())
	}
	return fmt.Errorf("unsupported xfs subcommand")
}

func handleZfs(cmd *cobra.Command, argv []string, file string, partIndex int) error {
	if len(argv) >= 2 {
		switch argv[1] {
		case "list":
			if file == "" {
				return fmt.Errorf("--file is required for 'zfs list' on image")
			}
			if fsType, err := diskimage.DetectFilesystem(file, partIndex); err != nil {
				return fmt.Errorf("detect filesystem for 'zfs list': %w", err)
			} else if fsType != diskimage.FSZfs {
				return fmt.Errorf("'zfs list' requires a ZFS image (detected: %s)", fsType)
			}
			return ZfsListFromImage(file, partIndex, cmd.OutOrStdout())
		case "create":
			if file == "" {
				return fmt.Errorf("--file is required for 'zfs create' on image")
			}
			if fsType, err := diskimage.DetectFilesystem(file, partIndex); err != nil {
				return fmt.Errorf("detect filesystem for 'zfs create': %w", err)
			} else if fsType != diskimage.FSZfs {
				return fmt.Errorf("'zfs create' requires a ZFS image (detected: %s)", fsType)
			}
			if len(argv) < 3 {
				return fmt.Errorf("zfs create requires a dataset name")
			}
			dataset := argv[2]
			return ZfsCreateFromImage(file, partIndex, dataset, cmd.OutOrStdout())
		case "get":
			if file == "" {
				return fmt.Errorf("--file is required for 'zfs get' on image")
			}
			if fsType, err := diskimage.DetectFilesystem(file, partIndex); err != nil {
				return fmt.Errorf("detect filesystem for 'zfs get': %w", err)
			} else if fsType != diskimage.FSZfs {
				return fmt.Errorf("'zfs get' requires a ZFS image (detected: %s)", fsType)
			}
			if len(argv) < 4 || argv[2] != "quota" {
				return fmt.Errorf("zfs get quota requires a dataset name")
			}
			dataset := argv[3]
			return ZfsGetQuotaFromImage(file, partIndex, dataset, cmd.OutOrStdout())
		case "set":
			if file == "" {
				return fmt.Errorf("--file is required for 'zfs set' on image")
			}
			if fsType, err := diskimage.DetectFilesystem(file, partIndex); err != nil {
				return fmt.Errorf("detect filesystem for 'zfs set': %w", err)
			} else if fsType != diskimage.FSZfs {
				return fmt.Errorf("'zfs set' requires a ZFS image (detected: %s)", fsType)
			}
			// Support both 'zfs set quota=VALUE DATASET' and 'zfs set quota DATASET VALUE'
			if len(argv) < 3 {
				return fmt.Errorf("zfs set requires arguments")
			}
			if strings.Contains(argv[2], "=") {
				parts := strings.SplitN(argv[2], "=", 2)
				if parts[0] != "quota" {
					return fmt.Errorf("unsupported zfs property: %s", parts[0])
				}
				if len(argv) < 4 {
					return fmt.Errorf("zfs set %s requires a dataset", argv[2])
				}
				dataset := argv[3]
				quota := parts[1]
				return ZfsSetQuotaFromImage(file, partIndex, dataset, quota, cmd.OutOrStdout())
			}
			if argv[2] != "quota" {
				return fmt.Errorf("unsupported zfs property: %s", argv[2])
			}
			if len(argv) < 5 {
				return fmt.Errorf("zfs set quota requires a dataset and value")
			}
			dataset := argv[3]
			quota := argv[4]
			return ZfsSetQuotaFromImage(file, partIndex, dataset, quota, cmd.OutOrStdout())
		}
	}
	return fmt.Errorf("unsupported zfs subcommand")
}

func handlePart(cmd *cobra.Command, argv []string, file string) error {
	if len(argv) >= 2 && argv[1] == "print" {
		if file == "" {
			return fmt.Errorf("--file is required for 'part print'")
		}
		return PartPrintFromImage(file, cmd.OutOrStdout())
	}
	return fmt.Errorf("unsupported part subcommand")
}

func handleDF(cmd *cobra.Command, argv []string, file, filesystemArg string, partIndex int) error {
	human := false
	for _, a := range argv[1:] {
		if a == "-h" {
			human = true
		}
	}
	if file == "" {
		return fmt.Errorf("--file is required for 'df'")
	}
	return DFFromImage(file, filesystemArg, partIndex, cmd.OutOrStdout(), human)
}

func handleDU(cmd *cobra.Command, argv []string, file, filesystemArg string, partIndex int) error {
	human := false
	summarize := false
	pathArg := "/"
	for _, a := range argv[1:] {
		if strings.HasPrefix(a, "-") {
			for _, c := range a[1:] {
				switch c {
				case 'h':
					human = true
				case 's':
					summarize = true
				}
			}
		} else {
			pathArg = a
		}
	}
	if file == "" {
		return fmt.Errorf("--file is required for 'du'")
	}
	if !summarize {
		summarize = true
	}
	if summarize {
		return DUSummaryFromImage(file, filesystemArg, partIndex, pathArg, cmd.OutOrStdout(), human)
	}
	return fmt.Errorf("unsupported du flags")
}

func handleCat(cmd *cobra.Command, argv []string, file, filesystemArg string, partIndex int) error {
	if file == "" {
		return fmt.Errorf("--file is required for 'cat'")
	}
	if len(argv) < 2 {
		return fmt.Errorf("cat requires a file path argument")
	}
	filePathArg := argv[1]
	opts := diskimage.FileOptions{
		Path:       file,
		Filesystem: diskimage.FilesystemType(strings.ToLower(filesystemArg)),
		PartIndex:  partIndex,
		FilePath:   filePathArg,
	}
	data, err := diskimage.ReadFile(opts)
	if err != nil {
		return err
	}
	_, _ = cmd.OutOrStdout().Write(data)
	return nil
}
