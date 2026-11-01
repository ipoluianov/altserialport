package mcp

import (
	"errors"
	"sync"
	"time"

	"github.com/ipoluianov/altserialport/remote"
	"github.com/ipoluianov/altserialport/serial"
)

// maxBuffer is the most received bytes kept for read and wait_for; the
// oldest are dropped
const maxBuffer = 1 << 20

// reconnectInterval is how often a gone device is looked for
const reconnectInterval = 250 * time.Millisecond

// port is a port opened by the agent. It reads in a goroutine of its own
// into a buffer that read and wait_for consume. When the device goes away
// (a reset, a re-flash) it waits for it and opens it again.
type port struct {
	name string

	mu       sync.Mutex
	cfg      serial.Config
	p        serial.Port // nil while the device is gone
	attached bool        // p works through a window of AltSerialPort
	state    string      // "open", "waiting", "closed"
	lastErr  error
	buf      []byte
	dropped  int64 // bytes dropped from a full buffer
	rx, tx   int64
	// lines set by the agent, applied again when the device comes back;
	// nil when not set
	dtr, rts *bool
	// changed is closed and made again whenever data comes or the state
	// changes
	changed chan struct{}
	stop    chan struct{}
}

func openPort(name string, cfg serial.Config) (*port, error) {
	p, attached, err := remote.Open(name, cfg)
	if err != nil {
		return nil, err
	}
	pt := &port{name: name, cfg: cfg, p: p, attached: attached, state: "open",
		changed: make(chan struct{}), stop: make(chan struct{})}
	go pt.reader(p)
	return pt, nil
}

// signal wakes up the waiting calls; mu is held
func (pt *port) signal() {
	close(pt.changed)
	pt.changed = make(chan struct{})
}

func (pt *port) reader(p serial.Port) {
	buf := make([]byte, 16*1024)
	for {
		n, err := p.Read(buf)
		if n > 0 {
			pt.mu.Lock()
			pt.rx += int64(n)
			pt.buf = append(pt.buf, buf[:n]...)
			if over := len(pt.buf) - maxBuffer; over > 0 {
				pt.dropped += int64(over)
				pt.buf = append(pt.buf[:0], pt.buf[over:]...)
			}
			pt.signal()
			pt.mu.Unlock()
		}
		if err == nil {
			continue
		}
		pt.mu.Lock()
		if pt.state == "closed" {
			pt.mu.Unlock()
			return
		}
		if errors.Is(err, serial.ErrClosed) && pt.p != p {
			pt.mu.Unlock()
			return
		}
		pt.state = "waiting"
		pt.p = nil
		pt.lastErr = err
		pt.signal()
		pt.mu.Unlock()
		p.Close()
		p = pt.reconnect()
		if p == nil {
			return
		}
	}
}

// reconnect opens the port again once it can; nil when the agent closed it
func (pt *port) reconnect() serial.Port {
	t := time.NewTicker(reconnectInterval)
	defer t.Stop()
	for {
		select {
		case <-pt.stop:
			return nil
		case <-t.C:
		}
		pt.mu.Lock()
		cfg := pt.cfg
		pt.mu.Unlock()
		p, attached, err := remote.Open(pt.name, cfg)
		if err != nil {
			pt.mu.Lock()
			pt.lastErr = err
			pt.mu.Unlock()
			continue
		}
		pt.mu.Lock()
		if pt.state == "closed" {
			pt.mu.Unlock()
			p.Close()
			return nil
		}
		if !attached {
			if pt.dtr != nil {
				p.SetDTR(*pt.dtr)
			}
			if pt.rts != nil {
				p.SetRTS(*pt.rts)
			}
		}
		pt.p, pt.attached, pt.state, pt.lastErr = p, attached, "open", nil
		pt.signal()
		pt.mu.Unlock()
		return p
	}
}

func (pt *port) close() {
	pt.mu.Lock()
	if pt.state == "closed" {
		pt.mu.Unlock()
		return
	}
	p := pt.p
	pt.state = "closed"
	pt.p = nil
	close(pt.stop)
	pt.signal()
	pt.mu.Unlock()
	if p != nil {
		p.Close()
	}
}

