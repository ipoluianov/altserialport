package forms

import (
	"strconv"
	"strings"

	"github.com/ipoluianov/altserialport/buffer"
	"github.com/ipoluianov/altserialport/config"
	"github.com/ipoluianov/altserialport/serial"
	"github.com/ipoluianov/altserialport/session"
	"github.com/ipoluianov/nui/ui"
)

// Port2 is the second port of the window: to watch both sides of a link
// with two adapters on one time line, to watch two boards side by side,
// or to pass the data between two ports and see it (a bridge)
type Port2 struct {
	ui.Panel
	m       *MainForm
	sess    *session.Session
	port    *ui.ComboBox
	baud    *ui.EditableComboBox
	frame   *ui.ComboBox
	connect *ui.Button
	view    *Segmented // the merged time line or side by side
	target  *Segmented // where the send line sends
	bridge  *ui.Checkbox

	ports    []serial.PortInfo
	cfg      serial.Config
	term     *Terminal // side by side
	buf      *buffer.Buffer
	split    bool
	pending  bool
	unbridge []func()
}

// newPort2 makes the bar of the second port, hidden
func newPort2(m *MainForm) *Port2 {
	p := &Port2{m: m, sess: session.New(), cfg: serial.DefaultConfig()}
	p.InitWidget()
	p.SetPanelPadding(4)
	p.sess.NoteText = func(n session.Note) string { return "B: " + noteText(n) }
	p.sess.Notify = func() { m.pending.Store(true) }

	p.buf = buffer.New(buffer.DefaultOptions(), 128<<20, 500_000)
	p.term = NewTerminal(p.buf)
	p.term.OnSend = func(data []byte) {
		if err := p.sess.Send(data, buffer.Keys); err != nil {
			m.status.ShowMessage("B: " + m.errorText(err))
		}
	}
	p.term.KeyBytes = m.keyBytes
	p.term.Hint = func() []string { return []string{T().Port2Title, T().Port2Hint} }
	p.term.PillText = m.term.PillText
	menu := ui.NewContextMenu(p.term.area)
	menu.AddItem(T().Copy, func() { p.term.Copy() })
	menu.AddItem(T().SelectAll, func() { p.term.SelectAll() })
	menu.AddItem(T().Clear, func() { p.term.Clear() })
	p.term.area.SetContextMenu(menu)
	p.term.SetVisible(false)

	label := ui.NewLabel("B")
	label.SetFontBold(true)
	label.SetForegroundColor(colorPortB.get())
	p.port = ui.NewComboBox()
	p.port.SetMinWidth(150)
	p.port.OnActivated.Connect(func(int) { p.portChosen() })
	p.baud = ui.NewEditableComboBox()
	var bauds []string
	for _, b := range serial.StandardBauds {
		bauds = append(bauds, strconv.Itoa(b))
	}
	p.baud.SetItems(bauds)
	p.baud.SetMinWidth(96)
	p.baud.SetMaxWidth(96)
	p.baud.SetText("115200")
	p.baud.SetOnItemSelected(func(int, string) { p.configChosen() })
	p.baud.SetOnAccept(func(string) { p.configChosen() })
	p.frame = ui.NewComboBox()
	for _, f := range serial.Frames {
		p.frame.AddItem(f, f)
	}
	p.frame.OnActivated.Connect(func(int) { p.configChosen() })
	p.connect = ui.NewButton("")
	p.connect.SetMinWidth(100)
	p.connect.SetCanBeFocused(false)
	p.connect.OnClick.Connect(func(ui.Widgeter) { p.toggle() })

	p.view = NewSegmented(T().Port2Merged, T().Port2Split)
	p.view.OnChosen = func(i int) { p.setSplit(i == 1) }
	p.view.SetTooltipFunc(func() string { return T().Port2ViewTip })
	p.target = NewSegmented(T().Port2SendA, T().Port2SendB)
	p.target.OnChosen = func(i int) { p.target.SetSelected(i) }
	p.target.SetTooltipFunc(func() string { return T().Port2TargetTip })
	p.bridge = ui.NewCheckbox(T().Port2Bridge)
	p.bridge.SetTooltipFunc(func() string { return T().Port2BridgeTip })
	p.bridge.OnActivated.Connect(func(on bool) { p.setBridge(on) })

	row := ui.NewHBox()
	row.Add(fixedSpace(4))
	row.Add(label)
	row.Add(fixedSpace(6))
	row.Add(p.port)
	row.Add(p.baud)
	row.Add(p.frame)
	row.Add(p.connect)
	row.Add(fixedSpace(8))
	row.Add(p.view)
	row.Add(fixedSpace(8))
	row.Add(p.target)
	row.Add(fixedSpace(8))
	row.Add(p.bridge)
	row.AddStretch()
	row.Add(iconButton("close", func() string { return T().FindClose }, m.togglePort2))
	p.AddWidget(0, 0, row)
	p.SetVisible(false)
	p.update()
	return p
}

// sendToB tells whether the send line sends to the second port
func (p *Port2) sendToB() bool { return p.IsVisible() && p.target.selected == 1 }

// setPorts fills the list with the ports of the main list
func (p *Port2) setPorts(ports []serial.PortInfo) {
	cur, ok := p.selected()
	state, info, _ := p.sess.State()
	if state != session.Closed {
		cur, ok = info, true
	}
	same := len(ports) == len(p.ports)
	for i := 0; same && i < len(ports); i++ {
		same = ports[i] == p.ports[i]
	}
	if same {
		return
	}
	p.ports = append([]serial.PortInfo(nil), ports...)
	p.port.Clear()
	for i, port := range p.ports {
		p.port.AddItem(port.Title(), i)
	}
	sel := -1
	for i, port := range p.ports {
		if ok && port.Name == cur.Name {
			sel = i
		}
	}
	// Not the port of A, by default
	if sel < 0 {
		_, a, _ := p.m.sess.State()
		aSel, _ := p.m.selectedPort()
		for i, port := range p.ports {
			if port.Name != a.Name && port.Name != aSel.Name {
				sel = i
				break
			}
		}
	}
	if sel >= 0 {
		p.port.SetSelectedIndex(sel)
		p.portChosen()
	}
}

