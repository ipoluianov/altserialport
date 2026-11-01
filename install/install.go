// Package install is the self-install of the Windows build, as the AltBins
// programs do it: the exe that was downloaded copies itself to
// %USERPROFILE%\.altbins\altserialport, makes the links in the Start menu
// and on the Desktop and an entry in "Installed apps", from where it is
// removed again. No rights of an administrator are needed.
//
// On Linux and macOS the program is installed by the packages, so there
// Status returns Unsupported and the rest does nothing.
//
// The settings in ~/.altbins/.altserialport/ are not touched: they stay
// when the program is removed and are found again when it is installed.
package install

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// dirName is the folder of the installed copy within ~/.altbins
	dirName = "altserialport"
	// exeName is the name of the installed exe
	exeName = "altserialport.exe"
	// keyName is the name of the uninstall entry in the registry and of
	// the links
	keyName = "AltSerialPort"
)

// State tells how the running copy relates to the installed one
type State int

const (
	// Unsupported: not Windows, the program is installed by a package
	Unsupported State = iota
	// Portable: a copy that is not installed, and nothing is installed
	Portable
	// Installed: the installed copy is running
	Installed
	// UpdateAvailable: a downloaded copy newer than the installed one is
	// running, it can replace it
	UpdateAvailable
	// OtherInstalled: a downloaded copy is running, and the installed one
	// is of the same version or newer
	OtherInstalled
)

func (s State) String() string {
	switch s {
	case Unsupported:
		return "Unsupported"
	case Portable:
		return "Portable"
	case Installed:
		return "Installed"
	case UpdateAvailable:
		return "UpdateAvailable"
	case OtherInstalled:
		return "OtherInstalled"
	}
	return "State(" + strconv.Itoa(int(s)) + ")"
}

// Dir returns the folder of the installed copy:
// %USERPROFILE%\.altbins\altserialport
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return dirIn(home)
}

// ExePath returns the path of the installed exe
func ExePath() string {
	return filepath.Join(Dir(), exeName)
}

func dirIn(home string) string {
	return filepath.Join(home, ".altbins", dirName)
}

// samePath tells whether two paths name the same file, as Windows compares
// them: without regard to case and to the form of the separators
func samePath(a, b string) bool {
	a = strings.ReplaceAll(filepath.Clean(a), "/", `\`)
	b = strings.ReplaceAll(filepath.Clean(b), "/", `\`)
	return strings.EqualFold(a, b)
}

// isNewer tells whether the running version should replace the installed
// one. A build that is not of a release tag ("dev", a bare commit hash)
// replaces anything: it is what the developer has just built.
func isNewer(running, installed string) bool {
	if _, ok := parseVersion(running); !ok && running != "" {
		return true
	}
	return compareVersions(running, installed) > 0
}

// version is "v1.2.3" or "v1.2.3-4-gabcdef" (4 commits after the tag, as
// git describe gives it) or "v1.2.3-rc1" (a pre-release, before v1.2.3)
type version struct {
	nums  [3]int
	after int    // commits after the tag
	pre   string // the pre-release
}

// parseVersion reads a version made of a tag "v<major>.<minor>.<patch>";
// the "v" and the missing parts may be left out ("1.2" is v1.2.0)
func parseVersion(s string) (version, bool) {
	var v version
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	if s == "" {
		return v, false
	}
	core, rest, _ := strings.Cut(s, "-")
	core, _, _ = strings.Cut(core, "+") // the build metadata does not count
	parts := strings.Split(core, ".")
	if len(parts) > 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v.nums[i] = n
	}
	if rest != "" {
		rest, _, _ = strings.Cut(rest, "+")
		// "4-gabcdef" (or "4-gabcdef-dirty") is git describe
		if n, tail, ok := strings.Cut(rest, "-"); ok && strings.HasPrefix(tail, "g") {
			if c, err := strconv.Atoi(n); err == nil {
				v.after = c
				return v, true
			}
		}
		v.pre = rest
	}
	return v, true
}

// compareVersions returns -1, 0 or 1 as a is older, the same or newer than
// b. A version that is not a tag counts as newer than any tag; an empty one
// (unknown) as older than anything.
func compareVersions(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return -1
	}
	if b == "" {
		return 1
	}
	va, okA := parseVersion(a)
	vb, okB := parseVersion(b)
	switch {
	case !okA && !okB:
		return 0
	case !okA:
		return 1
	case !okB:
		return -1
	}
	for i := range va.nums {
		if c := cmpInt(va.nums[i], vb.nums[i]); c != 0 {
			return c
		}
	}
	// A pre-release comes before the release
	switch {
	case va.pre != "" && vb.pre == "":
		return -1
	case va.pre == "" && vb.pre != "":
		return 1
	case va.pre != vb.pre:
		return comparePre(va.pre, vb.pre)
	}
	return cmpInt(va.after, vb.after)
}

// comparePre compares two pre-releases, "rc.1" and "rc.2", by their parts:
// numbers as numbers, the rest as text
func comparePre(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		na, errA := strconv.Atoi(pa[i])
		nb, errB := strconv.Atoi(pb[i])
		switch {
		case errA == nil && errB == nil:
			if c := cmpInt(na, nb); c != 0 {
				return c
			}
		case errA == nil:
			return -1
		case errB == nil:
			return 1
		default:
			if c := strings.Compare(pa[i], pb[i]); c != 0 {
				return c
			}
		}
	}
	return cmpInt(len(pa), len(pb))
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// hasFlag tells whether the arguments have the flag, given as "--name" or
// "-name"
func hasFlag(args []string, name string) bool {
	for _, a := range args {
		if a == "--"+name || a == "-"+name {
			return true
		}
	}
	return false
}
