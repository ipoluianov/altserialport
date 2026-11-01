// Package config keeps the settings of the program and the profile of each
// device: the port settings and the view that was used with it last time.
// Several copies of the program may run at once, so a save reads the file
// again and changes only what this copy changed.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/ipoluianov/altserialport/rules"
)

// Profile is what is remembered for a device
type Profile struct {
	Baud     int    `json:"baud"`
	Frame    string `json:"frame"`
	Flow     string `json:"flow"` // "", "rtscts", "xonxoff"
	DTR      bool   `json:"dtr"`
	RTS      bool   `json:"rts"`
	View     string `json:"view"`       // "text", "hex", "mixed"
	Time     string `json:"timestamps"` // "", "time", "micro", "delta", "since"
	Break    string `json:"line_breaks"`
	Encoding string `json:"encoding"`
	Wrap     bool   `json:"wrap"`
	ShowTX   bool   `json:"show_tx"`
	Echo     bool   `json:"echo"`
	HexWidth int    `json:"hex_width"`
	GapMs    int    `json:"gap_ms"`
	RS485    bool   `json:"rs485,omitempty"`
	// HideRT hides the time from a sent row to the answer
	HideRT bool `json:"hide_response_time,omitempty"`
	// AutoLog records to a file each time the device is connected
	AutoLog bool `json:"auto_log,omitempty"`

	SendMode   string   `json:"send_mode"` // "text", "hex"
	LineEnding string   `json:"line_ending"`
	Checksum   string   `json:"checksum"`
	Backspace  string   `json:"backspace"` // "del", "bs"
	History    []string `json:"history,omitempty"`
}

// DefaultProfile is the profile of a device seen for the first time
func DefaultProfile() Profile {
	return Profile{
		Baud: 115200, Frame: "8N1", DTR: true, RTS: true,
		View: "text", Wrap: true, ShowTX: true, HexWidth: 16, GapMs: 10,
		SendMode: "text", LineEnding: "CRLF", Backspace: "del",
	}
}

// Settings of the program
type Settings struct {
	Theme       string `json:"theme"`    // "", "light"
	Language    string `json:"language"` // "" is the system's
	AlwaysOnTop bool   `json:"always_on_top"`
	FontSize    int    `json:"font_size"`
	LastPort    string `json:"last_port"` // the key of the device
	// LastProfile is the profile of the last device: a device seen for the
	// first time starts with it, not with the defaults
	LastProfile *Profile            `json:"last_profile,omitempty"`
	Profiles    map[string]*Profile `json:"profiles"`
	LogDir      string              `json:"log_dir,omitempty"`
	// ScrollbackRows is the most rows the terminal keeps
	ScrollbackRows int `json:"scrollback_rows"`
	// LogMaxMB starts a new log file past this size; 0 never
	LogMaxMB int `json:"log_max_mb"`

	Rules  []rules.Rule `json:"rules"`
	Macros []Macro      `json:"macros"`
	// Named are the profiles saved by a name of the user
	Named map[string]Profile `json:"named_profiles,omitempty"`

	ShowMacros bool `json:"show_macros,omitempty"`
	ShowPlot   bool `json:"show_plot,omitempty"`
	PanelSize  int  `json:"panel_size,omitempty"`
	PlotSize   int  `json:"plot_size,omitempty"`
}

// Macro is a named command of the macro panel: lines sent one after
// another; a line "#wait 100" waits 100 ms
type Macro struct {
	Name string `json:"name"`
	Text string `json:"text"`
	Hex  bool   `json:"hex,omitempty"`
}

// MaxHistory is the most commands kept in the history of a device
const MaxHistory = 200

var (
	mu       sync.Mutex
	settings = defaults()
	dir      string
	// changed are the profiles changed by this copy since the last save
	changed       = map[string]bool{}
	globalChanged bool
)

func defaults() Settings {
	return Settings{Profiles: map[string]*Profile{}, FontSize: 14, ScrollbackRows: 1_000_000,
		LogMaxMB: 100, Rules: rules.LogLevels(), Macros: defaultMacros()}
}

// Dir is the directory of the settings: ~/.altbins/.altserialport
func Dir() string {
	mu.Lock()
	defer mu.Unlock()
	return directory()
}

func directory() string {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		dir = filepath.Join(home, ".altbins", ".altserialport")
	}
	return dir
}

