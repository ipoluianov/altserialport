// Package proto turns what the user types into bytes to send: text with C
// escapes or hex, a line ending, a checksum
package proto

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

var simpleEscapes = map[byte]byte{'r': '\r', 'n': '\n', 't': '\t', '0': 0, '\\': '\\',
	'e': 0x1B, 'a': 7, 'b': 8, 'f': 12, 'v': 11}

// ParseEscapes turns a text with C escapes into bytes: \r \n \t \0 \\ \e
// \a \b \f \v, \xHH and \uHHHH. A backslash before any other character is
// kept as it is, so a Windows path is not broken.
func ParseEscapes(s string) []byte {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			out = append(out, c)
			continue
		}
		e := s[i+1]
		if v, ok := simpleEscapes[e]; ok {
			out = append(out, v)
			i++
			continue
		}
		if (e == 'x' || e == 'X') && i+3 < len(s) && isHex(s[i+2]) && isHex(s[i+3]) {
			v, _ := strconv.ParseUint(s[i+2:i+4], 16, 8)
			out = append(out, byte(v))
			i += 3
			continue
		}
		if e == 'u' && i+5 < len(s) {
			if v, err := strconv.ParseUint(s[i+2:i+6], 16, 32); err == nil {
				out = utf8.AppendRune(out, rune(v))
				i += 5
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// ParseHex reads bytes written in hex: "01 03 00 0A", "0x01,0x03",
// "01030A", "01:03-0A". The separators are spaces, commas, colons, dashes
// and semicolons; a 0x prefix is allowed before each byte.
func ParseHex(s string) ([]byte, error) {
	var out []byte
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == ',' || r == ':' || r == '-' || r == ';' || r == '\t' || r == '\r' || r == '\n'
	})
	for _, f := range fields {
		if len(f) > 2 && (f[:2] == "0x" || f[:2] == "0X") {
			f = f[2:]
		}
		if len(f)%2 == 1 {
			if len(fields) > 1 && len(f) == 1 {
				f = "0" + f // "1 2 3": one digit per byte
			} else {
				return nil, fmt.Errorf("odd number of hex digits in %q", f)
			}
		}
		for i := 0; i < len(f); i += 2 {
			if !isHex(f[i]) || !isHex(f[i+1]) {
				return nil, fmt.Errorf("not a hex byte: %q", f[i:i+2])
			}
			v, _ := strconv.ParseUint(f[i:i+2], 16, 8)
			out = append(out, byte(v))
		}
	}
	if len(out) == 0 && strings.TrimSpace(s) != "" {
		return nil, errors.New("no hex bytes")
	}
	return out, nil
}

// FormatHex writes bytes as "01 03 0A"
func FormatHex(data []byte) string {
	const digits = "0123456789ABCDEF"
	var sb strings.Builder
	for i, c := range data {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteByte(digits[c>>4])
		sb.WriteByte(digits[c&15])
	}
	return sb.String()
}

// FormatCArray writes bytes as a C array: "0x01, 0x03, 0x0A", 16 a line
func FormatCArray(data []byte) string {
	var sb strings.Builder
	sb.WriteString("{\n")
	for i, c := range data {
		if i%16 == 0 {
			sb.WriteString("    ")
		}
		fmt.Fprintf(&sb, "0x%02X", c)
		if i < len(data)-1 {
			sb.WriteByte(',')
			if i%16 != 15 {
				sb.WriteByte(' ')
			}
		}
		if i%16 == 15 || i == len(data)-1 {
			sb.WriteByte('\n')
		}
	}
	sb.WriteString("}")
	return sb.String()
}

// LineEnding is added to a line sent in the text mode
type LineEnding uint8

const (
	EndNone LineEnding = iota
	EndCR
	EndLF
	EndCRLF
)

// LineEndings are the endings offered, with their names
var LineEndings = []struct {
	End  LineEnding
	Name string
}{{EndNone, "None"}, {EndCR, "CR"}, {EndLF, "LF"}, {EndCRLF, "CRLF"}}

// Bytes returns the bytes of the ending
func (e LineEnding) Bytes() []byte {
	switch e {
	case EndCR:
		return []byte{'\r'}
	case EndLF:
		return []byte{'\n'}
	case EndCRLF:
		return []byte{'\r', '\n'}
	}
	return nil
}

// Name returns the short name of the ending
func (e LineEnding) Name() string {
	for _, x := range LineEndings {
		if x.End == e {
			return x.Name
		}
	}
	return "None"
}

// LineEndingByName returns the ending of the name ("CRLF"), none for others
func LineEndingByName(name string) LineEnding {
	for _, x := range LineEndings {
		if x.Name == name {
			return x.End
		}
	}
	return EndNone
}
