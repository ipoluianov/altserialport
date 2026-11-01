package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/ipoluianov/altserialport/buffer"
	"github.com/ipoluianov/altserialport/proto"
	"github.com/ipoluianov/altserialport/session"
)

// send writes the data, then prints what comes until the expected pattern
// matches or the time is out
func send(o options, e env) int {
	if len(o.args) != 2 {
		return usageError(e, errors.New("send needs a port and the data"))
	}
	name, text := o.args[0], o.args[1]
	var data []byte
	if o.hex {
		var err error
		if data, err = proto.ParseHex(text); err != nil {
			return usageError(e, err)
		}
	} else {
		data = proto.ParseEscapes(text)
	}
	data = append(data, o.ending...)

	var log *session.Logger
	if o.log != "" {
		var err error
		if log, err = session.NewLogger(o.log, session.LogText, true); err != nil {
			fmt.Fprintln(e.stderr, "altserialport:", err)
			return exitFail
		}
		defer log.Close()
	}

	p, err := open(name, o.cfg, e)
	if err != nil {
		fmt.Fprintln(e.stderr, "altserialport:", err)
		return exitFail
	}
	defer p.Close()

	type result struct {
		data []byte
		err  error
	}
	rx := make(chan result, 64)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := p.Read(buf)
			rx <- result{append([]byte(nil), buf[:n]...), err}
			if err != nil {
				return
			}
		}
	}()

	if len(data) > 0 {
		if log != nil {
			log.Write(buffer.Chunk{Time: time.Now().UnixNano(), Dir: buffer.TX, Data: data})
		}
		if _, err := p.Write(data); err != nil {
			fmt.Fprintln(e.stderr, "altserialport: write:", err)
			return exitFail
		}
	}

	timer := time.NewTimer(o.timeout)
	defer timer.Stop()
	var got []byte
	for {
		select {
		case <-timer.C:
			if o.expect != nil {
				fmt.Fprintf(e.stderr, "altserialport: timeout: no %q in %v\n", o.expect.String(), o.timeout)
				return exitFail
			}
			return exitOK
		case r := <-rx:
			if len(r.data) > 0 {
				if log != nil {
					log.Write(buffer.Chunk{Time: time.Now().UnixNano(), Dir: buffer.RX, Data: r.data})
				}
				e.stdout.Write(r.data)
				got = append(got, r.data...)
				if o.expect != nil && o.expect.Match(got) {
					return exitOK
				}
			}
			if r.err != nil {
				fmt.Fprintln(e.stderr, "altserialport: read:", r.err)
				return exitFail
			}
		}
	}
}
