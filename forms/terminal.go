package forms

import (
	"fmt"
	"image/color"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/ipoluianov/altserialport/buffer"
	"github.com/ipoluianov/altserialport/render"
	"github.com/ipoluianov/altserialport/rules"
	"github.com/ipoluianov/nui/ui"
)

// TimeMode is how the time of a row is shown at its left
type TimeMode int

const (
	TimeOff   TimeMode = iota
	TimeClock          // 12:00:01.234
	TimeMicro          // 12:00:01.234567
	TimeDelta          // +0.012: since the row before
	TimeSince          // 12.345: since the port was opened
)

var timeModeNames = []string{"", "time", "micro", "delta", "since"}

func timeModeFromName(s string) TimeMode {
	for i, n := range timeModeNames {
		if n == s {
			return TimeMode(i)
		}
	}
	return TimeOff
}

// cellPos is a place in the terminal: a row and a cell of it
type cellPos struct {
	row, cell int
}

func (a cellPos) less(b cellPos) bool {
	return a.row < b.row || a.row == b.row && a.cell < b.cell
}

// match is a found text: the cells [from, to) of a row
type match struct {
	row, from, to int
}

const (
	textPad     = 8 // the room at the left of the text, the mark of the sent rows in it
	maxCache    = 8192
	maxMatches  = 1_000_000
	wheelLines  = 3
	pillPadding = 10
)

// Terminal shows the rows of a buffer: a view of millions of rows that
// paints only those on the screen. Typing in it sends keys.
type Terminal struct {
	ui.Panel
	area *ui.Widget
	vbar *ui.ScrollBar
	hbar *ui.ScrollBar

	buf      *buffer.Buffer
	enc      render.Encoding
	timeMode TimeMode
	wrap     bool
	fontSize float64
	origin   int64 // the time "since" counts from

	top, topSub int // the row at the top and its line there (a wrapped row has several)
	follow      bool
	paused      bool
	pauseEnd    int
	hscroll     int
	newRows     bool // rows came while the view is not at the end
	maxCols     int  // the widest line painted, for the horizontal bar

	cw float64 // the width of a character
	lh int     // the height of a line

	cache   map[int][]render.Cell
	scratch []byte
	typed   []bool

	hasSel, selecting bool
	selA, selB        cellPos
	dragTimer         *ui.Timer
	dragY             int

	query   []rune
	regex   *regexp.Regexp // the query as a regular expression, when asked
	matches []match
	current int
	scanned int // the rows looked through for the query

	filterOn bool  // only the rows with matches are shown
	fRows    []int // the rows with matches, in order

	ruleSet   *rules.Set
	ruleColor map[int]string // the color of the rules of a row, "" none
	showRT    bool           // the response time after the sent rows

	pillRect [4]int // the place of the "new data" pill, for clicks

	// OnSend gets the bytes of the keys typed
	OnSend func(data []byte)
	// Hint gives the lines shown while there is no data
	Hint func() []string
	// PillText gives the text of the pill shown while the view is not at the end
	PillText func(paused bool) string
	// OnScrolled tells that the view left the end or came back to it
	OnScrolled func(follow bool)
	// KeyBytes turns a key into the bytes to send, nil for none
	KeyBytes func(e *ui.KeyEvent) []byte
}

// NewTerminal makes a view of the buffer
func NewTerminal(buf *buffer.Buffer) *Terminal {
	t := &Terminal{buf: buf, wrap: true, fontSize: 14, follow: true, showRT: true,
		cache: map[int][]render.Cell{}, ruleColor: map[int]string{}}
	t.InitWidget()
	t.SetPanelPadding(0)
	t.SetCellPadding(0)

	t.area = &ui.Widget{}
	t.area.InitWidget()
	t.area.SetXExpandable(true)
	t.area.SetYExpandable(true)
	t.area.SetMinSize(200, 100)
	t.area.SetCanBeFocused(true)
	t.area.SetMouseCursor(ui.MouseCursorIBeam)
	t.area.SetOnPaint(t.paint)
	t.area.OnMouseDown.Connect(t.mouseDown)
	t.area.OnMouseMove.Connect(t.mouseMove)
	t.area.OnMouseUp.Connect(t.mouseUp)
	t.area.OnMouseDoubleClick.Connect(t.mouseDoubleClick)
	t.area.OnWheel.Connect(t.wheel)
	t.area.OnKeyDown.Connect(t.keyDown)
	t.area.OnChar.Connect(t.char)
	t.area.OnResize.Connect(func(ui.Size) { t.updateBars() })

	t.vbar = ui.NewScrollBar(true)
	t.vbar.OnActivated.Connect(func(v int) {
		t.top = t.buf.FirstRow() + v
		if t.filtered() {
			t.top = t.fRows[min(v, len(t.fRows)-1)]
		}
		t.topSub = 0
		t.setFollow(v >= t.vbar.Max() && !t.paused)
		t.area.Update()
	})
	t.hbar = ui.NewScrollBar(false)
	t.hbar.OnActivated.Connect(func(v int) {
		t.hscroll = v
		t.area.Update()
	})
	t.hbar.SetVisible(false)

	t.AddWidget(0, 0, t.area)
	t.AddWidget(0, 1, t.vbar)
	t.AddWidget(1, 0, t.hbar)

	t.dragTimer = t.area.NewTimer(50*time.Millisecond, t.dragScroll)
	return t
}

// Focus gives the keyboard to the terminal
func (t *Terminal) Focus() { t.area.Focus() }

// SetEncoding sets the encoding of the text
func (t *Terminal) SetEncoding(e render.Encoding) {
	t.enc = e
	t.invalidate()
}

// Encoding returns the encoding of the text
func (t *Terminal) Encoding() render.Encoding { return t.enc }

