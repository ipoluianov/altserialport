// Package session keeps a port open for a window: it reads the port in a
// goroutine of its own, writes from a queue, logs to a file, and when the
// device goes away (a reset, a re-flash, a cable) waits for it and opens it
// again. The data goes to the window as chunks of the buffer package.
package session

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ipoluianov/altserialport/buffer"
	"github.com/ipoluianov/altserialport/serial"
)

// State of a session
type State int

const (
	Closed  State = iota
	Open          // the port is open
	Waiting       // the device is gone; the session waits for it
)

// NoteKind is the kind of a note the session adds to the data
type NoteKind int

const (
	NoteConnected NoteKind = iota
	NoteDisconnected
	NoteLost
	NoteReconnected
	NoteSettings
)

// Note is an event of the session, shown among the data
type Note struct {
	Kind    NoteKind
	Port    serial.PortInfo
	Config  serial.Config
	Elapsed time.Duration // NoteReconnected: how long the device was away
}

// DefaultNoteText writes a note in English
func DefaultNoteText(n Note) string {
	switch n.Kind {
	case NoteConnected:
		return fmt.Sprintf("Connected to %s at %d %s", n.Port.Short, n.Config.Baud, n.Config.Frame())
	case NoteDisconnected:
		return "Disconnected"
	case NoteLost:
		return "Device is gone. Waiting for it to come back..."
	case NoteReconnected:
		return fmt.Sprintf("Reconnected to %s after %.1f s", n.Port.Short, n.Elapsed.Seconds())
	case NoteSettings:
		return fmt.Sprintf("Settings changed: %d %s", n.Config.Baud, n.Config.Frame())
	}
	return ""
}

// ReconnectInterval is how often a lost device is looked for
const ReconnectInterval = 250 * time.Millisecond

// Session is the port of a window
type Session struct {
	// Notify is called from the goroutines of the session when there are
	// new chunks or the state changed; the window then calls Drain
	Notify func()
	// NoteText writes the notes in the language of the window
	NoteText func(n Note) string

	mu       sync.Mutex
	state    State
	port     serial.Port
	info     serial.PortInfo
	cfg      serial.Config
	dtr, rts bool
	gen      int // the number of the port opened; the goroutines of an older one stop
	pending  []buffer.Chunk
	lastErr  error
	lostAt   time.Time
	openedAt time.Time
	writes   chan []byte
	stop     chan struct{} // closed when the user closes the session

	log *Logger

	taps   map[int]func(c buffer.Chunk)
	nextID int
	// hidden: the chunks are not shown in the window (a file transfer)
	hidden atomic.Bool

	rx, tx atomic.Int64
	// reconnects counts the times the device came back
	reconnects atomic.Int64
}

// New makes a closed session
func New() *Session {
	return &Session{dtr: true, rts: true, NoteText: DefaultNoteText}
}

func (s *Session) notify() {
	if s.Notify != nil {
		s.Notify()
	}
}

// add puts a chunk in the queue of the window and in the log, and gives
// it to the taps
func (s *Session) add(c buffer.Chunk) {
	s.mu.Lock()
	if !s.hidden.Load() || c.Dir == buffer.Info {
		s.pending = append(s.pending, c)
	}
	log := s.log
	var taps []func(buffer.Chunk)
	for _, f := range s.taps {
		taps = append(taps, f)
	}
	s.mu.Unlock()
	if log != nil {
		log.Write(c)
	}
	for _, f := range taps {
		f(c)
	}
	s.notify()
}

// AddTap gives every chunk of the session to f from now on: received,
// sent, notes. f is called from the goroutine that made the chunk (the
// reader of the port for RX) and must not block. remove stops it.
func (s *Session) AddTap(f func(c buffer.Chunk)) (remove func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.taps == nil {
		s.taps = map[int]func(buffer.Chunk){}
	}
	s.nextID++
	id := s.nextID
	s.taps[id] = f
	return func() {
		s.mu.Lock()
		delete(s.taps, id)
		s.mu.Unlock()
	}
}

// SetHidden keeps the chunks out of the window (they still go to the
// taps and the log): a file transfer would flood the terminal
func (s *Session) SetHidden(h bool) { s.hidden.Store(h) }

// Reconnects counts the times the device came back since the port was opened
func (s *Session) Reconnects() int64 { return s.reconnects.Load() }

// Port returns the open port, nil when it is not open. It is for the
// tools that set the port themselves (the detection of the speed).
func (s *Session) Port() serial.Port {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != Open {
		return nil
	}
	return s.port
}

func (s *Session) note(n Note) {
	text := s.NoteText
	if text == nil {
		text = DefaultNoteText
	}
	s.add(buffer.Chunk{Time: time.Now().UnixNano(), Dir: buffer.Info, Data: []byte(text(n))})
}

// Drain takes the chunks that came since the last call
func (s *Session) Drain() []buffer.Chunk {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.pending
	s.pending = nil
	return p
}

// State returns the state, the port, its settings and the last error of
// opening it again
func (s *Session) State() (State, serial.PortInfo, serial.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, s.info, s.cfg
}

