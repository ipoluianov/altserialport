package forms

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ipoluianov/altserialport/app"
	"github.com/ipoluianov/altserialport/buffer"
	"github.com/ipoluianov/altserialport/config"
	"github.com/ipoluianov/altserialport/proto"
	"github.com/ipoluianov/altserialport/proto/xmodem"
	"github.com/ipoluianov/altserialport/render"
	"github.com/ipoluianov/altserialport/serial"
	"github.com/ipoluianov/altserialport/session"
	"github.com/ipoluianov/nui/ui"
)

// radio adds checkable items of which one is checked: the current value
// is asked each time the menu opens
func radio[T comparable](menu *ui.ContextMenu, items []T, name func(T) string, cur func() T, set func(T)) {
	var all []*ui.ContextMenuItem
	for _, v := range items {
		v := v
		it := menu.AddItem(name(v), func() { set(v) })
		it.SetCheckable(true)
		all = append(all, it)
	}
	menu.OnShow.Connect(func(*ui.ContextMenu) {
		c := cur()
		for k, it := range all {
			it.SetChecked(items[k] == c)
		}
	})
}

// check adds an item with a check mark that follows a value
func check(menu *ui.ContextMenu, text string, cur func() bool, set func(bool)) *ui.ContextMenuItem {
	var it *ui.ContextMenuItem
	it = menu.AddItem(text, func() { set(it.Checked()) })
	it.SetCheckable(true)
	menu.OnShow.Connect(func(*ui.ContextMenu) { it.SetChecked(cur()) })
	return it
}

