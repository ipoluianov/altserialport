package install

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.2.3", "v1.2.3", 0},
		{"v1.2.4", "v1.2.3", 1},
		{"v1.10.0", "v1.9.9", 1},
		{"v2.0.0", "v10.0.0", -1},
		{"1.2", "v1.2.0", 0},
		{"v1.2.3-4-gabcdef", "v1.2.3", 1},
		{"v1.2.3-4-gabcdef-dirty", "v1.2.3-5-g123456", -1},
		{"v1.2.3-rc1", "v1.2.3", -1},
		{"v1.2.3-rc.2", "v1.2.3-rc.10", -1},
		{"v1.2.3+build5", "v1.2.3", 0},
		{"dev", "v9.9.9", 1},
		{"v9.9.9", "dev", -1},
		{"abc1234", "v1.0.0", 1},
		{"v1.0.0", "", 1},
		{"", "v1.0.0", -1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
		if got := compareVersions(c.b, c.a); got != -c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.b, c.a, got, -c.want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		running, installed string
		want               bool
	}{
		{"v1.0.1", "v1.0.0", true},
		{"v1.0.0", "v1.0.0", false},
		{"v1.0.0", "v1.0.1", false},
		{"dev", "dev", true},
		{"dev", "v5.0.0", true},
		{"v1.0.0", "", true}, // nothing is known of the installed one
		{"", "v1.0.0", false},
	}
	for _, c := range cases {
		if got := isNewer(c.running, c.installed); got != c.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", c.running, c.installed, got, c.want)
		}
	}
}

func TestPaths(t *testing.T) {
	home := filepath.Join(string(filepath.Separator)+"home", "user")
	if got, want := dirIn(home), filepath.Join(home, ".altbins", "altserialport"); got != want {
		t.Errorf("dirIn = %q, want %q", got, want)
	}
	if !strings.HasSuffix(ExePath(), filepath.Join(".altbins", "altserialport", "altserialport.exe")) {
		t.Errorf("ExePath = %q", ExePath())
	}
	// The settings are in another folder: removing the program keeps them
	if strings.Contains(Dir(), ".altserialport") {
		t.Errorf("Dir %q is the folder of the settings", Dir())
	}
	if !samePath(`C:\Users\Me\.altbins\AltSerialPort\altserialport.EXE`, `c:/users/me/.altbins/altserialport/altserialport.exe`) {
		t.Error("samePath is case sensitive")
	}
	if samePath(`C:\a\b.exe`, `C:\a\c.exe`) {
		t.Error("samePath of different files")
	}
}

func TestFlags(t *testing.T) {
	if !hasFlag([]string{"--uninstall", "--quiet"}, "uninstall") || !hasFlag([]string{"-quiet"}, "quiet") {
		t.Error("the flags are not found")
	}
	if hasFlag([]string{"--uninstaller"}, "uninstall") {
		t.Error("a flag found by its prefix")
	}
}

func TestStubs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the real installer")
	}
	if Status() != Unsupported {
		t.Errorf("Status = %v", Status())
	}
	if HandleArgs([]string{"--uninstall"}) {
		t.Error("HandleArgs handled the arguments")
	}
	if Install() == nil || Uninstall(true) == nil || StartInstalled() == nil {
		t.Error("the stubs do something")
	}
}
