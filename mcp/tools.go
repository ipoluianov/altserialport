package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ipoluianov/altserialport/proto"
	"github.com/ipoluianov/altserialport/remote"
	"github.com/ipoluianov/altserialport/serial"
)

// tool is a tool of the server: its description for the agent and the
// function that does it
type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	run         func(s *server, args json.RawMessage) (any, error)
}

// Limits of the waits, so one call can't hang the server
const (
	maxTimeout = 5 * time.Minute
	maxBreak   = 10 * time.Second
)

func object(props map[string]any, required ...string) map[string]any {
	o := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		o["required"] = required
	}
	return o
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

var portProp = str(`The port as open_port got it: "/dev/ttyUSB0", "COM5"`)

var tools []*tool

func init() {
	tools = []*tool{
		{
			Name: "list_ports",
			Description: "Lists the serial ports of the computer: USB-UART adapters (CH340, CP210x, FTDI), " +
				"boards with native USB (ESP32, RP2040, STM32), built-in UARTs. Each has its description, " +
				"chip, USB vendor:product ID and serial number when known, whether this server has it open " +
				"and whether an AltSerialPort window has it (open_port then attaches to the window).",
			InputSchema: object(map[string]any{}),
			run:         listPorts,
		},
		{
			Name: "open_port",
			Description: "Opens a serial port. Data received from then on is kept in a buffer of up to 1 MB " +
				"(the oldest is dropped) that read and wait_for consume. If the port is busy because an " +
				"AltSerialPort window has it, the call attaches to the window: the user sees what you send " +
				"and receive, and the speed and frame are the window's. If the device resets or " +
				"re-enumerates, the port is opened again by itself. Opening a port that is already open " +
				"changes its settings. Note: opening may reset some boards (Arduino) through DTR.",
			InputSchema: object(map[string]any{
				"port": str(`The port: "/dev/ttyUSB0", "/dev/ttyACM0", "COM5", or "tcp://host:port" for a network serial server`),
				"baud": map[string]any{"type": "integer", "description": "Baud rate. Default 115200. ESP8266 boot messages are at 74880",
					"default": 115200, "minimum": 1},
				"frame": map[string]any{"type": "string", "description": `Data bits, parity (N, E, O, M, S), stop bits. Default "8N1"`,
					"default": "8N1"},
				"flow": map[string]any{"type": "string", "enum": []string{"none", "rtscts", "xonxoff"},
					"description": `Flow control. Default "none"`, "default": "none"},
			}, "port"),
			run: openPortTool,
		},
		{
			Name:        "close_port",
			Description: "Closes a port opened with open_port (an attached window keeps its port open).",
			InputSchema: object(map[string]any{"port": portProp}, "port"),
			run:         closePort,
		},
		{
			Name: "write",
			Description: "Writes to the port. The text may have escapes: \\r \\n \\t \\xHH. With hex=true data is " +
				`hex bytes: "01 03 00 0A". line_ending is added after the data; most command shells of boards ` +
				`(AT modems, MicroPython, Zephyr, U-Boot) want "crlf" or "cr" to run a command. ` +
				"Answers are collected with read or wait_for.",
			InputSchema: object(map[string]any{
				"port": portProp,
				"data": str("The text or the hex bytes to send"),
				"hex":  map[string]any{"type": "boolean", "description": "data is hex bytes", "default": false},
				"line_ending": map[string]any{"type": "string", "enum": []string{"none", "cr", "lf", "crlf"},
					"description": `Added after the data. Default "none"`, "default": "none"},
			}, "port", "data"),
			run: write,
		},
		{
			Name: "read",
			Description: "Returns the data received since the last read or wait_for, and removes it from the buffer. " +
				"Waits up to timeout_ms for something to come; once data comes it returns after 100 ms of quiet. " +
				"The result has the text (with \\r and \\n as JSON escapes) and, when the bytes are not " +
				"printable UTF-8, their hex.",
			InputSchema: object(map[string]any{
				"port":       portProp,
				"timeout_ms": map[string]any{"type": "integer", "description": "Longest wait. Default 500", "default": 500, "minimum": 0},
				"max_bytes":  map[string]any{"type": "integer", "description": "Most bytes returned. Default 65536", "default": 65536, "minimum": 1},
			}, "port"),
			run: read,
		},
		{
			Name: "wait_for",
			Description: "Waits until the received data matches a regular expression (Go RE2 syntax, e.g. \"OK|ERROR\", " +
				"\"(?m)^> $\", \"login: \"). Returns the text up to and including the match and removes it from " +
				"the buffer; what came after the match stays for the next read. Data received before this call " +
				"and not yet read counts too. On timeout the result is an error with what came; that data " +
				"stays in the buffer.",
			InputSchema: object(map[string]any{
				"port":       portProp,
				"pattern":    str("The regular expression"),
				"timeout_ms": map[string]any{"type": "integer", "description": "Longest wait. Default 5000", "default": 5000, "minimum": 0},
			}, "port", "pattern"),
			run: waitFor,
		},
		{
			Name: "set_lines",
			Description: "Sets the DTR and/or RTS output lines (true is asserted). On many boards DTR and RTS are " +
				"wired to reset and boot pins: for a reset prefer reset_board.",
			InputSchema: object(map[string]any{
				"port": portProp,
				"dtr":  map[string]any{"type": "boolean", "description": "DTR; left as it is when absent"},
				"rts":  map[string]any{"type": "boolean", "description": "RTS; left as it is when absent"},
			}, "port"),
			run: setLines,
		},
		{
			Name:        "get_signals",
			Description: "Reads the input lines of the port: CTS, DSR, DCD (carrier detect), RI (ring).",
			InputSchema: object(map[string]any{"port": portProp}, "port"),
			run:         getSignals,
		},
		{
			Name: "send_break",
			Description: "Holds the TX line low for ms milliseconds (a BREAK). Some devices take it as an " +
				"attention or reset signal; Linux consoles with SysRq enabled take it as SysRq.",
			InputSchema: object(map[string]any{
				"port": portProp,
				"ms":   map[string]any{"type": "integer", "description": "Length of the break. Default 250", "default": 250, "minimum": 1},
			}, "port"),
			run: sendBreak,
		},
		{
			Name: "reset_board",
			Description: "Resets the board through the DTR/RTS lines of the USB-UART adapter. " +
				`"dtr": a DTR pulse, the Arduino reset (Uno, Nano, Mega). ` +
				`"esp32_boot": the ESP32/ESP8266 auto-reset circuit into the ROM bootloader (download mode, for flashing). ` +
				`"esp32_run": the ESP32/ESP8266 reset into the firmware (normal boot). ` +
				"The boot messages that follow can be collected with read or wait_for. " +
				"Boards with native USB (ESP32-S3/C3 USB JTAG/serial) may disappear and come back; the port is opened again by itself.",
			InputSchema: object(map[string]any{
				"port": portProp,
				"kind": map[string]any{"type": "string", "enum": []string{"dtr", "esp32_boot", "esp32_run"},
					"description": "The reset sequence"},
			}, "port", "kind"),
			run: resetBoard,
		},
		{
			Name:        "status",
			Description: "Shows the ports this server has open: state (open, or waiting for a gone device), settings, whether attached to a window, bytes buffered, received and sent.",
			InputSchema: object(map[string]any{}),
			run:         status,
		},
	}
}

