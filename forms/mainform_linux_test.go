package forms

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ipoluianov/altserialport/buffer"
	"github.com/ipoluianov/altserialport/config"
	"github.com/ipoluianov/altserialport/proto/xmodem"
	"github.com/ipoluianov/altserialport/remote"
	"github.com/ipoluianov/altserialport/serial"
	"github.com/ipoluianov/altserialport/session"
	"github.com/ipoluianov/nui/ui"
	"github.com/ipoluianov/nui/ui/uitest"
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

// shot saves the picture of the window when ALTSERIAL_SHOTS names a directory
func shot(t *testing.T, w *uitest.Window, name string) {
	dir := os.Getenv("ALTSERIAL_SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, w.Screenshot())
}

// newTestForm makes the window with its settings in a temporary directory
// and a pseudo terminal as the device
func newTestForm(t *testing.T) (*MainForm, *uitest.Window, *os.File) {
	config.SetDir(t.TempDir())
	config.Load()
	master, slave := openPTY(t)
	t.Cleanup(func() { master.Close() })
	uitest.SetScreen(1280, 800, 1)
	m := NewMainForm()
	m.Form.SetSize(1100, 640)
	w := uitest.Show(t, m.Form)
	p := serial.PortInfo{Name: slave, Short: "ttyUSB0", Description: "CH340", USB: true, VID: 0x1a86, PID: 0x7523}
	m.custom = &p
	m.listPortsNow(p.Key())
	w.Settle()
	return m, w, master
}

// waitRows runs the timers until the terminal has the rows
func waitRows(t *testing.T, w *uitest.Window, m *MainForm, n int) {
	t.Helper()
	for i := 0; i < 100 && m.buf.RowCount() < n; i++ {
		w.Wait(30 * time.Millisecond)
	}
	if m.buf.RowCount() < n {
		t.Fatalf("rows: %d, want %d", m.buf.RowCount(), n)
	}
}

const bootLog = "\x1b[0;32mI (312) boot: ESP-IDF v5.2.1 2nd stage bootloader\x1b[0m\r\n" +
	"\x1b[0;32mI (318) boot: compile time Nov  1 2026 12:00:01\x1b[0m\r\n" +
	"\x1b[0;33mW (412) wifi: storage is full, oldest entries removed\x1b[0m\r\n" +
	"\x1b[0;32mI (1204) wifi: connected to ap SSID:lab, channel:6, rssi:-54\x1b[0m\r\n" +
	"\x1b[0;31mE (1530) sensor: CRC mismatch, expected 0x5A got 0x3C\x1b[0m\r\n" +
	"temp=23.4 hum=41.0 press=1013.2\r\n" +
	"temp=23.5 hum=40.8 press=1013.1\r\n"

func TestMainWindow(t *testing.T) {
	m, w, dev := newTestForm(t)
	shot(t, w, "01-start")

	w.Key(ui.KeyF5, ui.KeyModifiers{})
	w.Settle()
	if st, _, _ := m.sess.State(); st != session.Open {
		t.Fatal("not connected")
	}
	waitRows(t, w, m, 1)
	shot(t, w, "02-connected-empty")

	dev.Write([]byte(bootLog))
	waitRows(t, w, m, 8)

	// A command from the send line, as the user types it
	w.Click(m.send.text)
	w.Type("status\n")
	buf := make([]byte, 64)
	n, _ := dev.Read(buf)
	if string(buf[:n]) != "status\r\n" {
		t.Fatalf("device got %q", buf[:n])
	}
	dev.Write([]byte("OK uptime=1234s heap=182344\r\n> "))
	waitRows(t, w, m, 11)

	// Keys typed in the terminal go to the device at once
	w.Click(m.term.area)
	w.Type("ls")
	n, _ = dev.Read(buf)
	if got := string(buf[:n]); got != "l" && got != "ls" {
		t.Fatalf("keys: %q", got)
	}
	w.Wait(60 * time.Millisecond)
	shot(t, w, "03-text")

	m.setMode(1) // hex
	w.Wait(60 * time.Millisecond)
	shot(t, w, "04-hex")
	m.setMode(2) // mixed
	m.setTimeMode(TimeClock)
	w.Wait(60 * time.Millisecond)
	shot(t, w, "05-mixed-time")
	m.setMode(0)
	m.setTimeMode(TimeOff)

	// Find
	w.Shortcut("Ctrl+F")
	w.Type("temp")
	w.Wait(300 * time.Millisecond)
	if c, _ := m.term.Matches(); c != 2 {
		t.Fatalf("matches: %d", c)
	}
	shot(t, w, "06-find")
	w.Key(ui.KeyEsc, ui.KeyModifiers{})

	// The context menu of the terminal
	w.RightClick(uitest.At(m.term.area, 300, 120))
	w.Settle()
	shot(t, w, "07-menu")
	w.Key(ui.KeyEsc, ui.KeyModifiers{})
	w.Settle()

	// The history of the send line
	if w.Popup() != nil {
		t.Fatalf("popup still open: %T", w.Popup())
	}
	w.Click(m.send.text)
	if w.Focused() != ui.Widgeter(m.send.text) {
		t.Logf("focused %T %v", w.Focused(), w)
	}
	w.Key(ui.KeyArrowUp, ui.KeyModifiers{})
	if m.send.Text() != "status" {
		t.Fatalf("history: %q, %v %d, focused %T", m.send.Text(), m.send.history, m.send.hpos, w.Focused())
	}

	// The device goes away: the window waits for it
	dev.Close()
	for i := 0; i < 50; i++ {
		w.Wait(30 * time.Millisecond)
		if st, _, _ := m.sess.State(); st == session.Waiting {
			break
		}
	}
	if st, _, _ := m.sess.State(); st != session.Waiting {
		t.Fatal("not waiting")
	}
	w.Wait(60 * time.Millisecond)
	shot(t, w, "08-waiting")
	if !strings.Contains(w.Title(), "ttyUSB0") {
		t.Fatal("title", w.Title())
	}
	w.Key(ui.KeyF5, ui.KeyModifiers{})
	w.Settle()
	if st, _, _ := m.sess.State(); st != session.Closed {
		t.Fatal("not closed")
	}

	// The settings of the device are remembered
	config.Save()
	p := config.ProfileFor(m.key)
	if len(p.History) != 1 || p.History[0] != "status" {
		t.Fatalf("profile %+v", p)
	}
}

func TestDarkTheme(t *testing.T) {
	m, w, dev := newTestForm(t)
	ui.ApplyDarkTheme()
	defer ui.ApplyLightTheme()
	w.Key(ui.KeyF5, ui.KeyModifiers{})
	w.Settle()
	dev.Write([]byte(bootLog))
	waitRows(t, w, m, 8)
	w.Click(m.send.text)
	w.Type("AT+GMR\n")
	dev.Read(make([]byte, 64))
	dev.Write([]byte("AT version:2.4.0.0\r\nOK\r\n"))
	waitRows(t, w, m, 11)
	m.setTimeMode(TimeClock)
	w.Wait(60 * time.Millisecond)
	shot(t, w, "09-dark")
	m.setMode(1)
	w.Wait(60 * time.Millisecond)
	shot(t, w, "10-dark-hex")
}

func TestPanelsAndTools(t *testing.T) {
	m, w, dev := newTestForm(t)
	w.Key(ui.KeyF5, ui.KeyModifiers{})
	w.Settle()
	m.togglePlot()
	m.toggleMacros()
	for i := 0; i < 40; i++ {
		fmt.Fprintf(dev, "t=%.1f h=%d\r\n", 20+float64(i%10)/3, 40+i%7)
	}
	dev.Write([]byte("E (1530) sensor: CRC mismatch\r\nW (1600) wifi: weak signal\r\n"))
	waitRows(t, w, m, 40)
	w.Wait(200 * time.Millisecond)
	if len(m.plotter.series) != 2 {
		t.Fatalf("series: %d", len(m.plotter.series))
	}
	if c := m.term.rowColor(m.buf.EndRow() - 2); c == "" {
		t.Fatal("the error row has no color")
	}
	shot(t, w, "11-plot-macros")

	// A macro goes to the device
	m.runMacro(0)
	buf := make([]byte, 64)
	n, _ := dev.Read(buf)
	if string(buf[:n]) != "AT\r\n" {
		t.Fatalf("macro sent %q", buf[:n])
	}

	// The filter shows only the matching rows
	m.find.Open()
	w.Type("wifi")
	w.Wait(300 * time.Millisecond)
	m.term.SetFilter(true)
	w.Wait(60 * time.Millisecond)
	if !m.term.filtered() || len(m.term.fRows) != 1 {
		t.Fatalf("filtered rows %v", m.term.fRows)
	}
	shot(t, w, "12-filter")
	m.find.Close()

	// The second port on one time line, and a bridge
	m2, slave2 := openPTY(t)
	defer m2.Close()
	m.togglePort2()
	p2 := serial.PortInfo{Name: slave2, Short: "ttyUSB1", USB: true, VID: 0x0403, PID: 0x6001, SerialNumber: "B1"}
	m.port2.setPorts(append(m.ports, p2))
	m.port2.port.SetSelectedIndex(len(m.port2.ports) - 1)
	m.port2.toggle()
	if st, _, _ := m.port2.sess.State(); st != session.Open {
		t.Fatal("B not open")
	}
	m.port2.setBridge(true)
	m2.Write([]byte("from B\r\n"))
	n, _ = dev.Read(buf)
	if string(buf[:n]) != "from B\r\n" {
		t.Fatalf("bridge passed %q", buf[:n])
	}
	dev.Write([]byte("from A\r\n"))
	n, _ = m2.Read(buf)
	if string(buf[:n]) != "from A\r\n" {
		t.Fatalf("bridge passed %q", buf[:n])
	}
	w.Wait(100 * time.Millisecond)
	shot(t, w, "13-port2")
	m.port2.setSplit(true)
	m2.Write([]byte("only B\r\n"))
	w.Wait(100 * time.Millisecond)
	shot(t, w, "14-split")
	if mw := m.Form.Panel().MinWidth(); mw > 1100 {
		t.Errorf("the window needs %d px", mw)
		for _, c := range m.Form.Panel().Widgets() {
			t.Logf("%T %d", c, c.MinWidth())
		}
		t.Logf("macros %d plot %d term2 %d status %d", m.macros.MinWidth(), m.plotter.MinWidth(), m.port2.term.MinWidth(), m.status.MinWidth())
	}

	// The export
	if h := string(m.exportHTML()); !strings.Contains(h, "CRC mismatch") {
		t.Fatal("html")
	}
	if d := string(m.exportHexDump()); !strings.Contains(d, "RX") {
		t.Fatal("hex dump")
	}
}

// fileLink is the device side of a pseudo terminal for a file protocol
func fileLink(dev *os.File) *xmodem.ChanLink {
	link := xmodem.NewChanLink(func(b []byte) error { _, err := dev.Write(b); return err })
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := dev.Read(buf)
			if err != nil {
				return
			}
			link.Feed(append([]byte(nil), buf[:n]...))
		}
	}()
	return link
}

