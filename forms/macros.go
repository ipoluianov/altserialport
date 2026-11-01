package forms

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ipoluianov/altserialport/config"
	"github.com/ipoluianov/altserialport/proto"
	"github.com/ipoluianov/nui/ui"
)

// MaxMacros is the macros that have a key: Shift+F1…F12
const MaxMacros = 12

// MacroPanel is the side panel of the named commands
type MacroPanel struct {
	ui.Panel
	list   *ui.VBox
	macros []config.Macro

	// OnRun runs the macro of the index
	OnRun func(i int)
	// OnChanged gets the macros after the user changed them
	OnChanged func(m []config.Macro)
	OnClose   func()
}

// NewMacroPanel makes the panel of the macros
func NewMacroPanel(macros []config.Macro) *MacroPanel {
	p := &MacroPanel{}
	p.InitWidget()
	p.SetPanelPadding(4)
	p.SetMinWidth(220)

	title := ui.NewLabel("")
	title.SetTextFunc(func() string { return T().MacrosTitle })
	title.SetFontBold(true)
	head := ui.NewHBox()
	head.Add(title)
	head.AddStretch()
	head.Add(iconButton("plus", func() string { return T().MacroAdd }, func() { p.edit(-1) }))
	head.Add(iconButton("close", func() string { return T().FindClose }, func() {
		if p.OnClose != nil {
			p.OnClose()
		}
	}))
	p.AddWidget(0, 0, head)

	p.list = ui.NewVBox()
	scroll := ui.NewScrollArea()
	scroll.SetAllowScroll(false, true)
	scroll.AddWidget(0, 0, p.list)
	scroll.SetYExpandable(true)
	p.AddWidget(1, 0, scroll)
	title.SetTooltipFunc(func() string { return T().MacrosHint })
	p.SetMacros(macros)
	return p
}

// SetMacros shows the macros
func (p *MacroPanel) SetMacros(macros []config.Macro) {
	p.macros = append([]config.Macro(nil), macros...)
	p.list.RemoveAllWidgets()
	for i, m := range p.macros {
		i := i
		text := m.Name
		if i < MaxMacros {
			text = fmt.Sprintf("⇧F%d   %s", i+1, m.Name)
		}
		b := ui.NewButton(text)
		b.SetCanBeFocused(false)
		b.SetTooltip(strings.ReplaceAll(m.Text, "\n", "  ⏎  "))
		b.OnClick.Connect(func(ui.Widgeter) {
			if p.OnRun != nil {
				p.OnRun(i)
			}
		})
		menu := ui.NewContextMenu(b)
		menu.AddItem(T().MacroRun, func() { p.OnRun(i) })
		menu.AddItem(T().RuleEdit, func() { p.edit(i) })
		menu.AddItem(T().RuleUp, func() { p.move(i, -1) })
		menu.AddItem(T().RuleDown, func() { p.move(i, 1) })
		menu.AddSeparator()
		menu.AddItem(T().RuleRemove, func() {
			p.macros = append(p.macros[:i], p.macros[i+1:]...)
			p.changed()
		})
		b.SetContextMenu(menu)
		p.list.Add(b)
	}
	p.list.AddStretch()
}

// Macro returns the macro of the index
func (p *MacroPanel) Macro(i int) (config.Macro, bool) {
	if i < 0 || i >= len(p.macros) {
		return config.Macro{}, false
	}
	return p.macros[i], true
}

func (p *MacroPanel) changed() {
	p.SetMacros(p.macros)
	if p.OnChanged != nil {
		p.OnChanged(p.macros)
	}
}

func (p *MacroPanel) move(i, d int) {
	j := i + d
	if j < 0 || j >= len(p.macros) {
		return
	}
	p.macros[i], p.macros[j] = p.macros[j], p.macros[i]
	p.changed()
}

// edit opens the dialog of a macro; i < 0 adds one
func (p *MacroPanel) edit(i int) {
	m := config.Macro{}
	if i >= 0 {
		m = p.macros[i]
	}
	d := newMacroDialog(m)
	d.OnOK = func(m config.Macro) {
		if i < 0 {
			p.macros = append(p.macros, m)
		} else {
			p.macros[i] = m
		}
		p.changed()
	}
	p.ShowDialog(d)
}

type macroDialog struct {
	ui.DialogContent
	OnOK func(m config.Macro)
}

func newMacroDialog(m config.Macro) *macroDialog {
	d := &macroDialog{}
	d.InitWidget()
	name := ui.NewTextBox()
	name.SetText(m.Name)
	mode := ui.NewComboBox()
	mode.AddItem("Text", false)
	mode.AddItem("Hex", true)
	if m.Hex {
		mode.SetSelectedIndex(1)
	}
	text := ui.NewTextBox()
	text.SetMultiline(true)
	text.SetFontFamily(ui.FontFamilyMono)
	text.SetText(m.Text)
	text.SetMinSize(420, 160)
	fields := ui.NewFormLayout()
	fields.AddRow(T().MacroName, name)
	fields.AddRow(T().MacroMode, mode)
	fields.AddRow(T().MacroText, text)
	d.AddWidget(0, 0, fields)
	help := ui.NewLabel(T().MacroHelp)
	help.SetForegroundColor(colorSecondary.get())
	d.AddWidget(1, 0, help)
	buttons := ui.NewDialogButtonBox()
	buttons.AddOK(func() {
		nm := config.Macro{Name: strings.TrimSpace(name.Text()), Text: text.Text()}
		nm.Hex, _ = mode.SelectedItemData().(bool)
		if nm.Name == "" {
			nm.Name = strings.SplitN(strings.TrimSpace(nm.Text), "\n", 2)[0]
		}
		if _, err := macroSteps(nm); err != nil {
			ui.ShowMessageBox(d, T().MacrosTitle, err.Error())
			return
		}
		d.RunInParent(func() {
			if d.OnOK != nil {
				d.OnOK(nm)
			}
		})
		d.Form().Close()
	})
	buttons.AddCancel(func() { d.Form().Close() })
	d.AddWidget(2, 0, buttons)
	d.OnDialogShow = func() {
		d.Form().SetTitle(T().MacroTitle)
		name.Focus()
	}
	return d
}

// macroStep is a line of a macro: bytes to send, or a wait
type macroStep struct {
	data []byte
	wait time.Duration
}

// macroSteps reads the lines of a macro: each line is sent; "#wait 100"
// waits 100 ms; "#" starts a comment. The line ending and the checksum of
// the send line are added by the caller to the text lines.
func macroSteps(m config.Macro) ([]macroStep, error) {
	var steps []macroStep
	for n, line := range strings.Split(strings.ReplaceAll(m.Text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#wait") {
			ms, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(trimmed, "#wait")))
			if err != nil || ms < 0 {
				return nil, fmt.Errorf(T().MacroBadWait, n+1)
			}
			steps = append(steps, macroStep{wait: time.Duration(ms) * time.Millisecond})
			continue
		}
		if strings.HasPrefix(trimmed, "#") || trimmed == "" && m.Hex {
			continue
		}
		if m.Hex {
			b, err := proto.ParseHex(line)
			if err != nil {
				return nil, fmt.Errorf("%d: %v", n+1, err)
			}
			steps = append(steps, macroStep{data: b})
			continue
		}
		steps = append(steps, macroStep{data: proto.ParseEscapes(line)})
	}
	return steps, nil
}
