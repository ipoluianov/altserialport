// Package cli is the command line of the program: the list of the ports,
// a terminal in the console, sending a command and waiting for the answer
// (for scripts and CI) and the MCP server for AI agents. A port that an
// AltSerialPort window has open is used through the window.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ipoluianov/altserialport/app"
	"github.com/ipoluianov/altserialport/mcp"
	"github.com/ipoluianov/altserialport/remote"
	"github.com/ipoluianov/altserialport/serial"
)

const usage = `Usage:
  altserialport                       the window
  altserialport list                  the ports with their descriptions and USB IDs
  altserialport <port> [options]      a terminal in the console; Ctrl+] exits
  altserialport send <port> <data> [options]
                                      write, then print what comes
  altserialport mcp                   an MCP server on stdin/stdout for AI agents
  altserialport version

Ports: /dev/ttyUSB0, COM5, tcp://host:port, rfc2217://host:port, tcp-listen://:port

Options:
  -b, --baud N          baud rate (115200)
  -f, --frame F         data bits, parity, stop bits (8N1)
      --flow F          none, rtscts or xonxoff (none)
      --log FILE        append what comes and goes to the file
Terminal:
      --hex             show the data as hex bytes
      --timestamp       the time at the start of each line
      --echo            show what is typed
      --crlf            Enter sends CR LF
Send:
      --hex             data is hex bytes: "01 03 00 0A"; otherwise text
                        with \r \n \t \xHH escapes
      --cr, --lf, --crlf
                        add a line ending to the data
      --expect RE       wait for the regular expression: exit 0 on a match,
                        1 on timeout
      --timeout D       how long to wait (2s)
`

// Exit codes
const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2
)

// portRe matches a Windows COM port name
var portRe = regexp.MustCompile(`(?i)^(\\\\\.\\)?COM\d+$`)

// IsCommand tells whether the arguments ask for the command line, not the
// window
func IsCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "list", "send", "mcp", "help", "-h", "--help", "version", "--version":
		return true
	}
	return looksLikePort(args[0])
}

func looksLikePort(s string) bool {
	if strings.HasPrefix(s, "/dev/") || portRe.MatchString(s) {
		return true
	}
	for _, p := range []string{"tcp://", "rfc2217://", "tcp-listen://"} {
		if strings.HasPrefix(strings.ToLower(s), p) {
			return true
		}
	}
	return false
}

// env is where a command reads and writes; the tests give their own
type env struct {
	stdin          io.Reader
	stdout, stderr io.Writer
}

// Run runs the command of the arguments (without the program name) and
// returns the exit code
func Run(args []string) int {
	return run(args, env{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr})
}

func run(args []string, e env) int {
	if len(args) == 0 {
		fmt.Fprint(e.stderr, usage)
		return exitUsage
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(e.stdout, usage)
		return exitOK
	case "version", "--version":
		fmt.Fprintf(e.stdout, "%s %s\n", app.DisplayName, app.Version)
		return exitOK
	case "list":
		return list(e)
	case "mcp":
		if err := mcp.Serve(e.stdin, e.stdout); err != nil {
			fmt.Fprintln(e.stderr, "mcp:", err)
			return exitFail
		}
		return exitOK
	case "send":
		o, err := parseArgs(args[1:])
		if err != nil {
			return usageError(e, err)
		}
		return send(o, e)
	}
	// Any other first argument is a port: a pseudo terminal or a link
	// to a device may be anywhere
	if strings.HasPrefix(args[0], "-") {
		return usageError(e, fmt.Errorf("unknown option %s", args[0]))
	}
	o, err := parseArgs(args)
	if err != nil {
		return usageError(e, err)
	}
	return terminal(o, e)
}

func usageError(e env, err error) int {
	fmt.Fprintf(e.stderr, "altserialport: %v\n\n%s", err, usage)
	return exitUsage
}

// options are the parsed arguments of a command
type options struct {
	args []string // the arguments that are not options: the port, the data

	cfg     serial.Config
	log     string
	hex     bool
	stamp   bool
	echo    bool
	ending  string // "", "\r", "\n", "\r\n"
	expect  *regexp.Regexp
	timeout time.Duration
}

