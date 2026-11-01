package render

// sgr is the style set by the ANSI escapes of a line (Select Graphic
// Rendition, ESC [ ... m). Each line starts in the default style, as the
// log lines of ESP-IDF, Zephyr and others reset it at their end.
type sgr struct {
	fg, bg Color
	a      Attr
}

func (s *sgr) cell(r rune, b int, extra Attr) Cell {
	c := Cell{R: r, B: int32(b), FG: s.fg, BG: s.bg, A: s.a | extra}
	if s.a&Inverse != 0 {
		c.A &^= Inverse
		c.FG, c.BG = s.bg, s.fg
		if c.FG == 0 && c.BG == 0 {
			c.A |= Inverse // the view swaps its own colors
		}
	}
	return c
}

// escape skips the escape sequence at data[i] (an ESC), applying it when
// it sets the style, and returns the index after it. The sequences that
// move the cursor or clear the screen are dropped: the view is a log.
func (s *sgr) escape(data []byte, i int) int {
	i++ // ESC
	if i >= len(data) {
		return i
	}
	switch data[i] {
	case '[': // CSI: parameters, intermediates, a final byte
		start := i + 1
		j := start
		for j < len(data) && data[j] >= 0x30 && data[j] <= 0x3F {
			j++
		}
		params := data[start:j]
		for j < len(data) && data[j] >= 0x20 && data[j] <= 0x2F {
			j++
		}
		if j >= len(data) {
			return j
		}
		if data[j] == 'm' {
			s.apply(params)
		}
		return j + 1
	case ']', 'P', '_', '^': // OSC, DCS...: up to BEL or ST (ESC \)
		for j := i + 1; j < len(data); j++ {
			if data[j] == 0x07 {
				return j + 1
			}
			if data[j] == 0x1B && j+1 < len(data) && data[j+1] == '\\' {
				return j + 2
			}
		}
		return len(data)
	case '(', ')', '*', '+', '#': // the character set: one more byte
		return min(i+2, len(data))
	}
	return i + 1
}

// apply sets the style by the parameters of an SGR sequence ("1;31")
func (s *sgr) apply(params []byte) {
	var nums []int
	n, have := 0, false
	for _, c := range params {
		switch {
		case c >= '0' && c <= '9':
			n = n*10 + int(c-'0')
			have = true
		case c == ';' || c == ':':
			nums = append(nums, n)
			n, have = 0, false
		default:
			return // private parameters (<, =, >, ?): not a style
		}
	}
	if have || len(nums) == 0 || params[len(params)-1] == ';' {
		nums = append(nums, n)
	}
	for k := 0; k < len(nums); k++ {
		p := nums[k]
		switch {
		case p == 0:
			*s = sgr{}
		case p == 1:
			s.a |= Bold
		case p == 2:
			s.a |= Dim
		case p == 3:
			s.a |= Italic
		case p == 4:
			s.a |= Underline
		case p == 7:
			s.a |= Inverse
		case p == 22:
			s.a &^= Bold | Dim
		case p == 23:
			s.a &^= Italic
		case p == 24:
			s.a &^= Underline
		case p == 27:
			s.a &^= Inverse
		case p >= 30 && p <= 37:
			s.fg = PaletteColor(p - 30)
		case p >= 90 && p <= 97:
			s.fg = PaletteColor(p - 90 + 8)
		case p == 39:
			s.fg = 0
		case p >= 40 && p <= 47:
			s.bg = PaletteColor(p - 40)
		case p >= 100 && p <= 107:
			s.bg = PaletteColor(p - 100 + 8)
		case p == 49:
			s.bg = 0
		case p == 38 || p == 48:
			c, used := extendedColor(nums[k+1:])
			k += used
			if used == 0 {
				continue
			}
			if p == 38 {
				s.fg = c
			} else {
				s.bg = c
			}
		}
	}
}

// extendedColor reads "5;n" (a palette color) or "2;r;g;b" after 38 or 48
func extendedColor(rest []int) (Color, int) {
	if len(rest) >= 2 && rest[0] == 5 {
		return PaletteColor(rest[1]), 2
	}
	if len(rest) >= 4 && rest[0] == 2 {
		return RGBColor(uint8(rest[1]), uint8(rest[2]), uint8(rest[3])), 4
	}
	return 0, len(rest)
}
