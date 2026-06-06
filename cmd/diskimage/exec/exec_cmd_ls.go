package exec

import (
	"fmt"
	"io"
	"strings"

	"github.com/go-diskimages/diskimage"
	"github.com/spf13/cobra"
)

// lsExecute performs the listing after all options have been resolved.
func lsExecute(cmd *cobra.Command, file, filesystemArg string, partIndex int, dirPath string, showAll, long, classify, human bool) error {
	if file == "" {
		return fmt.Errorf("--file is required for 'ls'")
	}
	entries, err := fetchEntries(file, filesystemArg, partIndex, dirPath, long || classify)
	if err != nil {
		return err
	}
	if !showAll {
		entries = filterHidden(entries)
	}
	if len(entries) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "(empty directory)")
		return nil
	}
	if long {
		printLongWrite(cmd.OutOrStdout(), entries, human, classify)
	} else {
		printShortWrite(cmd.OutOrStdout(), entries, classify)
	}
	return nil
}

// fetchEntries resolves entries from the injected listFunc or testEntries stub.
func fetchEntries(file, filesystemArg string, partIndex int, dirPath string, fetchStat bool) ([]diskimage.ListEntry, error) {
	if testEntries != nil {
		return testEntries, nil
	}
	return listFunc(diskimage.ListOptions{
		Path:       file,
		Filesystem: diskimage.FilesystemType(strings.ToLower(filesystemArg)),
		PartIndex:  partIndex,
		DirPath:    dirPath,
		FetchStat:  fetchStat,
	})
}

// handleLS is the legacy dispatch adapter called by execDispatch.
func handleLS(cmd *cobra.Command, argv []string, file, filesystemArg string, partIndex int, dirPath string) error {
	showAll, long, classify, human, dir := parseLSArgs(argv, dirPath)
	return lsExecute(cmd, file, filesystemArg, partIndex, dir, showAll, long, classify, human)
}

// parseLSArgs extracts ls flags from a raw argv slice (legacy exec dispatch).
func parseLSArgs(argv []string, defaultDir string) (showAll, long, classify, human bool, dir string) {
	dir = defaultDir
	for _, arg := range argv[1:] {
		if strings.HasPrefix(arg, "-") {
			for _, c := range arg[1:] {
				switch c {
				case 'a':
					showAll = true
				case 'l':
					long = true
				case 'F':
					classify = true
				case 'h':
					human = true
				}
			}
		} else {
			dir = arg
		}
	}
	return
}

// filterHidden removes entries whose names start with '.'.
func filterHidden(entries []diskimage.ListEntry) []diskimage.ListEntry {
	out := entries[:0]
	for _, e := range entries {
		if !strings.HasPrefix(e.Name, ".") {
			out = append(out, e)
		}
	}
	return out
}

// printShortWrite writes the short (non-long) listing to w.
func printShortWrite(w io.Writer, entries []diskimage.ListEntry, classify bool) {
	for _, e := range entries {
		fmt.Fprintln(w, e.Name+indicator(e, classify))
	}
}

// printLongWrite writes the long (-l) listing to w.
func printLongWrite(w io.Writer, entries []diskimage.ListEntry, human, classify bool) {
	for _, e := range entries {
		perm := modeString(e.Mode)
		var size string
		if human {
			size = fmt.Sprintf("%8s", humanSize(e.Size))
		} else {
			size = fmt.Sprintf("%8d", e.Size)
		}
		fmt.Fprintf(w, "%s %s %s\n", perm, size, e.Name+indicator(e, classify))
	}
}