func toolList() []*tool { return tools }

func toolByName(name string) (*tool, bool) {
	for _, t := range tools {
		if t.Name == name {
			return t, true
		}
	}
	return nil, false
}

// parse reads the arguments into v
func parse(args json.RawMessage, v any) error {
	if err := json.Unmarshal(args, v); err != nil {
		return fmt.Errorf("invalid arguments: %v", err)
	}
	return nil
}

// find returns the open port of the name: the name given to open_port, or
// its short name, or another path to it
func (s *server) find(name string) (*port, error) {
	if name == "" {
		return nil, errors.New(`"port" is required`)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.ports[name]; ok {
		return p, nil
	}
	want := resolve(name)
	for key, p := range s.ports {
		if strings.EqualFold(filepath.Base(key), name) || resolve(key) == want {
			return p, nil
		}
	}
	return nil, fmt.Errorf("port %s is not open: call open_port first", name)
}

func resolve(name string) string {
	if p, err := filepath.EvalSymlinks(name); err == nil {
		return p
	}
	return name
}

func flowName(f serial.Flow) string {
	switch f {
	case serial.FlowRTSCTS:
		return "rtscts"
	case serial.FlowXonXoff:
		return "xonxoff"
	}
	return "none"
}

func listPorts(s *server, _ json.RawMessage) (any, error) {
	ports, err := serial.List()
	if err != nil {
		return nil, err
	}
	windows := remote.Entries()
	out := []map[string]any{}
	for _, p := range ports {
		m := map[string]any{"port": p.Name, "short": p.Short}
		add := func(k, v string) {
			if v != "" {
				m[k] = v
			}
		}
		add("description", p.Description)
		add("chip", p.Chip())
		add("usb_id", p.IDs())
		add("serial_number", p.SerialNumber)
		add("manufacturer", p.Manufacturer)
		add("product", p.Product)
		add("driver", p.Driver)
		if _, err := s.find(p.Name); err == nil {
			m["open_here"] = true
		}
		for _, e := range windows {
			if e.State != "closed" && (e.Port == p.Name || resolve(e.Port) == resolve(p.Name)) {
				m["window"] = fmt.Sprintf("open in an AltSerialPort window at %d %s", e.Baud, e.Frame)
			}
		}
		out = append(out, m)
	}
	return map[string]any{"ports": out}, nil
}

func openPortTool(s *server, args json.RawMessage) (any, error) {
	var a struct {
		Port  string `json:"port"`
		Baud  int    `json:"baud"`
		Frame string `json:"frame"`
		Flow  string `json:"flow"`
	}
	if err := parse(args, &a); err != nil {
		return nil, err
	}
	if a.Port == "" {
		return nil, errors.New(`"port" is required`)
	}
	cfg := serial.DefaultConfig()
	if a.Baud != 0 {
		cfg.Baud = a.Baud
	}
	if a.Frame != "" {
		if err := cfg.SetFrame(a.Frame); err != nil {
			return nil, err
		}
	}
	switch strings.ToLower(a.Flow) {
	case "", "none":
	case "rtscts":
		cfg.Flow = serial.FlowRTSCTS
	case "xonxoff":
		cfg.Flow = serial.FlowXonXoff
	default:
		return nil, fmt.Errorf("invalid flow %q: none, rtscts or xonxoff", a.Flow)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if pt, err := s.find(a.Port); err == nil {
		p, err := pt.current()
		if err != nil {
			return nil, err
		}
		pt.mu.Lock()
		attached := pt.attached
		pt.mu.Unlock()
		if attached {
			return fmt.Sprintf("%s is already open, attached to an AltSerialPort window; its settings are the window's", a.Port), nil
		}
		if err := p.SetConfig(cfg); err != nil {
			return nil, err
		}
		pt.mu.Lock()
		pt.cfg = cfg
		pt.mu.Unlock()
		return fmt.Sprintf("%s was already open; settings changed to %d %s, flow %s", a.Port, cfg.Baud, cfg.Frame(), flowName(cfg.Flow)), nil
	}

	pt, err := openPort(a.Port, cfg)
	if err != nil {
		return nil, openError(a.Port, err)
	}
	s.mu.Lock()
	s.ports[a.Port] = pt
	s.mu.Unlock()
	if pt.attached {
		msg := fmt.Sprintf("%s is open in an AltSerialPort window: attached to it. The user sees in the window what you send and receive.", a.Port)
		pt.mu.Lock()
		if rp, ok := pt.p.(*remote.Port); ok {
			info := rp.Info()
			msg += fmt.Sprintf(" The settings are the window's: %d %s.", info.Baud, info.Frame)
			pt.cfg.Baud = info.Baud
			pt.cfg.SetFrame(info.Frame)
		}
		pt.mu.Unlock()
		return msg, nil
	}
	return fmt.Sprintf("%s is open at %d %s, flow %s", a.Port, cfg.Baud, cfg.Frame(), flowName(cfg.Flow)), nil
}

// openError explains an error of opening to the agent
func openError(name string, err error) error {
	switch {
	case errors.Is(err, serial.ErrBusy):
		return fmt.Errorf("%s is busy: another program has it open (a window of AltSerialPort waiting for its device counts too)", name)
	case errors.Is(err, serial.ErrPermission):
		return fmt.Errorf("%s: access denied. On Linux the user must be in the dialout (or uucp) group; on Windows another program may have it", name)
	case errors.Is(err, serial.ErrNotFound):
		return fmt.Errorf("%s: no such port; call list_ports", name)
	}
	return fmt.Errorf("%s: %v", name, err)
}

func closePort(s *server, args json.RawMessage) (any, error) {
	var a struct {
		Port string `json:"port"`
	}
	if err := parse(args, &a); err != nil {
		return nil, err
	}
	pt, err := s.find(a.Port)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	for k, v := range s.ports {
		if v == pt {
			delete(s.ports, k)
		}
	}
	s.mu.Unlock()
	pt.close()
	return fmt.Sprintf("%s is closed", pt.name), nil
}

func write(s *server, args json.RawMessage) (any, error) {
	var a struct {
		Port       string `json:"port"`
		Data       string `json:"data"`
		Hex        bool   `json:"hex"`
		LineEnding string `json:"line_ending"`
	}
	if err := parse(args, &a); err != nil {
		return nil, err
	}
	pt, err := s.find(a.Port)
	if err != nil {
		return nil, err
	}
	var data []byte
	if a.Hex {
		if data, err = proto.ParseHex(a.Data); err != nil {
			return nil, err
		}
	} else {
		data = proto.ParseEscapes(a.Data)
	}
	switch strings.ToLower(a.LineEnding) {
	case "", "none":
	case "cr":
		data = append(data, '\r')
	case "lf":
		data = append(data, '\n')
	case "crlf":
		data = append(data, '\r', '\n')
	default:
		return nil, fmt.Errorf("invalid line_ending %q: none, cr, lf or crlf", a.LineEnding)
	}
	if len(data) == 0 {
		return nil, errors.New("nothing to send")
	}
	if err := pt.write(data); err != nil {
		return nil, err
	}
	return fmt.Sprintf("sent %d bytes", len(data)), nil
}

// describe is the result of the received bytes
func describe(data []byte) map[string]any {
	m := map[string]any{"bytes": len(data), "text": string(data)}
	if !printable(data) {
		m["hex"] = proto.FormatHex(data)
	}
	return m
}

// printable tells whether the bytes are UTF-8 text without control
// characters other than CR, LF and tab
func printable(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	for _, r := range string(data) {
		if r != '\r' && r != '\n' && r != '\t' && !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

func millis(v *int, def int) time.Duration {
	d := time.Duration(def) * time.Millisecond
	if v != nil {
		d = time.Duration(max(*v, 0)) * time.Millisecond
	}
	return min(d, maxTimeout)
}

func read(s *server, args json.RawMessage) (any, error) {
	var a struct {
		Port      string `json:"port"`
		TimeoutMs *int   `json:"timeout_ms"`
		MaxBytes  int    `json:"max_bytes"`
	}
	if err := parse(args, &a); err != nil {
		return nil, err
	}
	pt, err := s.find(a.Port)
	if err != nil {
		return nil, err
	}
	if a.MaxBytes <= 0 {
		a.MaxBytes = 65536
	}
	data := pt.read(millis(a.TimeoutMs, 500), min(a.MaxBytes, maxBuffer))
	m := describe(data)
	if st := pt.status(); st["state"] != "open" {
		m["state"] = st["state"]
	}
	if n := pt.status()["buffered_bytes"].(int); n > 0 {
		m["more_buffered"] = n
	}
	return m, nil
}

// timeoutError is a tool error with a body for the agent
type timeoutError struct{ body map[string]any }

func (e *timeoutError) Error() string {
	data, _ := json.MarshalIndent(e.body, "", "  ")
	return string(data)
}

func waitFor(s *server, args json.RawMessage) (any, error) {
	var a struct {
		Port      string `json:"port"`
		Pattern   string `json:"pattern"`
		TimeoutMs *int   `json:"timeout_ms"`
	}
	if err := parse(args, &a); err != nil {
		return nil, err
	}
	pt, err := s.find(a.Port)
	if err != nil {
		return nil, err
	}
	if a.Pattern == "" {
		return nil, errors.New(`"pattern" is required`)
	}
	re, err := regexp.Compile(a.Pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %v", err)
	}
	timeout := millis(a.TimeoutMs, 5000)
	data, ok := pt.waitFor(func(b []byte) int {
		if loc := re.FindIndex(b); loc != nil {
			return loc[1]
		}
		return -1
	}, timeout)
	m := describe(data)
	if !ok {
		m["matched"] = false
		m["error"] = fmt.Sprintf("no match of %q in %d ms; the data that came is kept for read", a.Pattern, timeout.Milliseconds())
		if st := pt.status(); st["state"] != "open" {
			m["state"] = st["state"]
		}
		return nil, &timeoutError{m}
	}
	m["matched"] = true
	return m, nil
}

func setLines(s *server, args json.RawMessage) (any, error) {
	var a struct {
		Port string `json:"port"`
		DTR  *bool  `json:"dtr"`
		RTS  *bool  `json:"rts"`
	}
	if err := parse(args, &a); err != nil {
		return nil, err
	}
	pt, err := s.find(a.Port)
	if err != nil {
		return nil, err
	}
	if a.DTR == nil && a.RTS == nil {
		return nil, errors.New(`give "dtr", "rts" or both`)
	}
	var done []string
	if a.DTR != nil {
		if err := pt.setDTR(*a.DTR); err != nil {
			return nil, fmt.Errorf("DTR: %v", err)
		}
		done = append(done, fmt.Sprintf("DTR=%d", boolInt(*a.DTR)))
	}
	if a.RTS != nil {
		if err := pt.setRTS(*a.RTS); err != nil {
			return nil, fmt.Errorf("RTS: %v", err)
		}
		done = append(done, fmt.Sprintf("RTS=%d", boolInt(*a.RTS)))
	}
	return strings.Join(done, ", "), nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func getSignals(s *server, args json.RawMessage) (any, error) {
	var a struct {
		Port string `json:"port"`
	}
	if err := parse(args, &a); err != nil {
		return nil, err
	}
	pt, err := s.find(a.Port)
	if err != nil {
		return nil, err
	}
	p, err := pt.current()
	if err != nil {
		return nil, err
	}
	sig, err := p.Signals()
	if err != nil {
		return nil, err
	}
	return map[string]any{"cts": sig.CTS, "dsr": sig.DSR, "dcd": sig.DCD, "ri": sig.RI}, nil
}

func sendBreak(s *server, args json.RawMessage) (any, error) {
	var a struct {
		Port string `json:"port"`
		Ms   *int   `json:"ms"`
	}
	if err := parse(args, &a); err != nil {
		return nil, err
	}
	pt, err := s.find(a.Port)
	if err != nil {
		return nil, err
	}
	p, err := pt.current()
	if err != nil {
		return nil, err
	}
	d := min(millis(a.Ms, 250), maxBreak)
	if d <= 0 {
		d = 250 * time.Millisecond
	}
	if err := p.Break(d); err != nil {
		return nil, err
	}
	return fmt.Sprintf("break of %d ms sent", d.Milliseconds()), nil
}

// step is a step of a reset sequence: the lines to set (nil keeps one)
// and the time to wait after it
type step struct {
	dtr, rts *bool
	wait     time.Duration
}

func resetBoard(s *server, args json.RawMessage) (any, error) {
	var a struct {
		Port string `json:"port"`
		Kind string `json:"kind"`
	}
	if err := parse(args, &a); err != nil {
		return nil, err
	}
	pt, err := s.find(a.Port)
	if err != nil {
		return nil, err
	}
	on, off := true, false
	var steps []step
	switch a.Kind {
	case "dtr":
		steps = []step{{dtr: &off, wait: 100 * time.Millisecond}, {dtr: &on}}
	case "esp32_boot":
		// RTS pulls EN (reset) low, DTR pulls GPIO0 low
		steps = []step{
			{dtr: &off, rts: &on, wait: 100 * time.Millisecond},
			{dtr: &on, rts: &off, wait: 50 * time.Millisecond},
			{dtr: &off},
		}
	case "esp32_run":
		steps = []step{{dtr: &off, rts: &on, wait: 100 * time.Millisecond}, {rts: &off}}
	default:
		return nil, fmt.Errorf("invalid kind %q: dtr, esp32_boot or esp32_run", a.Kind)
	}
	for _, st := range steps {
		if st.dtr != nil {
			if err := pt.setDTR(*st.dtr); err != nil {
				return nil, fmt.Errorf("DTR: %v", err)
			}
		}
		if st.rts != nil {
			if err := pt.setRTS(*st.rts); err != nil {
				return nil, fmt.Errorf("RTS: %v", err)
			}
		}
		time.Sleep(st.wait)
	}
	return fmt.Sprintf("reset %s done; read or wait_for collects the boot messages", a.Kind), nil
}

func status(s *server, _ json.RawMessage) (any, error) {
	s.mu.Lock()
	list := make([]*port, 0, len(s.ports))
	for _, p := range s.ports {
		list = append(list, p)
	}
	s.mu.Unlock()
	sort.Slice(list, func(i, j int) bool { return list[i].name < list[j].name })
	out := []map[string]any{}
	for _, p := range list {
		out = append(out, p.status())
	}
	return map[string]any{"open_ports": out}, nil
}
