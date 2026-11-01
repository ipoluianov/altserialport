package forms

import (
	"errors"
	"fmt"
	"image"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ipoluianov/altserialport/app"
	"github.com/ipoluianov/altserialport/buffer"
	"github.com/ipoluianov/altserialport/config"
	"github.com/ipoluianov/altserialport/install"
	"github.com/ipoluianov/altserialport/proto"
	"github.com/ipoluianov/altserialport/remote"
	"github.com/ipoluianov/altserialport/rules"
	"github.com/ipoluianov/altserialport/serial"
	"github.com/ipoluianov/altserialport/session"
	"github.com/ipoluianov/nui/ui"
)

// otherPort is the data of the last item of the port list: a path typed by hand
const otherPort = -1

// newPortTime is how long a port that has just appeared is marked as new
const newPortTime = 20 * time.Second

// MainForm is the window of the program: one port
type MainForm struct {
	Form   *ui.Form
	sess   *session.Session
	buf    *buffer.Buffer
	term   *Terminal
	find   *FindBar
	send   *SendBar
	status *StatusBar

	portCombo  *ui.ComboBox
	baudCombo  *ui.EditableComboBox
	frameCombo *ui.ComboBox
	connectBtn *ui.Button

	views             *Segmented
	actTime, actPause *ui.Action
	actRecord         *ui.Action
	recordBtn         *ui.ToolButton

	ports     []serial.PortInfo // the ports in the list, in its order
	custom    *serial.PortInfo  // a path typed by hand
	firstSeen map[string]time.Time
	listed    bool // the ports were listed once: what appears later is new
	listing   atomic.Bool
	lastList  atomic.Value // the list seen last, as text
	marked    atomic.Bool  // some port in the list is marked as new

	key     string // the device whose profile is in use
	profile config.Profile
	cfg     serial.Config

	pending atomic.Bool // the session has chunks to take

	port2         *Port2
	installStatus install.Status
	menuBtn       *ui.ToolButton
	stopRemote    func()
	plotter       *Plotter
	macros        *MacroPanel
	hsplit        *ui.Splitter // the terminal and the macros
	vsplit        *ui.Splitter // the terminal and the plot
	ruleSet       *rules.Set
	lines         proto.Lines
	lastTrigger   map[int]time.Time
	stats         stats
	lastTX        int64 // the time of the last chunk sent, for the response times
	checksum      proto.Checksum
	repeat        *ui.Timer
	repeatText    string
	saveTimer     *ui.Timer
}

// NewMainForm builds the window
func NewMainForm() *MainForm {
	m := &MainForm{firstSeen: map[string]time.Time{}}
	m.Form = ui.NewForm()
	m.Form.SetTitle(app.DisplayName)
	m.Form.SetSize(1100, 700)

	st := config.Get()
	m.buf = buffer.New(buffer.DefaultOptions(), 256<<20, st.ScrollbackRows)
	m.sess = session.New()
	m.sess.NoteText = noteText
	m.sess.Notify = func() { m.pending.Store(true) }

	m.term = NewTerminal(m.buf)
	m.term.SetFontSize(float64(st.FontSize))
	m.term.Hint = m.hint
	m.term.PillText = func(paused bool) string {
		if paused {
			return T().PausedPill
		}
		return T().NewDataPill
	}
	m.term.OnSend = m.sendKeys
	m.term.KeyBytes = m.keyBytes
	m.term.area.SetContextMenu(m.terminalMenu())

	m.find = NewFindBar(m.term)
	m.send = NewSendBar()
	m.send.OnSend = m.sendLine
	m.send.OnChanged = m.saveProfile
	m.send.text.SetContextMenu(m.sendMenu())
	m.status = NewStatusBar()
	m.status.DTR.OnClick = func() { m.toggleLine(true) }
	m.status.RTS.OnClick = func() { m.toggleLine(false) }
	m.status.SetContextMenu(m.statusMenu())
	// A downloaded copy offers to install or update itself, the installed one to be removed
	m.installStatus = install.CurrentStatus()
	if m.installStatus != install.StatusNone {
		m.status.AddLink(func() string { return installText(m.installStatus) }, m.onInstallLink)
	}

	p := m.Form.Panel()
	p.SetPanelPadding(0)
	p.SetCellPadding(0)
	m.port2 = newPort2(m)
	p.AddWidget(0, 0, m.toolbar())
	p.AddWidget(1, 0, m.port2)
	p.AddWidget(2, 0, m.find)
	p.AddWidget(3, 0, m.center(st))
	p.AddWidget(4, 0, m.send)
	p.AddWidget(5, 0, m.status)

	m.shortcuts()

	m.saveTimer = m.status.NewTimer(time.Second, func() { config.Save() })
	m.saveTimer.SetSingleShot(true)
	m.repeat = m.status.NewTimer(time.Second, m.repeatSend)

	// The data of the session, by frames: a fast port doesn't repaint the
	// window on every read
	m.term.area.AddTimer(30, m.pull)
	m.status.AddTimer(1000, m.tick)
	m.status.AddTimer(200, m.pollLines)
	m.status.AddTimer(1000, m.listPorts)

	m.setRules(st.Rules)
	m.applyProfile(config.ProfileFor(st.LastPort))
	m.listPortsNow(st.LastPort)
	m.updateState()
	m.Form.SetOnFilesDropped(func(files []string, x, y int) {
		if len(files) > 0 {
			m.loadFile(files[0])
		}
	})
	// The command line and the MCP server of the program attach to the
	// port of this window when they find it open here
	if stop, err := remote.Serve(m.sess); err == nil {
		m.stopRemote = stop
	}
	m.Form.OnClose = func() bool {
		if m.stopRemote != nil {
			m.stopRemote()
		}
		m.sess.StopLog()
		m.port2.setBridge(false)
		m.port2.sess.Close()
		m.sess.Close()
		m.saveProfile()
		config.Save()
		return true
	}
	return m
}