func (p *Port2) selected() (serial.PortInfo, bool) {
	i := p.port.SelectedIndex()
	if i < 0 || i >= len(p.ports) {
		return serial.PortInfo{}, false
	}
	return p.ports[i], true
}

// portChosen takes the speed and the frame of the profile of the device
func (p *Port2) portChosen() {
	info, ok := p.selected()
	if !ok {
		return
	}
	prof := config.ProfileFor(info.Key())
	p.cfg.Baud = prof.Baud
	p.cfg.SetFrame(prof.Frame)
	p.cfg.Flow = flowFromName(prof.Flow)
	p.baud.SetText(strconv.Itoa(p.cfg.Baud))
	if i := p.frame.FindData(p.cfg.Frame()); i >= 0 {
		p.frame.SetSelectedIndex(i)
	}
}

func (p *Port2) configChosen() {
	b, err := strconv.Atoi(strings.TrimSpace(p.baud.Text()))
	if err != nil || b <= 0 {
		p.baud.SetText(strconv.Itoa(p.cfg.Baud))
		return
	}
	c := p.cfg
	c.Baud = b
	if f, ok := p.frame.SelectedItemData().(string); ok {
		c.SetFrame(f)
	}
	if err := p.sess.SetConfig(c); err != nil {
		p.m.status.ShowMessage("B: " + p.m.errorText(err))
		return
	}
	p.cfg = c
	p.update()
}

func (p *Port2) toggle() {
	if state, _, _ := p.sess.State(); state != session.Closed {
		p.setBridge(false)
		p.sess.Close()
		p.m.pending.Store(true)
		p.update()
		return
	}
	info, ok := p.selected()
	if !ok {
		return
	}
	p.configChosen()
	if err := p.sess.Open(info, p.cfg, true, true); err != nil {
		p.m.showOpenError(info, err)
		return
	}
	p.term.SetOrigin(p.sess.OpenedAt())
	p.m.pending.Store(true)
	p.update()
}

// update shows the state of the second port
func (p *Port2) update() {
	state, _, _ := p.sess.State()
	if state == session.Closed {
		p.connect.SetText(T().Connect)
		p.connect.SetRole("primary")
	} else {
		p.connect.SetText(T().Disconnect)
		p.connect.SetRole("")
	}
	p.port.SetEnabled(state == session.Closed)
	p.bridge.SetChecked(len(p.unbridge) > 0)
}

// setSplit shows the second port in a terminal of its own, or in the
// time line of the first
func (p *Port2) setSplit(split bool) {
	p.split = split
	p.view.SetSelected(map[bool]int{false: 0, true: 1}[split])
	p.term.SetVisible(split && p.IsVisible())
	o := p.m.layoutOptions()
	p.term.SetLayout(o)
	p.term.SetEncoding(p.m.term.Encoding())
	p.term.SetTimeMode(p.m.term.TimeMode())
}

// take routes the chunks of the second port to its view
func (p *Port2) take() {
	chunks := p.sess.Drain()
	if len(chunks) == 0 {
		return
	}
	if p.split {
		p.term.Append(chunks)
		return
	}
	for k := range chunks {
		switch chunks[k].Dir {
		case buffer.RX:
			chunks[k].Dir = buffer.RX2
		case buffer.TX, buffer.Keys:
			chunks[k].Dir = buffer.TX2
		}
	}
	p.m.term.Append(chunks)
	p.update()
}

// setBridge passes what each port receives to the other one
func (p *Port2) setBridge(on bool) {
	for _, f := range p.unbridge {
		f()
	}
	p.unbridge = nil
	if on {
		a, b := p.m.sess, p.sess
		sa, _, _ := a.State()
		sb, _, _ := b.State()
		if sa != session.Open || sb != session.Open {
			p.m.status.ShowMessage(T().Port2BridgeNeedsBoth)
			p.bridge.SetChecked(false)
			return
		}
		p.unbridge = append(p.unbridge,
			a.AddTap(func(c buffer.Chunk) {
				if c.Dir == buffer.RX {
					b.Forward(c.Data)
				}
			}),
			b.AddTap(func(c buffer.Chunk) {
				if c.Dir == buffer.RX {
					a.Forward(c.Data)
				}
			}))
		p.m.status.ShowMessage(T().Port2BridgeOn)
	}
	p.update()
}

// togglePort2 shows or hides the second port; hiding closes it
func (m *MainForm) togglePort2() {
	on := !m.port2.IsVisible()
	if !on {
		m.port2.setBridge(false)
		m.port2.sess.Close()
		m.port2.term.SetVisible(false)
		m.pending.Store(true)
	}
	m.port2.SetVisible(on)
	if on {
		m.port2.setPorts(m.ports)
		m.port2.setSplit(m.port2.split)
		m.status.ShowMessage(T().Port2Shown)
	}
	m.port2.update()
}

// port2Tick runs every second: the bridge stops when a port is gone
func (m *MainForm) port2Tick() {
	p := m.port2
	if len(p.unbridge) == 0 {
		return
	}
	sa, _, _ := m.sess.State()
	sb, _, _ := p.sess.State()
	if sa == session.Closed || sb == session.Closed {
		p.setBridge(false)
	}
}
