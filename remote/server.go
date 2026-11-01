package remote

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"sync"
	"time"

	"github.com/ipoluianov/altserialport/buffer"
	"github.com/ipoluianov/altserialport/session"
)

// pollInterval is how often the state of the session is looked at for the
// registry file
const pollInterval = 500 * time.Millisecond

// clientQueue is the most frames waiting for a client; a client that falls
// this far behind is disconnected, so the reader of the port never waits
const clientQueue = 1024

// helloTimeout is how long a new connection may take to authenticate
const helloTimeout = 5 * time.Second

// welcome is the answer to a good hello
type welcome struct {
	Port  string `json:"port"`
	Short string `json:"short"`
	Baud  int    `json:"baud"`
	Frame string `json:"frame"`
}

type server struct {
	sess  *session.Session
	ln    net.Listener
	token string
	entry Entry // as written to the file last

	mu      sync.Mutex
	clients map[*client]bool
	stopped bool
	done    chan struct{}
	wg      sync.WaitGroup
}

// client is a connection that has authenticated
type client struct {
	conn net.Conn
	out  chan frame
	once sync.Once
	dead chan struct{}
}

type frame struct {
	typ     byte
	payload []byte
}

// kill closes the connection once; the goroutines of the client see it
func (c *client) kill() {
	c.once.Do(func() {
		close(c.dead)
		c.conn.Close()
	})
}

// push queues a frame without waiting; a full queue disconnects the client
func (c *client) push(f frame) {
	select {
	case <-c.dead:
	case c.out <- f:
	default:
		c.kill()
	}
}

// Serve lets other processes of the program use the port of the session
// (see the package comment). stop closes the listener and the connections
// and removes the registry file.
func Serve(sess *session.Session) (stop func(), err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	var key [16]byte
	if _, err := rand.Read(key[:]); err != nil {
		ln.Close()
		return nil, err
	}
	s := &server{
		sess:    sess,
		ln:      ln,
		token:   hex.EncodeToString(key[:]),
		clients: map[*client]bool{},
		done:    make(chan struct{}),
	}
	s.entry = s.current()
	if err := writeEntry(s.entry); err != nil {
		ln.Close()
		return nil, err
	}
	removeTap := sess.AddTap(s.tap)
	s.wg.Add(2)
	go s.accept()
	go s.poll()
	var once sync.Once
	return func() {
		once.Do(func() {
			removeTap()
			s.mu.Lock()
			s.stopped = true
			clients := s.clients
			s.clients = map[*client]bool{}
			s.mu.Unlock()
			close(s.done)
			ln.Close()
			for c := range clients {
				c.kill()
			}
			s.wg.Wait()
			os.Remove(entryPath(s.entry.PID))
		})
	}, nil
}

// current is the registry entry for the state of the session now
func (s *server) current() Entry {
	state, info, cfg := s.sess.State()
	e := Entry{PID: os.Getpid(), Addr: s.ln.Addr().String(), Token: s.token}
	switch state {
	case session.Open:
		e.State = "open"
	case session.Waiting:
		e.State = "waiting"
	default:
		e.State = "closed"
		return e
	}
	e.Port, e.Short = info.Name, info.Short
	e.Baud, e.Frame = cfg.Baud, cfg.Frame()
	return e
}

// poll rewrites the registry file when the port of the session changes,
// and tells the clients when the port is not open any more
func (s *server) poll() {
	defer s.wg.Done()
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-t.C:
		}
		e := s.current()
		if e.State != "open" {
			s.dropAll()
		}
		if e != s.entry && writeEntry(e) == nil {
			s.entry = e
		}
	}
}

// tap gets every chunk of the session, from the goroutine that made it
func (s *server) tap(c buffer.Chunk) {
	switch c.Dir {
	case buffer.RX:
		s.mu.Lock()
		for cl := range s.clients {
			cl.push(frame{fData, c.Data})
		}
		s.mu.Unlock()
	case buffer.Info:
		// The notes come when the port is closed, lost or opened again;
		// a client of the old port is done with it
		if state, _, _ := s.sess.State(); state != session.Open {
			s.dropAll()
		}
	}
}