// terminalMenu is the context menu of the terminal: the selection, the
// view and the way the data is laid out
func (m *MainForm) terminalMenu() *ui.ContextMenu {
	owner := m.term.area
	menu := ui.NewContextMenu(owner)
	hasSel := func() bool { return m.term.HasSelection() }

	cp := menu.AddItem(T().Copy, m.term.Copy).SetShortcut("Ctrl+Shift+C")
	cpHex := menu.AddItem(T().CopyHex, func() {
		if b := m.term.SelectedBytes(); len(b) > 0 {
			ui.ClipboardSetText(proto.FormatHex(b))
		}
	})
	cpC := menu.AddItem(T().CopyC, func() {
		if b := m.term.SelectedBytes(); len(b) > 0 {
			ui.ClipboardSetText(proto.FormatCArray(b))
		}
	})
	paste := menu.AddItem(T().PasteToDevice, m.pasteToDevice).SetShortcut("Ctrl+Shift+V")
	menu.AddItem(T().SelectAll, m.term.SelectAll).SetShortcut("Ctrl+Shift+A")
	menu.OnShow.Connect(func(*ui.ContextMenu) {
		cp.SetEnabled(hasSel())
		cpHex.SetEnabled(hasSel())
		cpC.SetEnabled(hasSel())
		state, _, _ := m.sess.State()
		paste.SetEnabled(state == session.Open)
	})
	menu.AddSeparator()
	menu.AddItem(T().Find, func() { m.find.Open() }).SetShortcut("Ctrl+F")
	menu.AddItem(T().Clear, m.clear).SetShortcut("Ctrl+Shift+L")
	menu.AddItem(T().SaveAs, m.saveAs)
	menu.AddSeparator()

	times := ui.NewContextMenu(owner)
	radio(times, []TimeMode{TimeOff, TimeClock, TimeMicro, TimeDelta, TimeSince},
		func(t TimeMode) string { return T().TimeModes[t] },
		m.term.TimeMode, m.setTimeMode)
	menu.AddItemWithSubmenu(T().Timestamps, times)

	breaks := ui.NewContextMenu(owner)
	radio(breaks, []buffer.LineBreak{buffer.BreakAuto, buffer.BreakLF, buffer.BreakCR, buffer.BreakCRLF, buffer.BreakNone},
		func(b buffer.LineBreak) string { return T().LineBreaks[b] },
		func() buffer.LineBreak { return m.buf.Options().Break },
		func(b buffer.LineBreak) {
			m.profile.Break = breakNames[b]
			m.setLayout()
		})
	menu.AddItemWithSubmenu(T().LineBreaksMenu, breaks)

	enc := ui.NewContextMenu(owner)
	var encs []render.Encoding
	for _, e := range render.Encodings {
		encs = append(encs, e.Enc)
	}
	radio(enc, encs, render.EncodingName, m.term.Encoding, func(e render.Encoding) {
		m.term.SetEncoding(e)
		m.term.rescan()
		m.saveProfile()
	})
	menu.AddItemWithSubmenu(T().EncodingMenu, enc)

	gap := ui.NewContextMenu(owner)
	radio(gap, []int{0, 2, 5, 10, 20, 50, 100, 500},
		func(ms int) string {
			if ms == 0 {
				return T().Off
			}
			return fmt.Sprintf(T().Milliseconds, ms)
		},
		func() int { return m.profile.GapMs },
		func(ms int) {
			m.profile.GapMs = ms
			m.setLayout()
		})
	menu.AddItemWithSubmenu(T().PacketGap, gap)

	width := ui.NewContextMenu(owner)
	radio(width, []int{8, 16, 24, 32, 64},
		func(n int) string { return fmt.Sprintf(T().BytesPerRow, n) },
		func() int { return m.profile.HexWidth },
		func(n int) {
			m.profile.HexWidth = n
			m.setLayout()
		})
	menu.AddItemWithSubmenu(T().HexWidth, width)
	menu.AddSeparator()

	check(menu, T().WrapLines, m.term.Wrap, func(on bool) {
		m.term.SetWrap(on)
		m.saveProfile()
	})
	check(menu, T().ShowSent, func() bool { return m.profile.ShowTX }, func(on bool) {
		m.profile.ShowTX = on
		m.setLayout()
	})
	check(menu, T().EchoKeys, func() bool { return m.profile.Echo }, func(on bool) {
		m.profile.Echo = on
		m.setLayout()
	})
	bs := ui.NewContextMenu(owner)
	radio(bs, []string{"del", "bs"},
		func(s string) string {
			if s == "bs" {
				return "BS (0x08)"
			}
			return "DEL (0x7F)"
		},
		func() string { return m.profile.Backspace },
		func(s string) {
			m.profile.Backspace = s
			m.saveProfile()
		})
	menu.AddItemWithSubmenu(T().BackspaceMenu, bs)
	check(menu, T().MenuResponseTime, m.term.ResponseTime, func(on bool) {
		m.term.SetResponseTime(on)
		m.profile.HideRT = !on
		m.saveProfile()
	})
	menu.AddSeparator()
	menu.AddItem(T().MenuRules, m.editRules)
	return menu
}

// sendMenu is the context menu of the send line
func (m *MainForm) sendMenu() *ui.ContextMenu {
	owner := m.send.text
	menu := ui.NewContextMenu(owner)
	menu.AddItem(T().Cut, m.send.text.Cut).SetShortcut("Ctrl+X")
	menu.AddItem(T().Copy, m.send.text.Copy).SetShortcut("Ctrl+C")
	menu.AddItem(T().Paste, m.send.text.Paste).SetShortcut("Ctrl+V")
	menu.AddSeparator()

	sums := ui.NewContextMenu(owner)
	var all []proto.Checksum
	for _, c := range proto.Checksums {
		all = append(all, c.Sum)
	}
	radio(sums, all, func(c proto.Checksum) string {
		if c == proto.SumNone {
			return T().Off
		}
		return c.Name()
	}, func() proto.Checksum { return m.checksum }, func(c proto.Checksum) {
		m.checksum = c
		m.saveProfile()
		if c != proto.SumNone {
			m.status.ShowMessage(fmt.Sprintf(T().ChecksumOn, c.Name()))
		}
	})
	menu.AddItemWithSubmenu(T().AppendChecksum, sums)
	menu.AddItem(T().RepeatMenu, m.startRepeat)
	menu.AddItem(T().SendFile, m.sendFile)
	menu.AddSeparator()
	menu.AddItem(T().EscapesHelp, func() {
		ui.ShowMessageBox(m.Form.Panel(), T().EscapesHelpTitle, T().EscapesHelpText)
	})
	menu.AddItem(T().ClearHistory, func() {
		m.send.ClearHistory()
		m.saveProfile()
	})
	return menu
}