// SetTimeMode sets the time shown at the left of the rows
func (t *Terminal) SetTimeMode(m TimeMode) {
	t.timeMode = m
	t.area.Update()
}

// TimeMode returns the time shown at the left of the rows
func (t *Terminal) TimeMode() TimeMode { return t.timeMode }

// SetOrigin sets the time "since" counts from
func (t *Terminal) SetOrigin(at time.Time) {
	t.origin = at.UnixNano()
	t.area.Update()
}

// SetWrap wraps the long lines, or scrolls them sideways
func (t *Terminal) SetWrap(w bool) {
	t.wrap = w
	t.hscroll = 0
	t.updateBars()
	t.area.Update()
}

// Wrap tells whether the long lines are wrapped
func (t *Terminal) Wrap() bool { return t.wrap }

// SetFontSize sets the size of the text
func (t *Terminal) SetFontSize(s float64) {
	t.fontSize = math.Max(8, math.Min(40, s))
	t.cw = 0
	t.updateBars()
	t.area.Update()
}

// FontSize returns the size of the text
func (t *Terminal) FontSize() float64 { return t.fontSize }

// SetLayout lays the rows out again with the options; the row at the top
// stays at the top
func (t *Terminal) SetLayout(o buffer.Options) {
	var anchor buffer.Pos
	keep := !t.follow && t.buf.Has(t.top)
	if keep {
		anchor = t.buf.Row(t.top).Start
	}
	t.buf.SetOptions(o)
	t.invalidate()
	t.hasSel = false
	if t.paused {
		t.pauseEnd = t.buf.EndRow()
	}
	t.top, t.topSub = t.buf.FirstRow(), 0
	if keep {
		t.top = t.rowAt(anchor)
	}
	t.rescan()
	t.updateBars()
}

// rowAt finds the row that holds the place
func (t *Terminal) rowAt(p buffer.Pos) int {
	first, n := t.buf.FirstRow(), t.buf.RowCount()
	k := sort.Search(n, func(k int) bool {
		e := t.buf.Row(first + k).End
		return e.Chunk > p.Chunk || e.Chunk == p.Chunk && e.Off > p.Off
	})
	return min(first+k, max(first, t.buf.EndRow()-1))
}

func (t *Terminal) invalidate() {
	clear(t.cache)
	clear(t.ruleColor)
	t.area.Update()
}

// Append adds the chunks to the buffer and shows them
func (t *Terminal) Append(chunks []buffer.Chunk) {
	if len(chunks) == 0 {
		return
	}
	total := t.buf.Total()
	// The last row may grow
	delete(t.cache, t.buf.EndRow()-1)
	for _, c := range chunks {
		t.buf.Append(c)
	}
	delete(t.cache, t.buf.EndRow()-1)
	delete(t.ruleColor, t.buf.EndRow()-1)
	if len(t.cache) > maxCache {
		clear(t.cache)
		clear(t.ruleColor)
	}
	if t.top < t.buf.FirstRow() {
		t.top, t.topSub = t.firstVisible(), 0
	}
	if t.hasSel && t.selA.row < t.buf.FirstRow() {
		t.hasSel = false
	}
	if (!t.follow || t.paused) && t.buf.Total() != total {
		t.newRows = true
	}
	t.scan()
	t.updateBars()
	t.area.Update()
}

// Clear drops all the rows
func (t *Terminal) Clear() {
	t.buf.Clear()
	t.invalidate()
	t.hasSel = false
	t.top, t.topSub = t.buf.FirstRow(), 0
	t.pauseEnd = t.buf.EndRow()
	t.matches = nil
	t.current = -1
	t.scanned = t.buf.EndRow()
	t.newRows = false
	t.setFollow(true)
	t.updateBars()
}

// SetPaused freezes the view; the rows keep coming to the buffer
func (t *Terminal) SetPaused(p bool) {
	t.paused = p
	t.pauseEnd = t.buf.EndRow()
	t.newRows = false
	if !p {
		t.setFollow(true)
	}
	t.updateBars()
	t.area.Update()
}

// Paused tells whether the view is frozen
func (t *Terminal) Paused() bool { return t.paused }

// Following tells whether the view follows the new rows
func (t *Terminal) Following() bool { return t.follow && !t.paused }

// ScrollToEnd shows the newest rows and follows them
func (t *Terminal) ScrollToEnd() {
	if t.paused {
		t.SetPaused(false)
		return
	}
	t.setFollow(true)
	t.updateBars()
	t.area.Update()
}

// ScrollToStart shows the oldest rows
func (t *Terminal) ScrollToStart() {
	t.top, t.topSub = t.firstVisible(), 0
	t.setFollow(false)
	t.updateBars()
	t.area.Update()
}

func (t *Terminal) setFollow(f bool) {
	if f {
		t.newRows = false
	}
	if t.follow == f {
		return
	}
	t.follow = f
	if t.OnScrolled != nil {
		t.OnScrolled(f)
	}
}

// end is the row after the last one shown
func (t *Terminal) end() int {
	if t.paused {
		return max(t.buf.FirstRow(), min(t.pauseEnd, t.buf.EndRow()))
	}
	return t.buf.EndRow()
}

// measure takes the size of a character of the font
func (t *Terminal) measure() {
	if t.cw > 0 {
		return
	}
	const probe = "00000000000000000000000000000000"
	w, h, err := ui.MeasureText(ui.FontFamilyMono, t.fontSize, probe)
	if err != nil || w == 0 {
		w, h = len(probe)*8, 16
	}
	t.cw = float64(w) / float64(len(probe))
	t.lh = h + 2
}

func (t *Terminal) visibleLines() int {
	t.measure()
	return max(1, t.area.Height()/t.lh)
}

// gutterCols is the width of the time at the left, in characters
func (t *Terminal) gutterCols() int {
	switch t.timeMode {
	case TimeClock:
		return 13
	case TimeMicro:
		return 16
	case TimeDelta, TimeSince:
		return 11
	}
	return 0
}

