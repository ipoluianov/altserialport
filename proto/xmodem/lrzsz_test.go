package xmodem

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// tool runs a program of lrzsz in a new folder and links to its stdin and
// stdout; the test is skipped when the program is not installed
func tool(t *testing.T, ctx context.Context, name string, args ...string) (*ChanLink, func(), string) {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s is not installed", name)
	}
	dir := t.TempDir()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = dir
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	link := NewChanLink(func(p []byte) error {
		_, err := stdin.Write(p)
		return err
	})
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			link.Feed(buf[:n])
			if err != nil {
				return
			}
		}
	}()
	wait := func() {
		stdin.Close()
		<-done
		if err := cmd.Wait(); err != nil {
			t.Errorf("%s: %v\n%s", name, err, stderr.String())
		}
	}
	return link, wait, dir
}

func TestLrzszSend(t *testing.T) {
	cases := []struct {
		p    Protocol
		tool string
		args []string
	}{
		{XModem, "rx", []string{"-b", "out.bin"}},
		{XModem1K, "rx", []string{"-b", "out.bin"}},
		{YModem, "rb", []string{"-b"}},
	}
	for _, c := range cases {
		for _, n := range []int{1, 129, 5000} {
			t.Run(fmt.Sprintf("%v/%d", c.p, n), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
				defer cancel()
				link, wait, dir := tool(t, ctx, c.tool, c.args...)
				data := testData(n)
				err := Send(ctx, link, c.p, "out.bin", data, nil)
				wait()
				if err != nil {
					t.Fatalf("send: %v", err)
				}
				got, err := os.ReadFile(filepath.Join(dir, "out.bin"))
				if err != nil {
					t.Fatal(err)
				}
				if c.p != YModem {
					got = bytes.TrimRight(got, "\x1a") // rx keeps the padding
				}
				if !bytes.Equal(got, data) {
					t.Fatalf("got %d bytes, want %d", len(got), len(data))
				}
			})
		}
	}
}

func TestLrzszReceive(t *testing.T) {
	cases := []struct {
		p    Protocol
		tool string
		args []string
	}{
		{XModem, "sx", []string{"-b", "in.bin"}},
		{XModem1K, "sx", []string{"-k", "-b", "in.bin"}},
		{YModem, "sb", []string{"-k", "-b", "in.bin"}},
	}
	for _, c := range cases {
		for _, n := range []int{1, 129, 5000} {
			t.Run(fmt.Sprintf("%v/%d", c.p, n), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
				defer cancel()
				if _, err := exec.LookPath(c.tool); err != nil {
					t.Skipf("%s is not installed", c.tool)
				}
				data := testData(n)
				// the file must be there before the program starts
				src := t.TempDir()
				file := filepath.Join(src, "in.bin")
				if err := os.WriteFile(file, data, 0o644); err != nil {
					t.Fatal(err)
				}
				args := append(c.args[:len(c.args)-1:len(c.args)-1], file)
				link, wait, _ := tool(t, ctx, c.tool, args...)
				name, got, err := Receive(ctx, link, c.p, nil)
				wait()
				if err != nil {
					t.Fatalf("receive: %v", err)
				}
				if c.p == YModem && name != "in.bin" {
					t.Fatalf("name %q", name)
				}
				if !bytes.Equal(got, data) {
					t.Fatalf("got %d bytes, want %d", len(got), len(data))
				}
			})
		}
	}
}
