package forms

import (
	"github.com/ipoluianov/altserialport/app"
	"github.com/ipoluianov/altserialport/install"
	"github.com/ipoluianov/nui/ui"
)

// installMenu adds Install, Update or Uninstall where the program installs
// itself (Windows); elsewhere the packages of the system do it
func (m *MainForm) installMenu(menu *ui.ContextMenu) {
	switch install.Status() {
	case install.Portable:
		menu.AddItem(T().InstallLink, m.install)
	case install.UpdateAvailable:
		menu.AddItem(T().UpdateLink, m.install)
	case install.Installed:
		menu.AddItem(T().UninstallLink, m.uninstall)
	}
}

// install copies the program to its place, then the installed copy takes
// over the window
func (m *MainForm) install() {
	if err := install.Install(); err != nil {
		ui.ShowMessageBox(m.Form.Panel(), app.DisplayName, err.Error())
		return
	}
	ui.ShowMessageBox(m.Form.Panel(), app.DisplayName, T().Installed)
	if err := install.StartInstalled(); err == nil {
		m.Form.RequestClose()
	}
}

func (m *MainForm) uninstall() {
	ui.ShowQuestionMessageBoxYesNo(m.Form.Panel(), app.DisplayName, T().UninstallLink+"?", func() {
		if err := install.Uninstall(true); err != nil {
			ui.ShowMessageBox(m.Form.Panel(), app.DisplayName, err.Error())
			return
		}
		m.Form.RequestClose()
	}, nil)
}
