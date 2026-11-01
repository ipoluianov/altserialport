//go:build !windows

package install

import "errors"

// errUnsupported: on Linux and macOS the program is installed by the
// packages
var errUnsupported = errors.New("install: only the Windows build installs itself")

// Status returns Unsupported: the program is installed by the packages
func Status() State { return Unsupported }

// Install does nothing here
func Install() error { return errUnsupported }

// Uninstall does nothing here
func Uninstall(quiet bool) error { return errUnsupported }

// HandleArgs handles nothing here
func HandleArgs(args []string) (handled bool) { return false }

// StartInstalled does nothing here
func StartInstalled() error { return errUnsupported }