// toolbar builds the row of the main functions
func (m *MainForm) toolbar() ui.Widgeter {
	tb := ui.NewToolBar()
	tb.SetPanelPadding(0)

	m.portCombo = ui.NewComboBox()
	m.portCombo.SetMinWidth(240)
	m.portCombo.SetTooltipFunc(func() string { return T().PortTip })
	m.portCombo.OnActivated.Connect(func(int) { m.portChosen() })
	m.portCombo.SetContextMenu(m.portMenu())
	tb.Add(m.portCombo)

	m.baudCombo = ui.NewEditableComboBox()
	var bauds []string
	for _, b := range serial.StandardBauds {
		bauds = append(bauds, strconv.Itoa(b))
	}
	m.baudCombo.SetItems(bauds)
	m.baudCombo.SetMinWidth(110)
	m.baudCombo.SetMaxWidth(110)
	m.baudCombo.SetTooltipFunc(func() string { return T().BaudTip })
	m.baudCombo.SetOnItemSelected(func(int, string) { m.baudChosen() })
	m.baudCombo.SetOnAccept(func(string) { m.baudChosen() })
	m.baudCombo.TextBox().OnEditingFinished.Connect(func(string) { m.baudChosen() })
	tb.Add(m.baudCombo)

	m.frameCombo = ui.NewComboBox()
	for _, f := range serial.Frames {
		m.frameCombo.AddItem(f, f)
	}
	m.frameCombo.AddItem("…", "")
	m.frameCombo.SetTooltipFunc(func() string { return T().FrameTip })
	m.frameCombo.OnActivated.Connect(func(int) { m.frameChosen() })
	tb.Add(m.frameCombo)

	m.connectBtn = ui.NewButton("")
	m.connectBtn.SetMinWidth(120)
	m.connectBtn.OnClick.Connect(func(ui.Widgeter) { m.toggleConnect() })
	m.connectBtn.SetTooltipFunc(func() string { return T().ConnectTip + " (F5)" })
	m.connectBtn.SetCanBeFocused(false)
	tb.Add(m.connectBtn)
	tb.AddSpacing(14)

	m.views = NewSegmented("Text", "Hex", "Mixed")
	m.views.OnChosen = func(i int) { m.setMode(buffer.Mode(i)) }
	m.views.SetTooltipFunc(func() string {
		switch m.views.hover {
		case 1:
			return T().ViewHexTip + " (Ctrl+2)"
		case 2:
			return T().ViewMixedTip + " (Ctrl+3)"
		}
		return T().ViewTextTip + " (Ctrl+1)"
	})
	tb.Add(m.views)
	tb.AddSpacing(14)

	m.actTime = ui.NewAction("", func() {
		mode := TimeOff
		if m.actTime.Checked() {
			mode = TimeClock
			if m.profile.Time != "" && m.profile.Time != "off" {
				mode = timeModeFromName(m.profile.Time)
			}
		}
		m.setTimeMode(mode)
	}).SetCheckable(true)
	themedIcon("clock", func(img image.Image) { m.actTime.SetIcon(img) })
	m.actTime.SetTooltip(T().TimestampsTip + " (Ctrl+Shift+T)")
	addTool(tb, m.actTime)

	m.actPause = ui.NewAction("", func() { m.setPaused(m.actPause.Checked()) }).SetCheckable(true)
	themedIcon("pause", func(img image.Image) { m.actPause.SetIcon(img) })
	m.actPause.SetTooltip(T().PauseTip + " (Ctrl+Shift+P)")
	addTool(tb, m.actPause)

	m.actRecord = ui.NewAction("", m.toggleRecord).SetCheckable(true)
	themedIcon("record", func(img image.Image) { m.actRecord.SetIcon(img) })
	m.actRecord.SetTooltip(T().RecordTip + " (Ctrl+Shift+R)")
	if b, ok := addTool(tb, m.actRecord).(*ui.ToolButton); ok {
		m.recordBtn = b
	}

	find := ui.NewAction("", func() { m.find.Open() })
	themedIcon("find", func(img image.Image) { find.SetIcon(img) })
	find.SetTooltip(T().FindTip + " (Ctrl+F)")
	addTool(tb, find)

	clearAct := ui.NewAction("", m.clear)
	themedIcon("clear", func(img image.Image) { clearAct.SetIcon(img) })
	clearAct.SetTooltip(T().ClearTip + " (Ctrl+Shift+L)")
	addTool(tb, clearAct)

	tb.SetXExpandable(true)
	row := ui.NewPanel()
	row.SetPanelPadding(4)
	row.AddWidget(0, 0, m.menuButton())
	row.AddWidget(0, 1, tb)
	return row
}

