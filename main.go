// AltSerialPort is a terminal for serial ports: simple outside, deep inside
package main

import (
	"os"
	"path/filepath"

	"github.com/ipoluianov/altserialport/appicon"
	"github.com/ipoluianov/altserialport/cli"
	"github.com/ipoluianov/altserialport/config"
	"github.com/ipoluianov/altserialport/forms"
	"github.com/ipoluianov/altserialport/install"
	"github.com/ipoluianov/nui/ui"
)

func main() {
	args := os.Args[1:]
	// Started from "Installed apps" to remove it
	if install.HandleArgs(args) {
		return
	}
	// list, send, mcp, a port name: the program works in the console
	if cli.IsCommand(args) {
		cli.AttachConsole()
		os.Exit(cli.Run(args))
	}

	config.Load()
	st := config.Get()
	forms.SetLanguage(st.Language)
	forms.ApplyTheme(st.Theme)
	ui.SetAppIcon(appicon.Image(256))

	m := forms.NewMainForm()
	// Each copy of the program is a window of its own: they all remember
	// the place of the last one closed
	if ws, err := ui.OpenSettingsFile(filepath.Join(config.Dir(), "window.json")); err == nil {
		m.Form.RememberState(ws, "main")
	}
	m.Form.Show()
	m.Form.Invoke(func() {
		m.Form.SetAlwaysOnTop(st.AlwaysOnTop)
		m.Activate()
	})
	m.Form.Exec()
}
