//go:build windows

package install

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/ipoluianov/altserialport/app"
	"github.com/ipoluianov/altserialport/appicon"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// uninstallKey is the entry of the program in "Installed apps", for the
// current user only
const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\` + keyName

// Status tells how the running copy relates to the installed one
func Status() State {
	exe, err := runningExe()
	if err != nil {
		return Portable
	}
	target := ExePath()
	if samePath(exe, target) {
		return Installed
	}
	if _, err := os.Stat(target); err != nil {
		return Portable
	}
	if isNewer(app.Version, installedVersion()) {
		return UpdateAvailable
	}
	return OtherInstalled
}

// Install copies the running exe to the place of the installed copy (when
// it is not that copy), makes the links in the Start menu and on the
// Desktop and writes the entry in "Installed apps". Over an installed copy
// that is running, the exe is renamed to .old first: Windows lets a running
// exe be renamed, not replaced. The .old files are deleted on the next
// start, by HandleArgs.
func Install() error {
	exe, err := runningExe()
	if err != nil {
		return err
	}
	dir := Dir()
	target := ExePath()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if !samePath(exe, target) {
		if err := moveAside(target); err != nil {
			return err
		}
		if err := copyFile(exe, target); err != nil {
			return err
		}
	}
	// The exe has no icon of its own: the links and the entry use this file
	if ico, err := appicon.ICO([]int{16, 24, 32, 48, 64, 128, 256}); err == nil {
		os.WriteFile(iconPath(), ico, 0o644)
	}
	if err := writeShortcuts(target, dir); err != nil {
		return fmt.Errorf("the links are not made: %w", err)
	}
	if err := writeUninstallEntry(target, dir); err != nil {
		return fmt.Errorf("the entry in Installed apps is not made: %w", err)
	}
	return nil
}

// Uninstall removes the links, the entry in "Installed apps" and the files
// of the installed copy. When the installed copy is the one running, its
// folder is removed by cmd a second after the program exits. The settings
// are kept. quiet is not used here: the questions are asked by HandleArgs.
func Uninstall(quiet bool) error {
	_ = quiet
	dir := Dir()
	exe, err := runningExe()
	if err == nil && isWithin(exe, dir) {
		if err := removeAfterExit(dir); err != nil {
			return err
		}
	} else if err := os.RemoveAll(dir); err != nil {
		// Another window of the installed copy is still open
		return fmt.Errorf("%s is running: close its windows and try again (%w)", app.DisplayName, err)
	}
	var errs []error
	for _, lnk := range shortcutPaths() {
		if err := os.Remove(lnk); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	if err := registry.DeleteKey(registry.CURRENT_USER, uninstallKey); err != nil && !errors.Is(err, registry.ErrNotExist) {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// HandleArgs does what the command line asks of the installer and tells
// whether the program should exit then. It also deletes the .old files
// left by an update. "--uninstall" removes the program after asking, with
// "--quiet" too without asking and without telling the result: that is
// what "Installed apps" runs.
func HandleArgs(args []string) (handled bool) {
	removeOld()
	if !hasFlag(args, "uninstall") {
		return false
	}
	quiet := hasFlag(args, "quiet")
	if !quiet {
		q := fmt.Sprintf("Remove %s from this computer?\n\nThe settings are kept.", app.DisplayName)
		if messageBox(q, mbYesNo|mbIconQuestion) != idYes {
			return true
		}
	}
	err := Uninstall(quiet)
	if !quiet {
		if err != nil {
			messageBox(fmt.Sprintf("%s is not removed completely:\n\n%v", app.DisplayName, err), mbOK|mbIconError)
		} else {
			messageBox(fmt.Sprintf("%s is removed.", app.DisplayName), mbOK|mbIconInformation)
		}
	}
	return true
}

// StartInstalled starts the installed copy, apart from this process
func StartInstalled() error {
	cmd := exec.Command(ExePath())
	cmd.Dir = Dir()
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// runningExe returns the path of the running exe with the links resolved
func runningExe() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	return exe, nil
}

// isWithin tells whether the path is in the folder
func isWithin(path, dir string) bool {
	rel, err := filepath.Rel(strings.ToLower(filepath.Clean(dir)), strings.ToLower(filepath.Clean(path)))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, `..\`)
}

// installedVersion returns the version of the installed copy written in
// its entry in "Installed apps", or "" when there is none
func installedVersion() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, uninstallKey, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue("DisplayVersion")
	if err != nil {
		return ""
	}
	return v
}

// moveAside makes the place of the exe free: deletes it, or, when it is
// running, renames it to a free name ending in .old
func moveAside(target string) error {
	err := os.Remove(target)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	for i := 0; i < 100; i++ {
		old := target + ".old"
		if i > 0 {
			old = fmt.Sprintf("%s.%d.old", target, i)
		}
		if _, err := os.Lstat(old); err == nil {
			if os.Remove(old) != nil {
				continue // an older copy is still running too
			}
		}
		if err := os.Rename(target, old); err == nil {
			return nil
		}
	}
	return fmt.Errorf("the installed copy cannot be replaced: %w", err)
}

// removeOld deletes the exe files left by updates. Those still running
// are left for the next time.
func removeOld() {
	files, _ := filepath.Glob(filepath.Join(Dir(), "*.old"))
	for _, f := range files {
		_ = os.Remove(f)
	}
}

// copyFile copies the exe through a temporary file, so a copy broken
// halfway is never left in its place
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// shortcutPaths returns the paths of the links: in the Start menu and on
// the Desktop
func shortcutPaths() []string {
	name := keyName + ".lnk"
	programs, err := windows.KnownFolderPath(windows.FOLDERID_Programs, windows.KF_FLAG_DEFAULT)
	if err != nil {
		programs = filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`)
	}
	desktop, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, windows.KF_FLAG_DEFAULT)
	if err != nil {
		desktop = filepath.Join(os.Getenv("USERPROFILE"), "Desktop")
	}
	return []string{filepath.Join(programs, name), filepath.Join(desktop, name)}
}

