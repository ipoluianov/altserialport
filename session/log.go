package session

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ipoluianov/altserialport/buffer"
)

// LogFormat is the kind of a log file
type LogFormat int

const (
	// LogText is the lines as the text view shows them: the received text,
	// the sent lines after "> ", the notes between "---"
	LogText LogFormat = iota
	// LogRaw is the received bytes as they came, nothing else
	LogRaw
)

// Logger writes the chunks of a session to a file. It is written from the
// reader of the port, not from the screen, so nothing is lost while the
// window is busy.
type Logger struct {
	mu         sync.Mutex
	path       string
	f          *os.File
	w          *bufio.Writer
	format     LogFormat
	timestamps bool
	bytes      int64
	err        error
	maxSize    int64 // a new file is started past this size; 0 never
	files      int   // the files written, for the names of the next ones

	lineStart bool // nothing is written in the current line yet
	afterCR   bool // the line was ended by a CR: an LF next is its part
}

// NewLogger creates the file, or appends to it when it exists
func NewLogger(path string, format LogFormat, timestamps bool) (*Logger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &Logger{path: path, f: f, w: bufio.NewWriterSize(f, 64*1024), format: format,
		timestamps: timestamps, lineStart: true}, nil
}

// Path is the file being written
func (l *Logger) Path() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.path
}

// Size is the bytes written to the file, and the first error of writing
func (l *Logger) Size() (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.bytes, l.err
}

// Write adds a chunk to the file
func (l *Logger) Write(c buffer.Chunk) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return
	}
	if l.format == LogRaw {
		if c.Dir == buffer.RX {
			l.put(c.Data)
		}
	} else {
		l.text(c)
	}
	if err := l.w.Flush(); err != nil && l.err == nil {
		l.err = err
	}
	if l.maxSize > 0 && l.bytes >= l.maxSize && (l.format == LogRaw || l.lineStart) {
		l.rotate()
	}
}

// SetMaxSize starts a new file each time the log grows past n bytes
func (l *Logger) SetMaxSize(n int64) {
	l.mu.Lock()
	l.maxSize = n
	l.mu.Unlock()
}

// rotate closes the file and goes on in a new one: name-2.log, name-3.log
func (l *Logger) rotate() {
	l.w.Flush()
	l.f.Close()
	l.files++
	ext := filepath.Ext(l.path)
	base := strings.TrimSuffix(l.path, ext)
	if l.files > 1 {
		base = base[:strings.LastIndex(base, "-")]
	}
	next := fmt.Sprintf("%s-%d%s", base, l.files+1, ext)
	f, err := os.OpenFile(next, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		l.err = err
		l.f = nil
		return
	}
	l.f = f
	l.w.Reset(f)
	l.path = next
	l.bytes = 0
}

func (l *Logger) put(b []byte) {
	n, _ := l.w.Write(b)
	l.bytes += int64(n)
}

func (l *Logger) newline() {
	if !l.lineStart {
		l.put([]byte{'\n'})
		l.lineStart = true
	}
	l.afterCR = false
}

func (l *Logger) prefix(t int64) {
	if l.timestamps {
		l.put([]byte(time.Unix(0, t).Format("2006-01-02 15:04:05.000 ")))
	}
	l.lineStart = false
}

func (l *Logger) text(c buffer.Chunk) {
	switch c.Dir {
	case buffer.Keys:
		return // the device echoes what it takes
	case buffer.Info:
		l.newline()
		l.prefix(c.Time)
		l.put([]byte("--- " + string(c.Data) + " ---"))
		l.newline()
		return
	case buffer.TX:
		l.newline()
		l.prefix(c.Time)
		line := make([]byte, 0, len(c.Data)+2)
		line = append(line, '>', ' ')
		for _, b := range c.Data {
			if b != '\r' && b != '\n' {
				line = append(line, b)
			}
		}
		l.put(line)
		l.newline()
		return
	}
	for _, b := range c.Data {
		if b == '\n' && l.afterCR {
			l.afterCR = false
			continue
		}
		l.afterCR = false
		if b == '\r' || b == '\n' {
			if l.lineStart {
				l.prefix(c.Time) // an empty line keeps its time
			}
			l.newline()
			l.afterCR = b == '\r'
			continue
		}
		if l.lineStart {
			l.prefix(c.Time)
		}
		l.w.WriteByte(b)
		l.bytes++
	}
}

// Close finishes the last line and closes the file
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	if l.format == LogText {
		l.newline()
	}
	err := l.w.Flush()
	if cerr := l.f.Close(); err == nil {
		err = cerr
	}
	l.f = nil
	return err
}
