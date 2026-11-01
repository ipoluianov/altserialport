//go:build linux || darwin

package cli

import (
	"os"

	"golang.org/x/sys/unix"
)

// makeRaw puts the terminal of f in the raw mode: every key comes at once,
// nothing is echoed, Ctrl+C is a byte for the device. The output keeps
// its processing, so a bare LF from the device starts a new line.
// restore puts the old mode back.
func makeRaw(f *os.File) (restore func(), err error) {
	fd := int(f.Fd())
	old, err := unix.IoctlGetTermios(fd, ioctlGetTermios)
	if err != nil {
		return nil, err // not a terminal
	}
	t := *old
	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag &^= unix.CSIZE | unix.PARENB
	t.Cflag |= unix.CS8
	t.Cc[unix.VMIN] = 1
	t.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, ioctlSetTermios, &t); err != nil {
		return nil, err
	}
	return func() { unix.IoctlSetTermios(fd, ioctlSetTermios, old) }, nil
}
