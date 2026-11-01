package xmodem

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type sender struct {
	ctx  context.Context
	link Link
	crc  bool // the receiver asked for a CRC
}

// Send sends data as one file. The name matters only to YModem; its path
// is cut off. On a cancel of ctx it sends CAN CAN CAN and returns ctx.Err().
func Send(ctx context.Context, link Link, p Protocol, name string, data []byte, progress Progress) (err error) {
	s := &sender{ctx: ctx, link: link}
	defer func() {
		if err != nil && err != ErrCancelled {
			abort(link)
		}
	}()
	if err := s.waitStart(timing.start); err != nil {
		return err
	}
	if p == YModem {
		if err := s.sendBlock(0, header(name, len(data))); err != nil {
			return err
		}
		if err := s.waitStart(timing.reply); err != nil {
			return err
		}
	}
	size := 128
	if p != XModem {
		size = 1024
	}
	if progress != nil {
		progress(0, len(data))
	}
	num := byte(1)
	for off := 0; off < len(data); num++ {
		n := size
		if len(data)-off <= 128 {
			n = 128 // a short tail goes in a short block
		}
		blk := make([]byte, n)
		k := copy(blk, data[off:])
		for i := k; i < n; i++ {
			blk[i] = sub
		}
		if err := s.sendBlock(num, blk); err != nil {
			return err
		}
		off += k
		if progress != nil {
			progress(off, len(data))
		}
	}
	if err := s.sendEOT(); err != nil {
		return err
	}
	if p == YModem {
		// an empty block 0 ends the batch
		if err := s.waitStart(timing.reply); err != nil {
			return err
		}
		return s.sendBlock(0, make([]byte, 128))
	}
	return nil
}

// header is block 0 of YModem: "name\x00size mtime mode", the numbers of
// time and mode in octal
func header(name string, size int) []byte {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	if len(name) > 900 {
		name = name[:900]
	}
	h := fmt.Appendf(nil, "%s\x00%d %o %o", name, size, time.Now().Unix(), 0100644)
	n := 128
	if len(h) >= n {
		n = 1024
	}
	blk := make([]byte, n)
	copy(blk, h)
	return blk
}

// waitStart waits for the receiver to ask for blocks: 'C' for a CRC, NAK
// for a checksum. When several are waiting, the last one counts.
func (s *sender) waitStart(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	got := false
	for {
		wait := time.Until(deadline)
		if got {
			wait = 0 // only what is already here
		} else if wait <= 0 {
			return ErrTimeout
		}
		b, err := readByte(s.ctx, s.link, wait)
		if err == ErrTimeout && got {
			return nil
		}
		if err != nil {
			return err
		}
		switch b {
		case crcReq:
			s.crc, got = true, true
		case nak:
			s.crc, got = false, true
		case can:
			if c, err := cancelled(s.ctx, s.link); c || err != nil {
				return orCancelled(err)
			}
		}
	}
}

// reply waits for ACK or NAK; ErrTimeout when neither came
func (s *sender) reply() (byte, error) {
	deadline := time.Now().Add(timing.reply)
	for {
		left := time.Until(deadline)
		if left <= 0 {
			return 0, ErrTimeout
		}
		b, err := readByte(s.ctx, s.link, left)
		if err != nil {
			return 0, err
		}
		switch b {
		case ack, nak:
			return b, nil
		case can:
			if c, err := cancelled(s.ctx, s.link); c || err != nil {
				return 0, orCancelled(err)
			}
		}
	}
}

// sendBlock sends a block until the receiver takes it
func (s *sender) sendBlock(num byte, data []byte) error {
	frame := make([]byte, 0, len(data)+5)
	if len(data) == 1024 {
		frame = append(frame, stx)
	} else {
		frame = append(frame, soh)
	}
	frame = append(frame, num, ^num)
	frame = append(frame, data...)
	if s.crc {
		c := crc16(data)
		frame = append(frame, byte(c>>8), byte(c))
	} else {
		frame = append(frame, sum8(data))
	}
	return s.retry(frame)
}

// sendEOT ends the file; a receiver of YModem NAKs the first EOT
func (s *sender) sendEOT() error {
	return s.retry([]byte{eot})
}

func (s *sender) retry(frame []byte) error {
	for range maxRetries {
		if err := s.drain(); err != nil {
			return err
		}
		if err := s.link.Write(frame); err != nil {
			return err
		}
		r, err := s.reply()
		if err == ErrTimeout {
			continue
		}
		if err != nil {
			return err
		}
		if r == ack {
			return nil
		}
	}
	return ErrTooManyErrors
}

// drain drops the replies already waiting, so an old ACK is not taken for
// the reply to this block. CAN CAN still cancels.
func (s *sender) drain() error {
	for {
		b, err := readByte(s.ctx, s.link, 0)
		if err == ErrTimeout {
			return nil
		}
		if err != nil {
			return err
		}
		if b == can {
			if c, err := cancelled(s.ctx, s.link); c || err != nil {
				return orCancelled(err)
			}
		}
	}
}

// orCancelled is err, or ErrCancelled when there is none
func orCancelled(err error) error {
	if err != nil {
		return err
	}
	return ErrCancelled
}
