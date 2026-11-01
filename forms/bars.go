package forms

import (
	"fmt"
	"time"

	"github.com/ipoluianov/nui/ui"
)

// iconButton is a flat button with an icon of the toolbar size
func iconButton(name string, tooltip func() string, onClick func()) *ui.ToolButton {
	b := ui.NewToolButton(nil, "", onClick)
	themedIcon(name, b.SetImage)
	b.SetFlat(true)
	side := ui.ThemeControlHeight()
	b.SetButtonSize(side, side)
	b.SetTooltipFunc(tooltip)
	b.SetCanBeFocused(false)
	return b
}

// FindBar is the search over the terminal
type FindBar struct {
	ui.Panel
	text  *ui.TextBox
	count *ui.Label
	term  *Terminal
	timer *ui.Timer
	// searched is the text the matches were found for
	searched string
	regex    bool
	err      string // the error of the regular expression

	regexBtn, filterBtn *ui.ToolButton
}

// NewFindBar makes the search bar of the terminal, hidden
func NewFindBar(term *Terminal) *FindBar {
	f := &FindBar{term: term}
	f.InitWidget()
	f.SetPanelPadding(4)
	f.text = ui.NewTextBox()
	f.text.SetHintFunc(func() string { return T().FindHint })
	f.text.SetMinWidth(260)
	f.text.SetMaxWidth(420)
	f.count = ui.NewLabel("")
	f.count.SetMinWidth(110)

	f.timer = f.NewTimer(200*time.Millisecond, f.search)
	f.timer.SetSingleShot(true)
	f.text.OnTextChanged.Connect(func(string) { f.timer.Start() })
	f.text.OnKeyDown.Connect(func(e *ui.KeyEvent) {
		if popupOpen(&f.text.Widget) {
			return
		}
		switch e.Key {
		case ui.KeyEnter:
			if f.timer.Running() || f.searched != f.text.Text() {
				f.timer.Stop()
				f.search()
			} else {
				f.term.NextMatch(e.Mods.Shift)
				f.showCount()
			}
			e.Handled = true
		case ui.KeyEsc:
			f.Close()
			e.Handled = true
		}
	})

	row := ui.NewHBox()
	row.Add(f.text)
	row.Add(iconButton("up", func() string { return T().FindPrev + " (Shift+Enter)" }, func() {
		f.term.NextMatch(true)
		f.showCount()
	}))
	row.Add(iconButton("down", func() string { return T().FindNext + " (Enter)" }, func() {
		f.term.NextMatch(false)
		f.showCount()
	}))
	f.regexBtn = iconButton("regex", func() string { return T().FindRegex }, func() {
		f.regex = !f.regex
		f.regexBtn.SetChecked(f.regex)
		f.search()
	})
	f.filterBtn = iconButton("filter", func() string { return T().FindFilter }, func() {
		f.term.SetFilter(!f.term.Filter())
		f.filterBtn.SetChecked(f.term.Filter())
		f.showCount()
	})
	row.Add(f.regexBtn)
	row.Add(f.filterBtn)
	row.Add(f.count)
	row.AddStretch()
	row.Add(iconButton("close", func() string { return T().FindClose + " (Esc)" }, f.Close))
	f.AddWidget(0, 0, row)
	f.SetVisible(false)
	return f
}

// Open shows the bar and puts the selected text in it
func (f *FindBar) Open() {
	if s := f.term.SelectedText(); s != "" && len(s) < 100 {
		f.text.SetText(s)
	}
	f.SetVisible(true)
	f.text.Focus()
	f.text.SelectAllText()
	f.search()
}

// Close hides the bar and the matches
func (f *FindBar) Close() {
	f.SetVisible(false)
	f.term.SetQuery("", false)
	if f.term.Filter() {
		f.term.SetFilter(false)
		f.filterBtn.SetChecked(false)
	}
	f.term.Focus()
}

func (f *FindBar) search() {
	f.searched = f.text.Text()
	f.err = ""
	if _, err := f.term.SetQuery(f.searched, f.regex); err != nil {
		f.err = T().FindBadRegex
	}
	f.showCount()
}

// showCount shows "3 of 12"; called also when new rows come
func (f *FindBar) showCount() {
	if !f.IsVisible() {
		return
	}
	n, cur := f.term.Matches()
	switch {
	case f.err != "":
		f.count.SetText(f.err)
	case f.text.Text() == "":
		f.count.SetText("")
	case n == 0:
		f.count.SetText(T().FindNone)
	default:
		f.count.SetText(fmt.Sprintf(T().FindCount, cur, n))
	}
}

// SendBar is the line of the commands under the terminal
type SendBar struct {
	ui.Panel
	text   *ui.TextBox
	mode   *ui.ComboBox
	ending *ui.ComboBox
	button *ui.Button

	history []string
	hpos    int    // the place in the history while it is walked with Up and Down
	draft   string // what was typed before the walk

	// OnSend gets the text typed when it is sent
	OnSend func(text string)
	// OnChanged tells that the mode or the ending changed
	OnChanged func()
}

