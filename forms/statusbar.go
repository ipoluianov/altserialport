package forms

import (
	"fmt"
	"image/color"
	"time"

	"github.com/ipoluianov/nui/ui"
)

// led is a line of the port in the status bar: a light and its name. An
// output line (DTR, RTS) is switched by a click.
type led struct {
	ui.Widget
	name    string
	on      bool
	known   bool // the state is known: the port is open and has the line
	output  bool
	OnClick func()
}

func newLED(name string, output bool) *led {
	l := &led{name: name, output: output}
	l.InitWidget()
	w, h, _ := ui.MeasureText(ui.FontFamilyMono, ui.ThemeFontSize()-2, name)
	l.SetMinSize(w+18, h+6)
	l.SetMaxSize(w+18, h+6)
	if output {
		l.SetMouseCursor(ui.MouseCursorPointer)
	}
	l.SetOnPaint(l.paint)
	l.OnMouseDown.Connect(func(e *ui.MouseEvent) {
		if e.Button == ui.MouseButtonLeft && l.output && l.OnClick != nil {
			l.OnClick()
		}
	})
	return l
}

func (l *led) set(on, known bool) {
	if l.on == on && l.known == known {
		return
	}
	l.on, l.known = on, known
	l.Update()
}

func (l *led) paint(cnv *ui.Canvas) {
	h := l.Height()
	col := colorLEDOff.get()
	if l.on && l.known {
		col = colorOK.get()
		if l.output {
			col = colorSent.get()
		}
	}
	if l.IsHovered() && l.output {
		cnv.SetColor(mix(colorSecondary.get(), ui.CurrentPalette().Window, 0.2))
		cnv.FillRoundedRect(0, 1, l.Width(), h-2, 4)
	}
	cnv.FillCircle(7, float64(h)/2, 3.5, col)
	cnv.SetFontFamily(ui.FontFamilyMono)
	cnv.SetFontSize(ui.ThemeFontSize() - 2)
	text := ui.CurrentPalette().WindowText
	if !l.known {
		text = colorSecondary.get()
	}
	cnv.SetColor(text)
	cnv.SetVAlign(ui.VAlignCenter)
	cnv.DrawText(13, 0, l.Width()-13, h, l.name)
}

// StatusBar is the line under the terminal: the state of the port, the
// counters, the log, the lines
type StatusBar struct {
	ui.Panel
	dot      *ui.Widget
	dotColor color.RGBA
	state    *ui.Label
	port     *ui.Label
	counters *ui.Label
	rate     *ui.Label
	logLabel *clipLabel
	message  *clipLabel
	msgTimer *ui.Timer

	DTR, RTS, CTS, DSR, DCD, RI *led
}

// NewStatusBar makes the status bar
func NewStatusBar() *StatusBar {
	s := &StatusBar{}
	s.InitWidget()
	s.SetPanelPadding(0)
	s.SetCellPadding(0)

	s.dot = &ui.Widget{}
	s.dot.InitWidget()
	s.dot.SetMinSize(16, 16)
	s.dot.SetMaxSize(16, 16)
	s.dot.SetOnPaint(func(cnv *ui.Canvas) {
		cnv.FillCircle(8, 8, 4.5, s.dotColor)
	})
	s.dotColor = colorLEDOff.get()

	s.state = ui.NewLabel("")
	s.port = ui.NewLabel("")
	s.port.SetForegroundColor(colorSecondary.get())
	s.counters = ui.NewLabel("")
	s.counters.SetFontFamily(ui.FontFamilyMono)
	s.rate = ui.NewLabel("")
	s.rate.SetFontFamily(ui.FontFamilyMono)
	s.rate.SetForegroundColor(colorSecondary.get())
	s.logLabel = newClipLabel(colorRecord.get)
	s.logLabel.SetMaxWidth(260)
	s.logLabel.SetVisible(false)
	s.message = newClipLabel(colorWait.get)

	s.DTR = newLED("DTR", true)
	s.RTS = newLED("RTS", true)
	s.CTS = newLED("CTS", false)
	s.DSR = newLED("DSR", false)
	s.DCD = newLED("DCD", false)
	s.RI = newLED("RI", false)

	row := ui.NewHBox()
	row.Add(fixedSpace(6))
	row.Add(s.dot)
	row.Add(s.state)
	row.Add(fixedSpace(12))
	row.Add(s.port)
	row.Add(fixedSpace(16))
	row.Add(s.counters)
	row.Add(fixedSpace(10))
	row.Add(s.rate)
	row.Add(fixedSpace(16))
	row.Add(s.logLabel)
	row.Add(fixedSpace(16))
	row.Add(s.message)
	row.Add(s.DTR)
	row.Add(s.RTS)
	row.Add(fixedSpace(10))
	row.Add(s.CTS)
	row.Add(s.DSR)
	row.Add(s.DCD)
	row.Add(s.RI)
	row.Add(fixedSpace(6))
	s.AddWidget(0, 0, row)

	s.msgTimer = s.NewTimer(4*time.Second, func() { s.message.SetText("") })
	s.msgTimer.SetSingleShot(true)

	ui.OnThemeChanged.Connect(func(bool) {
		s.port.SetForegroundColor(colorSecondary.get())
		s.rate.SetForegroundColor(colorSecondary.get())
	})
	return s
}

