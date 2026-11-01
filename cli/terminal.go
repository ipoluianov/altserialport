package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ipoluianov/altserialport/buffer"
	"github.com/ipoluianov/altserialport/remote"
	"github.com/ipoluianov/altserialport/serial"
	"github.com/ipoluianov/altserialport/session"
)

// exitKey is Ctrl+]: it ends the terminal, as in telnet
const exitKey = 0x1d

// reconnectInterval is how often a gone device is looked for
const reconnectInterval = 250 * time.Millisecond

// hexGap is the pause that starts a new row of the hex dump
const hexGap = 100 * time.Millisecond

// console is the terminal in the console: what is typed goes to the port,
// what comes goes to stdout
type console struct {
	o   options
	e   env
	out *output
	log *session.Logger

	mu sync.Mutex
	p  serial.Port // nil while the device is gone
}

func terminal(o options, e env) int {
	if len(o.args) != 1 {
		return usageError(e, errors.New("one port is needed"))
	}
	name := o.args[0]
	c := &console{o: o, e: e, out: &output{w: e.stdout, hex: o.hex, stamp: o.stamp, atStart: true}}
	if o.log != "" {
		l, err := session.NewLogger(o.log, session.LogText, o.stamp)
		if err != nil {
			fmt.Fprintln(e.stderr, "altserialport:", err)
			return exitFail
		}
		c.log = l
		defer l.Close()
	}
	p, err := open(name, o.cfg, e)
	if err != nil {
		fmt.Fprintln(e.stderr, "altserialport:", err)
		return exitFail
	}
	c.p = p

	restore := func() {}
	if f, ok := e.stdin.(*os.File); ok {
		if r, err := makeRaw(f); err == nil {
			restore = r
		}
	}
	defer restore()
	fmt.Fprintf(e.stderr, "Connected to %s at %d %s. Ctrl+] exits.\r\n", name, o.cfg.Baud, o.cfg.Frame())

	done := make(chan int, 1)
	quit := func(code int) {
		select {
		case done <- code:
		default:
		}
	}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)
	go func() {
		if _, ok := <-sigs; ok {
			quit(exitOK)
		}
	}()

	go c.keys(quit)
	go c.reader(name, p, quit)
	code := <-done

	c.mu.Lock()
	p = c.p
	c.p = nil
	c.mu.Unlock()
	if p != nil {
		p.Close()
	}
	c.out.finish()
	return code
}

// keys sends what is typed to the port until Ctrl+] or the end of stdin
func (c *console) keys(quit func(int)) {
	buf := make([]byte, 1024)
	for {
		n, err := c.e.stdin.Read(buf)
		data := buf[:n]
		for i, b := range data {
			if b == exitKey {
				c.sendKeys(data[:i])
				quit(exitOK)
				return
			}
		}
		c.sendKeys(data)
		if err != nil {
			// Stdin is a file or a pipe that ended: keep showing the data
			// until a signal
			return
		}
	}
}

// sendKeys sends the typed bytes, with Enter as CR LF when asked
func (c *console) sendKeys(data []byte) {
	if len(data) == 0 {
		return
	}
	if c.o.ending == "\r\n" {
		var out []byte
		for _, b := range data {
			out = append(out, b)
			if b == '\r' {
				out = append(out, '\n')
			}
		}
		data = out
	}
	if c.o.echo {
		c.out.write(data)
	}
	c.mu.Lock()
	p := c.p
	c.mu.Unlock()
	if p == nil {
		return // the device is gone: the keys are lost
	}
	if c.log != nil {
		c.log.Write(buffer.Chunk{Time: time.Now().UnixNano(), Dir: buffer.Keys, Data: append([]byte(nil), data...)})
	}
	p.Write(data)
}

// reader shows what comes; when the device goes away it waits for it and
// opens the port again
func (c *console) reader(name string, p serial.Port, quit func(int)) {
	buf := make([]byte, 16*1024)
	for {
		n, err := p.Read(buf)
		if n > 0 {
			data := append([]byte(nil), buf[:n]...)
			if c.log != nil {
				c.log.Write(buffer.Chunk{Time: time.Now().UnixNano(), Dir: buffer.RX, Data: data})
			}
			c.out.write(data)
		}
		if err == nil {
			continue
		}
		c.mu.Lock()
		if c.p != p {
			c.mu.Unlock()
			return // closed by the exit
		}
		c.p = nil
		c.mu.Unlock()
		p.Close()
		if errors.Is(err, serial.ErrClosed) {
			quit(exitOK)
			return
		}
		c.out.note("Device is gone. Waiting for it to come back...")
		lost := time.Now()
		for {
			time.Sleep(reconnectInterval)
			np, attached, err := remote.Open(name, c.o.cfg)
			if err != nil {
				continue
			}
			c.mu.Lock()
			c.p = np
			c.mu.Unlock()
			how := ""
			if attached {
				how = " (through the AltSerialPort window)"
			}
			c.out.note(fmt.Sprintf("Reconnected after %.1f s%s", time.Since(lost).Seconds(), how))
			p = np
			break
		}
	}
}

// output writes the data to stdout as it is, or as a hex dump, with the
// time at the start of each line when asked
type output struct {
	mu      sync.Mutex
	w       io.Writer
	hex     bool
	stamp   bool
	atStart bool
	col     int // bytes in the current row of the hex dump
	last    time.Time
}

func (o *output) prefix(now time.Time) []byte {
	if !o.stamp {
		return nil
	}
	return []byte(now.Format("15:04:05.000 "))
}

func (o *output) write(data []byte) {
	o.mu.Lock()
	defer o.mu.Unlock()
	now := time.Now()
	var out []byte
	if o.hex {
		const digits = "0123456789ABCDEF"
		if o.col > 0 && now.Sub(o.last) > hexGap {
			out = append(out, '\r', '\n')
			o.col = 0
		}
		for _, b := range data {
			if o.col == 0 {
				out = append(out, o.prefix(now)...)
			}
			out = append(out, digits[b>>4], digits[b&15], ' ')
			o.col++
			if o.col == 16 {
				out = append(out, '\r', '\n')
				o.col = 0
			}
		}
	} else if o.stamp {
		for _, b := range data {
			if o.atStart {
				out = append(out, o.prefix(now)...)
				o.atStart = false
			}
			out = append(out, b)
			if b == '\n' {
				o.atStart = true
			}
		}
	} else {
		out = data
		if len(data) > 0 {
			o.atStart = data[len(data)-1] == '\n'
		}
	}
	o.last = now
	o.w.Write(out)
}

// note writes a message of the program on a line of its own
func (o *output) note(text string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	var out []byte
	if !o.atStart || o.col > 0 {
		out = append(out, '\r', '\n')
	}
	out = append(out, "--- "+text+" ---\r\n"...)
	o.atStart, o.col = true, 0
	o.w.Write(out)
}

// finish ends the last line of the hex dump
func (o *output) finish() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.hex && o.col > 0 {
		o.w.Write([]byte("\r\n"))
		o.col = 0
	}
}