// menuButton is the first button of the toolbar: everything that is not
// on the toolbar is in its menu
func (m *MainForm) menuButton() *ui.ToolButton {
	b := iconButton("menu", func() string { return T().MenuTip }, nil)
	b.SetOnClick(func() {
		x, y := b.RectClientAreaOnWindow()
		m.Form.ShowContextMenu(m.mainMenu(), x, y+b.Height())
	})
	m.menuBtn = b
	return b
}

// addTool adds the button of the action, out of the focus chain
func addTool(tb *ui.ToolBar, a *ui.Action) ui.Widgeter {
	w := tb.AddAction(a)
	if b, ok := w.(*ui.ToolButton); ok {
		b.SetCanBeFocused(false)
	}
	return w
}

func (m *MainForm) shortcuts() {
	f := m.Form
	f.AddShortcut("F5", m.toggleConnect)
	f.AddShortcut("Mod+1", func() { m.setMode(buffer.ModeText) })
	f.AddShortcut("Mod+2", func() { m.setMode(buffer.ModeHex) })
	f.AddShortcut("Mod+3", func() { m.setMode(buffer.ModeMixed) })
	f.AddShortcut("Mod+Shift+T", func() { m.actTime.Trigger() })
	f.AddShortcut("Mod+Shift+P", func() { m.actPause.Trigger() })
	f.AddShortcut("Mod+Shift+R", func() { m.actRecord.Trigger() })
	f.AddShortcut("Mod+Shift+L", m.clear)
	f.AddShortcut("Mod+F", func() { m.find.Open() })
	f.AddShortcut("Mod+Shift+G", m.togglePlot)
	f.AddShortcut("Mod+Shift+M", m.toggleMacros)
	for i := 0; i < MaxMacros; i++ {
		i := i
		f.AddShortcut(fmt.Sprintf("Shift+F%d", i+1), func() { m.runMacro(i) })
	}
	f.AddShortcut("F3", func() { m.term.NextMatch(false); m.find.showCount() })
	f.AddShortcut("Shift+F3", func() { m.term.NextMatch(true); m.find.showCount() })
	f.AddShortcut("Mod+Shift+V", m.pasteToDevice)
	f.AddShortcut("Shift+Insert", m.pasteToDevice)
	f.AddShortcut("Mod+=", func() { m.zoom(1) })
	f.AddShortcut("Mod+Shift+=", func() { m.zoom(1) })
	f.AddShortcut("Mod+-", func() { m.zoom(-1) })
	f.AddShortcut("Mod+0", func() { m.zoom(0) })
	f.AddShortcut("F6", func() {
		if m.send.text.IsFocused() {
			m.term.Focus()
		} else {
			m.send.Focus()
		}
	})
	f.AddShortcut("F1", m.showShortcuts)
}

// hint is the text of the empty terminal
func (m *MainForm) hint() []string {
	state, info, _ := m.sess.State()
	switch state {
	case session.Open:
		return []string{fmt.Sprintf(T().HintOpenTitle, info.Short), T().HintOpen1, T().HintOpen2}
	case session.Waiting:
		return []string{T().HintWaitingTitle, T().HintWaiting1}
	}
	if len(m.ports) == 0 {
		return []string{T().HintNoPortsTitle, T().HintNoPorts1, T().HintNoPorts2}
	}
	return []string{T().HintTitle, T().Hint1, T().Hint2}
}

// pull takes the chunks of the session; a timer calls it
func (m *MainForm) pull() {
	if !m.pending.Swap(false) {
		return
	}
	m.port2.take()
	chunks := m.sess.Drain()
	m.term.Append(chunks)
	m.lookAtLines(chunks)
	m.stats.count(chunks)
	m.noteResponse(chunks)
	m.find.showCount()
	m.updateState()
}

