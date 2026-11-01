package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTwoCopiesKeepTheirProfiles(t *testing.T) {
	dir := t.TempDir()
	SetDir(dir)
	Load()
	p := DefaultProfile()
	p.Baud = 9600
	SetProfile("usb:a", p)
	if err := Save(); err != nil {
		t.Fatal(err)
	}

	// Another copy writes its profile to the file meanwhile
	var disk Settings
	data, _ := os.ReadFile(filepath.Join(dir, "settings.json"))
	json.Unmarshal(data, &disk)
	other := DefaultProfile()
	other.Baud = 57600
	disk.Profiles["usb:b"] = &other
	data, _ = json.Marshal(disk)
	os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644)

	p.Baud = 115200
	SetProfile("usb:a", p)
	if err := Save(); err != nil {
		t.Fatal(err)
	}
	Load()
	if got := ProfileFor("usb:a").Baud; got != 115200 {
		t.Fatal("own profile", got)
	}
	if got := ProfileFor("usb:b").Baud; got != 57600 {
		t.Fatal("profile of the other copy", got)
	}
	// A new device starts with the last profile, without its history
	p.History = []string{"AT"}
	SetProfile("usb:a", p)
	if np := ProfileFor("usb:new"); np.Baud != 115200 || len(np.History) != 0 {
		t.Fatalf("%+v", np)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatal("left files:", entries)
	}
}
