// Package buffer keeps what came from a port and what was sent to it, as
// chunks of bytes with the time they came, and lays them out into rows:
// the lines of the text views and the packets of the hex view. One buffer
// is the source of every view; changing the view lays the rows out again
// from the same chunks.
package buffer

import "time"

// Dir is where the bytes of a chunk go
type Dir uint8

const (
	RX   Dir = iota // received from the device
	TX              // sent from the send line, a macro, a file
	Keys            // typed in the terminal, sent as keys
	Info            // a note of the program: connected, disconnected
	RX2             // received from the second port
	TX2             // sent to the second port
)

// Second tells whether the chunk is of the second port
func (d Dir) Second() bool { return d == RX2 || d == TX2 }

// Sent tells whether the bytes were sent, to either port
func (d Dir) Sent() bool { return d == TX || d == TX2 }

// Chunk is the bytes of one read or one write
type Chunk struct {
	Time int64 // Unix time, ns
	Dir  Dir
	Data []byte
}

// Mode is the kind of the rows
type Mode uint8

const (
	ModeText  Mode = iota // lines of text
	ModeHex               // rows of bytes, split by pauses
	ModeMixed             // lines of text with every control byte shown
)

// LineBreak is what ends a line in the text views
type LineBreak uint8

const (
	BreakAuto LineBreak = iota // CR, LF or CRLF
	BreakLF
	BreakCR
	BreakCRLF
	BreakNone // only the width limit and the direction split lines
)

// Options of the layout
type Options struct {
	Mode  Mode
	Break LineBreak
	// HexWidth is the bytes in a row of the hex view
	HexWidth int
	// Gap starts a new row after a pause of the data at least this long,
	// in the hex and the mixed views; 0 turns it off
	Gap time.Duration
	// ShowTX shows the sent data (TX chunks) as rows of their own
	ShowTX bool
	// Echo shows the typed keys inside the received lines
	Echo bool
}

// DefaultOptions are the options of a new view
func DefaultOptions() Options {
	return Options{Mode: ModeText, Break: BreakAuto, HexWidth: 16, Gap: 10 * time.Millisecond, ShowTX: true}
}

// MaxLine is the most bytes in a row of the text views: a stream without
// line breaks (binary data at a wrong speed) still makes rows
const MaxLine = 4096

// Pos is a place in the chunks: a chunk by its index and a byte in it
type Pos struct {
	Chunk int
	Off   int32
}

// Row is a line or a packet: the bytes from Start up to End
type Row struct {
	Start, End Pos
	Time       int64 // the time of the first byte
	Len        int32 // the bytes in the row
	Dir        Dir   // RX (with the echoed keys), TX or Info
}

// Buffer is the chunks and their rows. It is not safe for concurrent use:
// the program feeds it on the UI thread.
type Buffer struct {
	opts Options

	chunks     []Chunk
	firstChunk int // the index of chunks[0]; the older are dropped
	bytes      int

	rows     []Row
	firstRow int // the index of rows[0]

	maxBytes int
	maxRows  int

	open      bool  // the last row may still grow
	afterCR   bool  // the last row ended with a CR: an LF next joins it
	prevCR    bool  // the last byte laid out was a CR (BreakCRLF)
	lastTime  int64 // the time of the last chunk laid out
	totalRows int   // rows made since the start: tells the views about new rows
}

// New makes an empty buffer that keeps up to maxBytes of data and maxRows
// rows; the oldest are dropped past them
func New(opts Options, maxBytes, maxRows int) *Buffer {
	b := &Buffer{maxBytes: maxBytes, maxRows: maxRows}
	b.opts = normalize(opts)
	return b
}

func normalize(o Options) Options {
	if o.HexWidth <= 0 {
		o.HexWidth = 16
	}
	return o
}

// Options returns the options of the layout
func (b *Buffer) Options() Options { return b.opts }

// SetOptions lays all the rows out again with the options
func (b *Buffer) SetOptions(o Options) {
	b.opts = normalize(o)
	b.relayout()
}