// portMenu is the context menu of the port list
func (m *MainForm) portMenu() *ui.ContextMenu {
	menu := ui.NewContextMenu(m.portCombo)
	menu.AddItem(T().RefreshPorts, func() { m.listPortsNow("") })
	menu.AddItem(T().PortDetails, m.portDetails)
	menu.AddItem(T().PortSettings, m.portSettings)
	return menu
}

// linesMenu holds the commands of the lines: the reset of the boards, BREAK
func (m *MainForm) linesMenu(owner ui.Widgeter, menu *ui.ContextMenu) {
	on, off := true, false
	reset := ui.NewContextMenu(owner)
	reset.AddItem(T().ResetDTR, func() {
		m.pulse([]session.LineStep{{DTR: &off, Wait: 100 * time.Millisecond}, {DTR: &on}})
	})
	// The circuit of the ESP32 boards: RTS pulls EN (reset) low, DTR pulls GPIO0 low
	reset.AddItem(T().ResetESPBoot, func() {
		m.pulse([]session.LineStep{
			{DTR: &off, RTS: &on, Wait: 100 * time.Millisecond},
			{DTR: &on, RTS: &off, Wait: 50 * time.Millisecond},
			{DTR: &off},
		})
	})
	reset.AddItem(T().ResetESPRun, func() {
		m.pulse([]session.LineStep{
			{DTR: &off, RTS: &on, Wait: 100 * time.Millisecond},
			{RTS: &off},
		})
	})
	menu.AddItemWithSubmenu(T().ResetBoard, reset)
	menu.AddItem(T().SendBreak, func() {
		if err := m.sess.Break(250 * time.Millisecond); err != nil {
			m.status.ShowMessage(m.errorText(err))
		}
	})
}

// pulse runs a reset sequence; the lines end as the sequence leaves them
func (m *MainForm) pulse(steps []session.LineStep) {
	state, _, _ := m.sess.State()
	if state != session.Open {
		m.status.ShowMessage(T().ErrNotOpen)
		return
	}
	for _, st := range steps {
		if st.DTR != nil {
			m.profile.DTR = *st.DTR
		}
		if st.RTS != nil {
			m.profile.RTS = *st.RTS
		}
	}
	m.sess.Pulse(steps)
	m.saveProfile()
}

// statusMenu is the context menu of the status bar
func (m *MainForm) statusMenu() *ui.ContextMenu {
	menu := ui.NewContextMenu(m.status)
	check(menu, "DTR", func() bool { return m.profile.DTR }, func(bool) { m.toggleLine(true) })
	check(menu, "RTS", func() bool { return m.profile.RTS }, func(bool) { m.toggleLine(false) })
	menu.AddSeparator()
	m.linesMenu(m.status, menu)
	menu.AddSeparator()
	menu.AddItem(T().ResetCounters, func() {
		m.sess.ResetCounters()
		m.updateState()
	})
	return menu
}