// textCols is the characters of the text that fit in a line
func (t *Terminal) textCols() int {
	t.measure()
	w := t.area.Width() - textPad - 4
	return max(8, int(float64(w)/t.cw)-t.gutterCols())
}

// continues tells whether the row goes on the packet of the row before
// it: in the hex view a long packet takes several rows
func (t *Terminal) continues(i int, r buffer.Row) bool {
	if t.buf.Options().Mode != buffer.ModeHex || i <= t.buf.FirstRow() || r.Dir == buffer.Info {
		return false
	}
	prev := t.buf.Row(i - 1)
	return prev.Dir == r.Dir && prev.End == r.Start && int(prev.Len) == t.buf.Options().HexWidth
}

// packetOffset is the offset of the row in its packet
func (t *Terminal) packetOffset(i int, r buffer.Row) int {
	off := 0
	for k := i; k > t.buf.FirstRow() && off < 1<<20 && t.continues(k, t.buf.Row(k)); k-- {
		off += t.buf.Options().HexWidth
	}
	return off
}

func (t *Terminal) gutter(i int, r buffer.Row) string {
	if t.timeMode != TimeOff && t.continues(i, r) {
		return fmt.Sprintf("%*s", t.gutterCols()-1, fmt.Sprintf("+%04X", t.packetOffset(i, r)))
	}
	switch t.timeMode {
	case TimeClock:
		return time.Unix(0, r.Time).Format("15:04:05.000")
	case TimeMicro:
		return time.Unix(0, r.Time).Format("15:04:05.000000")
	case TimeDelta:
		if i > t.buf.FirstRow() {
			d := time.Duration(r.Time - t.buf.Row(i-1).Time)
			return fmt.Sprintf("%+10.3f", d.Seconds())
		}
		return fmt.Sprintf("%+10.3f", 0.0)
	case TimeSince:
		if t.origin == 0 || r.Time < t.origin {
			return ""
		}
		return fmt.Sprintf("%10.3f", time.Duration(r.Time-t.origin).Seconds())
	}
	return ""
}

// cells returns the rendered row
func (t *Terminal) cells(i int) []render.Cell {
	if c, ok := t.cache[i]; ok {
		return c
	}
	cells := t.renderRow(nil, i)
	t.cache[i] = cells
	return cells
}

// renderRow renders the row into dst, without keeping it
func (t *Terminal) renderRow(dst []render.Cell, i int) []render.Cell {
	r := t.buf.Row(i)
	opts := t.buf.Options()
	cells := dst[:0]
	if r.Dir == buffer.Info {
		t.scratch = t.buf.Data(t.scratch[:0], r)
		cells = render.NoteCells(cells, string(t.scratch))
	} else {
		t.scratch = t.scratch[:0]
		t.typed = t.typed[:0]
		echo := false
		t.buf.Segments(r, func(d buffer.Dir, data []byte) {
			t.scratch = append(t.scratch, data...)
			for range data {
				t.typed = append(t.typed, d == buffer.Keys)
			}
			echo = echo || d == buffer.Keys
		})
		in := render.Input{Data: t.scratch}
		if echo {
			in.Typed = t.typed
		}
		switch opts.Mode {
		case buffer.ModeHex:
			cells = render.Hex(cells, in, opts.HexWidth)
		case buffer.ModeMixed:
			cells = render.Mixed(cells, in, t.enc)
		default:
			cells = render.Text(cells, in, t.enc)
		}
	}
	return cells
}

// lines is the lines the row takes on the screen
func (t *Terminal) lines(i int) int {
	if !t.wrap {
		return 1
	}
	n := len(t.cells(i))
	cols := t.textCols()
	return max(1, (n+cols-1)/cols)
}

// lastTop is the top of the view at the end: the last line at the bottom
func (t *Terminal) lastTop() (int, int) {
	need := t.visibleLines()
	got := 0
	first := t.buf.FirstRow()
	for i := t.prevRow(t.end()); i >= first; i = t.prevRow(i) {
		n := t.lines(i)
		if got+n >= need {
			return i, got + n - need
		}
		got += n
	}
	return t.firstVisible(), 0
}

// SetRules sets the rules that color the rows
func (t *Terminal) SetRules(s *rules.Set) {
	t.ruleSet = s
	clear(t.ruleColor)
	t.area.Update()
}

// rowColor is the color the rules give the row, "" for none
func (t *Terminal) rowColor(i int) string {
	if t.ruleSet.Empty() {
		return ""
	}
	if c, ok := t.ruleColor[i]; ok {
		return c
	}
	r := t.buf.Row(i)
	t.scratch = t.buf.Data(t.scratch[:0], r)
	c := t.ruleSet.Color(render.String(t.cells(i)), t.scratch)
	t.ruleColor[i] = c
	return c
}

// SetResponseTime shows the time from a sent row to the answer
func (t *Terminal) SetResponseTime(on bool) {
	t.showRT = on
	t.area.Update()
}

// ResponseTime tells whether the time of the answers is shown
func (t *Terminal) ResponseTime() bool { return t.showRT }

// responseTime is the time from the sent row to the first byte received
// after it; ok is false while nothing came
func (t *Terminal) responseTime(i int, r buffer.Row) (time.Duration, bool) {
	for ci := r.End.Chunk; ci < r.End.Chunk+4096; ci++ {
		c := t.buf.Chunk(ci)
		if c == nil {
			return 0, false
		}
		want := buffer.RX
		if r.Dir == buffer.TX2 {
			want = buffer.RX2
		}
		if c.Dir == want && c.Time >= r.Time {
			return time.Duration(c.Time - r.Time), true
		}
		if ci > r.End.Chunk && c.Dir == r.Dir {
			return 0, false // the next command went before an answer
		}
	}
	return 0, false
}

