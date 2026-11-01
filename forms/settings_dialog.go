package forms

import (
	"strconv"
	"strings"

	"github.com/ipoluianov/altserialport/serial"
	"github.com/ipoluianov/nui/ui"
)

// portDialog sets everything of the port: the speed, the frame, the flow
// control and the lines at the open
type portDialog struct {
	ui.DialogContent
	baud   *ui.EditableComboBox
	data   *ui.ComboBox
	parity *ui.ComboBox
	stop   *ui.ComboBox
	flow   *ui.ComboBox
	dtr    *ui.Checkbox
	rts    *ui.Checkbox
	rs485  *ui.Checkbox
	OnOK   func(c serial.Config, dtr, rts bool)
}

func newPortDialog(c serial.Config, dtr, rts bool) *portDialog {
	d := &portDialog{}
	d.InitWidget()

	d.baud = ui.NewEditableComboBox()
	var bauds []string
	for _, b := range serial.StandardBauds {
		bauds = append(bauds, strconv.Itoa(b))
	}
	d.baud.SetItems(bauds)
	d.baud.SetText(strconv.Itoa(c.Baud))

	d.data = ui.NewComboBox()
	for _, n := range []int{8, 7, 6, 5} {
		d.data.AddItem(strconv.Itoa(n), n)
	}
	d.data.SetSelectedIndex(d.data.FindData(c.DataBits))

	d.parity = ui.NewComboBox()
	parities := []serial.Parity{serial.ParityNone, serial.ParityEven, serial.ParityOdd, serial.ParityMark, serial.ParitySpace}
	for k, p := range parities {
		d.parity.AddItem(T().Parities[k], p)
	}
	d.parity.SetSelectedIndex(d.parity.FindData(c.Parity))

	d.stop = ui.NewComboBox()
	d.stop.AddItem("1", serial.StopBits1)
	d.stop.AddItem("1.5", serial.StopBits1Half)
	d.stop.AddItem("2", serial.StopBits2)
	d.stop.SetSelectedIndex(d.stop.FindData(c.StopBits))

	d.flow = ui.NewComboBox()
	d.flow.AddItem(T().FlowNone, serial.FlowNone)
	d.flow.AddItem("RTS/CTS", serial.FlowRTSCTS)
	d.flow.AddItem("XON/XOFF", serial.FlowXonXoff)
	d.flow.SetSelectedIndex(d.flow.FindData(c.Flow))

	d.dtr = ui.NewCheckbox("DTR")
	d.dtr.SetChecked(dtr)
	d.rts = ui.NewCheckbox("RTS")
	d.rts.SetChecked(rts)
	d.rs485 = ui.NewCheckbox(T().RS485Label)
	d.rs485.SetChecked(c.RS485)
	lines := ui.NewHBox()
	lines.Add(d.dtr)
	lines.Add(d.rts)
	lines.AddStretch()

	fields := ui.NewFormLayout()
	fields.AddRow(T().BaudLabel, d.baud)
	fields.AddRow(T().DataBitsLabel, d.data)
	fields.AddRow(T().ParityLabel, d.parity)
	fields.AddRow(T().StopBitsLabel, d.stop)
	fields.AddRow(T().FlowLabel, d.flow)
	fields.AddRow(T().LinesLabel, lines)
	fields.AddRowWidget(d.rs485)
	d.AddWidget(0, 0, fields)
	hint := ui.NewLabel(T().LinesHint)
	hint.SetForegroundColor(colorSecondary.get())
	d.AddWidget(1, 0, hint)
	d.AddVSpacer(2, 0)

	buttons := ui.NewDialogButtonBox()
	buttons.AddOK(func() {
		b, err := strconv.Atoi(strings.TrimSpace(d.baud.Text()))
		if err != nil || b <= 0 {
			ui.ShowMessageBox(d, T().PortSettings, T().BadBaudShort)
			return
		}
		cfg := serial.Config{Baud: b}
		cfg.DataBits, _ = d.data.SelectedItemData().(int)
		cfg.Parity, _ = d.parity.SelectedItemData().(serial.Parity)
		cfg.StopBits, _ = d.stop.SelectedItemData().(serial.StopBits)
		cfg.Flow, _ = d.flow.SelectedItemData().(serial.Flow)
		cfg.RS485 = d.rs485.Checked()
		dtr, rts := d.dtr.Checked(), d.rts.Checked()
		d.RunInParent(func() {
			if d.OnOK != nil {
				d.OnOK(cfg, dtr, rts)
			}
		})
		d.Form().Close()
	})
	buttons.AddCancel(func() { d.Form().Close() })
	d.AddWidget(3, 0, buttons)
	d.OnDialogShow = func() {
		d.Form().SetTitle(T().PortSettings)
		d.baud.Focus()
	}
	return d
}

// portSettings opens the dialog of the port settings
func (m *MainForm) portSettings() {
	d := newPortDialog(m.cfg, m.profile.DTR, m.profile.RTS)
	d.OnOK = func(c serial.Config, dtr, rts bool) {
		if dtr != m.profile.DTR {
			m.toggleLine(true)
		}
		if rts != m.profile.RTS {
			m.toggleLine(false)
		}
		if c != m.cfg {
			m.applyConfig(c)
		}
	}
	m.Form.Panel().ShowDialog(d)
}
