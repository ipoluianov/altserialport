package remote

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/ipoluianov/altserialport/serial"
)

// dialTimeout is how long connecting and the hello may take
const dialTimeout = 3 * time.Second

// replyTimeout is how long a request waits for its reply
const replyTimeout = 5 * time.Second

// maxPending is the most received bytes kept for Read; the oldest are
// dropped when the reader of the port is this far behind
const maxPending = 16 << 20

// writePiece is the most bytes sent in one write request
const writePiece = 64 << 10

// Info is what the window tells about its port when a client attaches
type Info struct {
	Port  string // the path the window opened
	Short string
	Baud  int
	Frame string
}

// Port is a serial.Port that works through the session of a window
type Port struct {
	conn net.Conn
	info Info

	reqMu   sync.Mutex // one request at a time: the replies come in order
	replies chan []byte

	mu      sync.Mutex
	cond    *sync.Cond
	pending []byte
	err     error // set once: ErrGone or ErrClosed
	closed  bool
}

// Dial attaches to the port of the window of the entry
func Dial(e Entry) (serial.Port, error) {
	return DialPort(e)
}

// DialPort is Dial that returns the concrete type, with the Info
func DialPort(e Entry) (*Port, error) {
	conn, err := net.DialTimeout("tcp", e.Addr, dialTimeout)
	if err != nil {
		return nil, serial.ErrGone
	}
	conn.SetDeadline(time.Now().Add(dialTimeout))
	if err := writeFrame(conn, fHello, []byte(e.Token)); err != nil {
		conn.Close()
		return nil, serial.ErrGone
	}
	typ, payload, err := readFrame(conn)
	if err != nil || typ != fWelcome {
		conn.Close()
		// A wrong token is dropped silently; a closed port says gone
		return nil, serial.ErrGone
	}
	var w welcome
	json.Unmarshal(payload, &w)
	conn.SetDeadline(time.Time{})
	p := &Port{
		conn:    conn,
		info:    Info{Port: w.Port, Short: w.Short, Baud: w.Baud, Frame: w.Frame},
		replies: make(chan []byte, 1),
	}
	p.cond = sync.NewCond(&p.mu)
	go p.reader()
	return p, nil
}

// Info returns what the window told about its port
func (p *Port) Info() Info { return p.info }

func (p *Port) reader() {
	for {
		typ, payload, err := readFrame(p.conn)
		if err != nil {
			p.fail(serial.ErrGone)
			return
		}
		switch typ {
		case fData:
			p.mu.Lock()
			p.pending = append(p.pending, payload...)
			if over := len(p.pending) - maxPending; over > 0 {
				p.pending = append(p.pending[:0], p.pending[over:]...)
			}
			p.mu.Unlock()
			p.cond.Broadcast()
		case fReply:
			select {
			case p.replies <- payload:
			default: // nobody waits for it any more
			}
		case fGone:
			p.fail(serial.ErrGone)
			p.conn.Close()
			return
		}
	}
}

// fail ends the port with err, unless it has ended already
func (p *Port) fail(err error) {
	p.mu.Lock()
	if p.err == nil {
		p.err = err
	}
	p.mu.Unlock()
	p.cond.Broadcast()
}

// Read returns the received bytes; ErrGone when the window closed the
// port or the connection broke, ErrClosed after Close
func (p *Port) Read(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for len(p.pending) == 0 && p.err == nil {
		p.cond.Wait()
	}
	if p.closed {
		return 0, serial.ErrClosed
	}
	if len(p.pending) > 0 {
		n := copy(b, p.pending)
		p.pending = p.pending[n:]
		if len(p.pending) == 0 {
			p.pending = nil
		}
		return n, nil
	}
	return 0, p.err
}

