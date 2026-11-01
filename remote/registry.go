package remote

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/ipoluianov/altserialport/config"
)

// Entry is a window that serves its port, as its registry file tells
type Entry struct {
	PID   int    `json:"pid"`
	Addr  string `json:"addr"`  // "127.0.0.1:41234"
	Token string `json:"token"` // the secret a client sends first
	// State is "open", "waiting" (the device is gone and the window waits
	// for it) or "closed"
	State string `json:"state"`
	Port  string `json:"port"`  // the path the window opened: "/dev/ttyUSB0", "COM5"
	Short string `json:"short"` // "ttyUSB0", "COM5"
	Baud  int    `json:"baud,omitempty"`
	Frame string `json:"frame,omitempty"`
}

func registryDir() string {
	return filepath.Join(config.Dir(), "instances")
}

func entryPath(pid int) string {
	return filepath.Join(registryDir(), strconv.Itoa(pid)+".json")
}

// writeEntry writes the file through a temporary one, so a reader never
// sees half of it
func writeEntry(e Entry) error {
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	dir := registryDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "entry-*.tmp")
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), entryPath(e.PID))
	}
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}

// Entries returns the windows that serve a port, the files of the dead
// processes removed
func Entries() []Entry {
	files, err := os.ReadDir(registryDir())
	if err != nil {
		return nil
	}
	var list []Entry
	for _, f := range files {
		name := f.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		path := filepath.Join(registryDir(), name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var e Entry
		if json.Unmarshal(data, &e) != nil || e.PID <= 0 {
			continue
		}
		if !processAlive(e.PID) {
			os.Remove(path)
			continue
		}
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].PID < list[j].PID })
	return list
}

// Find returns the window that has the port open (or waits for its device
// to come back)
func Find(portName string) (Entry, bool) {
	want := canonical(portName)
	for _, e := range Entries() {
		if e.State == "closed" || e.Port == "" {
			continue
		}
		if sameName(want, canonical(e.Port)) || sameName(portName, e.Short) {
			return e, true
		}
	}
	return Entry{}, false
}

// canonical resolves the links of a unix path ("/dev/serial/by-id/..."),
// and drops the \\.\ prefix of a Windows name
func canonical(name string) string {
	if runtime.GOOS == "windows" {
		return strings.TrimPrefix(name, `\\.\`)
	}
	if p, err := filepath.EvalSymlinks(name); err == nil {
		return p
	}
	return name
}

func sameName(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
