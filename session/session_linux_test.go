package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ipoluianov/altserialport/buffer"
	"github.com/ipoluianov/altserialport/serial"
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

// waitFor drains the session until a chunk satisfies ok
func waitFor(t *testing.T, s *Session, what string, ok func(c buffer.Chunk) bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for _, c := range s.Drain() {
			if ok(c) {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no", what)
}

func TestSessionReconnects(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "ttyTEST")
	m, slave := openPTY(t)
	if err := os.Symlink(slave, link); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "log.txt")

	s := New()
	info := serial.PortInfo{Name: link, Short: "ttyTEST"}
	if err := s.Open(info, serial.DefaultConfig(), true, true); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.StartLog(logPath, LogText, false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, s, "connected note", func(c buffer.Chunk) bool { return c.Dir == buffer.Info })

	m.Write([]byte("hello\r\n"))
	waitFor(t, s, "data", func(c buffer.Chunk) bool { return c.Dir == buffer.RX })
	if err := s.Send([]byte("AT\r\n"), buffer.TX); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 16)
	n, _ := m.Read(got)
	if string(got[:n]) != "AT\r\n" {
		t.Fatalf("device got %q", got[:n])
	}

	// The device goes away: the session waits for it
	m.Close()
	waitFor(t, s, "lost note", func(c buffer.Chunk) bool {
		return c.Dir == buffer.Info && strings.Contains(string(c.Data), "Waiting")
	})
	if st, _, _ := s.State(); st != Waiting {
		t.Fatal("state", st)
	}
	if err := s.Send([]byte("x"), buffer.TX); err != ErrNotOpen {
		t.Fatal("send while waiting:", err)
	}

	// It comes back under the same name
	m2, slave2 := openPTY(t)
	defer m2.Close()
	os.Remove(link)
	os.Symlink(slave2, link)
	waitFor(t, s, "reconnected note", func(c buffer.Chunk) bool {
		return c.Dir == buffer.Info && strings.Contains(string(c.Data), "Reconnected")
	})
	m2.Write([]byte("back\n"))
	waitFor(t, s, "data after reconnect", func(c buffer.Chunk) bool {
		return c.Dir == buffer.RX && string(c.Data) == "back\n"
	})

	s.StopLog()
	log, _ := os.ReadFile(logPath)
	want := "hello\n> AT\n--- Device is gone. Waiting for it to come back... ---\n"
	if !strings.HasPrefix(string(log), want) || !strings.HasSuffix(string(log), "back\n") {
		t.Fatalf("log:\n%s", log)
	}
	rx, tx := s.Counters()
	if rx != 12 || tx != 4 {
		t.Fatal("counters", rx, tx)
	}
}

func TestLogTimestamps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "l.log")
	l, err := NewLogger(path, LogText, true)
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2026, 11, 1, 12, 0, 1, 234_000_000, time.Local).UnixNano()
	l.Write(buffer.Chunk{Time: ts, Dir: buffer.RX, Data: []byte("a\r\n\r\nb")})
	l.Write(buffer.Chunk{Time: ts, Dir: buffer.RX, Data: []byte("c\r")})
	l.Write(buffer.Chunk{Time: ts, Dir: buffer.RX, Data: []byte("\nd")})
	l.Close()
	got, _ := os.ReadFile(path)
	p := "2026-11-01 12:00:01.234 "
	want := p + "a\n" + p + "\n" + p + "bc\n" + p + "d\n"
	if string(got) != want {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
}

func TestRawLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "l.bin")
	l, _ := NewLogger(path, LogRaw, true)
	l.Write(buffer.Chunk{Dir: buffer.RX, Data: []byte{0, 1, '\r'}})
	l.Write(buffer.Chunk{Dir: buffer.TX, Data: []byte("x")})
	l.Close()
	got, _ := os.ReadFile(path)
	if string(got) != "\x00\x01\r" {
		t.Fatalf("%q", got)
	}
}

func TestLogRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "port.log")
	l, _ := NewLogger(path, LogText, false)
	l.SetMaxSize(10)
	for i := 0; i < 5; i++ {
		l.Write(buffer.Chunk{Dir: buffer.RX, Data: []byte("0123456789\n")})
	}
	l.Close()
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if fmt.Sprint(names) != "[port-2.log port-3.log port-4.log port-5.log port-6.log port.log]" {
		t.Fatal(names)
	}
}