// mainMenu is the menu of the button at the right of the toolbar
func (m *MainForm) mainMenu() *ui.ContextMenu {
	owner := m.Form.Panel()
	menu := ui.NewContextMenu(nil)
	menu.AddItem(T().PortSettings, m.portSettings)
	menu.AddItem(T().PortDetails, m.portDetails)
	menu.AddItemWithSubmenu(T().Profiles, m.profilesMenu(owner))
	m.linesMenu(owner, menu)
	menu.AddSeparator()
	check(menu, T().MenuPlot, func() bool { return m.plotter.IsVisible() }, func(bool) { m.togglePlot() }).SetShortcut("Ctrl+Shift+G")
	check(menu, T().MenuMacros, func() bool { return m.macros.IsVisible() }, func(bool) { m.toggleMacros() }).SetShortcut("Ctrl+Shift+M")
	check(menu, T().MenuPort2, func() bool { return m.port2.IsVisible() }, func(bool) { m.togglePort2() })
	tools := ui.NewContextMenu(owner)
	tools.AddItem(T().Statistics, m.showStatistics)
	tools.AddItem(T().Loopback, m.loopbackTest)
	tools.AddItem(T().DetectBaud, m.detectBaud)
	menu.AddItemWithSubmenu(T().MenuTools, tools)
	menu.AddSeparator()
	menu.AddItem(T().SendFile, m.sendFile)
	recv := ui.NewContextMenu(owner)
	recv.AddItem("XMODEM", func() { m.receiveFile(xmodem.XModem) })
	recv.AddItem("XMODEM-1K", func() { m.receiveFile(xmodem.XModem1K) })
	recv.AddItem("YMODEM", func() { m.receiveFile(xmodem.YModem) })
	menu.AddItemWithSubmenu(T().MenuReceive, recv)
	menu.AddItem(T().OpenFile, m.openFile)
	check(menu, T().MenuAutoLog, func() bool { return m.profile.AutoLog }, func(on bool) {
		m.profile.AutoLog = on
		m.saveProfile()
		if on {
			m.startAutoLog()
		}
	})
	menu.AddSeparator()
	menu.AddItem(T().NewWindow, m.newWindow)
	menu.AddSeparator()

	theme := ui.NewContextMenu(owner)
	radio(theme, []string{themeDark, themeLight}, func(t string) string {
		if t == themeLight {
			return T().ThemeLight
		}
		return T().ThemeDark
	}, func() string { return config.Get().Theme }, func(t string) {
		config.Update(func(s *config.Settings) { s.Theme = t })
		ApplyTheme(t)
		m.term.invalidate()
		m.recordHighlight(m.sess.Log() != nil)
	})
	menu.AddItemWithSubmenu(T().Theme, theme)

	lang := ui.NewContextMenu(owner)
	radio(lang, Languages, LanguageName, func() string { return ui.Language() }, func(l string) {
		config.Update(func(s *config.Settings) { s.Language = l })
		SetLanguage(l)
	})
	menu.AddItemWithSubmenu(T().Language, lang)

	font := ui.NewContextMenu(owner)
	font.AddItem(T().FontBigger, func() { m.zoom(1) }).SetShortcut("Ctrl+=")
	font.AddItem(T().FontSmaller, func() { m.zoom(-1) }).SetShortcut("Ctrl+-")
	font.AddItem(T().FontReset, func() { m.zoom(0) }).SetShortcut("Ctrl+0")
	menu.AddItemWithSubmenu(T().FontSize, font)
	check(menu, T().AlwaysOnTop, func() bool { return config.Get().AlwaysOnTop }, func(on bool) {
		config.Update(func(s *config.Settings) { s.AlwaysOnTop = on })
		m.Form.SetAlwaysOnTop(on)
	})
	menu.AddSeparator()
	menu.AddItem(T().Shortcuts, m.showShortcuts).SetShortcut("F1")
	menu.AddItem(T().About, m.about)
	return menu
}

// newWindow starts another copy of the program: a window for another port
func (m *MainForm) newWindow() {
	exe, err := os.Executable()
	if err == nil {
		err = startProcess(exe)
	}
	if err != nil {
		ui.ShowMessageBox(m.Form.Panel(), app.DisplayName, err.Error())
	}
}