// formatDuration writes a short time: 850 µs, 12.3 ms, 1.25 s
func formatDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%d µs", d.Microseconds())
	case d < time.Second:
		return fmt.Sprintf("%.1f ms", float64(d)/float64(time.Millisecond))
	}
	return fmt.Sprintf("%.2f s", d.Seconds())
}

// filtered tells whether only the rows with matches are shown
func (t *Terminal) filtered() bool { return t.filterOn && t.hasQuery() }

func (t *Terminal) hasQuery() bool { return len(t.query) > 0 || t.regex != nil }

// nextRow is the row shown after the row i, or the end
func (t *Terminal) nextRow(i int) int {
	if !t.filtered() {
		return i + 1
	}
	k := sort.SearchInts(t.fRows, i+1)
	if k < len(t.fRows) && t.fRows[k] < t.end() {
		return t.fRows[k]
	}
	return t.end()
}

// prevRow is the row shown before the row i, or one before the first row
func (t *Terminal) prevRow(i int) int {
	if !t.filtered() {
		return i - 1
	}
	k := sort.SearchInts(t.fRows, i) - 1
	for k >= 0 && t.fRows[k] >= t.end() {
		k--
	}
	if k >= 0 {
		return t.fRows[k]
	}
	return t.buf.FirstRow() - 1
}

// firstVisible is the first row shown
func (t *Terminal) firstVisible() int {
	return t.nextRow(t.buf.FirstRow() - 1)
}

// SetFilter shows only the rows with matches of the query, or all
func (t *Terminal) SetFilter(on bool) {
	t.filterOn = on
	t.hasSel = false
	t.top, t.topSub = t.firstVisible(), 0
	t.setFollow(true)
	t.updateBars()
	t.area.Update()
}

// Filter tells whether only the rows with matches are shown
func (t *Terminal) Filter() bool { return t.filterOn }

// clampTop keeps the top within the rows, and the view follows once it is
// at the end
func (t *Terminal) clampTop() {
	first := t.buf.FirstRow()
	if t.top < first {
		t.top, t.topSub = t.firstVisible(), 0
	}
	if t.filtered() && t.nextRow(t.top-1) != t.top {
		t.top, t.topSub = t.nextRow(t.top-1), 0
	}
	lt, ls := t.lastTop()
	if t.follow || t.top > lt || t.top == lt && t.topSub >= ls {
		t.top, t.topSub = lt, ls
		return
	}
	if n := t.lines(t.top); t.topSub >= n {
		t.topSub = n - 1
	}
}

// scrollLines moves the view by lines: up for negative
func (t *Terminal) scrollLines(delta int) {
	t.clampTop()
	first := t.buf.FirstRow()
	for ; delta < 0; delta++ {
		if t.topSub > 0 {
			t.topSub--
		} else if p := t.prevRow(t.top); p >= first {
			t.top = p
			t.topSub = t.lines(t.top) - 1
		} else {
			break
		}
		t.setFollow(false)
	}
	lt, ls := t.lastTop()
	for ; delta > 0; delta-- {
		if t.top > lt || t.top == lt && t.topSub >= ls {
			break
		}
		if t.topSub < t.lines(t.top)-1 {
			t.topSub++
		} else {
			t.top = t.nextRow(t.top)
			t.topSub = 0
		}
	}
	if t.top > lt || t.top == lt && t.topSub >= ls {
		t.top, t.topSub = lt, ls
		t.setFollow(!t.paused)
	}
	t.updateBars()
	t.area.Update()
}

func (t *Terminal) updateBars() {
	if t.area.Width() == 0 {
		return
	}
	n := t.buf.RowCount()
	page := t.visibleLines()
	if t.follow {
		t.clampTop()
	}
	pos := t.top - t.buf.FirstRow()
	if t.filtered() {
		n = len(t.fRows)
		pos = sort.SearchInts(t.fRows, t.top)
	}
	t.vbar.SetRange(0, max(0, n-1))
	t.vbar.SetPageStep(page)
	t.vbar.SetValue(pos)
	if !t.wrap {
		cols := t.textCols()
		t.hbar.SetVisible(t.maxCols > cols)
		t.hbar.SetRange(0, max(0, t.maxCols-cols))
		t.hbar.SetPageStep(cols)
		t.hbar.SetValue(t.hscroll)
	} else {
		t.hbar.SetVisible(false)
	}
}

