//go:build gumworkflowe2e

package main

// The tagged binary is used only by tests/e2e to exercise the interactive run
// pipeline without adding a PTY dependency. Production builds use the real
// terminal check in terminal_mode.go.
func commandStdinIsInteractive() bool { return true }
