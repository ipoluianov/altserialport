// Package remote lets another process of the program use the port a window
// has open: the command line and the MCP server attach to the window
// instead of failing with a busy port, and the window shows what they send
// and receive.
//
// The window listens on a loopback TCP port and writes a registry file
// with the address and a random token to config.Dir()/instances. A client
// finds the window by the port name, connects, authenticates with the
// token and gets a serial.Port that works through the session of the
// window.
//
// The protocol is frames of a type byte, a 4-byte big-endian length and
// the payload. The client sends a hello with the token first; the server
// answers with the port and its settings, then sends every received chunk
// as a data frame. Each request of the client gets one reply frame.
package remote

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Frame types
const (
	// client → server
	fHello   byte = 'H' // the token
	fWrite   byte = 'W' // the bytes to send
	fDTR     byte = 'T' // 1 byte: 0 or 1
	fRTS     byte = 'S' // 1 byte: 0 or 1
	fSignals byte = 'I' // no payload; the reply has 1 byte of flags
	fBreak   byte = 'B' // 4 bytes: milliseconds

	// server → client
	fWelcome byte = 'O' // JSON of the port: the answer to a good hello
	fReply   byte = 'R' // a status byte, then the result or the message
	fData    byte = 'D' // received bytes
	fGone    byte = 'G' // the session of the window is not open any more
)

// Reply statuses
const (
	rOK    byte = 0
	rGone  byte = 1 // the port of the window is not open
	rError byte = 2 // the message follows
)

// Bits of the signals in the reply to fSignals
const (
	sigCTS = 1 << iota
	sigDSR
	sigDCD
	sigRI
)

// maxFrame is the largest payload taken; bigger writes are split
const maxFrame = 1 << 20

func writeFrame(w io.Writer, typ byte, payload []byte) error {
	if len(payload) > maxFrame {
		return errors.New("frame too large")
	}
	buf := make([]byte, 5+len(payload))
	buf[0] = typ
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(payload)))
	copy(buf[5:], payload)
	_, err := w.Write(buf)
	return err
}

func readFrame(r io.Reader) (byte, []byte, error) {
	var head [5]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(head[1:])
	if n > maxFrame {
		return 0, nil, fmt.Errorf("frame of %d bytes is too large", n)
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return head[0], payload, nil
}