// updateState shows the state of the session in the toolbar, the title
// and the status bar
func (m *MainForm) updateState() {
	state, info, cfg := m.sess.State()
	rx, tx := m.sess.Counters()
	m.status.SetCounters(rx, tx)
	portText := ""
	if state != session.Closed {
		portText = fmt.Sprintf("%s  %d %s", info.Short, cfg.Baud, cfg.Frame())
		if cfg.Flow != serial.FlowNone {
			portText += " " + flowName(cfg.Flow)
		}
	}
	switch state {
	case session.Open:
		m.connectBtn.SetText(T().Disconnect)
		m.connectBtn.SetRole("")
		m.status.SetState(T().StateOpen, colorOK.get(), portText)
	case session.Waiting:
		m.connectBtn.SetText(T().Disconnect)
		m.connectBtn.SetRole("")
		text := T().StateWaiting
		if err := m.sess.LastError(); err != nil && !errors.Is(err, serial.ErrNotFound) {
			text += " — " + m.errorText(err)
		}
		m.status.SetState(text, colorWait.get(), portText)
	default:
		m.connectBtn.SetText(T().Connect)
		m.connectBtn.SetRole("primary")
		m.status.SetState(T().StateClosed, colorLEDOff.get(), "")
	}
	m.portCombo.SetEnabled(state == session.Closed)
	if l := m.sess.Log(); l != nil {
		size, err := l.Size()
		text := fmt.Sprintf("● %s  %s", filepath.Base(l.Path()), formatBytes(size))
		if err != nil {
			text = "● " + err.Error()
		}
		m.status.SetLog(text)
	} else {
		m.status.SetLog("")
	}
	title := app.DisplayName
	if state != session.Closed {
		title = info.Short + " — " + app.DisplayName
		if state == session.Waiting {
			title = "… " + title
		}
	}
	m.Form.SetTitle(title)
}

// pollLines reads the input lines of the port
func (m *MainForm) pollLines() {
	sig, ok := m.sess.Signals()
	m.status.CTS.set(sig.CTS, ok)
	m.status.DSR.set(sig.DSR, ok)
	m.status.DCD.set(sig.DCD, ok)
	m.status.RI.set(sig.RI, ok)
	state, _, _ := m.sess.State()
	dtr, rts := m.sess.Lines()
	open := state == session.Open
	m.status.DTR.set(dtr, open)
	m.status.RTS.set(rts, open)
	if !open {
		m.status.DTR.set(m.profile.DTR, false)
		m.status.RTS.set(m.profile.RTS, false)
	}
	rx, tx := m.sess.Counters()
	m.status.SetCounters(rx, tx)
	if m.sess.Log() != nil {
		m.updateState()
	}
}

func (m *MainForm) toggleLine(dtr bool) {
	cur := m.profile.RTS
	if dtr {
		cur = m.profile.DTR
	}
	on := !cur
	if dtr {
		m.profile.DTR = on
		m.sess.SetDTR(on)
	} else {
		m.profile.RTS = on
		m.sess.SetRTS(on)
	}
	m.saveProfile()
	m.pollLines()
}

// listPorts lists the ports in the background; a timer calls it
func (m *MainForm) listPorts() {
	if m.listing.Swap(true) {
		return
	}
	go func() {
		ports, _ := serial.List()
		// The window is touched only when the list changed, or a mark of a
		// new port is to go
		sig := fmt.Sprint(ports)
		if sig == m.lastList.Load() && !m.marked.Load() {
			m.listing.Store(false)
			return
		}
		m.lastList.Store(sig)
		ui.Invoke(func() {
			m.listing.Store(false)
			m.setPorts(ports, "")
		})
	}()
}

// listPortsNow lists the ports at once and selects the device of the key
func (m *MainForm) listPortsNow(key string) {
	ports, _ := serial.List()
	m.setPorts(ports, key)
}

