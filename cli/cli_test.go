package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/ipoluianov/altserialport/serial"
)

func TestIsCommand(t *testing.T) {
	yes := [][]string{{"list"}, {"send", "x"}, {"mcp"}, {"help"}, {"-h"}, {"--help"}, {"version"},
		{"--version"}, {"/dev/ttyUSB0"}, {"COM5"}, {"com12", "-b", "9600"}, {`\\.\COM10`},
		{"tcp://10.0.0.5:4001"}, {"rfc2217://host:2217"}, {"tcp-listen://:5000"}}
	for _, a := range yes {
		if !IsCommand(a) {
			t.Error("not a command:", a)
		}
	}
	no := [][]string{nil, {}, {"-psn_0_12345"}, {"COMPUTER"}, {"file.txt"}, {"-b"}}
	for _, a := range no {
		if IsCommand(a) {
			t.Error("a command:", a)
		}
	}
}

func TestParseArgs(t *testing.T) {
	o, err := parseArgs([]string{"COM5", "-b", "9600", "--frame=7E1", "--flow", "rtscts",
		"--log", "out.log", "--hex", "--timestamp", "--echo", "--crlf"})
	if err != nil {
		t.Fatal(err)
	}
	if len(o.args) != 1 || o.args[0] != "COM5" {
		t.Fatal("args", o.args)
	}
	want := serial.Config{Baud: 9600, DataBits: 7, Parity: serial.ParityEven, StopBits: serial.StopBits1, Flow: serial.FlowRTSCTS}
	if o.cfg != want {
		t.Fatalf("config %+v", o.cfg)
	}
	if o.log != "out.log" || !o.hex || !o.stamp || !o.echo || o.ending != "\r\n" {
		t.Fatalf("options %+v", o)
	}

	o, err = parseArgs([]string{"/dev/ttyACM0", `AT\r`, "--expect", "OK|ERROR", "--timeout", "500ms", "--cr"})
	if err != nil {
		t.Fatal(err)
	}
	if len(o.args) != 2 || o.args[1] != `AT\r` || o.expect.String() != "OK|ERROR" ||
		o.timeout != 500*time.Millisecond || o.ending != "\r" || o.cfg.Baud != 115200 {
		t.Fatalf("options %+v", o)
	}
	if o, _ = parseArgs([]string{"p", "--timeout=1.5"}); o.timeout != 1500*time.Millisecond {
		t.Fatal("timeout", o.timeout)
	}
	// After "--" a data that starts with a dash is data
	if o, _ = parseArgs([]string{"p", "--", "-1"}); len(o.args) != 2 || o.args[1] != "-1" {
		t.Fatal("args", o.args)
	}

	for _, bad := range [][]string{{"p", "-b"}, {"p", "-b", "fast"}, {"p", "-f", "9X1"}, {"p", "--flow", "dsr"},
		{"p", "--expect", "("}, {"p", "--timeout", "soon"}, {"p", "--what"}, {"p", "--hex=1"}} {
		if _, err := parseArgs(bad); err == nil {
			t.Error("accepted:", bad)
		}
	}
}

func TestRunUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	e := env{stdin: strings.NewReader(""), stdout: &out, stderr: &errOut}
	if code := run([]string{"version"}, e); code != 0 || !strings.Contains(out.String(), "AltSerialPort") {
		t.Fatal("version:", code, out.String())
	}
	if code := run([]string{"help"}, e); code != 0 || !strings.Contains(out.String(), "Usage") {
		t.Fatal("help:", code)
	}
	if code := run([]string{"send", "COM5"}, e); code != exitUsage {
		t.Fatal("send without data:", code)
	}
	if code := run([]string{"--bogus"}, e); code != exitUsage {
		t.Fatal("unknown command:", code)
	}
}

func TestOutput(t *testing.T) {
	var b bytes.Buffer
	o := &output{w: &b, hex: true, atStart: true}
	o.write([]byte{0, 1, 0xab})
	o.finish()
	if b.String() != "00 01 AB \r\n" {
		t.Fatalf("%q", b.String())
	}
	b.Reset()
	o = &output{w: &b, stamp: true, atStart: true}
	o.write([]byte("a\nb"))
	lines := strings.Split(b.String(), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[0], " a") || !strings.HasSuffix(lines[1], " b") || len(lines[1]) != len("15:04:05.000 b") {
		t.Fatalf("%q", b.String())
	}
}
