package serial

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
)

// The schemes of the network ports
const (
	schemeTCP     = "tcp://"
	schemeListen  = "tcp-listen://"
	schemeRFC2217 = "rfc2217://"
)

// dialTimeout is how long a network port waits for the server to answer
const dialTimeout = 3 * time.Second

// errNoLines is returned by the calls for the lines of a raw TCP port
var errNoLines = errors.New("a TCP port has no control lines")

// IsNetwork tells whether the name is of a network port: "tcp://...",
// "tcp-listen://...", "rfc2217://..."
func IsNetwork(name string) bool {
	scheme, _ := splitNetwork(name)
	return scheme != ""
}

// NetworkInfo describes a network port as List describes the others
func NetworkInfo(name string) PortInfo {
	scheme, addr := splitNetwork(name)
	p := PortInfo{Name: name, Short: addr}
	switch scheme {
	case schemeTCP:
		p.Description = "TCP"
	case schemeListen:
		p.Description = "TCP server"
	case schemeRFC2217:
		p.Description = "RFC 2217"
	}
	return p
}

// splitNetwork returns the scheme in lower case and the address; the
// scheme is "" for a name of a serial port
func splitNetwork(name string) (scheme, addr string) {
	n := strings.ToLower(name)
	for _, s := range []string{schemeTCP, schemeListen, schemeRFC2217} {
		if strings.HasPrefix(n, s) {
			return s, strings.TrimRight(name[len(s):], "/")
		}
	}
	return "", name
}

func openNetwork(name string, c Config) (Port, error) {
	if c.RS485 {
		return nil, errors.New("RS-485 mode is not available on a network port")
	}
	scheme, addr := splitNetwork(name)
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" || (host == "" && scheme != schemeListen) {
		return nil, fmt.Errorf("invalid address %q: host:port is needed", addr)
	}
	switch scheme {
	case schemeListen:
		return listen(addr)
	case schemeRFC2217:
		conn, err := dial(addr)
		if err != nil {
			return nil, err
		}
		return newRFC2217(conn, c)
	}
	conn, err := dial(addr)
	if err != nil {
		return nil, err
	}
	return &tcpPort{conn: conn}, nil
}

// dial connects to the server. The address is checked before, so a failure
// means the server isn't there (refused, unreachable, unknown name, no
// answer): ErrNotFound, which the reconnection keeps trying on.
func dial(addr string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		// Small writes, such as keys typed one by one, go out at once
		tc.SetNoDelay(true)
		tc.SetKeepAlive(true)
	}
	return conn, nil
}

// connState is the closing of a network port shared by its kinds
type connState struct {
	mu     sync.Mutex
	closed bool
}

func (s *connState) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// setClosed marks the port closed; false when it already was
func (s *connState) setClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.closed = true
	return true
}

// connError turns an error of the connection into ErrClosed after Close
// and into ErrGone otherwise: the other side closed or reset it
func (s *connState) connError() error {
	if s.isClosed() {
		return ErrClosed
	}
	return ErrGone
}

// tcpPort is a raw TCP connection: the bytes go as they are, there are no
// settings and no lines
type tcpPort struct {
	connState
	conn net.Conn
}

func (p *tcpPort) Read(b []byte) (int, error) {
	if p.isClosed() {
		return 0, ErrClosed
	}
	n, err := p.conn.Read(b)
	if n > 0 {
		return n, nil
	}
	if err != nil {
		return 0, p.connError()
	}
	return 0, nil
}

func (p *tcpPort) Write(b []byte) (int, error) {
	if p.isClosed() {
		return 0, ErrClosed
	}
	n, err := p.conn.Write(b)
	if err != nil {
		return n, p.connError()
	}
	return n, nil
}

func (p *tcpPort) Close() error {
	if !p.setClosed() {
		return nil
	}
	return p.conn.Close()
}

