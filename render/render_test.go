package render

import (
	"testing"
)

func text(s string) string  { return String(Text(nil, Input{Data: []byte(s)}, UTF8)) }
func mixed(s string) string { return String(Mixed(nil, Input{Data: []byte(s)}, UTF8)) }

func TestText(t *testing.T) {
	cases := map[string]string{
		"hello\r\n":            "hello",
		"a\tb":                 "a       b",
		"abc\b\bd":             "ad",
		"\x1b[1;31mERR\x1b[0m": "ERR",
		"\x1b]0;title\x07ok":   "ok",
		"\x1b[2K\x1b[1Gline":   "line",
		"Привет":               "Привет",
		"\x00\x07x\x7f":        "x",
		"bad\xffbyte":          "bad�byte",
	}
	for in, want := range cases {
		if got := text(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestTextColors(t *testing.T) {
	cells := Text(nil, Input{Data: []byte("\x1b[1;32mI\x1b[0m (\x1b[38;5;208mx\x1b[38;2;1;2;3my")}, UTF8)
	if len(cells) != 5 {
		t.Fatal(String(cells))
	}
	if cells[0].FG != PaletteColor(2) || cells[0].A&Bold == 0 {
		t.Fatalf("I: %+v", cells[0])
	}
	if cells[1].FG != 0 || cells[1].A != 0 {
		t.Fatalf("space: %+v", cells[1])
	}
	if cells[3].FG != PaletteColor(208) {
		t.Fatalf("x: %+v", cells[3])
	}
	if r, g, b, ok := cells[4].FG.RGB(); !ok || r != 1 || g != 2 || b != 3 {
		t.Fatalf("y: %+v", cells[4])
	}
	// The byte each character came from
	if cells[0].B != 7 || cells[1].B != 12 {
		t.Fatalf("bytes %d %d", cells[0].B, cells[1].B)
	}
}

func TestMixed(t *testing.T) {
	cases := map[string]string{
		"OK\r\n":       `OK\r\n`,
		"\x01\x03\x00": `\x01\x03\0`,
		"a\\b":         `a\\b`,
		"\x1b[0m":      `\e[0m`,
		"t\xffé":       `t\xFFé`,
		"\t":           `\t`,
	}
	for in, want := range cases {
		if got := mixed(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestEncodings(t *testing.T) {
	cp := []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2} // "Привет" in Windows-1251
	if got := String(Text(nil, Input{Data: cp}, CP1251)); got != "Привет" {
		t.Fatal(got)
	}
	koi := []byte{0xF0, 0xD2, 0xC9, 0xD7, 0xC5, 0xD4}
	if got := String(Text(nil, Input{Data: koi}, KOI8R)); got != "Привет" {
		t.Fatal(got)
	}
	if got := String(Text(nil, Input{Data: []byte("a\xC3b")}, ASCII)); got != "a·b" {
		t.Fatal(got)
	}
}

func TestHex(t *testing.T) {
	cells := Hex(nil, Input{Data: []byte("Hi\r\n\x00")}, 8)
	want := "48 69 0D 0A 00           Hi···"
	if got := String(cells); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if len(Hex(nil, Input{Data: make([]byte, 16)}, 16)) != HexColumns(16) {
		t.Fatal("columns")
	}
}

func TestTyped(t *testing.T) {
	cells := Text(nil, Input{Data: []byte("> ls"), Typed: []bool{false, false, true, true}}, UTF8)
	if cells[0].A&Typed != 0 || cells[2].A&Typed == 0 {
		t.Fatal(cells)
	}
}
