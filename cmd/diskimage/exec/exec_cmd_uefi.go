package exec

import (
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	filesystem_uefi "github.com/go-filesystems/uefi"
	"github.com/spf13/cobra"
)

// uefiStoreIface is the subset of filesystem_uefi.VariableStore used by this
// package. Tests may inject a fake implementation via SetOpenUEFIStoreFunc.
type uefiStoreIface interface {
	Close() error
	List() []filesystem_uefi.Variable
	Get(name string, guid filesystem_uefi.GUID) (filesystem_uefi.Variable, error)
	Set(v filesystem_uefi.Variable) error
	Delete(name string, guid filesystem_uefi.GUID) error
}

// openUEFIStoreFunc opens a UEFI variable store. Tests may replace this.
var openUEFIStoreFunc = func(path string) (uefiStoreIface, error) {
	return filesystem_uefi.Open(path)
}

// SetOpenUEFIStoreFunc replaces the store-opener and returns the previous
// value. Passing nil restores the default.
func SetOpenUEFIStoreFunc(f func(string) (uefiStoreIface, error)) (prev func(string) (uefiStoreIface, error)) {
	prev = openUEFIStoreFunc
	if f != nil {
		openUEFIStoreFunc = f
	}
	return prev
}

// handleUefi dispatches the uefi sub-command from a legacy exec argv.
//
// Supported forms:
//
//	"uefi list"
//	"uefi get <name>"
//	"uefi set <name> --data <hex>"
//	"uefi delete <name>"
func handleUefi(cmd *cobra.Command, argv []string, file string) error {
	if len(argv) < 2 {
		return fmt.Errorf("uefi requires a sub-command: list, get, set, delete")
	}
	if file == "" {
		return fmt.Errorf("--file is required for 'uefi'")
	}
	switch argv[1] {
	case "list":
		return handleUefiList(cmd.OutOrStdout(), file)
	case "get":
		if len(argv) < 3 {
			return fmt.Errorf("uefi get requires a variable name")
		}
		return handleUefiGet(cmd.OutOrStdout(), file, argv[2])
	case "set":
		return handleUefiSetDispatch(cmd.OutOrStdout(), argv[2:], file)
	case "delete":
		if len(argv) < 3 {
			return fmt.Errorf("uefi delete requires a variable name")
		}
		return handleUefiDelete(cmd.OutOrStdout(), file, argv[2])
	default:
		return fmt.Errorf("unsupported uefi sub-command: %q", argv[1])
	}
}

// handleUefiSetDispatch parses args after "uefi set" and calls handleUefiSet.
// Supports: <name> --data <hex>  and  <name> --data=<hex>
func handleUefiSetDispatch(w io.Writer, args []string, file string) error {
	if len(args) == 0 {
		return fmt.Errorf("uefi set requires a variable name")
	}
	name := args[0]
	dataHex := ""
	for i := 1; i < len(args); i++ {
		switch {
		case strings.HasPrefix(args[i], "--data="):
			dataHex = strings.TrimPrefix(args[i], "--data=")
		case args[i] == "--data" && i+1 < len(args):
			dataHex = args[i+1]
			i++
		}
	}
	return handleUefiSet(w, file, name, dataHex)
}

// handleUefiList lists all variables in the store.
func handleUefiList(w io.Writer, file string) error {
	s, err := openUEFIStoreFunc(file)
	if err != nil {
		return fmt.Errorf("uefi: open %q: %w", file, err)
	}
	defer s.Close() //nolint:errcheck
	vars := s.List()
	if len(vars) == 0 {
		fmt.Fprintln(w, "(no variables)")
		return nil
	}
	fmt.Fprintf(w, "%-40s  %-8s  %s\n", "Name", "Attrs", "Size")
	for _, v := range vars {
		fmt.Fprintf(w, "%-40s  %-8s  %d\n", v.Name, formatUEFIAttrs(v.Attributes), len(v.Data))
	}
	return nil
}

// handleUefiGet prints a variable's data as a hex string.
func handleUefiGet(w io.Writer, file, name string) error {
	s, err := openUEFIStoreFunc(file)
	if err != nil {
		return fmt.Errorf("uefi: open %q: %w", file, err)
	}
	defer s.Close() //nolint:errcheck
	v, err := s.Get(name, filesystem_uefi.DefaultNamespaceGUID)
	if err != nil {
		return fmt.Errorf("uefi: get %q: %w", name, err)
	}
	fmt.Fprintln(w, hex.EncodeToString(v.Data))
	return nil
}

// handleUefiSet creates or replaces a variable from a hex string.
func handleUefiSet(w io.Writer, file, name, dataHex string) error {
	data, err := hex.DecodeString(dataHex)
	if err != nil {
		return fmt.Errorf("uefi: invalid hex data %q: %w", dataHex, err)
	}
	s, err := openUEFIStoreFunc(file)
	if err != nil {
		return fmt.Errorf("uefi: open %q: %w", file, err)
	}
	defer s.Close() //nolint:errcheck
	if err := s.Set(filesystem_uefi.Variable{
		Name:       name,
		GUID:       filesystem_uefi.DefaultNamespaceGUID,
		Attributes: filesystem_uefi.AttrNonVolatile | filesystem_uefi.AttrBootServiceAccess | filesystem_uefi.AttrRuntimeAccess,
		Data:       data,
	}); err != nil {
		return fmt.Errorf("uefi: set %q: %w", name, err)
	}
	fmt.Fprintf(w, "set %s (%d bytes)\n", name, len(data))
	return nil
}

// handleUefiDelete removes a variable from the store.
func handleUefiDelete(w io.Writer, file, name string) error {
	s, err := openUEFIStoreFunc(file)
	if err != nil {
		return fmt.Errorf("uefi: open %q: %w", file, err)
	}
	defer s.Close() //nolint:errcheck
	if err := s.Delete(name, filesystem_uefi.DefaultNamespaceGUID); err != nil {
		return fmt.Errorf("uefi: delete %q: %w", name, err)
	}
	fmt.Fprintf(w, "deleted %s\n", name)
	return nil
}

// formatUEFIAttrs returns a compact human-readable representation of UEFI
// attribute flags (e.g. "NV+BS+RT").
func formatUEFIAttrs(a filesystem_uefi.Attributes) string {
	var parts []string
	if a&filesystem_uefi.AttrNonVolatile != 0 {
		parts = append(parts, "NV")
	}
	if a&filesystem_uefi.AttrBootServiceAccess != 0 {
		parts = append(parts, "BS")
	}
	if a&filesystem_uefi.AttrRuntimeAccess != 0 {
		parts = append(parts, "RT")
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, "+")
}