// setPorts puts the ports in the list, keeping the one selected; select
// is the key of a device to select instead
func (m *MainForm) setPorts(ports []serial.PortInfo, selectKey string) {
	now := time.Now()
	var appeared []serial.PortInfo
	for _, p := range ports {
		if _, ok := m.firstSeen[p.Name]; !ok {
			if m.listed {
				m.firstSeen[p.Name] = now
				appeared = append(appeared, p)
			} else {
				m.firstSeen[p.Name] = time.Time{}
			}
		}
	}
	// A port that is gone and comes back is new again
	for name := range m.firstSeen {
		found := false
		for _, p := range ports {
			found = found || p.Name == name
		}
		if !found {
			delete(m.firstSeen, name)
		}
	}
	m.listed = true

	cur, hasCur := m.selectedPort()
	state, sessInfo, _ := m.sess.State()
	if state != session.Closed {
		cur, hasCur = sessInfo, true
	}
	if m.custom != nil {
		ports = append(ports, *m.custom)
	}
	// The open port stays in the list while its device is away
	if hasCur && state != session.Closed {
		found := false
		for _, p := range ports {
			found = found || p.Name == cur.Name
		}
		if !found {
			ports = append(ports, cur)
		}
	}

	titles := make([]string, len(ports))
	marked := false
	for i, p := range ports {
		titles[i] = p.Title()
		if t := m.firstSeen[p.Name]; !t.IsZero() && now.Sub(t) < newPortTime {
			titles[i] += "  ● " + T().NewPort
			marked = true
		}
	}
	m.marked.Store(marked)
	if m.port2 != nil && m.port2.IsVisible() {
		m.port2.setPorts(ports)
	}
	same := len(ports) == len(m.ports) && m.portCombo.ItemCount() == len(ports)+1
	for i := 0; same && i < len(ports); i++ {
		same = ports[i] == m.ports[i] && m.portCombo.ItemText(i) == titles[i]
	}
	if same && selectKey == "" && len(appeared) == 0 {
		return
	}
	m.ports = ports
	m.portCombo.Clear()
	for i, p := range ports {
		m.portCombo.AddItem(titles[i], i)
		_ = p
	}
	m.portCombo.AddItem(T().OtherPort, otherPort)

	sel := -1
	for i, p := range ports {
		if selectKey != "" && p.Key() == selectKey {
			sel = i
			break
		}
	}
	if sel < 0 && hasCur {
		for i, p := range ports {
			if p.Name == cur.Name {
				sel = i
				break
			}
		}
	}
	// A device plugged in while the port is closed is what the user wants
	if state == session.Closed && len(appeared) > 0 {
		for i, p := range ports {
			if p.Name == appeared[0].Name {
				sel = i
			}
		}
		m.status.ShowMessage(fmt.Sprintf(T().PortAppeared, appeared[0].Title()))
	}
	if sel < 0 && len(ports) > 0 {
		sel = 0
	}
	if sel >= 0 {
		m.portCombo.SetSelectedIndex(sel)
		if state == session.Closed && (!hasCur || ports[sel].Name != cur.Name || selectKey != "") {
			m.portChosen()
		}
	}
	if len(ports) == 0 {
		m.portCombo.SetSelectedIndex(-1)
	}
	m.term.area.Update()
}

// selectedPort returns the port chosen in the list
func (m *MainForm) selectedPort() (serial.PortInfo, bool) {
	i := m.portCombo.SelectedIndex()
	if i < 0 || i >= len(m.ports) {
		return serial.PortInfo{}, false
	}
	return m.ports[i], true
}

// portChosen loads the profile of the device chosen
func (m *MainForm) portChosen() {
	if d, ok := m.portCombo.SelectedItemData().(int); ok && d == otherPort {
		m.askPath()
		return
	}
	p, ok := m.selectedPort()
	if !ok {
		return
	}
	if p.Key() == m.key {
		return
	}
	m.saveProfile()
	m.key = p.Key()
	m.applyProfile(config.ProfileFor(m.key))
}

// askPath asks for a port that is not in the list: a pseudo terminal, a
// symlink of udev, a COM port of a driver Windows doesn't list
func (m *MainForm) askPath() {
	value := ""
	if m.custom != nil {
		value = m.custom.Name
	} else if runtime.GOOS == "windows" {
		value = "COM"
	} else {
		value = "/dev/"
	}
	restore := func() {
		cur, _ := m.selectedPort()
		for i, p := range m.ports {
			if p.Name == cur.Name {
				m.portCombo.SetSelectedIndex(i)
			}
		}
	}
	prev := m.key
	ui.ShowInputDialog(m.Form.Panel(), ui.InputDialog{Title: T().OtherPortTitle, Prompt: T().OtherPortPrompt, Value: value},
		func(path string) {
			path = strings.TrimSpace(path)
			if path == "" {
				return
			}
			p := serial.PortInfo{Name: path, Short: filepath.Base(path), Description: T().OtherPortDesc}
			if serial.IsNetwork(path) {
				p = serial.NetworkInfo(path)
			}
			m.custom = &p
			m.listPortsNow(p.Key())
		})
	// Until a path is given the list shows the port before
	if prev != "" {
		for i, p := range m.ports {
			if p.Key() == prev {
				m.portCombo.SetSelectedIndex(i)
				return
			}
		}
	}
	restore()
}

// applyProfile shows the profile in the widgets and the terminal
func (m *MainForm) applyProfile(p config.Profile) {
	m.profile = p
	m.cfg = serial.DefaultConfig()
	m.cfg.Baud = p.Baud
	m.cfg.SetFrame(p.Frame)
	m.cfg.Flow = flowFromName(p.Flow)
	m.cfg.RS485 = p.RS485
	m.term.SetResponseTime(!p.HideRT)
	m.baudCombo.SetText(strconv.Itoa(p.Baud))
	m.showFrame()

	m.term.SetEncoding(encodingFromName(p.Encoding))
	m.term.SetTimeMode(timeModeFromName(p.Time))
	m.term.SetWrap(p.Wrap)
	m.actTime.SetChecked(m.term.TimeMode() != TimeOff)
	m.term.SetLayout(m.layoutOptions())
	m.showMode()

	m.send.SetHexMode(p.SendMode == "hex")
	m.send.SetEnding(p.LineEnding)
	m.send.SetHistory(p.History)
	m.checksum = checksumFromName(p.Checksum)
}

