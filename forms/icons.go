package forms

import (
	"image"
	"image/color"

	"github.com/ipoluianov/nui/ui"
)

// The icons are paths on a grid of 24 x 24, filled by the even-odd rule:
// a shape inside another one is a hole
var iconPaths = map[string]string{
	"clock": "M12 2a10 10 0 1 0 0 20a10 10 0 1 0 0-20zM12 4a8 8 0 1 1 0 16a8 8 0 1 1 0-16z" +
		"M11 7h2v4.4l3.2 1.9-1 1.7L11 12.6z",
	"pause":  "M6 5h4v14H6zM14 5h4v14h-4z",
	"record": "M12 5a7 7 0 1 0 0 14a7 7 0 1 0 0-14z",
	"stop":   "M6 6h12v12H6z",
	"find": "M10 3a7 7 0 1 0 0 14a7 7 0 1 0 0-14zM10 5a5 5 0 1 1 0 10a5 5 0 1 1 0-10z" +
		"M16.1 14.7l5.3 5.3-1.4 1.4-5.3-5.3z",
	"clear":  "M9 3h6l1 2h4v2H4V5h4zM6 9h12l-1.2 12H7.2zM9 11v8h1.6v-8zM13.4 11v8H15v-8z",
	"menu":   "M4 6h16v2H4zM4 11h16v2H4zM4 16h16v2H4z",
	"up":     "M12 7l7 7-1.4 1.4L12 9.8l-5.6 5.6L5 14z",
	"down":   "M12 17l-7-7 1.4-1.4 5.6 5.6 5.6-5.6L19 10z",
	"close":  "M6.4 5L12 10.6 17.6 5 19 6.4 13.4 12 19 17.6 17.6 19 12 13.4 6.4 19 5 17.6 10.6 12 5 6.4z",
	"send":   "M3 20l18-8L3 4v6l12 2-12 2z",
	"filter": "M3 5h18l-7 8.2V19l-4 2v-7.8z",
	"regex":  "M4 17a2.2 2.2 0 1 0 4.4 0a2.2 2.2 0 1 0-4.4 0zM15 3h2v4.6l4-2.3 1 1.7L18 9.3l4 2.3-1 1.7-4-2.3V15.6h-2V11l-4 2.3-1-1.7 4-2.3L10 7l1-1.7 4 2.3z",
	"plot":   "M3 3h2v16h16v2H3zM7 15l4-5 3 3 5-7 1.6 1.2-6.4 9-3-3-2.8 3.6z",
	"macro":  "M4 4h7v7H4zM13 4h7v7h-7zM4 13h7v7H4zM13 13h7v7h-7zM6 6v3h3V6zM15 6v3h3V6zM6 15v3h3v-3zM15 15v3h3v-3z",
	"plus":   "M11 5h2v6h6v2h-6v6h-2v-6H5v-2h6z",
}

// IconSize is the side of the toolbar icons
const IconSize = 18

// iconColor is the color of the icons in the theme
var iconColor = themeColor{hex("#c9d1d9"), hex("#455a64")}

// drawIcon renders the icon of the name in the color, side pixels wide
func drawIcon(name string, side int, col color.RGBA) image.Image {
	d, ok := iconPaths[name]
	if !ok {
		return nil
	}
	p, err := ui.ParseSVGPath(d)
	if err != nil {
		return nil
	}
	p.SetFillRule(ui.FillRuleEvenOdd)
	// An image is drawn at its size in logical pixels
	cnv, img := ui.NewImageCanvas(side, side, 1)
	cnv.ScaleBy(float64(side)/24, float64(side)/24)
	cnv.FillPath(p, col)
	return img
}

// icon renders the icon in the color of the theme
func icon(name string) image.Image {
	return drawIcon(name, IconSize, iconColor.get())
}

// themedIcon sets the icon now and again when the theme changes
func themedIcon(name string, set func(img image.Image)) {
	set(icon(name))
	ui.OnThemeChanged.Connect(func(bool) { set(icon(name)) })
}
