package forms

import (
	"fmt"
	"strings"

	"github.com/ipoluianov/altserialport/rules"
	"github.com/ipoluianov/nui/ui"
)

// ruleColors are the colors a rule may give a row
var ruleColors = []struct{ Hex, Name string }{
	{"", "—"},
	{"#f85149", "Red"}, {"#ff9e44", "Orange"}, {"#d29922", "Yellow"}, {"#3fb950", "Green"},
	{"#39c5cf", "Cyan"}, {"#58a6ff", "Blue"}, {"#bc8cff", "Magenta"}, {"#8b949e", "Gray"},
}

// ruleText describes a rule in one line of the list
func ruleText(r rules.Rule) string {
	var acts []string
	if r.Color != "" {
		for _, c := range ruleColors {
			if c.Hex == r.Color {
				acts = append(acts, T().ColorNames[indexOfColor(c.Hex)])
			}
		}
	}
	if r.Beep {
		acts = append(acts, T().RuleBeep)
	}
	if r.Notify {
		acts = append(acts, T().RuleNotify)
	}
	if r.Pause {
		acts = append(acts, T().RulePause)
	}
	if r.StopLog {
		acts = append(acts, T().RuleStopLog)
	}
	if r.Answer != "" {
		acts = append(acts, T().RuleAnswer+" "+r.Answer)
	}
	kind := ""
	switch {
	case r.Hex:
		kind = "hex "
	case r.Regex:
		kind = "re "
	}
	on := "✓ "
	if !r.Enabled {
		on = "   "
	}
	return fmt.Sprintf("%s%s%s  →  %s", on, kind, r.Pattern, strings.Join(acts, ", "))
}

func indexOfColor(hex string) int {
	for i, c := range ruleColors {
		if c.Hex == hex {
			return i
		}
	}
	return 0
}

// rulesDialog edits the list of the rules
type rulesDialog struct {
	ui.DialogContent
	list  *ui.ListView
	rules []rules.Rule
	OnOK  func(list []rules.Rule)
}

func newRulesDialog(list []rules.Rule) *rulesDialog {
	d := &rulesDialog{rules: append([]rules.Rule(nil), list...)}
	d.InitWidget()
	d.list = ui.NewListView()
	d.list.SetMinSize(560, 260)
	d.list.SetXExpandable(true)
	d.list.SetYExpandable(true)
	d.list.SetEmptyText(T().RulesEmpty)
	d.list.OnActivated.Connect(func(int) { d.edit() })
	d.refresh()

	about := ui.NewLabel(T().RulesAbout)
	about.SetForegroundColor(colorSecondary.get())
	d.AddWidget(0, 0, about)
	d.AddWidget(1, 0, d.list)

	side := ui.NewVBox()
	add := func(text string, f func()) {
		b := ui.NewButton(text)
		b.OnClick.Connect(func(ui.Widgeter) { f() })
		side.Add(b)
	}
	add(T().RuleAdd, func() { d.editRule(-1, rules.Rule{Enabled: true}) })
	add(T().RuleEdit, d.edit)
	add(T().RuleRemove, func() {
		if i := d.list.CurrentRow(); i >= 0 && i < len(d.rules) {
			d.rules = append(d.rules[:i], d.rules[i+1:]...)
			d.refresh()
		}
	})
	add(T().RuleUp, func() { d.move(-1) })
	add(T().RuleDown, func() { d.move(1) })
	add(T().RuleToggle, func() {
		if i := d.list.CurrentRow(); i >= 0 && i < len(d.rules) {
			d.rules[i].Enabled = !d.rules[i].Enabled
			d.refresh()
		}
	})
	add(T().RuleLogLevels, func() {
		d.rules = append(d.rules, rules.LogLevels()...)
		d.refresh()
	})
	side.AddStretch()
	d.AddWidget(1, 1, side)

	buttons := ui.NewDialogButtonBox()
	buttons.AddOK(func() {
		list := d.rules
		d.RunInParent(func() {
			if d.OnOK != nil {
				d.OnOK(list)
			}
		})
		d.Form().Close()
	})
	buttons.AddCancel(func() { d.Form().Close() })
	d.AddWidgetSpan(2, 0, 1, 2, buttons)
	d.OnDialogShow = func() {
		d.Form().SetTitle(T().RulesTitle)
		d.Form().SetSize(820, 460)
	}
	return d
}

func (d *rulesDialog) refresh() {
	cur := d.list.CurrentRow()
	var items []string
	for _, r := range d.rules {
		items = append(items, ruleText(r))
	}
	d.list.SetItems(items)
	if cur >= 0 && cur < len(items) {
		d.list.SetCurrentRow(cur)
	}
}

