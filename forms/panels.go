package forms

import (
	"fmt"
	"time"

	"github.com/ipoluianov/altserialport/buffer"
	"github.com/ipoluianov/altserialport/config"
	"github.com/ipoluianov/altserialport/proto"
	"github.com/ipoluianov/altserialport/rules"
	"github.com/ipoluianov/altserialport/session"
	"github.com/ipoluianov/nui/ui"
)

// center lays out the terminal with the panels shown on demand: the plot
// under it, the macros at its right
func (m *MainForm) center(st config.Settings) ui.Widgeter {
	m.lastTrigger = map[int]time.Time{}
	m.plotter = NewPlotter()
	m.plotter.OnClose = m.togglePlot
	m.macros = NewMacroPanel(st.Macros)
	m.macros.OnRun = m.runMacro
	m.macros.OnClose = m.toggleMacros
	m.macros.OnChanged = func(list []config.Macro) {
		config.Update(func(s *config.Settings) { s.Macros = list })
	}

	terms := ui.NewHSplitter()
	terms.SetWidgets(m.term, m.port2.term)
	m.vsplit = ui.NewVSplitter()
	m.vsplit.SetWidgets(terms, m.plotter)
	m.vsplit.SetSecondSize(max(st.PlotSize, 220))
	m.vsplit.OnActivated.Connect(func(size int) {
		config.Update(func(s *config.Settings) { s.PlotSize = size })
	})
	m.plotter.SetVisible(st.ShowPlot)

	m.hsplit = ui.NewHSplitter()
	m.hsplit.SetWidgets(m.vsplit, m.macros)
	m.hsplit.SetSecondSize(max(st.PanelSize, 240))
	m.hsplit.OnActivated.Connect(func(size int) {
		config.Update(func(s *config.Settings) { s.PanelSize = size })
	})
	m.macros.SetVisible(st.ShowMacros)
	return m.hsplit
}

// togglePlot shows or hides the chart
func (m *MainForm) togglePlot() {
	on := !m.plotter.IsVisible()
	m.plotter.SetVisible(on)
	config.Update(func(s *config.Settings) { s.ShowPlot = on })
	if on && len(m.plotter.series) == 0 {
		m.status.ShowMessage(T().PlotHint)
	}
}

// toggleMacros shows or hides the macro panel
func (m *MainForm) toggleMacros() {
	on := !m.macros.IsVisible()
	m.macros.SetVisible(on)
	config.Update(func(s *config.Settings) { s.ShowMacros = on })
}

// runMacro sends the lines of the macro, waiting where it says, in the
// background. A text line gets the checksum and the line ending of the
// send line.
func (m *MainForm) runMacro(i int) {
	mac, ok := m.macros.Macro(i)
	if !ok {
		return
	}
	steps, err := macroSteps(mac)
	if err != nil {
		m.status.ShowMessage(err.Error())
		return
	}
	if state, _, _ := m.sess.State(); state != session.Open {
		m.status.ShowMessage(T().ErrNotOpen + " — " + T().PressConnect)
		return
	}
	sum := m.checksum
	ending := proto.LineEndingByName(m.send.Ending()).Bytes()
	for k := range steps {
		if steps[k].data != nil || steps[k].wait == 0 {
			steps[k].data = sum.Append(steps[k].data)
			if !mac.Hex {
				steps[k].data = append(steps[k].data, ending...)
			}
		}
	}
	m.status.ShowMessage(fmt.Sprintf(T().MacroRunning, mac.Name))
	go func() {
		for _, st := range steps {
			if st.wait > 0 {
				time.Sleep(st.wait)
				continue
			}
			if err := m.sess.Send(st.data, buffer.TX); err != nil {
				ui.Invoke(func() { m.status.ShowMessage(m.errorText(err)) })
				return
			}
		}
	}()
	if !m.term.Paused() {
		m.term.ScrollToEnd()
	}
}

// setRules compiles the rules and gives them to the terminal
func (m *MainForm) setRules(list []rules.Rule) {
	m.ruleSet, _ = rules.Compile(list)
	m.term.SetRules(m.ruleSet)
}

// editRules opens the dialog of the highlight rules and the triggers
func (m *MainForm) editRules() {
	d := newRulesDialog(config.Get().Rules)
	d.OnOK = func(list []rules.Rule) {
		config.Update(func(s *config.Settings) { s.Rules = list })
		m.setRules(list)
	}
	m.Form.Panel().ShowDialog(d)
}

// lookAtLines reads the received lines for the plot and the triggers
func (m *MainForm) lookAtLines(chunks []buffer.Chunk) {
	plot := m.plotter.IsVisible()
	triggers := m.ruleSet.HasTriggers()
	if !plot && !triggers {
		return
	}
	for _, c := range chunks {
		switch c.Dir {
		case buffer.RX:
		case buffer.Info:
			m.lines.Reset()
			continue
		default:
			continue
		}
		at := time.Unix(0, c.Time)
		m.lines.Feed(c.Data, func(line []byte) {
			if plot {
				m.plotter.AddLine(string(line), at)
			}
			if triggers {
				m.trigger(line)
			}
		})
	}
}

// trigger runs the actions of the rules the line matches; a rule acts at
// most five times a second, so a flood of lines doesn't flood the user
func (m *MainForm) trigger(line []byte) {
	text := string(line)
	m.ruleSet.Match(text, line, func(i int, r rules.Rule) {
		if !r.Triggers() {
			return
		}
		now := time.Now()
		if now.Sub(m.lastTrigger[i]) < 200*time.Millisecond {
			return
		}
		m.lastTrigger[i] = now
		if r.Beep {
			m.Form.Beep()
		}
		if r.Notify {
			if len(text) > 120 {
				text = text[:120] + "…"
			}
			m.Form.ShowToast(text, ui.ToastWarning)
		}
		if r.Pause && !m.term.Paused() {
			m.setPaused(true)
		}
		if r.StopLog && m.sess.Log() != nil {
			m.toggleRecord()
		}
		if r.Answer != "" {
			m.sess.Send(proto.ParseEscapes(r.Answer), buffer.TX)
		}
	})
}

// tick runs every second: the speed of the data in the status bar
func (m *MainForm) tick() {
	m.stats.sample(m.sess)
	m.status.SetRate(m.stats.rxRate, m.stats.txRate)
	m.port2Tick()
}