// layoutOptions are the options of the buffer by the profile
func (m *MainForm) layoutOptions() buffer.Options {
	p := m.profile
	o := buffer.DefaultOptions()
	switch p.View {
	case "hex":
		o.Mode = buffer.ModeHex
	case "mixed":
		o.Mode = buffer.ModeMixed
	}
	o.Break = breakFromName(p.Break)
	o.HexWidth = p.HexWidth
	o.Gap = time.Duration(p.GapMs) * time.Millisecond
	o.ShowTX = p.ShowTX
	o.Echo = p.Echo
	return o
}

// saveProfile remembers the state of the widgets in the profile of the device
func (m *MainForm) saveProfile() {
	p := &m.profile
	p.Baud = m.cfg.Baud
	p.Frame = m.cfg.Frame()
	p.Flow = flowNames[m.cfg.Flow]
	p.RS485 = m.cfg.RS485
	p.Time = timeModeNames[m.term.TimeMode()]
	p.Encoding = encodingName(m.term.Encoding())
	p.Wrap = m.term.Wrap()
	p.SendMode = "text"
	if m.send.HexMode() {
		p.SendMode = "hex"
	}
	p.LineEnding = m.send.Ending()
	p.Checksum = m.checksum.Name()
	p.History = m.send.History()
	if m.key == "" {
		return
	}
	config.SetProfile(m.key, *p)
	m.saveTimer.Start()
}

// setLayout lays the rows out again after a change of the profile
func (m *MainForm) setLayout() {
	m.term.SetLayout(m.layoutOptions())
	m.showMode()
	m.saveProfile()
}

func (m *MainForm) setMode(mode buffer.Mode) {
	m.profile.View = [...]string{"text", "hex", "mixed"}[mode]
	m.setLayout()
}

func (m *MainForm) showMode() {
	m.views.SetSelected(int(m.buf.Options().Mode))
}

func (m *MainForm) setTimeMode(mode TimeMode) {
	m.term.SetTimeMode(mode)
	m.actTime.SetChecked(mode != TimeOff)
	if mode != TimeOff {
		m.saveProfile()
	} else {
		// Turned off by the button: the kind of the time is kept for the next time
		kind := m.profile.Time
		m.saveProfile()
		m.profile.Time = kind
		if m.key != "" {
			p := m.profile
			p.Time = ""
			config.SetProfile(m.key, p)
		}
	}
}

func (m *MainForm) setPaused(p bool) {
	m.term.SetPaused(p)
	m.actPause.SetChecked(p)
}

func (m *MainForm) clear() {
	m.term.Clear()
	m.find.showCount()
}

func (m *MainForm) zoom(d int) {
	size := m.term.FontSize() + float64(d)
	if d == 0 {
		size = 14
	}
	m.term.SetFontSize(size)
	config.Update(func(s *config.Settings) { s.FontSize = int(m.term.FontSize()) })
}

// baudChosen applies the speed typed or chosen
func (m *MainForm) baudChosen() {
	text := strings.TrimSpace(m.baudCombo.Text())
	b, err := strconv.Atoi(text)
	if err != nil || b <= 0 || b > 100_000_000 {
		m.status.ShowMessage(fmt.Sprintf(T().BadBaud, text))
		m.baudCombo.SetText(strconv.Itoa(m.cfg.Baud))
		return
	}
	if b == m.cfg.Baud {
		return
	}
	cfg := m.cfg
	cfg.Baud = b
	m.applyConfig(cfg)
}

func (m *MainForm) frameChosen() {
	f, _ := m.frameCombo.SelectedItemData().(string)
	if f == "" {
		m.showFrame()
		m.portSettings()
		return
	}
	cfg := m.cfg
	if err := cfg.SetFrame(f); err != nil {
		return
	}
	m.applyConfig(cfg)
}

// applyConfig sets the settings of the port, to the open port too
func (m *MainForm) applyConfig(cfg serial.Config) {
	if err := m.sess.SetConfig(cfg); err != nil {
		ui.ShowMessageBox(m.Form.Panel(), app.DisplayName, m.errorText(err))
		m.baudCombo.SetText(strconv.Itoa(m.cfg.Baud))
		m.showFrame()
		return
	}
	m.cfg = cfg
	m.baudCombo.SetText(strconv.Itoa(cfg.Baud))
	m.showFrame()
	m.saveProfile()
	m.updateState()
}

// showFrame selects the frame of the settings in the list, adding it when
// it is not there
func (m *MainForm) showFrame() {
	f := m.cfg.Frame()
	i := m.frameCombo.FindData(f)
	if i < 0 {
		m.frameCombo.InsertItem(m.frameCombo.ItemCount()-1, f, f)
		i = m.frameCombo.FindData(f)
	}
	m.frameCombo.SetSelectedIndex(i)
}

func (m *MainForm) toggleConnect() {
	state, _, _ := m.sess.State()
	if state != session.Closed {
		m.sess.Close()
		m.repeat.Stop()
		m.pending.Store(true)
		m.pull()
		return
	}
	m.connect()
}

