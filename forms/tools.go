package forms

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/ipoluianov/altserialport/buffer"
	"github.com/ipoluianov/altserialport/serial"
	"github.com/ipoluianov/altserialport/session"
	"github.com/ipoluianov/nui/ui"
)

// stats is the speed of the data, sampled every second
type stats struct {
	lastRx, lastTx int64
	rxRate, txRate float64 // bytes a second
	lines          int64   // lines received since the last sample
	lineRate       float64
	peakRx         float64
	rt             []time.Duration // the response times of the last commands
	// errBase is the reading of the error counters at the connection: some
	// drivers count since they were loaded
	errBase *serial.Errors
}

func (s *stats) sample(sess *session.Session) {
	// The counters of the errors start at the connection
	if s.errBase == nil {
		if ec, ok := sess.Port().(serial.ErrorCounter); ok && sess.Port() != nil {
			if e, err := ec.Errors(); err == nil {
				s.errBase = &e
			}
		}
	}
	rx, tx := sess.Counters()
	if rx < s.lastRx || tx < s.lastTx { // the counters were reset
		s.lastRx, s.lastTx = rx, tx
	}
	s.rxRate = float64(rx - s.lastRx)
	s.txRate = float64(tx - s.lastTx)
	s.lineRate = float64(s.lines)
	s.peakRx = max(s.peakRx, s.rxRate)
	s.lastRx, s.lastTx, s.lines = rx, tx, 0
}

// count takes the lines of the chunks into the statistics
func (s *stats) count(chunks []buffer.Chunk) {
	for _, c := range chunks {
		if c.Dir == buffer.RX {
			s.lines += int64(bytes.Count(c.Data, []byte{'\n'}))
		}
	}
}

// formatRate writes a speed: 0 B/s, 12.4 KB/s
func formatRate(r float64) string { return formatBytes(int64(r)) + "/s" }

// showStatistics opens a window with the numbers of the session, updated
// every second
func (m *MainForm) showStatistics() {
	var c ui.DialogContent
	c.InitWidget()
	text := ui.NewLabel("")
	text.SetFontFamily(ui.FontFamilyMono)
	text.SetMinSize(460, 300)
	c.AddWidget(0, 0, text)
	c.AddVSpacer(1, 0)
	buttons := ui.NewDialogButtonBox()
	buttons.AddButton(T().ResetCounters, ui.ButtonOther, func() {
		m.sess.ResetCounters()
		m.stats.peakRx = 0
	})
	buttons.AddOK(func() { c.Form().Close() })
	c.AddWidget(2, 0, buttons)
	update := func() { text.SetText(m.statisticsText()) }
	update()
	c.AddTimer(1000, update)
	c.OnDialogShow = func() { c.Form().SetTitle(T().Statistics) }
	m.Form.Panel().ShowDialog(&c)
}