func (p *tcpPort) SetConfig(c Config) error {
	if c.RS485 {
		return errors.New("RS-485 mode is not available on a network port")
	}
	return c.Validate()
}

func (p *tcpPort) SetDTR(on bool) error        { return errNoLines }
func (p *tcpPort) SetRTS(on bool) error        { return errNoLines }
func (p *tcpPort) Signals() (Signals, error)   { return Signals{}, errNoLines }
func (p *tcpPort) Break(d time.Duration) error { return errNoLines }

// listenPort is a TCP server for one client. Open only starts listening;
// Read waits for the client. When the client goes, the port is gone and
// a new Open listens again.
type listenPort struct {
	connState
	ln    net.Listener
	ready chan struct{} // closed once a client came or accepting failed
	done  chan struct{} // closed by Close
	conn  net.Conn      // the client, under mu
	err   error         // why accepting failed, under mu
}

func listen(addr string) (Port, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, listenError(err)
	}
	p := &listenPort{ln: ln, ready: make(chan struct{}), done: make(chan struct{})}
	go p.accept()
	return p, nil
}

// listenError tells a taken TCP port and a denied one (below 1024 without
// rights) from the other errors
func listenError(err error) error {
	switch {
	// 10048 and 10013 are WSAEADDRINUSE and WSAEACCES on Windows
	case errors.Is(err, syscall.EADDRINUSE), errors.Is(err, syscall.Errno(10048)):
		return fmt.Errorf("%w: %v", ErrBusy, err)
	case errors.Is(err, os.ErrPermission), errors.Is(err, syscall.Errno(10013)):
		return fmt.Errorf("%w: %v", ErrPermission, err)
	}
	return err
}

func (p *listenPort) accept() {
	conn, err := p.ln.Accept()
	// One client only: the next ones are refused
	p.ln.Close()
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.SetNoDelay(true)
		tc.SetKeepAlive(true)
	}
	p.mu.Lock()
	if p.closed {
		if conn != nil {
			conn.Close()
		}
	} else {
		p.conn, p.err = conn, err
	}
	p.mu.Unlock()
	close(p.ready)
}

// client returns the connected client, nil while there is none
func (p *listenPort) client() (net.Conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, ErrClosed
	}
	return p.conn, p.err
}

func (p *listenPort) Read(b []byte) (int, error) {
	select {
	case <-p.done:
		return 0, ErrClosed
	case <-p.ready:
	}
	conn, err := p.client()
	if err != nil {
		if errors.Is(err, ErrClosed) {
			return 0, err
		}
		return 0, ErrGone
	}
	n, err := conn.Read(b)
	if n > 0 {
		return n, nil
	}
	if err != nil {
		return 0, p.connError()
	}
	return 0, nil
}

func (p *listenPort) Write(b []byte) (int, error) {
	conn, err := p.client()
	if errors.Is(err, ErrClosed) {
		return 0, err
	}
	if conn == nil {
		return 0, fmt.Errorf("no client is connected to %s", p.ln.Addr())
	}
	n, err := conn.Write(b)
	if err != nil {
		return n, p.connError()
	}
	return n, nil
}

func (p *listenPort) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	conn := p.conn
	p.mu.Unlock()
	close(p.done)
	p.ln.Close()
	if conn != nil {
		return conn.Close()
	}
	return nil
}

// Addr is the address the port listens on, with the port number chosen by
// the system when it was 0
func (p *listenPort) Addr() net.Addr { return p.ln.Addr() }

func (p *listenPort) SetConfig(c Config) error {
	if c.RS485 {
		return errors.New("RS-485 mode is not available on a network port")
	}
	return c.Validate()
}

func (p *listenPort) SetDTR(on bool) error        { return errNoLines }
func (p *listenPort) SetRTS(on bool) error        { return errNoLines }
func (p *listenPort) Signals() (Signals, error)   { return Signals{}, errNoLines }
func (p *listenPort) Break(d time.Duration) error { return errNoLines }
