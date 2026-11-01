package xmodem

import "time"

// ChanLink makes a Link of a write function and the bytes fed to it from
// another goroutine (the reader of the port calls Feed)
type ChanLink struct {
	write func([]byte) error
	in    chan byte
}

// chanLinkSize is how many bytes wait to be read before Feed drops them
const chanLinkSize = 1 << 20

// NewChanLink makes a link that writes with write
func NewChanLink(write func([]byte) error) *ChanLink {
	return &ChanLink{write: write, in: make(chan byte, chanLinkSize)}
}

// Write sends the bytes to the port
func (l *ChanLink) Write(p []byte) error {
	return l.write(p)
}

// Feed gives the link bytes that came from the port. It never blocks: when
// the buffer is full, the rest is dropped.
func (l *ChanLink) Feed(data []byte) {
	for _, b := range data {
		select {
		case l.in <- b:
		default:
			return
		}
	}
}

// ReadByteTimeout waits for a byte up to the timeout
func (l *ChanLink) ReadByteTimeout(timeout time.Duration) (byte, error) {
	select {
	case b := <-l.in:
		return b, nil
	default:
	}
	if timeout <= 0 {
		return 0, ErrTimeout
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case b := <-l.in:
		return b, nil
	case <-t.C:
		return 0, ErrTimeout
	}
}
