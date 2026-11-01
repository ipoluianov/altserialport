package cli

import (
	"os"

	"golang.org/x/sys/windows"
)

// makeRaw puts the console in the raw mode: every key comes at once as VT
// sequences, nothing is echoed, Ctrl+C is a byte for the device; the
// output understands the VT sequences of the device. restore puts the old
// modes back.
func makeRaw(f *os.File) (restore func(), err error) {
	in := windows.Handle(f.Fd())
	var oldIn uint32
	if err := windows.GetConsoleMode(in, &oldIn); err != nil {
		return nil, err // not a console
	}
	mode := oldIn &^ (windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT | windows.ENABLE_PROCESSED_INPUT)
	mode |= windows.ENABLE_VIRTUAL_TERMINAL_INPUT
	if err := windows.SetConsoleMode(in, mode); err != nil {
		return nil, err
	}
	out := windows.Handle(os.Stdout.Fd())
	var oldOut uint32
	outOK := windows.GetConsoleMode(out, &oldOut) == nil
	if outOK {
		windows.SetConsoleMode(out, oldOut|windows.ENABLE_PROCESSED_OUTPUT|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	}
	return func() {
		windows.SetConsoleMode(in, oldIn)
		if outOK {
			windows.SetConsoleMode(out, oldOut)
		}
	}, nil
}