// paint draws the lines on the screen
func (t *Terminal) paint(cnv *ui.Canvas) {
	w, h := t.area.Width(), t.area.Height()
	back := colorTermBack.get()
	cnv.FillRect(0, 0, w, h, back)
	t.measure()
	cnv.SetFontFamily(ui.FontFamilyMono)
	cnv.SetFontSize(t.fontSize)
	cnv.SetHAlign(ui.HAlignLeft)
	cnv.SetVAlign(ui.VAlignCenter)

	if t.buf.RowCount() == 0 {
		t.paintHint(cnv, w, h)
		return
	}
	t.clampTop()
	cols := t.textCols()
	gutter := t.gutterCols()
	x0 := textPad
	if gutter > 0 {
		x0 += int(math.Round(float64(gutter) * t.cw))
	}
	t.maxCols = 0
	y := 0
	end := t.end()
	hexView := t.buf.Options().Mode == buffer.ModeHex
	for i, sub := t.top, t.topSub; y < h && i < end; i, sub = t.nextRow(i), 0 {
		row := t.buf.Row(i)
		cells := t.cells(i)
		t.maxCols = max(t.maxCols, len(cells))
		n := t.lines(i)
		for s := sub; s < n && y < h; s++ {
			from, to := 0, len(cells)
			if t.wrap {
				from = s * cols
				to = min(len(cells), from+cols)
			} else {
				from = min(len(cells), t.hscroll)
				to = min(len(cells), from+cols)
			}
			if s == 0 && gutter > 0 {
				cnv.SetColor(colorGutter.get())
				cnv.SetFontBold(false)
				cnv.SetFontItalic(false)
				cnv.SetUnderline(false)
				cnv.DrawText(textPad/2, y, x0, t.lh, t.gutter(i, row))
			}
			switch row.Dir {
			case buffer.TX:
				cnv.FillRect(2, y+1, 2, t.lh-2, colorSent.get())
			case buffer.RX2, buffer.TX2:
				cnv.FillRect(2, y+1, 2, t.lh-2, colorPortB.get())
			}
			// A line between the packets of the hex view
			if s == 0 && y > 0 && hexView && !t.continues(i, row) {
				cnv.FillRect(textPad, y, w-textPad*2, 1, mix(colorGutter.get(), back, 0.25))
			}
			t.paintLine(cnv, i, row, cells[from:to], from, x0, y)
			if row.Dir.Sent() && t.showRT && s == n-1 {
				if d, ok := t.responseTime(i, row); ok {
					cnv.SetColor(colorGutter.get())
					cnv.SetFontBold(false)
					cnv.SetFontItalic(false)
					cnv.SetUnderline(false)
					x := x0 + int(math.Round(float64(to-from+2)*t.cw))
					cnv.DrawText(x, y, w-x, t.lh, "→ "+formatDuration(d))
				}
			}
			y += t.lh
		}
	}
	if !t.wrap && t.hbar.IsVisible() != (t.maxCols > cols) {
		t.updateBars()
	}
	t.paintPill(cnv, w, h)
}

// style is what a run of cells is drawn with
type style struct {
	fg                     color.RGBA
	bold, italic, underlin bool
}

// paintLine draws the cells of a line; first is the index of cells[0] in the row
func (t *Terminal) paintLine(cnv *ui.Canvas, i int, row buffer.Row, cells []render.Cell, first, x0, y int) {
	back := colorTermBack.get()
	text := colorTermText.get()
	switch row.Dir {
	case buffer.TX:
		text = colorSent.get()
	case buffer.TX2:
		text = mix(colorPortB.get(), colorSent.get(), 0.5)
	case buffer.Info:
		text = colorNote.get()
	case buffer.RX2:
		text = colorPortB.get()
	default:
		if c := t.rowColor(i); c != "" {
			text = hex(c)
		}
	}
	xAt := func(c int) int { return x0 + int(math.Round(float64(c)*t.cw)) }

	// The backgrounds: of the cells, the matches, the selection
	for k := range cells {
		if cells[k].BG != 0 || cells[k].A&render.Inverse != 0 {
			bg := cellColor(cells[k].BG, back)
			if cells[k].A&render.Inverse != 0 {
				bg = cellColor(cells[k].FG, text)
			}
			cnv.FillRect(xAt(k), y, xAt(k+1)-xAt(k), t.lh, bg)
		}
	}
	for mi := t.firstMatch(i); mi < len(t.matches) && t.matches[mi].row == i; mi++ {
		m := t.matches[mi]
		a, b := max(m.from-first, 0), min(m.to-first, len(cells))
		if a < b {
			col := colorMatch.get()
			if mi == t.current {
				col = colorMatchCur.get()
			}
			cnv.FillRect(xAt(a), y, xAt(b)-xAt(a), t.lh, col)
		}
	}
	if t.hasSel {
		sa, sb := t.selection()
		if i >= sa.row && i <= sb.row {
			a, b := 0, len(cells)
			if i == sa.row {
				a = sa.cell - first
			}
			if i == sb.row {
				b = sb.cell - first
			}
			a, b = max(a, 0), min(b, len(cells))
			if a < b {
				cnv.FillRect(xAt(a), y, xAt(b)-xAt(a), t.lh, colorSelection.get())
			} else if i < sb.row && first+len(cells) == len(t.cells(i)) && a <= len(cells) {
				// The end of a selected line: a little mark of its break
				cnv.FillRect(xAt(len(cells)), y, int(t.cw/2)+1, t.lh, colorSelection.get())
			}
		}
	}

	// The text, by runs of one style
	start := 0
	var sb strings.Builder
	cur := t.cellStyle(cells, 0, text, back)
	flush := func(end int) {
		if sb.Len() == 0 {
			return
		}
		cnv.SetColor(cur.fg)
		cnv.SetFontBold(cur.bold)
		cnv.SetFontItalic(cur.italic)
		cnv.SetUnderline(cur.underlin)
		cnv.DrawText(xAt(start), y, xAt(end)-xAt(start)+int(t.cw)*2, t.lh, sb.String())
		sb.Reset()
	}
	for k := range cells {
		st := t.cellStyle(cells, k, text, back)
		if st != cur {
			flush(k)
			start = k
			cur = st
		}
		r := cells[k].R
		if r == ' ' && sb.Len() == 0 {
			start = k + 1
			continue
		}
		sb.WriteRune(r)
	}
	flush(len(cells))
}

func (t *Terminal) cellStyle(cells []render.Cell, k int, text, back color.RGBA) style {
	if k >= len(cells) {
		return style{fg: text}
	}
	c := cells[k]
	fg := cellColor(c.FG, text)
	switch {
	case c.A&render.Token != 0:
		fg = colorToken.get()
	case c.A&render.Typed != 0:
		fg = colorSent.get()
	}
	if c.A&render.Inverse != 0 {
		fg = cellColor(c.BG, back)
	}
	if c.A&render.Dim != 0 {
		fg = mix(fg, back, 0.5)
	}
	return style{fg: fg, bold: c.A&render.Bold != 0, italic: c.A&render.Italic != 0, underlin: c.A&render.Underline != 0}
}