var errWaiting = errors.New("the device is gone (reset or unplugged); waiting for it to come back. Try again in a moment")

// current returns the open port, or an error that says why there is none
func (pt *port) current() (serial.Port, error) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	switch pt.state {
	case "closed":
		return nil, errors.New("the port is closed")
	case "waiting":
		return nil, errWaiting
	}
	return pt.p, nil
}

func (pt *port) write(data []byte) error {
	p, err := pt.current()
	if err != nil {
		return err
	}
	n, err := p.Write(data)
	pt.mu.Lock()
	pt.tx += int64(n)
	pt.mu.Unlock()
	return err
}

func (pt *port) setDTR(on bool) error {
	p, err := pt.current()
	if err != nil {
		return err
	}
	pt.mu.Lock()
	pt.dtr = &on
	pt.mu.Unlock()
	return p.SetDTR(on)
}

func (pt *port) setRTS(on bool) error {
	p, err := pt.current()
	if err != nil {
		return err
	}
	pt.mu.Lock()
	pt.rts = &on
	pt.mu.Unlock()
	return p.SetRTS(on)
}

// take removes up to max bytes from the buffer
func (pt *port) take(max int) []byte {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	n := min(max, len(pt.buf))
	out := append([]byte(nil), pt.buf[:n]...)
	pt.buf = append(pt.buf[:0], pt.buf[n:]...)
	return out
}

// snapshot returns the buffer, the channel closed on the next change and
// whether the port is still open for the agent
func (pt *port) snapshot() ([]byte, <-chan struct{}, bool) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	return pt.buf, pt.changed, pt.state != "closed"
}

// read waits for data up to timeout; once data has come it returns after
// a quiet gap, at max bytes or at the timeout
func (pt *port) read(timeout time.Duration, max int) []byte {
	const quiet = 100 * time.Millisecond
	deadline := time.Now().Add(timeout)
	last := -1
	var lastAt time.Time
	for {
		buf, changed, open := pt.snapshot()
		n := len(buf)
		if n != last {
			last, lastAt = n, time.Now()
		}
		now := time.Now()
		if n >= max || !open || !now.Before(deadline) || n > 0 && now.Sub(lastAt) >= quiet {
			return pt.take(max)
		}
		wait := deadline.Sub(now)
		if n > 0 {
			wait = min(wait, quiet-now.Sub(lastAt))
		}
		t := time.NewTimer(wait)
		select {
		case <-changed:
		case <-t.C:
		}
		t.Stop()
	}
}

// waitFor waits until match finds something in the buffer; the bytes up
// to the end of the match are taken. On timeout nothing is taken and what
// is in the buffer is returned with ok false.
func (pt *port) waitFor(match func([]byte) int, timeout time.Duration) (data []byte, ok bool) {
	deadline := time.Now().Add(timeout)
	for {
		pt.mu.Lock()
		changed, open := pt.changed, pt.state != "closed"
		if end := match(pt.buf); end >= 0 {
			out := append([]byte(nil), pt.buf[:end]...)
			pt.buf = append(pt.buf[:0], pt.buf[end:]...)
			pt.mu.Unlock()
			return out, true
		}
		pt.mu.Unlock()
		now := time.Now()
		if !open || !now.Before(deadline) {
			pt.mu.Lock()
			out := append([]byte(nil), pt.buf...)
			pt.mu.Unlock()
			return out, false
		}
		t := time.NewTimer(deadline.Sub(now))
		select {
		case <-changed:
		case <-t.C:
		}
		t.Stop()
	}
}

// status describes the port for the agent
func (pt *port) status() map[string]any {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	st := map[string]any{
		"port":            pt.name,
		"state":           pt.state,
		"baud":            pt.cfg.Baud,
		"frame":           pt.cfg.Frame(),
		"flow":            flowName(pt.cfg.Flow),
		"attached_window": pt.attached,
		"buffered_bytes":  len(pt.buf),
		"rx_bytes":        pt.rx,
		"tx_bytes":        pt.tx,
	}
	if pt.dropped > 0 {
		st["dropped_bytes"] = pt.dropped
	}
	if pt.state == "waiting" && pt.lastErr != nil {
		st["last_error"] = pt.lastErr.Error()
	}
	return st
}
