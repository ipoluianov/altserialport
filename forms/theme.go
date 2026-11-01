package forms

import (
	"image/color"

	"github.com/ipoluianov/altserialport/render"
	"github.com/ipoluianov/nui/ui"
)

// The color themes offered in the settings
const (
	themeDark  = ""
	themeLight = "light"
)

// ApplyTheme switches the program to the theme of the settings
func ApplyTheme(theme string) {
	if theme == themeLight {
		ui.ApplyLightTheme()
	} else {
		ui.ApplyDarkTheme()
	}
}

// themeColor is a color of the program in the dark and the light theme
type themeColor struct {
	dark, light color.RGBA
}

func (c themeColor) get() color.RGBA {
	if ui.IsDarkTheme {
		return c.dark
	}
	return c.light
}

func hex(s string) color.RGBA { return ui.ColorFromHex(s) }

var (
	colorTermBack  = themeColor{hex("#16191d"), hex("#fcfcfd")}
	colorTermText  = themeColor{hex("#d7dae0"), hex("#1f2328")}
	colorSent      = themeColor{hex("#5cb3ff"), hex("#0a62c2")}
	colorPortB     = themeColor{hex("#e3a15b"), hex("#a65800")}
	colorNote      = themeColor{hex("#c49a4f"), hex("#9a6a12")}
	colorToken     = themeColor{hex("#c586c0"), hex("#9a3a93")}
	colorGutter    = themeColor{hex("#6b7280"), hex("#8a919b")}
	colorSelection = themeColor{hex("#264f78"), hex("#b6d4f7")}
	colorMatch     = themeColor{hex("#5c4b1a"), hex("#ffe58a")}
	colorMatchCur  = themeColor{hex("#a8710f"), hex("#ffb238")}
	colorPill      = themeColor{hex("#2f6fd6"), hex("#2f6fd6")}
	colorHint      = themeColor{hex("#8b929c"), hex("#7a828c")}
	colorOK        = themeColor{hex("#3fb950"), hex("#1a7f37")}
	colorWait      = themeColor{hex("#d29922"), hex("#b07a0a")}
	colorError     = themeColor{hex("#f85149"), hex("#cf222e")}
	colorRecord    = themeColor{hex("#f85149"), hex("#d1242f")}
	colorLEDOff    = themeColor{hex("#3a3f47"), hex("#c9ced6")}
	colorLink      = themeColor{hex("#3d8bf2"), hex("#1d6fb5")}
	colorSecondary = themeColor{hex("#8b929c"), hex("#6e7781")}
)

// The 16 ANSI colors, for a dark and a light background
var (
	ansiDark = [16]string{
		"#3b4048", "#ef6b73", "#8cc265", "#e5c07b", "#61afef", "#c678dd", "#56b6c2", "#d7dae0",
		"#5c6370", "#ff8a92", "#a5e075", "#f0d197", "#7cc3ff", "#de94f5", "#6fd3df", "#ffffff",
	}
	ansiLight = [16]string{
		"#24292f", "#cf222e", "#116329", "#8a6100", "#0550ae", "#8250df", "#1b7c83", "#6e7781",
		"#57606a", "#a40e26", "#1a7f37", "#7d4e00", "#0969da", "#a475f9", "#3192aa", "#8c959f",
	}
)

// cellColor turns a color of a cell into a color of the screen; def is the
// default one
func cellColor(c render.Color, def color.RGBA) color.RGBA {
	if c == 0 {
		return def
	}
	if r, g, b, ok := c.RGB(); ok {
		return color.RGBA{r, g, b, 255}
	}
	i, _ := c.Palette()
	switch {
	case i < 16:
		if ui.IsDarkTheme {
			return hex(ansiDark[i])
		}
		return hex(ansiLight[i])
	case i < 232: // the 6x6x6 cube
		i -= 16
		level := func(v int) uint8 {
			if v == 0 {
				return 0
			}
			return uint8(55 + v*40)
		}
		return color.RGBA{level(i / 36), level(i / 6 % 6), level(i % 6), 255}
	default: // the grays
		v := uint8(8 + (i-232)*10)
		return color.RGBA{v, v, v, 255}
	}
}

// mix returns a between b (t=0) and a (t=1)
func mix(a, b color.RGBA, t float64) color.RGBA {
	f := func(x, y uint8) uint8 { return uint8(float64(y) + (float64(x)-float64(y))*t) }
	return color.RGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), 255}
}
