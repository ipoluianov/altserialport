package remote

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ipoluianov/altserialport/buffer"
	"github.com/ipoluianov/altserialport/config"
	"github.com/ipoluianov/altserialport/serial"
	"github.com/ipoluianov/altserialport/session"
	"golang.org/x/sys/unix"
)

func openPTY(t *testing.T) (*os.File, string) {
	t.Helper()
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skip("no /dev/ptmx:", err)
	}
	fd := int(m.Fd())
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	return m, fmt.Sprintf("/dev/pts/%d", n)
}

// readN reads from r until n bytes came or the time is out
func readN(t *testing.T, r func([]byte) (int, error), n int) string {
	t.Helper()
	got := make(chan string, 1)
	go func() {
		var all []byte
		buf := make([]byte, 256)
		for len(all) < n {
			k, err := r(buf)
			all = append(all, buf[:k]...)
			if err != nil {
				break
			}
		}
		got <- string(all)
	}()
	select {
	case s := <-got:
		return s
	case <-time.After(3 * time.Second):
		t.Fatal("nothing came")
	}
	return ""
}

func TestServeFindDial(t *testing.T) {
	config.SetDir(t.TempDir())
	m, slave := openPTY(t)
	defer m.Close()
	dir := t.TempDir()
	link := filepath.Join(dir, "ttyLINK")
	if err := os.Symlink(slave, link); err != nil {
		t.Fatal(err)
	}

	sess := session.New()
	if err := sess.Open(serial.PortInfo{Name: slave, Short: filepath.Base(slave)}, serial.DefaultConfig(), true, true); err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	stop, err := Serve(sess)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	file := entryPath(os.Getpid())
	if _, err := os.Stat(file); err != nil {
		t.Fatal("no registry file:", err)
	}
	if _, ok := Find("/dev/nothing"); ok {
		t.Fatal("found a port nobody has")
	}
	// The name is matched through the link
	e, ok := Find(link)
	if !ok {
		t.Fatal("window not found")
	}
	if e.State != "open" || e.Port != slave {
		t.Fatalf("entry %+v", e)
	}
	bad := e
	bad.Token = "wrong"
	if _, err := Dial(bad); err == nil {
		t.Fatal("a wrong token was taken")
	}

	// The port is busy: Open attaches to the window
	p, attached, err := Open(link, serial.DefaultConfig())
	if err != nil || !attached {
		t.Fatal("open:", attached, err)
	}
	defer p.Close()
	if err := p.SetConfig(serial.Config{Baud: 9600, DataBits: 7}); err != nil {
		t.Fatal(err)
	}
	sess.Drain()

	// From the client to the device; the window sees it as sent
	if _, err := p.Write([]byte("AT\r\n")); err != nil {
		t.Fatal(err)
	}
	if got := readN(t, m.Read, 4); got != "AT\r\n" {
		t.Fatalf("device got %q", got)
	}
	sawTX := false
	for _, c := range sess.Drain() {
		if c.Dir == buffer.TX && string(c.Data) == "AT\r\n" {
			sawTX = true
		}
	}
	if !sawTX {
		t.Fatal("the window did not see the sent data")
	}

	// From the device to the client
	m.Write([]byte("OK\r\n"))
	if got := readN(t, p.Read, 4); got != "OK\r\n" {
		t.Fatalf("client got %q", got)
	}

	// A second client gets the same data
	p2, err := Dial(e)
	if err != nil {
		t.Fatal(err)
	}
	m.Write([]byte("two"))
	if got := readN(t, p2.Read, 3); got != "two" {
		t.Fatalf("second client got %q", got)
	}
	p2.Close()
	if _, err := p2.Read(make([]byte, 8)); !errors.Is(err, serial.ErrClosed) {
		t.Fatal("read after close:", err)
	}

	// The window closes the port: the client sees it gone
	sess.Close()
	errc := make(chan error, 1)
	go func() {
		buf := make([]byte, 64)
		for {
			if _, err := p.Read(buf); err != nil {
				errc <- err
				return
			}
		}
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, serial.ErrGone) {
			t.Fatal("read:", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the client did not see the port closed")
	}
	if _, err := p.Write([]byte("x")); err == nil {
		t.Fatal("write to a closed window port")
	}

	// The registry follows the session
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, ok := Find(link); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the closed port is still in the registry")
		}
		time.Sleep(50 * time.Millisecond)
	}

	stop()
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("registry file left after stop:", err)
	}
}

func TestDeadEntriesDropped(t *testing.T) {
	config.SetDir(t.TempDir())
	// A pid far above any pid_max
	dead := Entry{PID: 1 << 30, Addr: "127.0.0.1:1", Token: "x", State: "open", Port: "/dev/ttyDEAD", Short: "ttyDEAD"}
	if err := writeEntry(dead); err != nil {
		t.Fatal(err)
	}
	if _, ok := Find("/dev/ttyDEAD"); ok {
		t.Fatal("a dead window was found")
	}
	if _, err := os.Stat(entryPath(dead.PID)); !os.IsNotExist(err) {
		t.Fatal("the file of a dead window was kept")
	}
}

func TestSlowClientDropped(t *testing.T) {
	config.SetDir(t.TempDir())
	s := &server{clients: map[*client]bool{}}
	a, b := net.Pipe()
	defer b.Close()
	c := &client{conn: a, out: make(chan frame, 2), dead: make(chan struct{})}
	s.clients[c] = true
	for i := 0; i < 3; i++ {
		s.tap(buffer.Chunk{Dir: buffer.RX, Data: []byte("x")})
	}
	select {
	case <-c.dead:
	default:
		t.Fatal("a full client was not disconnected")
	}
}