// parseArgs reads the options, which may stand before, between and after
// the other arguments; "--" ends them
func parseArgs(args []string) (options, error) {
	o := options{cfg: serial.DefaultConfig(), timeout: 2 * time.Second}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			o.args = append(o.args, args[i+1:]...)
			break
		}
		if len(a) < 2 || a[0] != '-' {
			o.args = append(o.args, a)
			continue
		}
		name, value, hasValue := strings.Cut(a, "=")
		// value takes the value of the option: after "=" or the next argument
		takeValue := func() (string, error) {
			if hasValue {
				return value, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s needs a value", name)
			}
			i++
			return args[i], nil
		}
		flag := func() error {
			if hasValue {
				return fmt.Errorf("%s takes no value", name)
			}
			return nil
		}
		var err error
		switch name {
		case "-b", "--baud":
			var v string
			if v, err = takeValue(); err == nil {
				o.cfg.Baud, err = strconv.Atoi(v)
				if err != nil || o.cfg.Baud <= 0 {
					err = fmt.Errorf("invalid baud rate %q", v)
				}
			}
		case "-f", "--frame":
			var v string
			if v, err = takeValue(); err == nil {
				err = o.cfg.SetFrame(v)
			}
		case "--flow":
			var v string
			if v, err = takeValue(); err == nil {
				switch strings.ToLower(v) {
				case "none":
					o.cfg.Flow = serial.FlowNone
				case "rtscts":
					o.cfg.Flow = serial.FlowRTSCTS
				case "xonxoff":
					o.cfg.Flow = serial.FlowXonXoff
				default:
					err = fmt.Errorf("invalid flow %q: none, rtscts or xonxoff", v)
				}
			}
		case "--log":
			o.log, err = takeValue()
		case "--expect", "-e":
			var v string
			if v, err = takeValue(); err == nil {
				o.expect, err = regexp.Compile(v)
			}
		case "--timeout":
			var v string
			if v, err = takeValue(); err == nil {
				o.timeout, err = parseDuration(v)
			}
		case "--hex":
			o.hex, err = true, flag()
		case "--timestamp":
			o.stamp, err = true, flag()
		case "--echo":
			o.echo, err = true, flag()
		case "--crlf":
			o.ending, err = "\r\n", flag()
		case "--cr":
			o.ending, err = "\r", flag()
		case "--lf":
			o.ending, err = "\n", flag()
		default:
			err = fmt.Errorf("unknown option %s", name)
		}
		if err != nil {
			return o, err
		}
	}
	if err := o.cfg.Validate(); err != nil {
		return o, err
	}
	return o, nil
}

// parseDuration reads "2s", "500ms", or a number of seconds
func parseDuration(s string) (time.Duration, error) {
	if d, err := time.ParseDuration(s); err == nil && d >= 0 {
		return d, nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && f >= 0 {
		return time.Duration(f * float64(time.Second)), nil
	}
	return 0, fmt.Errorf("invalid timeout %q", s)
}

func list(e env) int {
	ports, err := serial.List()
	if err != nil {
		fmt.Fprintln(e.stderr, "altserialport:", err)
		return exitFail
	}
	if len(ports) == 0 {
		fmt.Fprintln(e.stderr, "No serial ports found")
		return exitOK
	}
	windows := remote.Entries()
	tw := tabwriter.NewWriter(e.stdout, 0, 0, 2, ' ', 0)
	for _, p := range ports {
		what := p.Description
		if chip := p.Chip(); chip != "" && !strings.Contains(what, chip) {
			if what != "" {
				what = chip + " · " + what
			} else {
				what = chip
			}
		}
		extra := ""
		if p.SerialNumber != "" {
			extra = "SN " + p.SerialNumber
		}
		for _, w := range windows {
			if w.State != "closed" && w.Port == p.Name {
				extra = strings.TrimSpace(extra + "  (open in a window)")
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", p.Name, what, p.IDs(), extra)
	}
	tw.Flush()
	return exitOK
}

// open opens the port of the command, or attaches to the window that has
// it and says so
func open(name string, cfg serial.Config, e env) (serial.Port, error) {
	p, attached, err := remote.Open(name, cfg)
	if err != nil {
		return nil, openError(name, err)
	}
	if attached {
		msg := "attached to the AltSerialPort window that has " + name
		if rp, ok := p.(*remote.Port); ok {
			msg += fmt.Sprintf(" (%d %s, the window's settings)", rp.Info().Baud, rp.Info().Frame)
		}
		fmt.Fprintln(e.stderr, msg)
	}
	return p, nil
}

func openError(name string, err error) error {
	switch {
	case errors.Is(err, serial.ErrBusy):
		return fmt.Errorf("%s is busy: another program has it open", name)
	case errors.Is(err, serial.ErrPermission):
		return fmt.Errorf("%s: access denied", name)
	case errors.Is(err, serial.ErrNotFound):
		return fmt.Errorf("%s: no such port (altserialport list shows them)", name)
	}
	return fmt.Errorf("%s: %v", name, err)
}