func TestFileTransferAndLoopback(t *testing.T) {
	m, w, dev := newTestForm(t)
	w.Key(ui.KeyF5, ui.KeyModifiers{})
	w.Settle()

	data := make([]byte, 3000)
	for i := range data {
		data[i] = byte(i * 7)
	}
	got := make(chan []byte, 1)
	link := fileLink(dev)
	go func() {
		_, d, err := xmodem.Receive(context.Background(), link, xmodem.YModem, nil)
		if err != nil {
			t.Error(err)
		}
		got <- d
	}()
	m.transferFile("fw.bin", data, fileYModem, 0, 0)
	select {
	case d := <-got:
		if !bytes.Equal(d, data) {
			t.Fatalf("received %d bytes, not the file", len(d))
		}
	case <-time.After(60 * time.Second):
		t.Fatal("the transfer did not end")
	}
	for i := 0; i < 20; i++ {
		w.Wait(50 * time.Millisecond)
	}
	// The protocol bytes are kept out of the terminal
	for i := m.buf.FirstRow(); i < m.buf.EndRow(); i++ {
		if r := m.buf.Row(i); r.Dir == buffer.TX && r.Len > 100 {
			t.Fatal("a block of the transfer is in the terminal")
		}
	}
}

func TestSharedPort(t *testing.T) {
	m, w, dev := newTestForm(t)
	w.Key(ui.KeyF5, ui.KeyModifiers{})
	w.Settle()
	_, info, _ := m.sess.State()
	var e remote.Entry
	ok := false
	for i := 0; i < 40 && !ok; i++ {
		w.Wait(50 * time.Millisecond)
		e, ok = remote.Find(info.Name)
	}
	if !ok {
		t.Fatal("the window doesn't offer its port")
	}
	p, err := remote.Dial(e)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if _, err := p.Write([]byte("from agent\r\n")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	n, _ := dev.Read(buf)
	if string(buf[:n]) != "from agent\r\n" {
		t.Fatalf("device got %q", buf[:n])
	}
	dev.Write([]byte("reply\r\n"))
	n, err = p.Read(buf)
	if err != nil || !strings.Contains(string(buf[:n]), "reply") {
		t.Fatalf("agent got %q %v", buf[:n], err)
	}
	waitRows(t, w, m, 3)
	found := false
	for i := m.buf.FirstRow(); i < m.buf.EndRow(); i++ {
		r := m.buf.Row(i)
		found = found || r.Dir == buffer.TX && strings.Contains(string(m.buf.Data(nil, r)), "from agent")
	}
	if !found {
		t.Fatal("the window doesn't show what the agent sent")
	}
}
