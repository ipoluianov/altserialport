package serial

import (
	"bytes"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

// recorder keeps all the bytes a test server has received
type recorder struct {
	mu  sync.Mutex
	buf []byte
}

func (r *recorder) run(c net.Conn) {
	b := make([]byte, 4096)
	for {
		n, err := c.Read(b)
		r.mu.Lock()
		r.buf = append(r.buf, b[:n]...)
		r.mu.Unlock()
		if err != nil {
			return
		}
	}
}

func (r *recorder) bytes() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]byte(nil), r.buf...)
}

// waitFor waits until the server has received want
func (r *recorder) waitFor(t *testing.T, what string, want []byte) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if bytes.Contains(r.bytes(), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s: % x not in % x", what, want, r.bytes())
}

// server listens on a free port and gives the first client to the test
func server(t *testing.T) (string, chan net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	conns := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			t.Cleanup(func() { c.Close() })
			conns <- c
		}
	}()
	return ln.Addr().String(), conns
}

func accepted(t *testing.T, conns chan net.Conn) net.Conn {
	t.Helper()
	select {
	case c := <-conns:
		return c
	case <-time.After(2 * time.Second):
		t.Fatal("no client")
	}
	return nil
}

// readN reads exactly n bytes from the port
func readN(t *testing.T, p Port, n int) []byte {
	t.Helper()
	got := []byte{}
	buf := make([]byte, 64)
	for len(got) < n {
		k, err := p.Read(buf)
		if err != nil {
			t.Fatalf("read after %q: %v", got, err)
		}
		got = append(got, buf[:k]...)
	}
	return got
}

// readErr waits for the error of a Read
func readErr(t *testing.T, p Port) error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		for {
			_, err := p.Read(make([]byte, 64))
			if err != nil {
				done <- err
				return
			}
		}
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("Read still blocked")
	}
	return nil
}

func TestNetworkNames(t *testing.T) {
	for _, c := range []struct{ name, short, desc string }{
		{"tcp://192.168.1.5:4000", "192.168.1.5:4000", "TCP"},
		{"TCP-LISTEN://:5000", ":5000", "TCP server"},
		{"rfc2217://ser2net.local:3001", "ser2net.local:3001", "RFC 2217"},
	} {
		if !IsNetwork(c.name) {
			t.Fatal(c.name)
		}
		info := NetworkInfo(c.name)
		if info.Name != c.name || info.Short != c.short || info.Description != c.desc {
			t.Fatalf("%s: %+v", c.name, info)
		}
	}
	for _, n := range []string{"/dev/ttyUSB0", "COM5", "tcp:/x"} {
		if IsNetwork(n) {
			t.Fatal(n)
		}
	}
	if Holder("tcp://127.0.0.1:1") != "" {
		t.Fatal("holder of a network port")
	}
	if _, err := Open("tcp://nohost", DefaultConfig()); err == nil {
		t.Fatal("address without a port accepted")
	}
	c := DefaultConfig()
	c.RS485 = true
	if _, err := Open("tcp://127.0.0.1:1", c); err == nil {
		t.Fatal("RS-485 on a network port")
	}
}

