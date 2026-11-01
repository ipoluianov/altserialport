package buffer

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// rows returns the rows as "dir:data" with the bytes quoted
func rows(b *Buffer) []string {
	var out []string
	for i := b.FirstRow(); i < b.EndRow(); i++ {
		r := b.Row(i)
		out = append(out, fmt.Sprintf("%d:%q", r.Dir, b.Data(nil, r)))
	}
	return out
}

func feed(b *Buffer, t int64, dir Dir, s string) {
	b.Append(Chunk{Time: t, Dir: dir, Data: []byte(s)})
}

func check(t *testing.T, b *Buffer, want ...string) {
	t.Helper()
	got := rows(b)
	if strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Fatalf("rows\n got: %v\nwant: %v", got, want)
	}
}

func TestTextLines(t *testing.T) {
	b := New(DefaultOptions(), 0, 0)
	feed(b, 1, RX, "one\r\ntw")
	feed(b, 2, RX, "o\r")
	feed(b, 3, RX, "\nthree\nfour\rfive")
	check(t, b, `0:"one\r\n"`, `0:"two\r\n"`, `0:"three\n"`, `0:"four\r"`, `0:"five"`)
	if !b.IsOpen() {
		t.Fatal("the last line is open")
	}
}

func TestLineBreakModes(t *testing.T) {
	data := "a\rb\nc\r\nd"
	cases := map[LineBreak][]string{
		BreakLF:   {`0:"a\rb\n"`, `0:"c\r\n"`, `0:"d"`},
		BreakCR:   {`0:"a\r"`, `0:"b\nc\r"`, `0:"\nd"`},
		BreakCRLF: {`0:"a\rb\nc\r\n"`, `0:"d"`},
		BreakNone: {`0:"a\rb\nc\r\nd"`},
	}
	for brk, want := range cases {
		o := DefaultOptions()
		o.Break = brk
		b := New(o, 0, 0)
		feed(b, 1, RX, data)
		check(t, b, want...)
	}
}

func TestTXSplitsLines(t *testing.T) {
	b := New(DefaultOptions(), 0, 0)
	feed(b, 1, RX, "temp=2")
	feed(b, 2, TX, "status\r\n")
	feed(b, 3, RX, "3.4\n")
	check(t, b, `0:"temp=2"`, `1:"status\r\n"`, `0:"3.4\n"`)

	// Hidden, the sent data leaves the line whole
	o := b.Options()
	o.ShowTX = false
	b.SetOptions(o)
	check(t, b, `0:"temp=23.4\n"`)
}

func TestEchoInsideLine(t *testing.T) {
	o := DefaultOptions()
	b := New(o, 0, 0)
	feed(b, 1, RX, "> ")
	feed(b, 2, Keys, "l")
	feed(b, 3, RX, "l")
	check(t, b, `0:"> l"`)
	o.Echo = true
	b.SetOptions(o)
	check(t, b, `0:"> ll"`)
	var dirs []Dir
	b.Segments(b.Row(b.FirstRow()), func(d Dir, _ []byte) { dirs = append(dirs, d) })
	if fmt.Sprint(dirs) != "[0 2 0]" {
		t.Fatal(dirs)
	}
}

func TestInfoRows(t *testing.T) {
	b := New(DefaultOptions(), 0, 0)
	feed(b, 1, RX, "abc")
	feed(b, 2, Info, "Disconnected")
	feed(b, 3, RX, "def\n")
	check(t, b, `0:"abc"`, `3:"Disconnected"`, `0:"def\n"`)
}

func TestHexRowsAndGap(t *testing.T) {
	o := DefaultOptions()
	o.Mode = ModeHex
	o.HexWidth = 4
	o.Gap = 10 * time.Millisecond
	b := New(o, 0, 0)
	ms := int64(time.Millisecond)
	feed(b, 1*ms, RX, "\x01\x03\x00")
	feed(b, 2*ms, RX, "\x00\x00\x0a") // the same packet: 2 ms later
	feed(b, 30*ms, RX, "\x01\x83")    // a new one after a pause
	feed(b, 31*ms, TX, "\x05")
	check(t, b, `0:"\x01\x03\x00\x00"`, `0:"\x00\n"`, `0:"\x01\x83"`, `1:"\x05"`)
}

func TestMixedGap(t *testing.T) {
	o := DefaultOptions()
	o.Mode = ModeMixed
	o.Gap = 10 * time.Millisecond
	b := New(o, 0, 0)
	ms := int64(time.Millisecond)
	feed(b, 1*ms, RX, "ab")
	feed(b, 50*ms, RX, "cd\n")
	check(t, b, `0:"ab"`, `0:"cd\n"`)
	// The text view doesn't split lines by pauses
	o.Mode = ModeText
	b.SetOptions(o)
	check(t, b, `0:"abcd\n"`)
}

func TestMaxLine(t *testing.T) {
	b := New(DefaultOptions(), 0, 0)
	feed(b, 1, RX, strings.Repeat("x", MaxLine+10))
	if b.RowCount() != 2 || b.Row(b.FirstRow()).Len != MaxLine {
		t.Fatal(b.RowCount())
	}
}

func TestTrimBytes(t *testing.T) {
	b := New(DefaultOptions(), 100, 0)
	for i := 0; i < 50; i++ {
		feed(b, int64(i+1), RX, "line\n")
	}
	if b.Bytes() > 100 {
		t.Fatal("kept", b.Bytes())
	}
	first := b.FirstRow()
	if first == 0 || b.EndRow() != 50 {
		t.Fatal(first, b.EndRow())
	}
	if got := string(b.Data(nil, b.Row(first))); got != "line\n" {
		t.Fatal(got)
	}
}

func TestTrimRows(t *testing.T) {
	b := New(DefaultOptions(), 0, 80)
	for i := 0; i < 1000; i++ {
		feed(b, int64(i+1), RX, fmt.Sprintf("%d\n", i))
	}
	if b.RowCount() > 80 {
		t.Fatal(b.RowCount())
	}
	if got := string(b.Data(nil, b.Row(b.EndRow()-1))); got != "999\n" {
		t.Fatal(got)
	}
	// The chunks of the dropped rows are gone too
	if b.Bytes() > 80*4 {
		t.Fatal("kept", b.Bytes())
	}
}

func TestClear(t *testing.T) {
	b := New(DefaultOptions(), 0, 0)
	feed(b, 1, RX, "a\nb")
	b.Clear()
	feed(b, 2, RX, "c\n")
	check(t, b, `0:"c\n"`)
}

func BenchmarkAppend(b *testing.B) {
	buf := New(DefaultOptions(), 64<<20, 1_000_000)
	line := []byte("I (1234) wifi: station connected, rssi=-54 channel=6\r\n")
	b.SetBytes(int64(len(line)))
	for i := 0; i < b.N; i++ {
		buf.Append(Chunk{Time: int64(i), Dir: RX, Data: line})
	}
}