func (d *rulesDialog) move(delta int) {
	i := d.list.CurrentRow()
	j := i + delta
	if i < 0 || j < 0 || j >= len(d.rules) {
		return
	}
	d.rules[i], d.rules[j] = d.rules[j], d.rules[i]
	d.refresh()
	d.list.SetCurrentRow(j)
}

func (d *rulesDialog) edit() {
	if i := d.list.CurrentRow(); i >= 0 && i < len(d.rules) {
		d.editRule(i, d.rules[i])
	}
}

// editRule opens the dialog of one rule; i < 0 adds it
func (d *rulesDialog) editRule(i int, r rules.Rule) {
	e := newRuleDialog(r)
	e.OnOK = func(r rules.Rule) {
		if i < 0 {
			d.rules = append(d.rules, r)
			i = len(d.rules) - 1
		} else {
			d.rules[i] = r
		}
		d.refresh()
		d.list.SetCurrentRow(i)
	}
	d.ShowDialog(e)
}

// ruleDialog edits one rule
type ruleDialog struct {
	ui.DialogContent
	OnOK func(r rules.Rule)
}

func newRuleDialog(r rules.Rule) *ruleDialog {
	d := &ruleDialog{}
	d.InitWidget()
	pattern := ui.NewTextBox()
	pattern.SetText(r.Pattern)
	pattern.SetMinWidth(360)
	pattern.SetFontFamily(ui.FontFamilyMono)
	kind := ui.NewComboBox()
	kind.AddItem(T().RuleKindText, "text")
	kind.AddItem(T().RuleKindRegex, "regex")
	kind.AddItem(T().RuleKindHex, "hex")
	switch {
	case r.Hex:
		kind.SetSelectedIndex(2)
	case r.Regex:
		kind.SetSelectedIndex(1)
	}
	caseBox := ui.NewCheckbox(T().RuleCase)
	caseBox.SetChecked(r.Case)
	color := ui.NewComboBox()
	for k := range ruleColors {
		color.AddItem(T().ColorNames[k], ruleColors[k].Hex)
	}
	color.SetSelectedIndex(indexOfColor(r.Color))
	beep := ui.NewCheckbox(T().RuleBeep)
	beep.SetChecked(r.Beep)
	notify := ui.NewCheckbox(T().RuleNotify)
	notify.SetChecked(r.Notify)
	pause := ui.NewCheckbox(T().RulePause)
	pause.SetChecked(r.Pause)
	stop := ui.NewCheckbox(T().RuleStopLog)
	stop.SetChecked(r.StopLog)
	answer := ui.NewTextBox()
	answer.SetText(r.Answer)
	answer.SetFontFamily(ui.FontFamilyMono)
	answer.SetHintFunc(func() string { return T().RuleAnswerHint })
	enabled := ui.NewCheckbox(T().RuleEnabled)
	enabled.SetChecked(r.Enabled)

	fields := ui.NewFormLayout()
	fields.AddRow(T().RulePattern, pattern)
	fields.AddRow(T().RuleKind, kind)
	fields.AddRowWidget(caseBox)
	fields.AddRow(T().RuleColor, color)
	acts := ui.NewHBox()
	acts.Add(beep)
	acts.Add(notify)
	acts.Add(pause)
	acts.Add(stop)
	fields.AddRow(T().RuleActions, acts)
	fields.AddRow(T().RuleAnswer, answer)
	fields.AddRowWidget(enabled)
	d.AddWidget(0, 0, fields)
	d.AddVSpacer(1, 0)

	buttons := ui.NewDialogButtonBox()
	buttons.AddOK(func() {
		nr := rules.Rule{
			Enabled: enabled.Checked(), Pattern: pattern.Text(),
			Regex: kind.SelectedIndex() == 1, Hex: kind.SelectedIndex() == 2, Case: caseBox.Checked(),
			Beep: beep.Checked(), Notify: notify.Checked(), Pause: pause.Checked(), StopLog: stop.Checked(),
			Answer: answer.Text(),
		}
		nr.Color, _ = color.SelectedItemData().(string)
		if _, errs := rules.Compile([]rules.Rule{nr}); len(errs) > 0 || strings.TrimSpace(nr.Pattern) == "" {
			ui.ShowMessageBox(d, T().RulesTitle, T().RuleBad)
			return
		}
		d.RunInParent(func() {
			if d.OnOK != nil {
				d.OnOK(nr)
			}
		})
		d.Form().Close()
	})
	buttons.AddCancel(func() { d.Form().Close() })
	d.AddWidget(2, 0, buttons)
	d.OnDialogShow = func() {
		d.Form().SetTitle(T().RuleTitle)
		pattern.Focus()
	}
	return d
}