// LastError is the error of the last attempt to open the port again
func (s *Session) LastError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

// OpenedAt is when the port was opened by the user
func (s *Session) OpenedAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.openedAt
}

// LostAt is when the device went away (state Waiting)
func (s *Session) LostAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lostAt
}

// Open opens the port; dtr and rts are the states of the lines to set
func (s *Session) Open(info serial.PortInfo, cfg serial.Config, dtr, rts bool) error {
	s.Close()
	p, err := serial.Open(info.Name, cfg)
	if err != nil {
		return err
	}
	// Some drivers don't have the lines (pseudo terminals): not an error
	p.SetDTR(dtr)
	p.SetRTS(rts)
	s.mu.Lock()
	s.info, s.cfg = info, cfg
	s.dtr, s.rts = dtr, rts
	s.stop = make(chan struct{})
	s.openedAt = time.Now()
	s.lastErr = nil
	s.reconnects.Store(0)
	s.start(p)
	s.mu.Unlock()
	s.note(Note{Kind: NoteConnected, Port: info, Config: cfg})
	return nil
}

// start runs the goroutines of an open port; s.mu is held
func (s *Session) start(p serial.Port) {
	s.gen++
	s.port = p
	s.state = Open
	s.writes = make(chan []byte, 256)
	go s.reader(p, s.gen)
	go s.writer(p, s.writes)
}

// Close closes the port; a lost device is not waited for any more
func (s *Session) Close() {
	s.mu.Lock()
	if s.state == Closed {
		s.mu.Unlock()
		return
	}
	p := s.port
	s.port = nil
	s.state = Closed
	s.gen++
	if s.stop != nil {
		close(s.stop)
		s.stop = nil
	}
	if s.writes != nil {
		close(s.writes)
		s.writes = nil
	}
	s.mu.Unlock()
	if p != nil {
		p.Close()
	}
	s.note(Note{Kind: NoteDisconnected})
}

func (s *Session) reader(p serial.Port, gen int) {
	buf := make([]byte, 16*1024)
	for {
		n, err := p.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			s.rx.Add(int64(n))
			s.add(buffer.Chunk{Time: time.Now().UnixNano(), Dir: buffer.RX, Data: data})
		}
		if err == nil {
			continue
		}
		if errors.Is(err, serial.ErrClosed) {
			return
		}
		s.lost(gen)
		return
	}
}

func (s *Session) writer(p serial.Port, writes chan []byte) {
	for data := range writes {
		// An error of a gone device is seen by the reader
		p.Write(data)
	}
}

// lost closes the port of a gone device and starts to wait for it
func (s *Session) lost(gen int) {
	s.mu.Lock()
	if s.gen != gen || s.state != Open {
		s.mu.Unlock()
		return
	}
	p := s.port
	s.port = nil
	s.state = Waiting
	s.lostAt = time.Now()
	if s.writes != nil {
		close(s.writes)
		s.writes = nil
	}
	stop := s.stop
	info, cfg := s.info, s.cfg
	s.mu.Unlock()
	p.Close()
	s.note(Note{Kind: NoteLost, Port: info})
	go s.reconnect(stop, info, cfg)
}

// reconnect looks for the device until it is back or the user closes the
// session. A USB device is found by its IDs and serial number, as its
// port may get another name.
func (s *Session) reconnect(stop chan struct{}, info serial.PortInfo, cfg serial.Config) {
	t := time.NewTicker(ReconnectInterval)
	defer t.Stop()
	key := info.Key()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		target := info
		found := false
		if ports, err := serial.List(); err == nil {
			for _, p := range ports {
				if p.Key() == key {
					target, found = p, true
					break
				}
			}
		}
		if !found && info.USB && info.SerialNumber != "" {
			continue
		}
		p, err := serial.Open(target.Name, cfg)
		if err != nil {
			s.mu.Lock()
			s.lastErr = err
			s.mu.Unlock()
			continue
		}
		s.mu.Lock()
		select {
		case <-stop:
			s.mu.Unlock()
			p.Close()
			return
		default:
		}
		p.SetDTR(s.dtr)
		p.SetRTS(s.rts)
		s.info = target
		s.lastErr = nil
		elapsed := time.Since(s.lostAt)
		s.reconnects.Add(1)
		s.start(p)
		s.mu.Unlock()
		s.note(Note{Kind: NoteReconnected, Port: target, Config: cfg, Elapsed: elapsed})
		return
	}
}

// ErrNotOpen is returned by the calls that need an open port
var ErrNotOpen = errors.New("port is not open")

// Send queues the bytes to be written; dir is TX or Keys. The chunk is
// added before the bytes are written, so an answer always comes after it.
func (s *Session) Send(data []byte, dir buffer.Dir) error {
	if len(data) == 0 {
		return nil
	}
	s.mu.Lock()
	if s.state != Open {
		s.mu.Unlock()
		return ErrNotOpen
	}
	writes := s.writes
	s.mu.Unlock()
	c := buffer.Chunk{Time: time.Now().UnixNano(), Dir: dir, Data: append([]byte(nil), data...)}
	if !queue(writes, c.Data) {
		return ErrQueueFull
	}
	s.tx.Add(int64(len(data)))
	s.add(c)
	return nil
}