func (m *MainForm) connect() {
	p, ok := m.selectedPort()
	if !ok {
		ui.ShowMessageBox(m.Form.Panel(), app.DisplayName, T().NoPortSelected)
		return
	}
	if p.Key() != m.key {
		m.portChosen()
	}
	if err := m.sess.Open(p, m.cfg, m.profile.DTR, m.profile.RTS); err != nil {
		m.showOpenError(p, err)
		return
	}
	m.term.SetOrigin(m.sess.OpenedAt())
	m.stats = stats{}
	m.saveProfile()
	config.Update(func(s *config.Settings) { s.LastPort = m.key })
	m.startAutoLog()
	m.pending.Store(true)
	m.pull()
	m.pollLines()
	m.term.Focus()
}

func (m *MainForm) showOpenError(p serial.PortInfo, err error) {
	text := fmt.Sprintf(T().CannotOpen, p.Short) + "\n\n" + m.errorText(err)
	if errors.Is(err, serial.ErrPermission) && runtime.GOOS == "linux" {
		text += "\n\n" + T().PermissionLinux + "\n\n    sudo usermod -aG dialout $USER"
	}
	if errors.Is(err, serial.ErrBusy) {
		if h := serial.Holder(p.Name); h != "" {
			text += "\n" + fmt.Sprintf(T().HeldBy, h)
		}
	}
	ui.ShowMessageBox(m.Form.Panel(), app.DisplayName, text)
}

func (m *MainForm) errorText(err error) string {
	switch {
	case errors.Is(err, serial.ErrBusy):
		return T().ErrBusy
	case errors.Is(err, serial.ErrPermission):
		return T().ErrPermission
	case errors.Is(err, serial.ErrNotFound):
		return T().ErrNotFound
	case errors.Is(err, session.ErrNotOpen):
		return T().ErrNotOpen
	case errors.Is(err, session.ErrQueueFull):
		return T().ErrQueueFull
	}
	return err.Error()
}

// enterBytes is what Enter in the terminal sends: the line ending of the
// send line, CR when it has none
func (m *MainForm) enterBytes() []byte {
	if b := proto.LineEndingByName(m.send.Ending()).Bytes(); b != nil {
		return b
	}
	return []byte{'\r'}
}

// keyBytes turns a key pressed in the terminal into bytes, as a terminal
// of the VT100 kind does
func (m *MainForm) keyBytes(e *ui.KeyEvent) []byte {
	ctrl := e.Mods.Ctrl || e.Mods.Cmd
	if e.Mods.Alt {
		return nil
	}
	if ctrl && !e.Mods.Shift && e.Key >= ui.KeyA && e.Key <= ui.KeyZ && e.Key != ui.KeyF {
		return []byte{byte(e.Key-ui.KeyA) + 1}
	}
	if ctrl {
		return nil
	}
	switch e.Key {
	case ui.KeyEnter:
		return m.enterBytes()
	case ui.KeyBackspace:
		if m.profile.Backspace == "bs" {
			return []byte{0x08}
		}
		return []byte{0x7F}
	case ui.KeyTab:
		return []byte{'\t'}
	case ui.KeyEsc:
		return []byte{0x1B}
	}
	if e.Mods.Shift {
		return nil // Shift+PageUp and the others scroll the view
	}
	seq := map[ui.Key]string{
		ui.KeyArrowUp: "\x1b[A", ui.KeyArrowDown: "\x1b[B", ui.KeyArrowRight: "\x1b[C", ui.KeyArrowLeft: "\x1b[D",
		ui.KeyHome: "\x1b[H", ui.KeyEnd: "\x1b[F", ui.KeyInsert: "\x1b[2~", ui.KeyDelete: "\x1b[3~",
		ui.KeyPageUp: "\x1b[5~", ui.KeyPageDown: "\x1b[6~",
	}
	if s, ok := seq[e.Key]; ok {
		return []byte(s)
	}
	return nil
}

// sendKeys sends the keys typed in the terminal
func (m *MainForm) sendKeys(data []byte) {
	if err := m.sess.Send(data, buffer.Keys); err != nil {
		m.status.ShowMessage(m.errorText(err) + " — " + T().PressConnect)
		return
	}
	if !m.term.Following() && !m.term.Paused() {
		m.term.ScrollToEnd()
	}
}

// lineBytes turns the text of the send line into the bytes to send
func (m *MainForm) lineBytes(text string) ([]byte, error) {
	var data []byte
	if m.send.HexMode() {
		b, err := proto.ParseHex(text)
		if err != nil {
			return nil, err
		}
		data = b
	} else {
		data = proto.ParseEscapes(text)
	}
	data = m.checksum.Append(data)
	if !m.send.HexMode() {
		data = append(data, proto.LineEndingByName(m.send.Ending()).Bytes()...)
	}
	return data, nil
}