func (m *MainForm) statisticsText() string {
	t := T()
	state, info, cfg := m.sess.State()
	rx, tx := m.sess.Counters()
	var sb strings.Builder
	row := func(name, value string) { fmt.Fprintf(&sb, "%-22s %s\n", name, value) }
	if state == session.Closed {
		row(t.StatPort, t.StateClosed)
	} else {
		row(t.StatPort, fmt.Sprintf("%s  %d %s", info.Short, cfg.Baud, cfg.Frame()))
		row(t.StatUptime, time.Since(m.sess.OpenedAt()).Round(time.Second).String())
		row(t.StatReconnects, fmt.Sprint(m.sess.Reconnects()))
	}
	sb.WriteByte('\n')
	row(t.StatReceived, fmt.Sprintf("%s (%d)", formatBytes(rx), rx))
	row(t.StatSent, fmt.Sprintf("%s (%d)", formatBytes(tx), tx))
	row(t.StatRxRate, formatRate(m.stats.rxRate)+"   "+t.StatPeak+" "+formatRate(m.stats.peakRx))
	row(t.StatTxRate, formatRate(m.stats.txRate))
	row(t.StatLines, fmt.Sprintf("%.0f", m.stats.lineRate))
	if cfg.Baud > 0 && state != session.Closed {
		// The share of the line in use: bytes a second against what the speed carries
		capacity := float64(time.Second) / float64(cfg.ByteTime())
		row(t.StatLoad, fmt.Sprintf("%.1f %%", m.stats.rxRate*100/capacity))
	}
	sb.WriteByte('\n')
	if p := m.sess.Port(); p != nil {
		if ec, ok := p.(serial.ErrorCounter); ok {
			if e, err := ec.Errors(); err == nil {
				if m.stats.errBase == nil {
					b := e
					m.stats.errBase = &b
				}
				b := *m.stats.errBase
				e.Framing, e.Parity, e.Overrun, e.Break = e.Framing-b.Framing, e.Parity-b.Parity, e.Overrun-b.Overrun, e.Break-b.Break
				row(t.StatFraming, fmt.Sprint(e.Framing))
				row(t.StatParity, fmt.Sprint(e.Parity))
				row(t.StatOverrun, fmt.Sprint(e.Overrun))
				row(t.StatBreaks, fmt.Sprint(e.Break))
			} else {
				row(t.StatErrors, t.StatNoErrors)
			}
		} else {
			row(t.StatErrors, t.StatNoErrors)
		}
	}
	if n := len(m.stats.rt); n > 0 {
		lo, hi, sum := m.stats.rt[0], m.stats.rt[0], time.Duration(0)
		for _, d := range m.stats.rt {
			lo, hi, sum = min(lo, d), max(hi, d), sum+d
		}
		sb.WriteByte('\n')
		row(t.StatResponse, fmt.Sprintf("%s / %s / %s  (%d)", formatDuration(lo), formatDuration(sum/time.Duration(n)), formatDuration(hi), n))
	}
	return sb.String()
}

// noteResponse remembers the response times for the statistics: the time
// from a sent chunk to the first received one after it
func (m *MainForm) noteResponse(chunks []buffer.Chunk) {
	for _, c := range chunks {
		switch c.Dir {
		case buffer.TX:
			m.lastTX = c.Time
		case buffer.RX:
			if m.lastTX != 0 {
				m.stats.rt = append(m.stats.rt, time.Duration(c.Time-m.lastTX))
				if len(m.stats.rt) > 1000 {
					m.stats.rt = m.stats.rt[len(m.stats.rt)-1000:]
				}
				m.lastTX = 0
			}
		}
	}
}

// tap collects the received bytes of the session for a tool, while the
// chunks are kept out of the terminal
type tap struct {
	ch     chan []byte
	remove func()
}

func (m *MainForm) startTap(hide bool) *tap {
	t := &tap{ch: make(chan []byte, 4096)}
	t.remove = m.sess.AddTap(func(c buffer.Chunk) {
		if c.Dir != buffer.RX {
			return
		}
		select {
		case t.ch <- c.Data:
		default:
		}
	})
	m.sess.SetHidden(hide)
	return t
}

func (m *MainForm) stopTap(t *tap) {
	t.remove()
	m.sess.SetHidden(false)
}

// collect takes the bytes that come within d, or until n bytes came
func (t *tap) collect(ctx context.Context, d time.Duration, n int) []byte {
	var out []byte
	timer := time.NewTimer(d)
	defer timer.Stop()
	for n <= 0 || len(out) < n {
		select {
		case b := <-t.ch:
			out = append(out, b...)
		case <-timer.C:
			return out
		case <-ctx.Done():
			return out
		}
	}
	return out
}

// drain drops what came before
func (t *tap) drain() {
	for {
		select {
		case <-t.ch:
		default:
			return
		}
	}
}

