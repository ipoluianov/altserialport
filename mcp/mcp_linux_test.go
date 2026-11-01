package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ipoluianov/altserialport/buffer"
	"github.com/ipoluianov/altserialport/config"
	"github.com/ipoluianov/altserialport/remote"
	"github.com/ipoluianov/altserialport/serial"
	"github.com/ipoluianov/altserialport/session"
	"golang.org/x/sys/unix"
)

func openPTY(t *testing.T) (*os.File, string) {
	t.Helper()
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skip("no /dev/ptmx:", err)
	}
	fd := int(m.Fd())
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	return m, fmt.Sprintf("/dev/pts/%d", n)
}

// rpc is a client of Serve over pipes
type rpc struct {
	t    *testing.T
	in   *io.PipeWriter
	out  *bufio.Scanner
	id   int
	done chan error
}

func start(t *testing.T) *rpc {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := &rpc{t: t, in: inW, out: bufio.NewScanner(outR), done: make(chan error, 1)}
	c.out.Buffer(nil, 4<<20)
	go func() {
		err := Serve(inR, outW)
		outW.Close()
		c.done <- err
	}()
	t.Cleanup(func() {
		inW.Close()
		select {
		case err := <-c.done:
			if err != nil {
				t.Error("serve:", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("Serve did not return at the end of the input")
		}
	})
	return c
}

func (c *rpc) line(s string) {
	if _, err := io.WriteString(c.in, s+"\n"); err != nil {
		c.t.Fatal(err)
	}
}

// call sends a request and returns the response
func (c *rpc) call(method string, params any) map[string]any {
	c.t.Helper()
	c.id++
	req, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": c.id, "method": method, "params": params})
	go c.line(string(req))
	if !c.out.Scan() {
		c.t.Fatal("no response to", method)
	}
	var resp map[string]any
	if err := json.Unmarshal(c.out.Bytes(), &resp); err != nil {
		c.t.Fatalf("bad response %s: %v", c.out.Bytes(), err)
	}
	if id, _ := resp["id"].(float64); int(id) != c.id {
		c.t.Fatalf("response id %v, want %d", resp["id"], c.id)
	}
	return resp
}

// tool calls a tool and returns its text and isError
func (c *rpc) tool(name string, args map[string]any) (string, bool) {
	c.t.Helper()
	resp := c.call("tools/call", map[string]any{"name": name, "arguments": args})
	if resp["error"] != nil {
		c.t.Fatalf("%s: %v", name, resp["error"])
	}
	res := resp["result"].(map[string]any)
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	isErr, _ := res["isError"].(bool)
	return text, isErr
}

// okTool calls a tool that must succeed
func (c *rpc) okTool(name string, args map[string]any) string {
	c.t.Helper()
	text, isErr := c.tool(name, args)
	if isErr {
		c.t.Fatalf("%s failed: %s", name, text)
	}
	return text
}

func decode(t *testing.T, text string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		t.Fatalf("not JSON: %s", text)
	}
	return m
}

func readAll(t *testing.T, m *os.File, n int) string {
	t.Helper()
	got := make(chan string, 1)
	go func() {
		var all []byte
		buf := make([]byte, 256)
		for len(all) < n {
			k, err := m.Read(buf)
			all = append(all, buf[:k]...)
			if err != nil {
				break
			}
		}
		got <- string(all)
	}()
	select {
	case s := <-got:
		return s
	case <-time.After(3 * time.Second):
		t.Fatal("the device got nothing")
	}
	return ""
}

func TestProtocol(t *testing.T) {
	config.SetDir(t.TempDir())
	c := start(t)
	resp := c.call("initialize", map[string]any{"protocolVersion": "2025-03-26",
		"capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"}})
	res := resp["result"].(map[string]any)
	if res["protocolVersion"] != "2025-03-26" {
		t.Fatal("version", res["protocolVersion"])
	}
	if res["serverInfo"].(map[string]any)["name"] != "altserialport" {
		t.Fatal("server info", res["serverInfo"])
	}
	if _, ok := res["capabilities"].(map[string]any)["tools"]; !ok {
		t.Fatal("no tools capability")
	}
	resp = c.call("initialize", map[string]any{"protocolVersion": "1999-01-01"})
	if v := resp["result"].(map[string]any)["protocolVersion"]; v != latestVersion {
		t.Fatal("unknown version answered with", v)
	}
	// A notification gets no answer: the next line is the answer to ping
	c.line(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if resp := c.call("ping", nil); resp["error"] != nil {
		t.Fatal(resp["error"])
	}

	resp = c.call("tools/list", nil)
	list := resp["result"].(map[string]any)["tools"].([]any)
	names := map[string]bool{}
	for _, x := range list {
		tl := x.(map[string]any)
		names[tl["name"].(string)] = true
		if tl["description"] == "" || tl["inputSchema"].(map[string]any)["type"] != "object" {
			t.Fatal("bad tool", tl)
		}
	}
	for _, n := range []string{"list_ports", "open_port", "close_port", "write", "read", "wait_for",
		"set_lines", "get_signals", "send_break", "reset_board", "status"} {
		if !names[n] {
			t.Fatal("no tool", n)
		}
	}

	if e := c.call("no/such", nil)["error"].(map[string]any); e["code"].(float64) != codeNoMethod {
		t.Fatal(e)
	}
	if c.call("tools/call", map[string]any{"name": "nope"})["error"] == nil {
		t.Fatal("unknown tool accepted")
	}
	if _, isErr := c.tool("read", map[string]any{"port": "/dev/none"}); !isErr {
		t.Fatal("read of a port not open")
	}
	c.line(`{bad json`)
	if !c.out.Scan() || !strings.Contains(c.out.Text(), "-32700") {
		t.Fatal("no parse error:", c.out.Text())
	}
}

func TestPortTools(t *testing.T) {
	config.SetDir(t.TempDir())
	m, slave := openPTY(t)
	defer m.Close()
	c := start(t)
	c.call("initialize", map[string]any{"protocolVersion": "2025-06-18"})

	text := c.okTool("open_port", map[string]any{"port": slave, "baud": 9600})
	if !strings.Contains(text, "9600 8N1") {
		t.Fatal(text)
	}
	if _, isErr := c.tool("open_port", map[string]any{"port": "/dev/pts/none"}); !isErr {
		t.Fatal("a missing port was opened")
	}

	c.okTool("write", map[string]any{"port": slave, "data": `AT\x21`, "line_ending": "crlf"})
	if got := readAll(t, m, 5); got != "AT!\r\n" {
		t.Fatalf("device got %q", got)
	}
	c.okTool("write", map[string]any{"port": slave, "data": "01 FF", "hex": true})
	if got := readAll(t, m, 2); got != "\x01\xff" {
		t.Fatalf("device got %q", got)
	}

	// Data that came before the call counts; what follows the match stays
	m.Write([]byte("boot...\r\nOK\r\nrest"))
	r := decode(t, c.okTool("wait_for", map[string]any{"port": slave, "pattern": `OK\r\n`}))
	if r["text"] != "boot...\r\nOK\r\n" || r["matched"] != true {
		t.Fatalf("wait_for: %v", r)
	}
	r = decode(t, c.okTool("read", map[string]any{"port": slave, "timeout_ms": 1000}))
	if r["text"] != "rest" || r["hex"] != nil {
		t.Fatalf("read: %v", r)
	}

	// Nothing comes: an empty read, a wait that times out
	r = decode(t, c.okTool("read", map[string]any{"port": slave, "timeout_ms": 50}))
	if r["bytes"].(float64) != 0 {
		t.Fatalf("read: %v", r)
	}
	m.Write([]byte("partial"))
	text, isErr := c.tool("wait_for", map[string]any{"port": slave, "pattern": "never", "timeout_ms": 200})
	if !isErr || !strings.Contains(text, "partial") {
		t.Fatalf("wait_for timeout: %v %s", isErr, text)
	}
	// The data of a timed out wait stays for read
	r = decode(t, c.okTool("read", map[string]any{"port": slave}))
	if r["text"] != "partial" {
		t.Fatalf("read after timeout: %v", r)
	}

	// Binary data comes with its hex
	m.Write([]byte{0x00, 0x41, 0xfe})
	r = decode(t, c.okTool("read", map[string]any{"port": slave, "timeout_ms": 1000}))
	if r["hex"] != "00 41 FE" {
		t.Fatalf("binary read: %v", r)
	}

	if _, isErr := c.tool("wait_for", map[string]any{"port": slave, "pattern": "("}); !isErr {
		t.Fatal("a bad pattern was taken")
	}
	st := decode(t, c.okTool("status", nil))
	ports := st["open_ports"].([]any)
	if len(ports) != 1 || ports[0].(map[string]any)["state"] != "open" {
		t.Fatalf("status: %v", st)
	}
	c.okTool("send_break", map[string]any{"port": slave, "ms": 10})

	c.okTool("close_port", map[string]any{"port": slave})
	if _, isErr := c.tool("write", map[string]any{"port": slave, "data": "x"}); !isErr {
		t.Fatal("write to a closed port")
	}
}

func TestAttachToWindow(t *testing.T) {
	config.SetDir(t.TempDir())
	m, slave := openPTY(t)
	defer m.Close()
	link := filepath.Join(t.TempDir(), "ttyBOARD")
	os.Symlink(slave, link)

	sess := session.New()
	if err := sess.Open(serial.PortInfo{Name: slave, Short: filepath.Base(slave)}, serial.DefaultConfig(), true, true); err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	stop, err := remote.Serve(sess)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	c := start(t)
	text := c.okTool("open_port", map[string]any{"port": link})
	if !strings.Contains(text, "attached") {
		t.Fatal(text)
	}
	sess.Drain()
	c.okTool("write", map[string]any{"port": link, "data": "hi", "line_ending": "lf"})
	if got := readAll(t, m, 3); got != "hi\n" {
		t.Fatalf("device got %q", got)
	}
	seen := false
	for _, ch := range sess.Drain() {
		if ch.Dir == buffer.TX && string(ch.Data) == "hi\n" {
			seen = true
		}
	}
	if !seen {
		t.Fatal("the window did not see the write")
	}
	m.Write([]byte("> "))
	r := decode(t, c.okTool("wait_for", map[string]any{"port": link, "pattern": "> $"}))
	if r["text"] != "> " {
		t.Fatalf("wait_for: %v", r)
	}
	st := decode(t, c.okTool("status", nil))
	if st["open_ports"].([]any)[0].(map[string]any)["attached_window"] != true {
		t.Fatalf("status: %v", st)
	}
}