// NewSendBar makes the send line
func NewSendBar() *SendBar {
	s := &SendBar{}
	s.InitWidget()
	s.SetPanelPadding(4)
	s.text = ui.NewTextBox()
	s.text.SetXExpandable(true)
	s.text.SetFontFamily(ui.FontFamilyMono)
	s.text.SetHintFunc(func() string {
		if s.HexMode() {
			return T().SendHintHex
		}
		return T().SendHint
	})
	s.text.OnKeyDown.Connect(s.keyDown)

	s.mode = ui.NewComboBox()
	s.mode.AddItem("Text", "text")
	s.mode.AddItem("Hex", "hex")
	s.mode.SetTooltipFunc(func() string { return T().SendModeTip })
	s.mode.SetMinWidth(80)
	s.mode.SetMaxWidth(80)
	s.mode.OnActivated.Connect(func(int) {
		s.ending.SetEnabled(!s.HexMode())
		s.text.Update()
		s.changed()
	})

	s.ending = ui.NewComboBox()
	for _, e := range []string{"None", "CR", "LF", "CRLF"} {
		s.ending.AddItem(e, e)
	}
	s.ending.SetTooltipFunc(func() string { return T().LineEndingTip })
	s.ending.SetMinWidth(90)
	s.ending.SetMaxWidth(90)
	s.ending.OnActivated.Connect(func(int) { s.changed() })

	s.button = ui.NewButton("")
	s.button.SetTextFunc(func() string { return T().Send })
	s.button.OnClick.Connect(func(ui.Widgeter) { s.submit() })
	s.button.SetCanBeFocused(false)
	s.button.SetMinWidth(110)
	s.button.SetMaxWidth(110)

	row := ui.NewHBox()
	row.Add(s.text)
	row.Add(s.mode)
	row.Add(s.ending)
	row.Add(s.button)
	s.AddWidget(0, 0, row)
	return s
}

func (s *SendBar) changed() {
	if s.OnChanged != nil {
		s.OnChanged()
	}
}

// HexMode tells whether the line is hex bytes
func (s *SendBar) HexMode() bool { return s.mode != nil && s.mode.SelectedItemData() == "hex" }

// SetHexMode sets the mode of the line
func (s *SendBar) SetHexMode(hex bool) {
	if hex {
		s.mode.SetSelectedIndex(1)
	} else {
		s.mode.SetSelectedIndex(0)
	}
	s.ending.SetEnabled(!hex)
}

// Ending returns the name of the line ending: "None", "CR", "LF", "CRLF"
func (s *SendBar) Ending() string {
	if v, ok := s.ending.SelectedItemData().(string); ok {
		return v
	}
	return "CRLF"
}

// SetEnding sets the line ending by its name
func (s *SendBar) SetEnding(name string) {
	if i := s.ending.FindData(name); i >= 0 {
		s.ending.SetSelectedIndex(i)
	}
}

// History returns the commands sent, the oldest first
func (s *SendBar) History() []string { return s.history }

// SetHistory sets the commands Up and Down walk through
func (s *SendBar) SetHistory(h []string) {
	s.history = append([]string(nil), h...)
	s.hpos = len(s.history)
}

// ClearHistory forgets the commands sent
func (s *SendBar) ClearHistory() { s.SetHistory(nil) }

// Focus puts the keyboard into the line
func (s *SendBar) Focus() { s.text.Focus() }

// Text is the line typed
func (s *SendBar) Text() string { return s.text.Text() }

func (s *SendBar) submit() {
	text := s.text.Text()
	if s.OnSend != nil {
		s.OnSend(text)
	}
}

// Sent remembers the line sent and clears it
func (s *SendBar) Sent(text string) {
	if text != "" && (len(s.history) == 0 || s.history[len(s.history)-1] != text) {
		s.history = append(s.history, text)
	}
	s.hpos = len(s.history)
	s.draft = ""
	s.text.SetText("")
}

func (s *SendBar) keyDown(e *ui.KeyEvent) {
	if popupOpen(&s.text.Widget) {
		return
	}
	switch e.Key {
	case ui.KeyEnter:
		s.submit()
	case ui.KeyArrowUp:
		if s.hpos == 0 {
			break
		}
		if s.hpos == len(s.history) {
			s.draft = s.text.Text()
		}
		s.hpos--
		s.text.SetText(s.history[s.hpos])
		s.text.MoveCursorToEnd()
	case ui.KeyArrowDown:
		if s.hpos >= len(s.history) {
			break
		}
		s.hpos++
		if s.hpos == len(s.history) {
			s.text.SetText(s.draft)
		} else {
			s.text.SetText(s.history[s.hpos])
		}
		s.text.MoveCursorToEnd()
	default:
		return
	}
	e.Handled = true
}