// SetLimits changes how much is kept
func (b *Buffer) SetLimits(maxBytes, maxRows int) {
	b.maxBytes, b.maxRows = maxBytes, maxRows
	b.trim()
}

// Append adds a chunk and lays it out. The data is kept as is: the caller
// doesn't change it afterwards.
func (b *Buffer) Append(c Chunk) {
	if len(c.Data) == 0 {
		return
	}
	b.chunks = append(b.chunks, c)
	b.bytes += len(c.Data)
	b.layChunk(b.firstChunk + len(b.chunks) - 1)
	b.trim()
}

// Clear drops everything
func (b *Buffer) Clear() {
	b.firstChunk += len(b.chunks)
	b.chunks = nil
	b.bytes = 0
	b.firstRow += len(b.rows)
	b.rows = nil
	b.resetState()
}

// Bytes is the size of the data kept
func (b *Buffer) Bytes() int { return b.bytes }

// FirstRow is the index of the oldest row kept
func (b *Buffer) FirstRow() int { return b.firstRow }

// EndRow is the index after the newest row
func (b *Buffer) EndRow() int { return b.firstRow + len(b.rows) }

// RowCount is the rows kept
func (b *Buffer) RowCount() int { return len(b.rows) }

// Has tells whether the row of the index is kept
func (b *Buffer) Has(i int) bool { return i >= b.firstRow && i < b.EndRow() }

// Row returns the row of the index; it must be kept (Has)
func (b *Buffer) Row(i int) Row { return b.rows[i-b.firstRow] }

// Total counts every row ever made, laid out again or not: a view that saw
// another count has rows to show
func (b *Buffer) Total() int { return b.totalRows }

// IsOpen tells whether the last row may still grow
func (b *Buffer) IsOpen() bool { return b.open }

// Chunks returns the range of the indexes of the chunks kept
func (b *Buffer) Chunks() (first, end int) { return b.firstChunk, b.firstChunk + len(b.chunks) }

// Chunk returns the chunk of the index, or nil when it was dropped
func (b *Buffer) Chunk(i int) *Chunk {
	if i < b.firstChunk || i >= b.firstChunk+len(b.chunks) {
		return nil
	}
	return &b.chunks[i-b.firstChunk]
}

// Segments calls f with each piece of the row that lies in one chunk, in
// order, with the direction of that chunk (Keys inside received lines)
func (b *Buffer) Segments(r Row, f func(dir Dir, data []byte)) {
	for ci := r.Start.Chunk; ci <= r.End.Chunk; ci++ {
		c := b.Chunk(ci)
		if c == nil {
			continue
		}
		from, to := 0, len(c.Data)
		if ci == r.Start.Chunk {
			from = int(r.Start.Off)
		}
		if ci == r.End.Chunk {
			to = int(r.End.Off)
		}
		if from >= to {
			continue
		}
		if r.Dir != Info && !b.visible(c.Dir) {
			continue // a hidden chunk between the pieces of the row
		}
		f(c.Dir, c.Data[from:to])
	}
}

// Data returns the bytes of the row, appended to dst
func (b *Buffer) Data(dst []byte, r Row) []byte {
	b.Segments(r, func(_ Dir, d []byte) { dst = append(dst, d...) })
	return dst
}

// visible tells whether the chunks of the direction are laid out
func (b *Buffer) visible(d Dir) bool {
	switch d {
	case TX, TX2:
		return b.opts.ShowTX
	case Keys:
		return b.opts.Echo
	}
	return true
}

func (b *Buffer) resetState() {
	b.open = false
	b.afterCR = false
	b.prevCR = false
	b.lastTime = 0
}

func (b *Buffer) relayout() {
	b.firstRow += len(b.rows)
	b.rows = b.rows[:0]
	b.resetState()
	for i := range b.chunks {
		b.layChunk(b.firstChunk + i)
	}
}

func (b *Buffer) closeRow() {
	b.open = false
	b.afterCR = false
	b.prevCR = false
}