func TestTCPClient(t *testing.T) {
	addr, conns := server(t)
	p, err := Open("tcp://"+addr, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	srv := accepted(t, conns)

	srv.Write([]byte("hello\xff\x00"))
	if got := readN(t, p, 7); string(got) != "hello\xff\x00" {
		t.Fatalf("read %q", got)
	}
	if _, err := p.Write([]byte("AT\r\xff")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	srv.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := srv.Read(buf)
	if err != nil || string(buf[:n]) != "AT\r\xff" {
		t.Fatalf("server read %q, %v", buf[:n], err)
	}

	if err := p.SetConfig(Config{Baud: 9600, DataBits: 7, Parity: ParityEven}); err != nil {
		t.Fatal(err)
	}
	if err := p.SetDTR(true); err == nil {
		t.Fatal("DTR on TCP")
	}
	if _, err := p.Signals(); err == nil {
		t.Fatal("signals on TCP")
	}
	if err := p.Break(time.Millisecond); err == nil {
		t.Fatal("break on TCP")
	}

	srv.Close()
	if err := readErr(t, p); !errors.Is(err, ErrGone) {
		t.Fatalf("read after the server closed: %v", err)
	}
}

func TestTCPCloseUnblocksRead(t *testing.T) {
	addr, conns := server(t)
	p, err := Open("tcp://"+addr, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	accepted(t, conns)
	go func() {
		time.Sleep(50 * time.Millisecond)
		p.Close()
	}()
	if err := readErr(t, p); !errors.Is(err, ErrClosed) {
		t.Fatalf("read after close: %v", err)
	}
	if _, err := p.Read(make([]byte, 1)); !errors.Is(err, ErrClosed) {
		t.Fatalf("second read: %v", err)
	}
	if _, err := p.Write([]byte("x")); !errors.Is(err, ErrClosed) {
		t.Fatalf("write after close: %v", err)
	}
}

func TestTCPRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	for _, scheme := range []string{"tcp://", "rfc2217://"} {
		if _, err := Open(scheme+addr, DefaultConfig()); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s: %v", scheme, err)
		}
	}
}

func TestTCPListen(t *testing.T) {
	start := time.Now()
	p, err := Open("tcp-listen://127.0.0.1:0", DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("Open waited for a client")
	}
	addr := p.(interface{ Addr() net.Addr }).Addr().String()
	if _, err := p.Write([]byte("x")); err == nil {
		t.Fatal("write without a client")
	}
	if _, err := Open("tcp-listen://"+addr, DefaultConfig()); !errors.Is(err, ErrBusy) {
		t.Fatalf("second listen: %v", err)
	}

	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	c.Write([]byte("ping"))
	if got := readN(t, p, 4); string(got) != "ping" {
		t.Fatalf("read %q", got)
	}
	if _, err := p.Write([]byte("pong")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if n, err := c.Read(buf); err != nil || string(buf[:n]) != "pong" {
		t.Fatalf("client read %q, %v", buf[:n], err)
	}

	c.Close()
	if err := readErr(t, p); !errors.Is(err, ErrGone) {
		t.Fatalf("read after the client left: %v", err)
	}
	// The listener is closed with the client; a new Open listens again
	if c, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
		c.Close()
		t.Fatal("a second client was taken")
	}
	p.Close()
	p2, err := Open("tcp-listen://"+addr, DefaultConfig())
	if err != nil {
		t.Fatal("listen again:", err)
	}
	p2.Close()
}

func TestTCPListenCloseUnblocksRead(t *testing.T) {
	p, err := Open("tcp-listen://127.0.0.1:0", DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		p.Close()
	}()
	if err := readErr(t, p); !errors.Is(err, ErrClosed) {
		t.Fatalf("read after close: %v", err)
	}
}

var negotiation = []byte{
	telIAC, telWILL, optBinary, telIAC, telDO, optBinary,
	telIAC, telWILL, optSGA, telIAC, telDO, optSGA,
	telIAC, telWILL, optComPort,
}

func sb(b ...byte) []byte {
	return append(append([]byte{telIAC, telSB, optComPort}, b...), telIAC, telSE)
}

func TestRFC2217(t *testing.T) {
	addr, conns := server(t)
	rec := &recorder{}
	srvConn := make(chan net.Conn, 1)
	go func() {
		c := <-conns
		srvConn <- c
		// The server takes the options and asks for one the client hasn't
		c.Write([]byte{telIAC, telDO, optBinary, telIAC, telWILL, optBinary,
			telIAC, telDO, optSGA, telIAC, telWILL, optSGA, telIAC, telDO, optComPort,
			telIAC, telDO, 99, telIAC, telWILL, 98})
		rec.run(c)
	}()
	// 115455 is 00 01 c2 ff: the last byte has to be doubled
	c := Config{Baud: 115455, DataBits: 7, Parity: ParityEven, StopBits: StopBits2, Flow: FlowXonXoff}
	start := time.Now()
	p, err := Open("rfc2217://"+addr, c)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if time.Since(start) > agreeTimeout/2 {
		t.Fatal("Open didn't see the server agree")
	}
	srv := accepted(t, srvConn)

	rec.waitFor(t, "baud", sb(cpSetBaudrate, 0x00, 0x01, 0xc2, 0xff, 0xff))
	if got := rec.bytes(); !bytes.HasPrefix(got, negotiation) {
		t.Fatalf("negotiation % x", got)
	}
	rec.waitFor(t, "data size", sb(cpSetDatasize, 7))
	rec.waitFor(t, "parity", sb(cpSetParity, 3))
	rec.waitFor(t, "stop size", sb(cpSetStopsize, 2))
	rec.waitFor(t, "flow", sb(cpSetControl, ctlFlowXonXoff))
	rec.waitFor(t, "DTR", sb(cpSetControl, ctlDTROn))
	rec.waitFor(t, "RTS", sb(cpSetControl, ctlRTSOn))
	rec.waitFor(t, "modem state mask", sb(cpSetModemstateMask, 0xff, 0xff))
	rec.waitFor(t, "WONT 99", []byte{telIAC, telWONT, 99})
	rec.waitFor(t, "DONT 98", []byte{telIAC, telDONT, 98})

	// The data with a doubled IAC, a notification of CTS and DSR and a
	// command in the middle
	srv.Write([]byte{'a', telIAC, telIAC, 'b'})
	srv.Write(sb(cpServer+cpNotifyModemstate, modemCTS|modemDSR))
	srv.Write([]byte{telIAC, 241, 'c'}) // NOP
	if got := readN(t, p, 4); string(got) != "a\xffbc" {
		t.Fatalf("read %q", got)
	}
	s, err := p.Signals()
	if err != nil || !s.CTS || !s.DSR || s.DCD || s.RI {
		t.Fatalf("signals %+v, %v", s, err)
	}

	if _, err := p.Write([]byte("x\xffy")); err != nil {
		t.Fatal(err)
	}
	rec.waitFor(t, "escaped data", []byte("x\xff\xffy"))

	if err := p.SetDTR(false); err != nil {
		t.Fatal(err)
	}
	rec.waitFor(t, "DTR off", sb(cpSetControl, ctlDTROff))
	if err := p.SetRTS(false); err != nil {
		t.Fatal(err)
	}
	rec.waitFor(t, "RTS off", sb(cpSetControl, ctlRTSOff))
	if err := p.Break(time.Millisecond); err != nil {
		t.Fatal(err)
	}
	rec.waitFor(t, "break", append(sb(cpSetControl, ctlBreakOn), sb(cpSetControl, ctlBreakOff)...))
	if err := p.SetConfig(Config{Baud: 9600, DataBits: 8, Parity: ParityNone}); err != nil {
		t.Fatal(err)
	}
	rec.waitFor(t, "new baud", sb(cpSetBaudrate, 0, 0, 0x25, 0x80))

	srv.Close()
	if err := readErr(t, p); !errors.Is(err, ErrGone) {
		t.Fatalf("read after the server closed: %v", err)
	}
}

func TestRFC2217Refused(t *testing.T) {
	addr, conns := server(t)
	go func() {
		c := <-conns
		c.Write([]byte{telIAC, telDONT, optComPort})
		(&recorder{}).run(c)
	}()
	if _, err := Open("rfc2217://"+addr, DefaultConfig()); err == nil {
		t.Fatal("a server without COM-PORT-OPTION accepted")
	}
}

func TestRFC2217CloseUnblocksRead(t *testing.T) {
	addr, conns := server(t)
	go func() {
		c := <-conns
		c.Write([]byte{telIAC, telDO, optComPort})
		(&recorder{}).run(c)
	}()
	p, err := Open("rfc2217://"+addr, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		p.Close()
	}()
	if err := readErr(t, p); !errors.Is(err, ErrClosed) {
		t.Fatalf("read after close: %v", err)
	}
	if _, err := p.Signals(); !errors.Is(err, ErrClosed) {
		t.Fatalf("signals after close: %v", err)
	}
}

// The commands may come split between reads of the connection
func TestTelnetParseSplit(t *testing.T) {
	p := &rfc2217Port{}
	in := []byte{'1', telIAC, telIAC, '2', telIAC, telSB, optComPort, cpServer + cpNotifyModemstate,
		modemDCD | modemRI, telIAC, telSE, '3', telIAC, telWONT, optSGA, '4'}
	var out []byte
	for _, b := range in {
		out = append(out, p.parse([]byte{b})...)
	}
	if string(out) != "1\xff234" {
		t.Fatalf("data %q", out)
	}
	if p.modem != modemDCD|modemRI {
		t.Fatalf("modem %x", p.modem)
	}
}
