// Package xmodem sends and receives a file over a serial link: XMODEM with
// a checksum or CRC, XMODEM-1K and YMODEM
package xmodem

import (
	"context"
	"errors"
	"time"
)

// Link is the port as the protocol sees it
type Link interface {
	Write(p []byte) error
	// ReadByteTimeout waits for a byte up to the timeout; ErrTimeout when
	// none came
	ReadByteTimeout(timeout time.Duration) (byte, error)
}

var (
	ErrTimeout       = errors.New("xmodem: timeout")
	ErrCancelled     = errors.New("xmodem: cancelled by the other side")
	ErrTooManyErrors = errors.New("xmodem: too many errors")
	ErrNoFile        = errors.New("xmodem: the sender has no file")
)

// Protocol is the kind of the transfer
type Protocol int

const (
	// XModem sends 128-byte blocks with a checksum or a CRC, as the
	// receiver asks: NAK or 'C'
	XModem Protocol = iota
	// XModem1K sends 1024-byte blocks with a CRC
	XModem1K
	// YModem sends the name and the size in block 0 before the data, in
	// 1024-byte blocks, and ends with an empty block 0
	YModem
)

// Protocols are the protocols offered, with their names
var Protocols = []struct {
	Protocol Protocol
	Name     string
}{{XModem, "XMODEM"}, {XModem1K, "XMODEM-1K"}, {YModem, "YMODEM"}}

// String returns the name of the protocol
func (p Protocol) String() string {
	for _, x := range Protocols {
		if x.Protocol == p {
			return x.Name
		}
	}
	return "XMODEM"
}

// Progress tells how many bytes of the file are done. total is -1 when the
// size is not known (a receiver of XMODEM).
type Progress func(done, total int)

const (
	soh    = 0x01 // a 128-byte block
	stx    = 0x02 // a 1024-byte block
	eot    = 0x04 // the end of the file
	ack    = 0x06
	nak    = 0x15
	can    = 0x18 // two in a row cancel the transfer
	sub    = 0x1A // pads the last block
	crcReq = 'C'  // the receiver asks for blocks with a CRC

	maxRetries = 10 // tries of a block, and bad blocks in a row
	maxStarts  = 20 // the receiver asks for the first block this many times
	crcStarts  = 3  // a receiver of XMODEM asks with 'C' before it falls back to NAK
)

// timing holds the waits; the tests make them shorter
var timing = struct {
	start    time.Duration // the sender waits for the first NAK or 'C'
	startTry time.Duration // the receiver asks again for the first block
	// the sender waits for ACK or NAK; longer than block, so the NAK of
	// the receiver comes first and the two don't both send again
	reply time.Duration
	block time.Duration // the receiver waits for the next block
	char  time.Duration // between the bytes of a block
	purge time.Duration // a quiet gap that ends the purge after a bad block
	poll  time.Duration // how often a wait looks at the context
}{
	start:    60 * time.Second,
	startTry: 3 * time.Second,
	reply:    15 * time.Second,
	block:    10 * time.Second,
	char:     time.Second,
	purge:    300 * time.Millisecond,
	poll:     50 * time.Millisecond,
}

// readByte waits for a byte up to the timeout, looking at ctx often. It
// asks the link at least once, so a zero timeout takes what is buffered.
func readByte(ctx context.Context, link Link, timeout time.Duration) (byte, error) {
	deadline := time.Now().Add(timeout)
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		left := max(min(time.Until(deadline), timing.poll), 0)
		b, err := link.ReadByteTimeout(left)
		if err != ErrTimeout || time.Until(deadline) <= 0 {
			return b, err
		}
	}
}

// purge reads and drops bytes until the line is quiet
func purge(ctx context.Context, link Link) error {
	for {
		_, err := readByte(ctx, link, timing.purge)
		if err == ErrTimeout {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// cancelled tells whether a second CAN follows the one just read
func cancelled(ctx context.Context, link Link) (bool, error) {
	b, err := readByte(ctx, link, timing.char)
	if err == ErrTimeout {
		return false, nil
	}
	return b == can, err
}

// abort tells the other side to stop
func abort(link Link) {
	_ = link.Write([]byte{can, can, can})
}

// crc16 is CRC-16/XMODEM: poly 0x1021, start 0
func crc16(data []byte) uint16 {
	var crc uint16
	for _, b := range data {
		crc ^= uint16(b) << 8
		for range 8 {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// sum8 is the checksum of the original XMODEM: the sum of the bytes
func sum8(data []byte) byte {
	var s byte
	for _, b := range data {
		s += b
	}
	return s
}