func (b *Buffer) startRow(at Pos, t int64, dir Dir) *Row {
	b.rows = append(b.rows, Row{Start: at, End: at, Time: t, Dir: dir})
	b.totalRows++
	b.open = true
	return &b.rows[len(b.rows)-1]
}

// layChunk adds the bytes of the chunk to the rows
func (b *Buffer) layChunk(ci int) {
	c := &b.chunks[ci-b.firstChunk]
	if c.Dir == Info {
		b.closeRow()
		r := b.startRow(Pos{ci, 0}, c.Time, Info)
		r.End = Pos{ci, int32(len(c.Data))}
		r.Len = int32(len(c.Data))
		b.closeRow()
		return
	}
	if !b.visible(c.Dir) {
		return
	}
	dir := c.Dir
	if dir == Keys {
		dir = RX // the echo stays in the line it was typed into
	}
	gap := b.opts.Gap > 0 && b.opts.Mode != ModeText && b.lastTime != 0 &&
		time.Duration(c.Time-b.lastTime) >= b.opts.Gap
	if b.open && (b.rows[len(b.rows)-1].Dir != dir || gap) {
		b.closeRow()
	}
	if len(b.rows) > 0 && b.rows[len(b.rows)-1].Dir != dir {
		b.afterCR = false
	}
	b.lastTime = c.Time

	hex := b.opts.Mode == ModeHex
	brk := b.opts.Break
	for i, ch := range c.Data {
		at := Pos{ci, int32(i)}
		next := Pos{ci, int32(i + 1)}
		if !b.open {
			if b.afterCR && ch == '\n' && !hex {
				// The LF of a CRLF whose CR has ended the line
				r := &b.rows[len(b.rows)-1]
				r.End = next
				r.Len++
				b.afterCR = false
				continue
			}
			b.afterCR = false
			b.startRow(at, c.Time, dir)
		}
		r := &b.rows[len(b.rows)-1]
		r.End = next
		r.Len++
		if hex {
			if int(r.Len) >= b.opts.HexWidth {
				b.closeRow()
			}
			continue
		}
		switch {
		case ch == '\n' && (brk == BreakAuto || brk == BreakLF || brk == BreakCRLF && b.prevCR):
			b.closeRow()
		case ch == '\r' && (brk == BreakAuto || brk == BreakCR):
			b.closeRow()
			b.afterCR = brk == BreakAuto
		case r.Len >= MaxLine:
			b.closeRow()
		default:
			b.prevCR = ch == '\r'
		}
	}
}

// trim drops the oldest data past the limits, an eighth more than needed
// so that it isn't done on every chunk
func (b *Buffer) trim() {
	if b.maxBytes > 0 && b.bytes > b.maxBytes {
		target := b.maxBytes - b.maxBytes/8
		n := 0
		for n < len(b.chunks)-1 && b.bytes > target {
			b.bytes -= len(b.chunks[n].Data)
			n++
		}
		b.dropChunks(n)
	}
	if b.maxRows > 0 && len(b.rows) > b.maxRows {
		drop := len(b.rows) - (b.maxRows - b.maxRows/8)
		b.dropRows(drop)
		// The chunks before the first row are seen by no view
		if len(b.rows) > 0 {
			n := b.rows[0].Start.Chunk - b.firstChunk
			for i := 0; i < n; i++ {
				b.bytes -= len(b.chunks[i].Data)
			}
			b.dropChunks(n)
		}
	}
}

func (b *Buffer) dropChunks(n int) {
	if n <= 0 {
		return
	}
	clear(b.chunks[:n]) // the data goes to the collector
	b.chunks = b.chunks[n:]
	b.firstChunk += n
	// The rows that start in the dropped chunks go with them
	k := 0
	for k < len(b.rows) && b.rows[k].Start.Chunk < b.firstChunk {
		k++
	}
	b.dropRows(k)
}

func (b *Buffer) dropRows(n int) {
	if n <= 0 {
		return
	}
	if n >= len(b.rows) {
		n = len(b.rows)
		b.open = false
		b.afterCR = false
	}
	b.rows = b.rows[n:]
	b.firstRow += n
}
