//go:build !windows

package remote

import (
	"errors"
	"syscall"
)

// processAlive tells whether the process exists: signal 0 checks without
// sending anything. EPERM means it exists but belongs to another user.
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
