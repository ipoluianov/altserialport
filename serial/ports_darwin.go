package serial

import (
	"path/filepath"
	"strings"
)

// List returns the callout devices (/dev/cu.*): opening them doesn't wait
// for the carrier, as the tty.* ones do. macOS keeps the USB IDs in the
// IORegistry; the kind of a port is told by its name.
func List() ([]PortInfo, error) {
	paths, err := filepath.Glob("/dev/cu.*")
	if err != nil {
		return nil, err
	}
	var ports []PortInfo
	for _, path := range paths {
		short := strings.TrimPrefix(path, "/dev/")
		p := PortInfo{Name: path, Short: short}
		p.Description, p.USB = describeDarwin(short)
		ports = append(ports, p)
	}
	sortPorts(ports)
	return ports, nil
}

func describeDarwin(short string) (string, bool) {
	n := strings.ToLower(short)
	switch {
	case strings.Contains(n, "wchusbserial"):
		return "CH340", true
	case strings.Contains(n, "slab_usbtouart"):
		return "CP210x", true
	case strings.Contains(n, "usbserial"):
		return "USB serial", true
	case strings.Contains(n, "usbmodem"):
		return "USB CDC", true
	case strings.Contains(n, "bluetooth"):
		return "Bluetooth", false
	}
	return "", false
}
