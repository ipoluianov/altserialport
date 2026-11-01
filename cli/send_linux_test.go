package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ipoluianov/altserialport/buffer"
	"github.com/ipoluianov/altserialport/config"
	"github.com/ipoluianov/altserialport/remote"
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

// device answers each command that ends with CR with the answer
func device(m *os.File, answer string) chan string {
	got := make(chan string, 1)
	go func() {
		var all []byte
		buf := make([]byte, 256)
		for {
			n, err := m.Read(buf)
			if err != nil {
				return
			}
			all = append(all, buf[:n]...)
			if bytes.HasSuffix(all, []byte("\r")) {
				m.Write([]byte(answer))
				got <- string(all)
				return
			}
		}
	}()
	return got
}

func TestSend(t *testing.T) {
	config.SetDir(t.TempDir())
	m, slave := openPTY(t)
	defer m.Close()
	// The master reads EIO while no one has the slave open: between the
	// runs of send it is held here
	hold, err := os.OpenFile(slave, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Close()
	logPath := filepath.Join(t.TempDir(), "send.log")

	got := device(m, "AT\r\r\nOK\r\n")
	var out, errOut bytes.Buffer
	e := env{stdin: strings.NewReader(""), stdout: &out, stderr: &errOut}
	code := run([]string{"send", slave, "AT", "--cr", "--expect", `OK\r\n`, "--timeout", "3s", "--log", logPath}, e)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if s := <-got; s != "AT\r" {
		t.Fatalf("device got %q", s)
	}
	if !strings.Contains(out.String(), "OK\r\n") {
		t.Fatalf("printed %q", out.String())
	}
	if log, _ := os.ReadFile(logPath); !strings.Contains(string(log), "> AT") || !strings.Contains(string(log), "OK") {
		t.Fatalf("log %q", log)
	}

	// No match in time: exit 1
	out.Reset()
	got = device(m, "ERROR\r\n")
	code = run([]string{"send", slave, `41 54 0d`, "--hex", "--expect", "^OK", "--timeout", "300ms"}, e)
	if code != 1 {
		t.Fatal("exit", code)
	}
	if s := <-got; s != "AT\r" {
		t.Fatalf("device got %q", s)
	}
	if !strings.Contains(out.String(), "ERROR") {
		t.Fatalf("printed %q", out.String())
	}

	// Without --expect: what came during the timeout, exit 0
	out.Reset()
	got = device(m, "hello")
	if code := run([]string{"send", slave, `x\r`, "--timeout", "300ms"}, e); code != 0 {
		t.Fatal("exit", code)
	}
	<-got
	if out.String() != "hello" {
		t.Fatalf("printed %q", out.String())
	}

	if code := run([]string{"send", "/dev/pts/none-such", "x"}, e); code != 1 {
		t.Fatal("missing port: exit", code)
	}
}

func TestSendThroughWindow(t *testing.T) {
	config.SetDir(t.TempDir())
	m, slave := openPTY(t)
	defer m.Close()
	sess := session.New()
	if err := sess.Open(serial.PortInfo{Name: slave, Short: filepath.Base(slave)}, serial.DefaultConfig(), true, true); err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	stop, err := remote.Serve(sess)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	sess.Drain()

	got := device(m, "OK\r\n")
	var out, errOut bytes.Buffer
	e := env{stdin: strings.NewReader(""), stdout: &out, stderr: &errOut}
	if code := run([]string{"send", slave, `AT\r`, "--expect", "OK"}, e); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	<-got
	if !strings.Contains(errOut.String(), "attached") {
		t.Fatalf("stderr %q", errOut.String())
	}
	seen := false
	for _, c := range sess.Drain() {
		if c.Dir == buffer.TX && string(c.Data) == "AT\r" {
			seen = true
		}
	}
	if !seen {
		t.Fatal("the window did not see the command")
	}
}

// syncBuffer is a buffer written from one goroutine and read from another
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("no", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestTerminal(t *testing.T) {
	config.SetDir(t.TempDir())
	m, slave := openPTY(t)
	link := filepath.Join(t.TempDir(), "ttyDEV")
	os.Symlink(slave, link)

	inR, inW := io.Pipe()
	var out, errOut syncBuffer
	code := make(chan int, 1)
	go func() {
		code <- run([]string{link, "--crlf", "--timestamp"}, env{stdin: inR, stdout: &out, stderr: &errOut})
	}()
	eventually(t, "hint", func() bool { return strings.Contains(errOut.String(), "Ctrl+]") })

	select {
	case c := <-code:
		t.Fatalf("exited %d: %q %q", c, out.String(), errOut.String())
	case <-time.After(300 * time.Millisecond):
	}
	inW.Write([]byte("ls\r"))
	buf := make([]byte, 16)
	n, _ := m.Read(buf)
	if string(buf[:n]) != "ls\r\n" {
		t.Fatalf("device got %q", buf[:n])
	}
	m.Write([]byte("a.txt\n"))
	eventually(t, "output", func() bool { return strings.HasSuffix(out.String(), " a.txt\n") })

	// The device goes away and comes back under the same name
	m.Close()
	eventually(t, "lost note", func() bool { return strings.Contains(out.String(), "Waiting") })
	m2, slave2 := openPTY(t)
	defer m2.Close()
	os.Remove(link)
	os.Symlink(slave2, link)
	eventually(t, "reconnect note", func() bool { return strings.Contains(out.String(), "Reconnected") })
	m2.Write([]byte("back\n"))
	eventually(t, "data after reconnect", func() bool { return strings.Contains(out.String(), " back\n") })

	inW.Write([]byte{exitKey})
	select {
	case c := <-code:
		if c != 0 {
			t.Fatal("exit", c)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Ctrl+] did not exit")
	}
}