// dropAll tells the clients the port is gone and disconnects them once
// the queued frames are sent
func (s *server) dropAll() {
	s.mu.Lock()
	clients := s.clients
	s.clients = map[*client]bool{}
	s.mu.Unlock()
	for c := range clients {
		c.push(frame{fGone, nil})
		select {
		case <-c.dead:
		case c.out <- frame{}: // the writer closes the connection on it
		default:
			c.kill()
		}
	}
}

func (s *server) accept() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go s.handle(conn)
	}
}

func (s *server) handle(conn net.Conn) {
	defer s.wg.Done()
	conn.SetDeadline(time.Now().Add(helloTimeout))
	typ, payload, err := readFrame(conn)
	if err != nil || typ != fHello || subtle.ConstantTimeCompare(payload, []byte(s.token)) != 1 {
		conn.Close()
		return
	}
	state, info, cfg := s.sess.State()
	if state != session.Open {
		writeFrame(conn, fGone, nil)
		conn.Close()
		return
	}
	w, _ := json.Marshal(welcome{Port: info.Name, Short: info.Short, Baud: cfg.Baud, Frame: cfg.Frame()})
	if err := writeFrame(conn, fWelcome, w); err != nil {
		conn.Close()
		return
	}
	conn.SetDeadline(time.Time{})

	c := &client{conn: conn, out: make(chan frame, clientQueue), dead: make(chan struct{})}
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		conn.Close()
		return
	}
	s.clients[c] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, c)
		s.mu.Unlock()
		c.kill()
	}()

	s.wg.Add(1)
	go s.writer(c)
	for {
		typ, payload, err := readFrame(conn)
		if err != nil {
			return
		}
		status, result := s.request(typ, payload)
		reply := append([]byte{status}, result...)
		// A reply waits for room in the queue: it is the client's own
		// goroutine that waits, never the port
		select {
		case c.out <- frame{fReply, reply}:
		case <-c.dead:
			return
		}
	}
}

// writer sends the queued frames; an empty frame ends the connection
func (s *server) writer(c *client) {
	defer s.wg.Done()
	for {
		select {
		case <-c.dead:
			return
		case f := <-c.out:
			if f.typ == 0 {
				c.kill()
				return
			}
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := writeFrame(c.conn, f.typ, f.payload); err != nil {
				c.kill()
				return
			}
		}
	}
}

// request does what the client asks and returns the reply
func (s *server) request(typ byte, payload []byte) (byte, []byte) {
	var err error
	switch typ {
	case fWrite:
		err = s.sess.Send(payload, buffer.TX)
	case fDTR:
		err = s.lines(func() error { return s.sess.SetDTR(len(payload) > 0 && payload[0] != 0) })
	case fRTS:
		err = s.lines(func() error { return s.sess.SetRTS(len(payload) > 0 && payload[0] != 0) })
	case fSignals:
		sig, ok := s.sess.Signals()
		if !ok {
			if state, _, _ := s.sess.State(); state != session.Open {
				return rGone, nil
			}
			return rError, []byte("the port has no modem lines")
		}
		var b byte
		if sig.CTS {
			b |= sigCTS
		}
		if sig.DSR {
			b |= sigDSR
		}
		if sig.DCD {
			b |= sigDCD
		}
		if sig.RI {
			b |= sigRI
		}
		return rOK, []byte{b}
	case fBreak:
		ms := 250
		if len(payload) == 4 {
			ms = int(binary.BigEndian.Uint32(payload))
		}
		err = s.sess.Break(time.Duration(ms) * time.Millisecond)
	default:
		err = errors.New("unknown request")
	}
	switch {
	case err == nil:
		return rOK, nil
	case errors.Is(err, session.ErrNotOpen):
		return rGone, nil
	}
	return rError, []byte(err.Error())
}

// lines sets a line of the open port; the session would keep the state
// for a closed one, which a client must not change
func (s *server) lines(set func() error) error {
	if state, _, _ := s.sess.State(); state != session.Open {
		return session.ErrNotOpen
	}
	return set()
}
