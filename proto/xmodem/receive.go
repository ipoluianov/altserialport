package xmodem

import (
	"bytes"
	"context"
	"strconv"
	"strings"
)

type receiver struct {
	ctx    context.Context
	link   Link
	p      Protocol
	crc    bool // blocks come with a CRC
	locked bool // a good block came, so the kind of check is known
	hello  bool // waiting for the first block of a run: ask with 'C' or NAK
	asks   int  // 'C' or NAK sent while waiting for a first block
	errs   int  // timeouts and bad blocks in a row
}

// block is what came from the sender: a block or EOT
type block struct {
	eot  bool
	num  byte
	data []byte
}

// Receive receives one file. The name is known only with YModem. XModem
// can't tell the padding from the data, so trailing 0x1A bytes are cut;
// YModem cuts the file to the size from block 0. When the sender of YModem
// has more files, the first one is kept and the rest are cancelled.
func Receive(ctx context.Context, link Link, p Protocol, progress Progress) (name string, data []byte, err error) {
	r := &receiver{ctx: ctx, link: link, p: p, crc: true}
	defer func() {
		if err != nil && err != ErrCancelled && err != ErrNoFile {
			abort(link)
		}
	}()
	size := -1
	if p == YModem {
		if err := r.start(); err != nil {
			return "", nil, err
		}
		for {
			b, err := r.get()
			if err != nil {
				return "", nil, err
			}
			if b.eot || b.num != 0 {
				// not a header: ask again
				if err := r.again(); err != nil {
					return "", nil, err
				}
				continue
			}
			name, size = parseHeader(b.data)
			if err := r.send(ack); err != nil {
				return "", nil, err
			}
			if name == "" {
				return "", nil, ErrNoFile
			}
			break
		}
	}

	if progress != nil {
		progress(0, size)
	}
	if err := r.start(); err != nil {
		return "", nil, err
	}
	nakedEOT := false
	expect := byte(1)
data:
	for {
		b, err := r.get()
		if err != nil {
			return "", nil, err
		}
		if b.eot {
			if p == YModem && !nakedEOT {
				// YModem asks for EOT twice, to be sure it is not noise
				nakedEOT = true
				r.hello = false
				if err := r.send(nak); err != nil {
					return "", nil, err
				}
				continue
			}
			if err := r.send(ack); err != nil {
				return "", nil, err
			}
			break data
		}
		switch b.num {
		case expect:
			data = append(data, b.data...)
			expect++
			r.hello = false
			if size >= 0 && len(data) > size {
				data = data[:size]
			}
			if progress != nil {
				progress(len(data), size)
			}
			err = r.send(ack)
		case expect - 1:
			// the sender missed our ACK and sent the block again
			err = r.send(ack)
			if err == nil && r.hello {
				err = r.send(r.ask())
			}
		default:
			err = r.again()
		}
		if err != nil {
			return "", nil, err
		}
	}

	if size >= 0 {
		data = data[:min(size, len(data))]
	} else {
		data = bytes.TrimRight(data, "\x1a")
	}
	if p != YModem {
		return name, data, nil
	}

	// the batch ends with an empty block 0
	if err := r.start(); err != nil {
		return "", nil, err
	}
	for {
		b, err := r.get()
		if err != nil {
			return "", nil, err
		}
		switch {
		case b.eot:
			err = r.send(ack) // the sender missed our ACK of EOT
			if err == nil {
				err = r.send(r.ask())
			}
		case b.num == 0:
			if err := r.send(ack); err != nil {
				return "", nil, err
			}
			if n, _ := parseHeader(b.data); n != "" {
				abort(link) // one file is all we take
			}
			return name, data, nil
		default:
			err = r.again()
		}
		if err != nil {
			return "", nil, err
		}
	}
}

