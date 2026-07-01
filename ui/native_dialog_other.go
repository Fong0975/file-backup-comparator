//go:build !windows

package ui

// bringNativeDialogToFront is a no-op on non-Windows platforms; the
// behind-the-main-window issue it works around is Windows-specific.
func bringNativeDialogToFront() {}