func (t *Terminal) paintHint(cnv *ui.Canvas, w, h int) {
	if t.Hint == nil {
		return
	}
	lines := t.Hint()
	cnv.SetFontFamily(ui.FontFamilySans)
	cnv.SetHAlign(ui.HAlignCenter)
	lh := int(float64(t.lh) * 1.6)
	y := (h - lh*len(lines)) / 2
	for k, line := range lines {
		if k == 0 {
			cnv.SetFontSize(t.fontSize + 4)
			cnv.SetColor(colorTermText.get())
		} else {
			cnv.SetFontSize(t.fontSize)
			cnv.SetColor(colorHint.get())
		}
		cnv.DrawText(0, y, w, lh, line)
		y += lh
	}
	cnv.SetFontFamily(ui.FontFamilyMono)
	cnv.SetFontSize(t.fontSize)
	cnv.SetHAlign(ui.HAlignLeft)
}

// paintPill draws the button that brings the view back to the new data
func (t *Terminal) paintPill(cnv *ui.Canvas, w, h int) {
	t.pillRect = [4]int{}
	if t.PillText == nil || !(t.paused || !t.follow && t.newRows) {
		return
	}
	text := t.PillText(t.paused)
	cnv.SetFontFamily(ui.FontFamilySans)
	cnv.SetFontSize(t.fontSize - 1)
	cnv.SetFontBold(false)
	cnv.SetFontItalic(false)
	cnv.SetUnderline(false)
	tw, th, _ := ui.MeasureText(ui.FontFamilySans, t.fontSize-1, text)
	pw, ph := tw+pillPadding*2, th+10
	x, y := (w-pw)/2, h-ph-12
	cnv.DrawShadow(float64(x), float64(y+2), float64(pw), float64(ph), float64(ph)/2, 8, color.RGBA{0, 0, 0, 90})
	cnv.SetColor(colorPill.get())
	cnv.FillRoundedRect(x, y, pw, ph, ph/2)
	cnv.SetColor(color.RGBA{255, 255, 255, 255})
	cnv.SetHAlign(ui.HAlignCenter)
	cnv.DrawText(x, y, pw, ph, text)
	cnv.SetHAlign(ui.HAlignLeft)
	cnv.SetFontFamily(ui.FontFamilyMono)
	cnv.SetFontSize(t.fontSize)
	t.pillRect = [4]int{x, y, pw, ph}
}

// hit returns the place under the point; ok is false below the last row
func (t *Terminal) hit(x, y int) (cellPos, bool) {
	t.measure()
	cols := t.textCols()
	x0 := textPad + int(math.Round(float64(t.gutterCols())*t.cw))
	col := int(math.Round(float64(x-x0) / t.cw))
	col = max(0, min(col, cols))
	line := y / t.lh
	if y < 0 {
		line = -1
	}
	i, sub := t.top, t.topSub
	end := t.end()
	if line < 0 {
		return cellPos{t.top, 0}, true
	}
	for ; i < end; i, sub = t.nextRow(i), 0 {
		n := t.lines(i)
		if line < n-sub {
			s := sub + line
			cell := col
			if t.wrap {
				cell = s*cols + col
			} else {
				cell = t.hscroll + col
			}
			return cellPos{i, min(cell, len(t.cells(i)))}, true
		}
		line -= n - sub
	}
	if last := t.prevRow(end); last >= t.buf.FirstRow() {
		return cellPos{last, len(t.cells(last))}, false
	}
	return cellPos{}, false
}

func (t *Terminal) inPill(x, y int) bool {
	r := t.pillRect
	return r[2] > 0 && x >= r[0] && x < r[0]+r[2] && y >= r[1] && y < r[1]+r[3]
}

func (t *Terminal) mouseDown(e *ui.MouseEvent) {
	t.area.Focus()
	if e.Button != ui.MouseButtonLeft {
		return
	}
	if t.inPill(e.X, e.Y) {
		t.ScrollToEnd()
		return
	}
	if t.buf.RowCount() == 0 {
		return
	}
	p, _ := t.hit(e.X, e.Y)
	if e.Mods.Shift && t.hasSel {
		t.selB = p
	} else {
		t.selA, t.selB = p, p
	}
	t.hasSel = true
	t.selecting = true
	t.area.Update()
}

func (t *Terminal) mouseMove(e *ui.MouseEvent) {
	if !t.selecting {
		if t.inPill(e.X, e.Y) {
			t.area.SetMouseCursor(ui.MouseCursorPointer)
		} else {
			t.area.SetMouseCursor(ui.MouseCursorIBeam)
		}
		return
	}
	t.dragY = e.Y
	if e.Y < 0 || e.Y >= t.area.Height() {
		t.dragTimer.Start()
	} else {
		t.dragTimer.Stop()
	}
	p, _ := t.hit(e.X, e.Y)
	t.selB = p
	t.area.Update()
}

func (t *Terminal) mouseUp(e *ui.MouseEvent) {
	if e.Button != ui.MouseButtonLeft {
		return
	}
	t.selecting = false
	t.dragTimer.Stop()
	if t.selA == t.selB {
		t.hasSel = false
	}
	t.area.Update()
}

// dragScroll scrolls while a selection is dragged above or below the view
func (t *Terminal) dragScroll() {
	if !t.selecting {
		t.dragTimer.Stop()
		return
	}
	if t.dragY < 0 {
		t.scrollLines(-1 - (-t.dragY)/t.lh)
		t.selB, _ = t.hit(0, -1)
	} else {
		t.scrollLines(1 + (t.dragY-t.area.Height())/t.lh)
		p, _ := t.hit(t.area.Width(), t.area.Height()-1)
		t.selB = p
	}
}

