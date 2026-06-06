package exec

import "github.com/go-diskimages/diskimage"

// listFunc is the function used to list directories. Tests may replace this
// to avoid opening real disk images.
var listFunc = diskimage.List

// defaultListFunc holds the original implementation so tests can restore it.
var defaultListFunc = diskimage.List

// SetListFunc replaces the list function used by the exec command. Passing
// nil restores the default implementation.
func SetListFunc(f func(diskimage.ListOptions) ([]diskimage.ListEntry, error)) {
	if f == nil {
		listFunc = defaultListFunc
	} else {
		listFunc = f
	}
}

// ResetListFunc restores the list function to the package default.
func ResetListFunc() { SetListFunc(nil) }

// testEntries, when non-nil, short-circuits the real listing logic and is used
// by tests to avoid touching disk images.
var testEntries []diskimage.ListEntry
