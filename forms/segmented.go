package forms

import (
	"github.com/ipoluianov/nui/ui"
)

// Segmented is a row of choices of which one is chosen: the views of the
// terminal. It is quieter than buttons, so the Connect button stays the
// one that draws the eye.
type Segmented struct {
	ui.Widget
	items    []string
	widths   []int
	selected int
	hover    int
	// OnChosen gets the index the user clicked
	OnChosen func(i int)
}

const segmentPadding = 14

// NewSegmented makes the choices
func NewSegmented(items ...string) *Segmented {
	s := &Segmented{items: items, hover: -1}
	s.InitWidget()
	s.measure()
	s.SetMouseCursor(ui.MouseCursorPointer)
	s.SetOnPaint(s.paint)
	s.OnMouseDown.Connect(func(e *ui.MouseEvent) {
		if e.Button != ui.MouseButtonLeft {
			return
		}
		if i := s.at(e.X); i >= 0 && s.OnChosen != nil {
			s.OnChosen(i)
		}
	})
	s.OnMouseMove.Connect(func(e *ui.MouseEvent) {
		if i := s.at(e.X); i != s.hover {
			s.hover = i
			s.Update()
		}
	})
	s.OnHoverChanged.Connect(func(in bool) {
		if !in {
			s.hover = -1
			s.Update()
		}
	})
	return s
}

func (s *Segmented) measure() {
	total := 0
	s.widths = s.widths[:0]
	for _, it := range s.items {
		w, _, _ := ui.MeasureText(ui.FontFamilySans, ui.ThemeFontSize(), it)
		s.widths = append(s.widths, w+segmentPadding*2)
		total += w + segmentPadding*2
	}
	h := ui.ThemeControlHeight()
	s.SetMinSize(total+2, h)
	s.SetMaxSize(total+2, h)
}

// SetSelected chooses the item
func (s *Segmented) SetSelected(i int) {
	if s.selected != i {
		s.selected = i
		s.Update()
	}
}

func (s *Segmented) at(x int) int {
	pos := 1
	for i, w := range s.widths {
		if x >= pos && x < pos+w {
			return i
		}
		pos += w
	}
	return -1
}

func (s *Segmented) paint(cnv *ui.Canvas) {
	p := ui.CurrentPalette()
	w, h := s.Width(), s.Height()
	cnv.SetColor(p.Border)
	cnv.FillRoundedRect(0, 0, w, h, 6)
	cnv.SetColor(p.Button)
	cnv.FillRoundedRect(1, 1, w-2, h-2, 5)
	cnv.SetFontFamily(ui.FontFamilySans)
	cnv.SetFontSize(ui.ThemeFontSize())
	cnv.SetHAlign(ui.HAlignCenter)
	cnv.SetVAlign(ui.VAlignCenter)
	x := 1
	for i, it := range s.items {
		sw := s.widths[i]
		switch {
		case i == s.selected:
			cnv.SetColor(mix(p.ButtonText, p.Button, 0.16))
			cnv.FillRoundedRect(x+2, 3, sw-4, h-6, 4)
		case i == s.hover:
			cnv.SetColor(mix(p.ButtonText, p.Button, 0.07))
			cnv.FillRoundedRect(x+2, 3, sw-4, h-6, 4)
		}
		cnv.SetFontBold(i == s.selected)
		if i == s.selected {
			cnv.SetColor(p.ButtonText)
		} else {
			cnv.SetColor(mix(p.ButtonText, p.Button, 0.7))
		}
		cnv.DrawText(x, 0, sw, h, it)
		x += sw
	}
	cnv.SetFontBold(false)
}
