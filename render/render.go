// Package render turns the bytes of a row into the cells of a terminal
// line: characters with their colors and the byte each came from. It
// knows the text encodings, the ANSI colors, the escapes of the mixed
// view and the hex dump.
package render

import (
	"strings"
	"unicode/utf8"
)

// Color of a cell: 0 is the default color of the view; a palette color
// (ANSI 0..255) or an RGB color otherwise
type Color uint32

const (
	colorPalette Color = 1 << 24
	colorRGB     Color = 2 << 24
)

// PaletteColor is the ANSI color of the index (0..15 the basic ones)
func PaletteColor(i int) Color { return colorPalette | Color(i&0xFF) }

// RGBColor is a color given by its components
func RGBColor(r, g, b uint8) Color {
	return colorRGB | Color(r)<<16 | Color(g)<<8 | Color(b)
}

// Palette tells whether the color is a palette one, and its index
func (c Color) Palette() (int, bool) { return int(c & 0xFF), c&0xFF000000 == colorPalette }

// RGB tells whether the color is an RGB one, and its components
func (c Color) RGB() (r, g, b uint8, ok bool) {
	return uint8(c >> 16), uint8(c >> 8), uint8(c), c&0xFF000000 == colorRGB
}

// Attr is the style of a cell
type Attr uint8

const (
	Bold Attr = 1 << iota
	Dim
	Italic
	Underline
	Inverse
	Token // a control byte shown as an escape, \r or \x1B
	Typed // a key typed in the terminal (echo)
	Note  // a note of the program
)

// Cell is one character of a line
type Cell struct {
	R      rune
	B      int32 // the byte of the row the character came from
	FG, BG Color
	A      Attr
}

// Encoding of the text
type Encoding uint8

const (
	UTF8 Encoding = iota
	ASCII
	Latin1
	CP1251
	CP866
	KOI8R
	CP437
)

// Encodings are the encodings offered, with their names
var Encodings = []struct {
	Enc  Encoding
	Name string
}{
	{UTF8, "UTF-8"}, {ASCII, "ASCII"}, {Latin1, "Latin-1 (ISO-8859-1)"},
	{CP1251, "Windows-1251"}, {CP866, "CP866 (DOS)"}, {KOI8R, "KOI8-R"}, {CP437, "CP437 (IBM PC)"},
}

// EncodingName returns the name of the encoding
func EncodingName(e Encoding) string {
	for _, x := range Encodings {
		if x.Enc == e {
			return x.Name
		}
	}
	return "UTF-8"
}

// decode returns the character at the start of b and its size; ok is false
// for a byte that is no character in the encoding
func decode(b []byte, enc Encoding) (r rune, size int, ok bool) {
	c := b[0]
	if c < 0x80 {
		return rune(c), 1, true
	}
	switch enc {
	case UTF8:
		r, size = utf8.DecodeRune(b)
		return r, size, r != utf8.RuneError || size > 1
	case Latin1:
		return rune(c), 1, c >= 0xA0
	case CP1251:
		r = cp1251High[c-0x80]
	case CP866:
		r = cp866High[c-0x80]
	case KOI8R:
		r = koi8rHigh[c-0x80]
	case CP437:
		r = cp437High[c-0x80]
	default:
		return rune(c), 1, false
	}
	return r, 1, r != utf8.RuneError
}

// Input is the bytes of a row; Typed, when given, marks the bytes typed in
// the terminal
type Input struct {
	Data  []byte
	Typed []bool
}

func (in Input) attr(i int) Attr {
	if in.Typed != nil && in.Typed[i] {
		return Typed
	}
	return 0
}

// TabWidth is the distance between the tab stops
const TabWidth = 8

// Text renders a line as a terminal shows it: the ANSI colors applied, the
// tabs expanded, a backspace taking the character before it back, the
// other control bytes and the line breaks not shown
func Text(dst []Cell, in Input, enc Encoding) []Cell {
	start := len(dst)
	var st sgr
	data := in.Data
	for i := 0; i < len(data); {
		c := data[i]
		switch {
		case c == 0x1B:
			i = st.escape(data, i)
			continue
		case c == '\t':
			for n := TabWidth - (len(dst)-start)%TabWidth; n > 0; n-- {
				dst = append(dst, st.cell(' ', i, in.attr(i)))
			}
			i++
			continue
		case c == '\b':
			if len(dst) > start {
				dst = dst[:len(dst)-1]
			}
			i++
			continue
		case c < 0x20 || c == 0x7F:
			i++
			continue
		}
		r, size, ok := decode(data[i:], enc)
		if !ok {
			cell := st.cell(placeholder(enc), i, in.attr(i))
			cell.A |= Dim
			dst = append(dst, cell)
			i += size
			continue
		}
		if r >= 0x80 && r < 0xA0 {
			i += size // a C1 control
			continue
		}
		dst = append(dst, st.cell(r, i, in.attr(i)))
		i += size
	}
	return dst
}