func fixedSpace(w int) ui.Widgeter {
	sp := ui.NewSpace()
	sp.SetSize(w, 0)
	return sp
}

// SetState shows the state of the port
func (s *StatusBar) SetState(text string, col color.RGBA, port string) {
	s.state.SetText(text)
	s.port.SetText(port)
	if s.dotColor != col {
		s.dotColor = col
		s.dot.Update()
	}
}

// SetCounters shows the bytes received and sent
func (s *StatusBar) SetCounters(rx, tx int64) {
	s.counters.SetText(fmt.Sprintf("RX %s  TX %s", formatBytes(rx), formatBytes(tx)))
}

// SetLog shows the log being written, or hides it for ""
func (s *StatusBar) SetLog(text string) {
	s.logLabel.SetText(text)
	s.logLabel.SetVisible(text != "")
}

// ShowMessage shows a message for a few seconds
func (s *StatusBar) ShowMessage(text string) {
	s.message.SetText(text)
	s.msgTimer.Start()
}

// formatBytes writes a size: 512 B, 12.4 KB, 3.1 MB
func formatBytes(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/1024/1024)
	}
	return fmt.Sprintf("%.2f GB", float64(n)/1024/1024/1024)
}

// SetRate shows the speed of the data; nothing while it doesn't flow
func (s *StatusBar) SetRate(rx, tx float64) {
	if rx == 0 && tx == 0 {
		s.rate.SetText("")
		return
	}
	s.rate.SetText("↓ " + formatRate(rx) + "  ↑ " + formatRate(tx))
}

// clipLabel is a text that takes the room it is given and ends with "…"
// when it doesn't fit: a long message doesn't widen the window
type clipLabel struct {
	ui.Widget
	text  string
	color func() color.RGBA
}

func newClipLabel(col func() color.RGBA) *clipLabel {
	l := &clipLabel{color: col}
	l.InitWidget()
	_, h, _ := ui.MeasureText(ui.FontFamilySans, ui.ThemeFontSize(), "Ag")
	l.SetMinSize(20, h+4)
	l.SetXExpandable(true)
	l.SetOnPaint(func(cnv *ui.Canvas) {
		if l.text == "" {
			return
		}
		cnv.SetFontFamily(ui.FontFamilySans)
		cnv.SetFontSize(ui.ThemeFontSize())
		cnv.SetColor(l.color())
		cnv.SetVAlign(ui.VAlignCenter)
		text := l.text
		for len(text) > 1 {
			w, _, _ := ui.MeasureText(ui.FontFamilySans, ui.ThemeFontSize(), text)
			if w <= l.Width() {
				break
			}
			r := []rune(text)
			text = string(r[:len(r)-2]) + "…"
		}
		cnv.DrawText(0, 0, l.Width(), l.Height(), text)
	})
	return l
}

func (l *clipLabel) SetText(s string) {
	if l.text != s {
		l.text = s
		l.Update()
	}
}