// SetDir keeps the settings in another directory (tests)
func SetDir(d string) {
	mu.Lock()
	dir = d
	mu.Unlock()
}

func path() string { return filepath.Join(directory(), "settings.json") }

// Load reads the settings; a missing or broken file gives the defaults
func Load() {
	mu.Lock()
	defer mu.Unlock()
	settings = read()
}

func read() Settings {
	s := defaults()
	data, err := os.ReadFile(path())
	if err == nil {
		json.Unmarshal(data, &s)
	}
	if s.Profiles == nil {
		s.Profiles = map[string]*Profile{}
	}
	if s.FontSize < 8 || s.FontSize > 40 {
		s.FontSize = 14
	}
	if s.ScrollbackRows < 1000 {
		s.ScrollbackRows = 1_000_000
	}
	return s
}

// Get returns a copy of the settings
func Get() Settings {
	mu.Lock()
	defer mu.Unlock()
	s := settings
	return s
}

// Update changes the settings of the program (not the profiles)
func Update(f func(s *Settings)) {
	mu.Lock()
	f(&settings)
	globalChanged = true
	mu.Unlock()
	Save()
}

// ProfileFor returns the profile of the device: its own, or the last one
// used, or the defaults
func ProfileFor(key string) Profile {
	mu.Lock()
	defer mu.Unlock()
	if p, ok := settings.Profiles[key]; ok {
		return fixProfile(*p)
	}
	if settings.LastProfile != nil {
		p := *settings.LastProfile
		p.History = nil
		return fixProfile(p)
	}
	return DefaultProfile()
}

// FixProfile fills the values a profile lacks with the defaults
func FixProfile(p Profile) Profile { return fixProfile(p) }

func fixProfile(p Profile) Profile {
	d := DefaultProfile()
	if p.Baud <= 0 {
		p.Baud = d.Baud
	}
	if p.Frame == "" {
		p.Frame = d.Frame
	}
	if p.View == "" {
		p.View = d.View
	}
	if p.HexWidth <= 0 {
		p.HexWidth = d.HexWidth
	}
	if p.SendMode == "" {
		p.SendMode = d.SendMode
	}
	if p.LineEnding == "" {
		p.LineEnding = d.LineEnding
	}
	if p.Backspace == "" {
		p.Backspace = d.Backspace
	}
	return p
}

// SetProfile remembers the profile of the device and makes it the last one
func SetProfile(key string, p Profile) {
	if len(p.History) > MaxHistory {
		p.History = p.History[len(p.History)-MaxHistory:]
	}
	mu.Lock()
	cp := p
	settings.Profiles[key] = &cp
	last := p
	last.History = nil
	settings.LastProfile = &last
	settings.LastPort = key
	changed[key] = true
	globalChanged = true
	mu.Unlock()
}

// Save writes the settings: the file is read again, and what this copy
// changed is put into it, so other copies keep their profiles
func Save() error {
	mu.Lock()
	defer mu.Unlock()
	if !globalChanged && len(changed) == 0 {
		return nil
	}
	disk := read()
	for key := range changed {
		if p, ok := settings.Profiles[key]; ok {
			disk.Profiles[key] = p
		}
	}
	if globalChanged {
		profiles := disk.Profiles
		disk = settings
		disk.Profiles = profiles
	}
	// The profiles changed by the other copies are seen from now on
	for k, p := range disk.Profiles {
		if !changed[k] {
			settings.Profiles[k] = p
		}
	}
	data, err := json.MarshalIndent(disk, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(directory(), 0o755); err != nil {
		return err
	}
	// A file of its own: another copy may be saving at the same moment
	f, err := os.CreateTemp(directory(), "settings-*.tmp")
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), path())
	}
	if err != nil {
		os.Remove(f.Name())
		return err
	}
	changed = map[string]bool{}
	globalChanged = false
	return nil
}

// defaultMacros are the first macros, to show what they are for
func defaultMacros() []Macro {
	return []Macro{
		{Name: "AT", Text: "AT"},
		{Name: "Version", Text: "AT+GMR"},
		{Name: "Reset (AT)", Text: "AT+RST"},
		{Name: "Modbus: read 10 regs", Text: "01 03 00 00 00 0A C5 CD", Hex: true},
	}
}