// placeholder stands for a byte that is not a character: the replacement
// character of Unicode, a dot in ASCII
func placeholder(enc Encoding) rune {
	if enc == ASCII {
		return '·'
	}
	return utf8.RuneError
}

// Mixed renders a line with every byte visible: the text as text, the
// control bytes and the bytes that are not characters as C escapes (\r,
// \n, \x00), so that a line copied from the view can be sent back as is
func Mixed(dst []Cell, in Input, enc Encoding) []Cell {
	data := in.Data
	for i := 0; i < len(data); {
		c := data[i]
		if c == '\\' {
			dst = token(dst, `\\`, i, in.attr(i))
			i++
			continue
		}
		if c < 0x20 || c == 0x7F {
			dst = token(dst, Escape(c), i, in.attr(i))
			i++
			continue
		}
		r, size, ok := decode(data[i:], enc)
		if !ok || r >= 0x80 && r < 0xA0 {
			for k := 0; k < size; k++ {
				dst = token(dst, Escape(data[i+k]), i+k, in.attr(i+k))
			}
			i += size
			continue
		}
		dst = append(dst, Cell{R: r, B: int32(i), A: in.attr(i)})
		i += size
	}
	return dst
}

// Escape returns the C escape of a byte: \r, \n, \t, \0, \e or \xNN
func Escape(c byte) string {
	switch c {
	case 0:
		return `\0`
	case 7:
		return `\a`
	case 8:
		return `\b`
	case '\t':
		return `\t`
	case '\n':
		return `\n`
	case 11:
		return `\v`
	case 12:
		return `\f`
	case '\r':
		return `\r`
	case 0x1B:
		return `\e`
	case '\\':
		return `\\`
	}
	const digits = "0123456789ABCDEF"
	return `\x` + string(digits[c>>4]) + string(digits[c&15])
}

func token(dst []Cell, s string, b int, a Attr) []Cell {
	for _, r := range s {
		dst = append(dst, Cell{R: r, B: int32(b), A: Token | a})
	}
	return dst
}

// Hex renders a row of up to width bytes as a dump: the bytes in hex, in
// groups of eight, then the bytes as ASCII
func Hex(dst []Cell, in Input, width int) []Cell {
	const digits = "0123456789ABCDEF"
	data := in.Data
	for i := 0; i < width; i++ {
		if i > 0 && i%8 == 0 {
			dst = append(dst, Cell{R: ' ', B: -1})
		}
		if i >= len(data) {
			dst = append(dst, Cell{R: ' ', B: -1}, Cell{R: ' ', B: -1}, Cell{R: ' ', B: -1})
			continue
		}
		c := data[i]
		a := hexAttr(c) | in.attr(i)
		dst = append(dst,
			Cell{R: rune(digits[c>>4]), B: int32(i), A: a},
			Cell{R: rune(digits[c&15]), B: int32(i), A: a},
			Cell{R: ' ', B: -1})
	}
	dst = append(dst, Cell{R: ' ', B: -1})
	for i, c := range data {
		if c >= 0x20 && c < 0x7F {
			dst = append(dst, Cell{R: rune(c), B: int32(i), A: in.attr(i)})
		} else {
			dst = append(dst, Cell{R: '·', B: int32(i), A: Dim | in.attr(i)})
		}
	}
	return dst
}

// HexColumns is the width of a row of the hex dump in cells
func HexColumns(width int) int {
	return width*3 + (width-1)/8 + 1 + width
}

func hexAttr(c byte) Attr {
	switch {
	case c == 0:
		return Dim
	case c < 0x20 || c == 0x7F:
		return Token
	}
	return 0
}

// NoteCells renders a note of the program
func NoteCells(dst []Cell, text string) []Cell {
	for i, r := range text {
		dst = append(dst, Cell{R: r, B: int32(i), A: Note | Italic})
	}
	return dst
}

// String returns the characters of the cells
func String(cells []Cell) string {
	var sb strings.Builder
	for _, c := range cells {
		sb.WriteRune(c.R)
	}
	return sb.String()
}