// request sends a frame and waits for its reply
func (p *Port) request(typ byte, payload []byte) ([]byte, error) {
	p.reqMu.Lock()
	defer p.reqMu.Unlock()
	p.mu.Lock()
	err := p.err
	p.mu.Unlock()
	if err != nil {
		return nil, err
	}
	// A reply left from a request that timed out is not ours
	select {
	case <-p.replies:
	default:
	}
	p.conn.SetWriteDeadline(time.Now().Add(replyTimeout))
	if err := writeFrame(p.conn, typ, payload); err != nil {
		p.fail(serial.ErrGone)
		return nil, p.ended()
	}
	t := time.NewTimer(replyTimeout)
	defer t.Stop()
	check := time.NewTicker(50 * time.Millisecond)
	defer check.Stop()
	for {
		select {
		case r := <-p.replies:
			if len(r) == 0 {
				return nil, errors.New("empty reply")
			}
			switch r[0] {
			case rOK:
				return r[1:], nil
			case rGone:
				return nil, serial.ErrGone
			}
			return nil, errors.New(string(r[1:]))
		case <-t.C:
			return nil, errors.New("the window does not answer")
		case <-check.C:
			if err := p.ended(); err != nil {
				return nil, err
			}
		}
	}
}

func (p *Port) ended() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

// Write sends the bytes through the window, which shows them as sent
func (p *Port) Write(b []byte) (int, error) {
	done := 0
	for done < len(b) {
		n := min(len(b)-done, writePiece)
		if _, err := p.request(fWrite, b[done:done+n]); err != nil {
			return done, err
		}
		done += n
	}
	return done, nil
}

// Close detaches from the window; the port of the window stays open
func (p *Port) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.err = serial.ErrClosed
	p.mu.Unlock()
	p.cond.Broadcast()
	return p.conn.Close()
}

// SetConfig does nothing: the settings are the window's
func (p *Port) SetConfig(c serial.Config) error { return nil }

func (p *Port) SetDTR(on bool) error {
	_, err := p.request(fDTR, []byte{boolByte(on)})
	return err
}

func (p *Port) SetRTS(on bool) error {
	_, err := p.request(fRTS, []byte{boolByte(on)})
	return err
}

func (p *Port) Signals() (serial.Signals, error) {
	r, err := p.request(fSignals, nil)
	if err != nil {
		return serial.Signals{}, err
	}
	if len(r) < 1 {
		return serial.Signals{}, errors.New("bad reply")
	}
	b := r[0]
	return serial.Signals{CTS: b&sigCTS != 0, DSR: b&sigDSR != 0, DCD: b&sigDCD != 0, RI: b&sigRI != 0}, nil
}

// Break asks the window for a break of d and waits as long as it lasts
func (p *Port) Break(d time.Duration) error {
	var ms [4]byte
	binary.BigEndian.PutUint32(ms[:], uint32(d/time.Millisecond))
	if _, err := p.request(fBreak, ms[:]); err != nil {
		return err
	}
	time.Sleep(d)
	return nil
}

func boolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

// Open opens the port, or attaches to the window that has it. attached
// tells which: the settings of an attached port are the window's, and cfg
// is not applied. While a window waits for its device to come back, the
// port is the window's and Open returns serial.ErrBusy.
func Open(name string, cfg serial.Config) (p serial.Port, attached bool, err error) {
	if rp, ok, aerr := attach(name); ok {
		if aerr != nil {
			return nil, false, aerr
		}
		return rp, true, nil
	}
	p, err = serial.Open(name, cfg)
	if err == nil {
		return p, false, nil
	}
	// Windows says "access denied" about a port taken by another process
	if errors.Is(err, serial.ErrBusy) || errors.Is(err, serial.ErrPermission) {
		// The window may have opened it a moment ago
		if rp, ok, aerr := attach(name); ok && aerr == nil {
			return rp, true, nil
		}
	}
	return nil, false, err
}

// attach dials the window that has the port; ok is false when no window
// has it
func attach(name string) (serial.Port, bool, error) {
	e, ok := Find(name)
	if !ok {
		return nil, false, nil
	}
	if e.State != "open" {
		return nil, true, serial.ErrBusy
	}
	rp, err := DialPort(e)
	if err != nil {
		// The window has just closed the port: it may be free now
		return nil, false, nil
	}
	return rp, true, nil
}