func (t *Terminal) mouseDoubleClick(e *ui.MouseEvent) {
	if e.Button != ui.MouseButtonLeft || t.buf.RowCount() == 0 {
		return
	}
	p, ok := t.hit(e.X, e.Y)
	if !ok {
		return
	}
	cells := t.cells(p.row)
	word := func(k int) bool {
		if k < 0 || k >= len(cells) {
			return false
		}
		r := cells[k].R
		return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_.-:/", r)
	}
	a, b := p.cell, p.cell
	for word(a - 1) {
		a--
	}
	for word(b) {
		b++
	}
	if a == b {
		return
	}
	t.selA, t.selB = cellPos{p.row, a}, cellPos{p.row, b}
	t.hasSel = true
	t.selecting = false
	t.area.Update()
}

func (t *Terminal) wheel(e *ui.WheelEvent) {
	e.Handled = true
	if e.Mods.Ctrl || e.Mods.Cmd {
		t.SetFontSize(t.fontSize + float64(e.DY))
		return
	}
	if e.Mods.Shift && !t.wrap {
		t.scrollSideways(-e.DY * 4)
		return
	}
	if e.DX != 0 && !t.wrap {
		t.scrollSideways(-e.DX * 4)
	}
	if e.DY != 0 {
		t.scrollLines(-e.DY * wheelLines)
	}
}

func (t *Terminal) scrollSideways(d int) {
	t.hscroll = max(0, min(t.hscroll+d, max(0, t.maxCols-t.textCols())))
	t.updateBars()
	t.area.Update()
}

func (t *Terminal) keyDown(e *ui.KeyEvent) {
	if popupOpen(t.area) {
		return
	}
	ctrl := e.Mods.Ctrl || e.Mods.Cmd
	switch {
	case e.Key == ui.KeyPageUp && e.Mods.Shift:
		t.scrollLines(-t.visibleLines() + 1)
	case e.Key == ui.KeyPageDown && e.Mods.Shift:
		t.scrollLines(t.visibleLines() - 1)
	case e.Key == ui.KeyArrowUp && ctrl && e.Mods.Shift:
		t.scrollLines(-1)
	case e.Key == ui.KeyArrowDown && ctrl && e.Mods.Shift:
		t.scrollLines(1)
	case e.Key == ui.KeyHome && ctrl:
		t.ScrollToStart()
	case e.Key == ui.KeyEnd && ctrl:
		t.ScrollToEnd()
	case e.Key == ui.KeyC && ctrl && (t.hasSel || e.Mods.Shift):
		t.Copy()
	case e.Key == ui.KeyA && ctrl && e.Mods.Shift:
		t.SelectAll()
	default:
		if t.KeyBytes == nil || t.OnSend == nil {
			return
		}
		data := t.KeyBytes(e)
		if data == nil {
			return
		}
		t.OnSend(data)
	}
	e.Handled = true
}

func (t *Terminal) char(e *ui.CharEvent) {
	if popupOpen(t.area) || t.OnSend == nil || e.Char < 0x20 || e.Mods.Ctrl || e.Mods.Cmd {
		return
	}
	t.OnSend([]byte(string(e.Char)))
	e.Handled = true
}

// selection returns the ends of the selection in order
func (t *Terminal) selection() (cellPos, cellPos) {
	if t.selB.less(t.selA) {
		return t.selB, t.selA
	}
	return t.selA, t.selB
}

// HasSelection tells whether some text is selected
func (t *Terminal) HasSelection() bool { return t.hasSel }

// SelectAll selects all the rows
func (t *Terminal) SelectAll() {
	if t.buf.RowCount() == 0 {
		return
	}
	last := t.prevRow(t.end())
	if last < t.buf.FirstRow() {
		return
	}
	t.selA = cellPos{t.firstVisible(), 0}
	t.selB = cellPos{last, len(t.cells(last))}
	t.hasSel = true
	t.area.Update()
}

// SelectedText returns the text of the selection, the rows by lines
func (t *Terminal) SelectedText() string {
	if !t.hasSel {
		return ""
	}
	a, b := t.selection()
	var sb strings.Builder
	for i := max(a.row, t.buf.FirstRow()); i <= b.row && i < t.buf.EndRow(); i = t.nextRow(i) {
		cells := t.cells(i)
		from, to := 0, len(cells)
		if i == a.row {
			from = min(a.cell, to)
		}
		if i == b.row {
			to = min(b.cell, to)
		}
		if i > a.row {
			sb.WriteByte('\n')
		}
		if gutter := t.gutter(i, t.buf.Row(i)); gutter != "" && from == 0 {
			sb.WriteString(gutter + " ")
		}
		sb.WriteString(strings.TrimRight(render.String(cells[from:to]), " "))
	}
	return sb.String()
}

// SelectedBytes returns the bytes the selected characters came from; the
// rows are joined as they came, with their line breaks
func (t *Terminal) SelectedBytes() []byte {
	if !t.hasSel {
		return nil
	}
	a, b := t.selection()
	var out []byte
	for i := max(a.row, t.buf.FirstRow()); i <= b.row && i < t.buf.EndRow(); i = t.nextRow(i) {
		r := t.buf.Row(i)
		if r.Dir == buffer.Info {
			continue
		}
		data := t.buf.Data(nil, r)
		cells := t.cells(i)
		from, to := 0, len(data)
		if i == a.row {
			from = byteAt(cells, a.cell, len(data))
		}
		if i == b.row {
			to = byteAt(cells, b.cell, len(data))
		}
		if from < to {
			out = append(out, data[from:to]...)
		}
	}
	return out
}

// byteAt is the byte where the cell k starts; past the cells, the end
func byteAt(cells []render.Cell, k, n int) int {
	for ; k < len(cells); k++ {
		if cells[k].B >= 0 {
			return int(cells[k].B)
		}
	}
	// After the last cell: the rest of the row, its line break too
	return n
}

// Copy puts the selected text on the clipboard
func (t *Terminal) Copy() {
	if s := t.SelectedText(); s != "" {
		ui.ClipboardSetText(s)
	}
}