// saveAs writes the rows of the terminal to a file as they are shown
func (m *MainForm) saveAs() {
	m.Form.ShowSaveFileDialog(ui.SaveFileDialogOptions{
		Title:            T().SaveAs,
		DefaultFileName:  "terminal_" + time.Now().Format("2006-01-02_15-04-05") + ".txt",
		DefaultDirectory: config.Get().LogDir,
		Filters: []ui.FileDialogFilter{
			{DisplayName: T().LogText, Patterns: []string{"*.txt", "*.log"}},
			{DisplayName: T().MenuHTML, Patterns: []string{"*.html"}},
			{DisplayName: T().MenuHexDump, Patterns: []string{"*.hexdump"}},
			{DisplayName: T().LogRaw, Patterns: []string{"*.bin"}},
		},
	}, func(path string, err error) {
		if err != nil || path == "" {
			return
		}
		var data []byte
		switch strings.ToLower(filepath.Ext(path)) {
		case ".html", ".htm":
			data = m.exportHTML()
		case ".hexdump":
			data = m.exportHexDump()
		case ".bin":
			for i := m.buf.FirstRow(); i < m.buf.EndRow(); i++ {
				if r := m.buf.Row(i); r.Dir == buffer.RX {
					data = m.buf.Data(data, r)
				}
			}
		default:
			for i := m.buf.FirstRow(); i < m.buf.EndRow(); i++ {
				if g := m.term.gutter(i, m.buf.Row(i)); g != "" {
					data = append(data, g+" "...)
				}
				data = append(data, render.String(m.term.cells(i))...)
				data = append(data, '\n')
			}
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			ui.ShowMessageBox(m.Form.Panel(), app.DisplayName, err.Error())
			return
		}
		m.status.ShowMessage(fmt.Sprintf(T().LogSaved, path))
	})
}

// portDetails shows what is known of the selected port
func (m *MainForm) portDetails() {
	p, ok := m.selectedPort()
	state, info, _ := m.sess.State()
	if state != session.Closed {
		p, ok = info, true
	}
	if !ok {
		ui.ShowMessageBox(m.Form.Panel(), T().PortDetails, T().NoPortSelected)
		return
	}
	text := ""
	for _, line := range p.Details() {
		text += line + "\n"
	}
	ui.ShowMessageBox(m.Form.Panel(), T().PortDetails, text)
}

func (m *MainForm) showShortcuts() {
	ui.ShowMessageBox(m.Form.Panel(), T().Shortcuts, T().ShortcutsText)
}

func (m *MainForm) about() {
	ui.ShowMessageBox(m.Form.Panel(), T().About,
		app.DisplayName+" "+app.Version+"\n\n"+T().AboutText+"\n\n"+app.Website+"\n"+app.Copyright()+"\n"+T().LicenseText)
}

// Names of the values kept in the profiles

var flowNames = map[serial.Flow]string{serial.FlowNone: "", serial.FlowRTSCTS: "rtscts", serial.FlowXonXoff: "xonxoff"}

func flowFromName(s string) serial.Flow {
	for f, n := range flowNames {
		if n == s {
			return f
		}
	}
	return serial.FlowNone
}

func flowName(f serial.Flow) string {
	switch f {
	case serial.FlowRTSCTS:
		return "RTS/CTS"
	case serial.FlowXonXoff:
		return "XON/XOFF"
	}
	return ""
}

var breakNames = map[buffer.LineBreak]string{buffer.BreakAuto: "", buffer.BreakLF: "lf",
	buffer.BreakCR: "cr", buffer.BreakCRLF: "crlf", buffer.BreakNone: "none"}

func breakFromName(s string) buffer.LineBreak {
	for b, n := range breakNames {
		if n == s {
			return b
		}
	}
	return buffer.BreakAuto
}

func encodingName(e render.Encoding) string { return render.EncodingName(e) }

func encodingFromName(s string) render.Encoding {
	for _, e := range render.Encodings {
		if e.Name == s {
			return e.Enc
		}
	}
	return render.UTF8
}

func checksumFromName(s string) proto.Checksum {
	for _, c := range proto.Checksums {
		if c.Name == s {
			return c.Sum
		}
	}
	return proto.SumNone
}

// noteText writes the notes of the session in the language of the window
func noteText(n session.Note) string {
	switch n.Kind {
	case session.NoteConnected:
		return fmt.Sprintf(T().NoteConnected, n.Port.Short, n.Config.Baud, n.Config.Frame())
	case session.NoteDisconnected:
		return T().NoteDisconnected
	case session.NoteLost:
		return T().NoteLost
	case session.NoteReconnected:
		return fmt.Sprintf(T().NoteReconnected, n.Port.Short, n.Elapsed.Seconds())
	case session.NoteSettings:
		return fmt.Sprintf(T().NoteSettings, n.Config.Baud, n.Config.Frame())
	}
	return session.DefaultNoteText(n)
}