// loopbackTest sends packets of random bytes and checks that they come
// back: a cable with TX joined to RX, or a device that echoes
func (m *MainForm) loopbackTest() {
	state, _, cfg := m.sess.State()
	if state != session.Open {
		m.status.ShowMessage(T().ErrNotOpen + " — " + T().PressConnect)
		return
	}
	const packets, size = 50, 64
	type result struct {
		sent, got, lost, bad int
		lo, hi, sum          time.Duration
		ok                   int
	}
	ui.RunWithProgress(m.Form.Panel(), T().Loopback, T().LoopbackRunning, func(ctx context.Context, p *ui.Progress) (result, error) {
		var r result
		t := m.startTap(true)
		defer m.stopTap(t)
		t.drain()
		// Time for the bytes to go and come back, and for the USB latency
		wait := 2*time.Duration(size)*cfg.ByteTime() + 300*time.Millisecond
		for i := 0; i < packets && ctx.Err() == nil; i++ {
			out := make([]byte, size)
			rand.Read(out)
			start := time.Now()
			if err := m.sess.Send(out, buffer.TX); err != nil {
				return r, err
			}
			r.sent += size
			in := t.collect(ctx, wait, size)
			d := time.Since(start)
			r.got += len(in)
			if len(in) < size {
				r.lost += size - len(in)
			}
			for k := 0; k < min(len(in), size); k++ {
				if in[k] != out[k] {
					r.bad++
				}
			}
			if len(in) >= size {
				if r.ok == 0 || d < r.lo {
					r.lo = d
				}
				r.hi = max(r.hi, d)
				r.sum += d
				r.ok++
			}
			p.Report(float64(i+1)/packets, fmt.Sprintf("%d / %d", i+1, packets))
		}
		return r, ctx.Err()
	}, func(r result, err error) {
		if err != nil && !ctxErr(err) {
			ui.ShowMessageBox(m.Form.Panel(), T().Loopback, m.errorText(err))
			return
		}
		text := fmt.Sprintf(T().LoopbackResult, r.sent, r.got, r.lost, r.bad)
		if r.ok > 0 {
			text += "\n" + fmt.Sprintf(T().LoopbackTime, formatDuration(r.lo), formatDuration(r.sum/time.Duration(r.ok)), formatDuration(r.hi))
		}
		switch {
		case r.got == 0:
			text += "\n\n" + T().LoopbackNothing
		case r.lost == 0 && r.bad == 0:
			text += "\n\n" + T().LoopbackOK
		}
		ui.ShowMessageBox(m.Form.Panel(), T().Loopback, text)
	})
}

// detectBauds are the speeds tried, the most common first
var detectBauds = []int{115200, 9600, 57600, 38400, 19200, 230400, 74880, 460800, 921600,
	250000, 500000, 1000000, 2000000, 4800, 2400, 1200}

// textScore is how much the bytes look like text: the share of printable
// ASCII and line breaks, with a bonus for having line breaks at all
func textScore(b []byte) float64 {
	if len(b) < 8 {
		return 0
	}
	good, breaks := 0, 0
	for _, c := range b {
		switch {
		case c == '\n' || c == '\r':
			good++
			breaks++
		case c == '\t' || c >= 0x20 && c < 0x7F:
			good++
		}
	}
	s := float64(good) / float64(len(b))
	if breaks > 0 {
		s += 0.05
	}
	return s
}

// detectBaud tries the speeds on the data the device sends and takes the
// one at which it looks like text
func (m *MainForm) detectBaud() {
	state, _, cfg := m.sess.State()
	if state != session.Open || m.sess.Port() == nil {
		m.status.ShowMessage(T().ErrNotOpen + " — " + T().PressConnect)
		return
	}
	orig := cfg
	type found struct {
		baud  int
		score float64
	}
	ui.RunWithProgress(m.Form.Panel(), T().DetectBaud, T().DetectBaudRunning, func(ctx context.Context, p *ui.Progress) (found, error) {
		best := found{}
		t := m.startTap(true)
		defer m.stopTap(t)
		for i, b := range detectBauds {
			if ctx.Err() != nil {
				break
			}
			port := m.sess.Port()
			if port == nil {
				return best, session.ErrNotOpen
			}
			c := orig
			c.Baud = b
			if err := port.SetConfig(c); err != nil {
				continue
			}
			p.Report(float64(i)/float64(len(detectBauds)), fmt.Sprint(b))
			time.Sleep(30 * time.Millisecond)
			t.drain()
			data := t.collect(ctx, 600*time.Millisecond, 4096)
			if s := textScore(data); s > best.score {
				best = found{b, s}
			}
			if best.score >= 0.98 {
				break
			}
		}
		if port := m.sess.Port(); port != nil {
			port.SetConfig(orig)
		}
		return best, ctx.Err()
	}, func(f found, err error) {
		if ctxErr(err) {
			return
		}
		if f.score < 0.85 {
			ui.ShowMessageBox(m.Form.Panel(), T().DetectBaud, T().DetectBaudNone)
			return
		}
		c := orig
		c.Baud = f.baud
		m.applyConfig(c)
		m.status.ShowMessage(fmt.Sprintf(T().DetectBaudFound, f.baud))
	})
}