// sendLine sends the text of the send line
func (m *MainForm) sendLine(text string) {
	if m.repeat.Running() {
		m.repeat.Stop()
		m.send.button.SetTextFunc(func() string { return T().Send })
		return
	}
	data, err := m.lineBytes(text)
	if err != nil {
		m.status.ShowMessage(err.Error())
		return
	}
	if len(data) == 0 {
		return
	}
	target := m.sess
	if m.port2.sendToB() {
		target = m.port2.sess
	}
	if err := target.Send(data, buffer.TX); err != nil {
		m.status.ShowMessage(m.errorText(err) + " — " + T().PressConnect)
		return
	}
	m.send.Sent(text)
	m.saveProfile()
	if !m.term.Paused() {
		m.term.ScrollToEnd()
	}
}

// startRepeat sends the line again and again with the interval
func (m *MainForm) startRepeat() {
	text := m.send.Text()
	if _, err := m.lineBytes(text); err != nil || text == "" {
		m.status.ShowMessage(T().RepeatEmpty)
		return
	}
	ui.ShowInputDialog(m.Form.Panel(), ui.InputDialog{Title: T().RepeatTitle, Prompt: T().RepeatPrompt, Value: "1000",
		Validate: func(s string) error {
			if n, err := strconv.Atoi(strings.TrimSpace(s)); err != nil || n < 10 {
				return errors.New(T().RepeatBad)
			}
			return nil
		}}, func(s string) {
		n, _ := strconv.Atoi(strings.TrimSpace(s))
		m.repeatText = text
		m.repeat.SetInterval(time.Duration(n) * time.Millisecond)
		m.repeat.Start()
		m.send.button.SetTextFunc(func() string { return T().StopRepeat })
		m.repeatSend()
	})
}

func (m *MainForm) repeatSend() {
	data, err := m.lineBytes(m.repeatText)
	if err != nil || m.sess.Send(data, buffer.TX) != nil {
		m.repeat.Stop()
		m.send.button.SetTextFunc(func() string { return T().Send })
	}
}

// pasteToDevice sends the text of the clipboard as typed keys; a long text
// is asked about first
func (m *MainForm) pasteToDevice() {
	text, err := ui.ClipboardGetText()
	if err != nil || text == "" {
		return
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	data := []byte(strings.ReplaceAll(text, "\n", string(m.enterBytes())))
	lines := strings.Count(text, "\n")
	if len(data) <= 256 && lines <= 1 {
		m.sendKeys(data)
		return
	}
	ui.ShowQuestionMessageBoxYesNo(m.Form.Panel(), T().PasteTitle,
		fmt.Sprintf(T().PasteQuestion, formatBytes(int64(len(data))), lines+1), func() { m.sendKeys(data) }, nil)
}

// toggleRecord starts and stops the log file
func (m *MainForm) toggleRecord() {
	if m.sess.Log() != nil {
		path := m.sess.Log().Path()
		m.sess.StopLog()
		m.actRecord.SetChecked(false)
		m.recordHighlight(false)
		m.status.ShowMessage(fmt.Sprintf(T().LogSaved, path))
		m.updateState()
		return
	}
	m.actRecord.SetChecked(false)
	_, info, _ := m.sess.State()
	name := info.Short
	if name == "" {
		if p, ok := m.selectedPort(); ok {
			name = p.Short
		}
	}
	if name == "" {
		name = "serial"
	}
	name = strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(name)
	def := fmt.Sprintf("%s_%s.log", name, time.Now().Format("2006-01-02_15-04-05"))
	m.Form.ShowSaveFileDialog(ui.SaveFileDialogOptions{
		Title:            T().RecordTitle,
		DefaultFileName:  def,
		DefaultDirectory: config.Get().LogDir,
		Filters: []ui.FileDialogFilter{
			{DisplayName: T().LogText, Patterns: []string{"*.log", "*.txt"}},
			{DisplayName: T().LogRaw, Patterns: []string{"*.bin"}},
		},
	}, func(path string, err error) {
		if err != nil || path == "" {
			return
		}
		format := session.LogText
		if strings.EqualFold(filepath.Ext(path), ".bin") {
			format = session.LogRaw
		}
		if err := m.sess.StartLog(path, format, true); err != nil {
			ui.ShowMessageBox(m.Form.Panel(), app.DisplayName, err.Error())
			return
		}
		config.Update(func(s *config.Settings) { s.LogDir = filepath.Dir(path) })
		m.limitLog()
		m.actRecord.SetChecked(true)
		m.recordHighlight(true)
		m.updateState()
	})
}

func (m *MainForm) recordHighlight(on bool) {
	if m.recordBtn == nil {
		return
	}
	if on {
		m.recordBtn.SetImage(drawIcon("record", IconSize, colorRecord.get()))
		m.recordBtn.SetHighlight(colorRecord.get())
	} else {
		m.recordBtn.SetImage(icon("record"))
		m.recordBtn.SetHighlight(nil)
	}
}

// Activate puts the keyboard into the terminal at the start
func (m *MainForm) Activate() {
	m.term.Focus()
}