// parseHeader reads block 0 of YModem; size is -1 when it is not there
func parseHeader(data []byte) (name string, size int) {
	i := bytes.IndexByte(data, 0)
	if i < 0 {
		return "", -1
	}
	name = string(data[:i])
	rest := data[i+1:]
	if j := bytes.IndexByte(rest, 0); j >= 0 {
		rest = rest[:j]
	}
	size = -1
	if f := strings.Fields(string(rest)); len(f) > 0 {
		if n, err := strconv.Atoi(f[0]); err == nil && n >= 0 {
			size = n
		}
	}
	return name, size
}

func (r *receiver) send(b byte) error {
	return r.link.Write([]byte{b})
}

// ask is what asks for the first block: 'C' for a CRC; a receiver of
// XModem falls back to NAK, a checksum, when 'C' gets no answer
func (r *receiver) ask() byte {
	if r.p == XModem && !r.locked && r.asks >= crcStarts {
		r.crc = false
	}
	if r.crc {
		return crcReq
	}
	return nak
}

// start asks for the first block of a run
func (r *receiver) start() error {
	r.hello = true
	r.asks++
	return r.send(r.ask())
}

// bad counts a bad block, drops what is left of it and asks again
func (r *receiver) bad() error {
	if err := purge(r.ctx, r.link); err != nil {
		return err
	}
	return r.again()
}

// again counts an error and asks for the block again
func (r *receiver) again() error {
	r.errs++
	limit := maxRetries
	if r.hello {
		limit = maxStarts
	}
	if r.errs > limit {
		return ErrTooManyErrors
	}
	if r.hello {
		r.asks++
		return r.send(r.ask())
	}
	return r.send(nak)
}

// get waits for a good block or EOT, asking again on timeouts and bad
// blocks
func (r *receiver) get() (block, error) {
	for {
		wait := timing.block
		if r.hello {
			wait = timing.startTry
		}
		c, err := readByte(r.ctx, r.link, wait)
		if err == ErrTimeout {
			if err := r.again(); err != nil {
				return block{}, err
			}
			continue
		}
		if err != nil {
			return block{}, err
		}
		switch c {
		case soh, stx:
			b, ok, err := r.readBlock(c)
			if err != nil {
				return block{}, err
			}
			if !ok {
				if err := r.bad(); err != nil {
					return block{}, err
				}
				continue
			}
			r.errs = 0
			return b, nil
		case eot:
			r.errs = 0
			return block{eot: true}, nil
		case can:
			if c, err := cancelled(r.ctx, r.link); c || err != nil {
				return block{}, orCancelled(err)
			}
		}
	}
}

// readBlock reads the rest of a block after SOH or STX. Until a good block
// came, it takes a block with either kind of check, as the sender may
// have heard an older ask.
func (r *receiver) readBlock(first byte) (block, bool, error) {
	n := 128
	if first == stx {
		n = 1024
	}
	buf := make([]byte, n+3)
	for i := range buf {
		b, err := readByte(r.ctx, r.link, timing.char)
		if err == ErrTimeout {
			return block{}, false, nil
		}
		if err != nil {
			return block{}, false, err
		}
		buf[i] = b
	}
	num, data, c1 := buf[0], buf[2:n+2], buf[n+2]
	if buf[1] != ^num {
		return block{}, false, nil
	}
	good := false
	if r.crc {
		c2, err := readByte(r.ctx, r.link, timing.char)
		switch {
		case err == nil:
			good = crc16(data) == uint16(c1)<<8|uint16(c2)
		case err == ErrTimeout:
			if !r.locked && sum8(data) == c1 {
				r.crc, good = false, true
			}
		default:
			return block{}, false, err
		}
	} else {
		good = sum8(data) == c1
		if !good && !r.locked {
			c2, err := readByte(r.ctx, r.link, timing.char)
			if err != nil && err != ErrTimeout {
				return block{}, false, err
			}
			if err == nil && crc16(data) == uint16(c1)<<8|uint16(c2) {
				r.crc, good = true, true
			}
		}
	}
	if good {
		r.locked = true
	}
	return block{num: num, data: data}, good, nil
}