// ErrQueueFull is returned by Send when the port doesn't take the data:
// the flow control holds it, or the line is much slower than the sending
var ErrQueueFull = errors.New("the port doesn't take the data: the send queue is full")

// queue puts the data in the queue of the writer without waiting; false
// when the queue is full or was closed meanwhile
func queue(writes chan []byte, data []byte) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	select {
	case writes <- data:
		return true
	default:
		return false
	}
}

// Forward queues the bytes to be written without a chunk of them: the
// data a bridge passes on, shown already as received from the other port
func (s *Session) Forward(data []byte) error {
	s.mu.Lock()
	writes, open := s.writes, s.state == Open
	s.mu.Unlock()
	if !open {
		return ErrNotOpen
	}
	if !queue(writes, append([]byte(nil), data...)) {
		return ErrQueueFull
	}
	s.tx.Add(int64(len(data)))
	return nil
}

// WriteSync writes the bytes and waits until they are written: for files
// sent in pieces
func (s *Session) WriteSync(data []byte) error {
	s.mu.Lock()
	p := s.port
	open := s.state == Open
	s.mu.Unlock()
	if !open || p == nil {
		return ErrNotOpen
	}
	s.tx.Add(int64(len(data)))
	s.add(buffer.Chunk{Time: time.Now().UnixNano(), Dir: buffer.TX, Data: append([]byte(nil), data...)})
	_, err := p.Write(data)
	return err
}

// SetConfig changes the settings of the open port, or those to use when it
// is opened again
func (s *Session) SetConfig(cfg serial.Config) error {
	s.mu.Lock()
	p, state := s.port, s.state
	s.mu.Unlock()
	if state == Open && p != nil {
		if err := p.SetConfig(cfg); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	if state != Closed {
		s.note(Note{Kind: NoteSettings, Config: cfg})
	}
	return nil
}

// Lines returns the states of DTR and RTS set by the session
func (s *Session) Lines() (dtr, rts bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dtr, s.rts
}

// SetDTR sets the DTR line; it is kept when the device comes back
func (s *Session) SetDTR(on bool) error {
	s.mu.Lock()
	s.dtr = on
	p := s.port
	s.mu.Unlock()
	if p == nil {
		return nil
	}
	return p.SetDTR(on)
}

// SetRTS sets the RTS line; it is kept when the device comes back
func (s *Session) SetRTS(on bool) error {
	s.mu.Lock()
	s.rts = on
	p := s.port
	s.mu.Unlock()
	if p == nil {
		return nil
	}
	return p.SetRTS(on)
}

// Signals reads the input lines; ok is false when the port is not open or
// has no lines
func (s *Session) Signals() (serial.Signals, bool) {
	s.mu.Lock()
	p := s.port
	s.mu.Unlock()
	if p == nil {
		return serial.Signals{}, false
	}
	sig, err := p.Signals()
	return sig, err == nil
}

// Break sends a break of d, in the background
func (s *Session) Break(d time.Duration) error {
	s.mu.Lock()
	p := s.port
	s.mu.Unlock()
	if p == nil {
		return ErrNotOpen
	}
	go p.Break(d)
	return nil
}

// Pulse sets DTR and RTS by steps, waiting after each: the reset
// sequences of the boards. It runs in the background.
func (s *Session) Pulse(steps []LineStep) {
	go func() {
		for _, st := range steps {
			if st.DTR != nil {
				s.SetDTR(*st.DTR)
			}
			if st.RTS != nil {
				s.SetRTS(*st.RTS)
			}
			time.Sleep(st.Wait)
		}
		s.notify()
	}()
}

// LineStep is a step of a reset sequence: the lines to set (nil keeps a
// line) and the time to wait after it
type LineStep struct {
	DTR, RTS *bool
	Wait     time.Duration
}

// Counters returns the bytes received and sent
func (s *Session) Counters() (rx, tx int64) { return s.rx.Load(), s.tx.Load() }

// ResetCounters sets the counters to zero
func (s *Session) ResetCounters() {
	s.rx.Store(0)
	s.tx.Store(0)
}

// StartLog writes what comes and goes to a file from now on
func (s *Session) StartLog(path string, format LogFormat, timestamps bool) error {
	l, err := NewLogger(path, format, timestamps)
	if err != nil {
		return err
	}
	s.mu.Lock()
	old := s.log
	s.log = l
	s.mu.Unlock()
	if old != nil {
		old.Close()
	}
	return nil
}

// StopLog closes the log file
func (s *Session) StopLog() error {
	s.mu.Lock()
	l := s.log
	s.log = nil
	s.mu.Unlock()
	if l == nil {
		return nil
	}
	return l.Close()
}

// Log returns the log being written, or nil
func (s *Session) Log() *Logger {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.log
}