// SetQuery looks for the text in the rows (case-insensitive), or for the
// regular expression; it returns the number of the matches
func (t *Terminal) SetQuery(q string, regex bool) (int, error) {
	t.query, t.regex = nil, nil
	var err error
	if regex && q != "" {
		t.regex, err = regexp.Compile("(?i)" + q)
	} else {
		t.query = []rune(strings.ToLower(q))
	}
	t.rescan()
	if t.filtered() {
		t.top, t.topSub = t.firstVisible(), 0
		t.setFollow(true)
	}
	if len(t.matches) > 0 {
		// The match nearest the end: the newest data is what is looked at
		t.current = len(t.matches) - 1
		t.showMatch()
	}
	t.updateBars()
	t.area.Update()
	return len(t.matches), err
}

func (t *Terminal) rescan() {
	t.matches = t.matches[:0]
	t.fRows = t.fRows[:0]
	t.current = -1
	t.scanned = t.buf.FirstRow()
	t.scan()
}

// scan looks for the query in the rows that came since the last scan
func (t *Terminal) scan() {
	first := t.buf.FirstRow()
	// The matches of the dropped rows
	k := 0
	for k < len(t.matches) && t.matches[k].row < first {
		k++
	}
	if k > 0 {
		t.matches = t.matches[k:]
		t.current -= k
	}
	k = 0
	for k < len(t.fRows) && t.fRows[k] < first {
		k++
	}
	t.fRows = t.fRows[k:]
	if !t.hasQuery() {
		t.scanned = t.buf.EndRow()
		return
	}
	// The last row was looked through while it grew: again, without its old matches
	from := max(first, min(t.scanned, t.buf.EndRow())-1)
	for len(t.matches) > 0 && t.matches[len(t.matches)-1].row >= from {
		t.matches = t.matches[:len(t.matches)-1]
	}
	for len(t.fRows) > 0 && t.fRows[len(t.fRows)-1] >= from {
		t.fRows = t.fRows[:len(t.fRows)-1]
	}
	q := t.query
	// In the text view the characters shown are the bytes of the row, less
	// the control ones: a row whose bytes don't hold the letters of the
	// query in order is not rendered at all
	var must []byte
	if t.buf.Options().Mode == buffer.ModeText && t.regex == nil {
		for _, r := range q {
			if r < 0x80 && r != ' ' {
				must = append(must, byte(r))
			}
		}
	}
	var cells []render.Cell
	var rs []rune
	var offsets []int
	var sb strings.Builder
	for i := from; i < t.buf.EndRow() && len(t.matches) < maxMatches; i++ {
		if len(must) > 0 {
			t.scratch = t.buf.Data(t.scratch[:0], t.buf.Row(i))
			if !subsequenceFold(t.scratch, must) {
				continue
			}
		}
		if c, ok := t.cache[i]; ok {
			cells = c
		} else {
			cells = t.renderRow(cells[:0], i)
		}
		before := len(t.matches)
		if t.regex != nil {
			// The offsets of the bytes of the text, to find the cells
			sb.Reset()
			offsets = offsets[:0]
			for _, c := range cells {
				offsets = append(offsets, sb.Len())
				sb.WriteRune(c.R)
			}
			text := sb.String()
			for _, m := range t.regex.FindAllStringIndex(text, -1) {
				if m[0] == m[1] {
					continue
				}
				a := sort.SearchInts(offsets, m[0])
				b := sort.SearchInts(offsets, m[1])
				t.matches = append(t.matches, match{i, a, b})
			}
		} else {
			rs = rs[:0]
			for _, c := range cells {
				rs = append(rs, unicode.ToLower(c.R))
			}
			for s := 0; s+len(q) <= len(rs); s++ {
				if equalRunes(rs[s:s+len(q)], q) {
					t.matches = append(t.matches, match{i, s, s + len(q)})
					s += len(q) - 1
				}
			}
		}
		if len(t.matches) > before {
			t.fRows = append(t.fRows, i)
		}
	}
	t.scanned = t.buf.EndRow()
	if t.current >= len(t.matches) {
		t.current = len(t.matches) - 1
	}
}

// subsequenceFold tells whether the bytes hold those of sub in order,
// ignoring the case of the ASCII letters (sub is in lower case)
func subsequenceFold(data, sub []byte) bool {
	k := 0
	for _, c := range data {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c == sub[k] {
			k++
			if k == len(sub) {
				return true
			}
		}
	}
	return false
}

func equalRunes(a, b []rune) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// firstMatch is the index of the first match of the row
func (t *Terminal) firstMatch(row int) int {
	return sort.Search(len(t.matches), func(k int) bool { return t.matches[k].row >= row })
}

// Matches returns the number of the matches and the current one (from 1)
func (t *Terminal) Matches() (count, current int) { return len(t.matches), t.current + 1 }

// NextMatch goes to the next match, or the previous one
func (t *Terminal) NextMatch(back bool) {
	if len(t.matches) == 0 {
		return
	}
	if back {
		t.current = (t.current - 1 + len(t.matches)) % len(t.matches)
	} else {
		t.current = (t.current + 1) % len(t.matches)
	}
	t.showMatch()
}

// showMatch scrolls to the current match, a third from the top
func (t *Terminal) showMatch() {
	if t.current < 0 || t.current >= len(t.matches) {
		return
	}
	m := t.matches[t.current]
	t.setFollow(false)
	t.top, t.topSub = m.row, 0
	if t.wrap {
		t.topSub = min(m.from/t.textCols(), t.lines(m.row)-1)
	} else {
		cols := t.textCols()
		if m.from < t.hscroll || m.to > t.hscroll+cols {
			t.hscroll = max(0, m.from-cols/3)
		}
	}
	t.scrollLines(-t.visibleLines() / 3)
	t.updateBars()
	t.area.Update()
}
