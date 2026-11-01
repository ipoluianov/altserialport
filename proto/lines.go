package proto

// Lines cuts a stream of received bytes into lines: CR, LF and CRLF end a
// line. The triggers and the plotter read the data so, whatever view the
// terminal shows.
type Lines struct {
	cur     []byte
	afterCR bool
}

// MaxLineLength cuts a line that never ends
const MaxLineLength = 4096

// Feed adds bytes and calls f with each line completed, without its end;
// the slice is valid only during the call
func (l *Lines) Feed(data []byte, f func(line []byte)) {
	for _, c := range data {
		if c == '\n' && l.afterCR {
			l.afterCR = false
			continue
		}
		l.afterCR = c == '\r'
		if c == '\r' || c == '\n' {
			f(l.cur)
			l.cur = l.cur[:0]
			continue
		}
		l.cur = append(l.cur, c)
		if len(l.cur) >= MaxLineLength {
			f(l.cur)
			l.cur = l.cur[:0]
		}
	}
}

// Reset forgets the line begun
func (l *Lines) Reset() {
	l.cur = l.cur[:0]
	l.afterCR = false
}