// writeShortcuts makes the links to the exe with WScript.Shell through
// PowerShell, in a hidden window: Go has no COM of its own
func writeShortcuts(exe, dir string) error {
	var sb strings.Builder
	sb.WriteString("$ErrorActionPreference = 'Stop'\n")
	sb.WriteString("$sh = New-Object -ComObject WScript.Shell\n")
	for _, lnk := range shortcutPaths() {
		if err := os.MkdirAll(filepath.Dir(lnk), 0o755); err != nil {
			return err
		}
		fmt.Fprintf(&sb, "$s = $sh.CreateShortcut(%s)\n", psQuote(lnk))
		fmt.Fprintf(&sb, "$s.TargetPath = %s\n", psQuote(exe))
		fmt.Fprintf(&sb, "$s.WorkingDirectory = %s\n", psQuote(dir))
		fmt.Fprintf(&sb, "$s.IconLocation = %s\n", psQuote(iconPath()))
		fmt.Fprintf(&sb, "$s.Description = %s\n", psQuote(app.DisplayName+": a terminal for serial ports"))
		sb.WriteString("$s.Save()\n")
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "-")
	cmd.Stdin = strings.NewReader(sb.String())
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// iconPath is the icon file next to the installed exe
func iconPath() string { return filepath.Join(Dir(), "altserialport.ico") }

// psQuote quotes a string for PowerShell: within single quotes nothing is
// expanded, and a quote is written twice
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// writeUninstallEntry writes the entry of the program in "Installed apps"
func writeUninstallEntry(exe, dir string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	var sizeKB uint32
	if fi, err := os.Stat(exe); err == nil {
		sizeKB = uint32((fi.Size() + 1023) / 1024)
	}
	strs := []struct{ name, value string }{
		{"DisplayName", app.DisplayName},
		{"DisplayVersion", app.Version},
		{"Publisher", app.Author},
		{"DisplayIcon", iconPath()},
		{"InstallLocation", dir},
		{"UninstallString", `"` + exe + `" --uninstall`},
		{"QuietUninstallString", `"` + exe + `" --uninstall --quiet`},
		{"URLInfoAbout", app.Website},
	}
	for _, s := range strs {
		if err := k.SetStringValue(s.name, s.value); err != nil {
			return err
		}
	}
	dwords := []struct {
		name  string
		value uint32
	}{
		{"EstimatedSize", sizeKB},
		{"NoModify", 1},
		{"NoRepair", 1},
	}
	for _, d := range dwords {
		if err := k.SetDWordValue(d.name, d.value); err != nil {
			return err
		}
	}
	return nil
}

// removeAfterExit starts cmd, apart from this process, to remove the
// folder a second after: when the running exe has exited and can be
// deleted. ping is the sleep that cmd has.
func removeAfterExit(dir string) error {
	cmd := exec.Command("cmd.exe")
	// The command line is given as it is: cmd does not read the quotes
	// as Go would write them
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine:       fmt.Sprintf(`cmd.exe /c ping 127.0.0.1 -n 2 > nul & rmdir /s /q "%s"`, dir),
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP,
	}
	// Not within the folder: a folder that is the current one of a process
	// cannot be removed
	cmd.Dir = os.TempDir()
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// The message box of Windows: the program may have no window yet when it
// asks
const (
	mbOK              = 0x00000000
	mbYesNo           = 0x00000004
	mbIconError       = 0x00000010
	mbIconQuestion    = 0x00000020
	mbIconInformation = 0x00000040
	mbSetForeground   = 0x00010000
	idYes             = 6
)

var procMessageBoxW = windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW")

func messageBox(text string, flags uint32) int {
	t, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return 0
	}
	c, _ := windows.UTF16PtrFromString(app.DisplayName)
	r, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), uintptr(flags|mbSetForeground))
	return int(r)
}
