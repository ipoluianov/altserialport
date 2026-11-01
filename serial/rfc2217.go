package serial

import (
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"time"
)

// Telnet commands and options
const (
	telSE   = 240
	telSB   = 250
	telWILL = 251
	telWONT = 252
	telDO   = 253
	telDONT = 254
	telIAC  = 255

	optBinary  = 0
	optSGA     = 3
	optComPort = 44 // COM-PORT-OPTION of RFC 2217
)

// The commands of COM-PORT-OPTION sent by the client; the server answers
// and notifies with the same numbers plus 100
const (
	cpSetBaudrate       = 1
	cpSetDatasize       = 2
	cpSetParity         = 3
	cpSetStopsize       = 4
	cpSetControl        = 5
	cpNotifyModemstate  = 7
	cpSetModemstateMask = 11
	cpServer            = 100
)

// The values of SET-CONTROL
const (
	ctlFlowNone     = 1
	ctlFlowXonXoff  = 2
	ctlFlowHardware = 3
	ctlBreakOn      = 5
	ctlBreakOff     = 6
	ctlDTROn        = 8
	ctlDTROff       = 9
	ctlRTSOn        = 11
	ctlRTSOff       = 12
)

// The bits of the modem state
const (
	modemCTS = 0x10
	modemDSR = 0x20
	modemRI  = 0x40
	modemDCD = 0x80
)

// agreeTimeout is how long Open waits for the server to take
// COM-PORT-OPTION; a server that doesn't answer gets the settings anyway
const agreeTimeout = time.Second

// The states of the telnet parser
const (
	stData = iota
	stIAC
	stOption
	stSB
	stSBIAC
)

// rfc2217Port is a serial port of a Telnet COM port server (ser2net, a
// device server). A goroutine reads the connection: it takes the telnet
// commands out of the data and answers them.
type rfc2217Port struct {
	connState // mu also guards comPort, modem, dtr, rts
	conn      net.Conn
	data      chan []byte   // the received bytes; closed when the connection ends
	done      chan struct{} // closed by Close
	agreed    chan struct{} // closed when the server answered about COM-PORT-OPTION
	agreeOnce sync.Once
	comPort   bool // the server takes COM-PORT-OPTION

	modem    byte
	dtr, rts bool

	rmu  sync.Mutex // one Read at a time
	rest []byte     // received bytes not read yet, under rmu
	wmu  sync.Mutex // one write to the connection at a time

	// the parser, used by the reading goroutine only
	state int
	cmd   byte
	sb    []byte
}

func newRFC2217(conn net.Conn, c Config) (Port, error) {
	p := &rfc2217Port{
		conn:   conn,
		data:   make(chan []byte, 4),
		done:   make(chan struct{}),
		agreed: make(chan struct{}),
		dtr:    true,
		rts:    true,
	}
	// The options are asked for before any answer to the server goes out
	err := p.send([]byte{
		telIAC, telWILL, optBinary, telIAC, telDO, optBinary,
		telIAC, telWILL, optSGA, telIAC, telDO, optSGA,
		telIAC, telWILL, optComPort,
	})
	if err != nil {
		conn.Close()
		return nil, ErrGone
	}
	go p.reader()
	select {
	case <-p.agreed:
		p.mu.Lock()
		ok := p.comPort
		p.mu.Unlock()
		if !ok {
			p.Close()
			return nil, errors.New("the server doesn't support RFC 2217 (COM-PORT-OPTION)")
		}
	case <-time.After(agreeTimeout):
	}
	// Every change of the lines is notified
	if err := p.command(cpSetModemstateMask, 0xFF); err != nil {
		p.Close()
		return nil, err
	}
	if err := p.SetConfig(c); err != nil {
		p.Close()
		return nil, err
	}
	return p, nil
}

func (p *rfc2217Port) reader() {
	defer close(p.data)
	buf := make([]byte, 16*1024)
	for {
		n, err := p.conn.Read(buf)
		if n > 0 {
			if data := p.parse(buf[:n]); len(data) > 0 {
				select {
				case p.data <- data:
				case <-p.done:
					return
				}
			}
		}
		if err != nil {
			return
		}
	}
}

// parse returns the data of the received bytes and handles the telnet
// commands among them; a command may be split between two reads
func (p *rfc2217Port) parse(in []byte) []byte {
	out := make([]byte, 0, len(in))
	for _, b := range in {
		switch p.state {
		case stData:
			if b == telIAC {
				p.state = stIAC
			} else {
				out = append(out, b)
			}
		case stIAC:
			switch b {
			case telIAC:
				out = append(out, telIAC)
				p.state = stData
			case telWILL, telWONT, telDO, telDONT:
				p.cmd = b
				p.state = stOption
			case telSB:
				p.sb = p.sb[:0]
				p.state = stSB
			default:
				// NOP, GA and the like mean nothing here
				p.state = stData
			}
		case stOption:
			p.option(p.cmd, b)
			p.state = stData
		case stSB:
			if b == telIAC {
				p.state = stSBIAC
			} else if len(p.sb) < 256 {
				p.sb = append(p.sb, b)
			}
		case stSBIAC:
			if b == telIAC {
				if len(p.sb) < 256 {
					p.sb = append(p.sb, telIAC)
				}
				p.state = stSB
				continue
			}
			// IAC SE, or a broken subnegotiation ended by another command
			p.subnegotiation(p.sb)
			p.state = stData
		}
	}
	return out
}

// option answers a negotiation. The options asked for at the start are
// agreed by the answers; any other is refused.
func (p *rfc2217Port) option(cmd, opt byte) {
	switch cmd {
	case telDO:
		switch opt {
		case optBinary, optSGA:
		case optComPort:
			p.agree(true)
		default:
			go p.send([]byte{telIAC, telWONT, opt})
		}
	case telDONT:
		if opt == optComPort {
			p.agree(false)
		}
	case telWILL:
		switch opt {
		case optBinary, optSGA:
		default:
			go p.send([]byte{telIAC, telDONT, opt})
		}
	}
}

func (p *rfc2217Port) agree(ok bool) {
	p.agreeOnce.Do(func() {
		p.mu.Lock()
		p.comPort = ok
		p.mu.Unlock()
		close(p.agreed)
	})
}

// subnegotiation keeps the modem state the server notifies; the answers
// to the settings are not checked
func (p *rfc2217Port) subnegotiation(sb []byte) {
	if len(sb) < 3 || sb[0] != optComPort || sb[1] != cpServer+cpNotifyModemstate {
		return
	}
	p.mu.Lock()
	p.modem = sb[2]
	p.mu.Unlock()
}

// send writes bytes as they are, the commands included
func (p *rfc2217Port) send(b []byte) error {
	p.wmu.Lock()
	defer p.wmu.Unlock()
	_, err := p.conn.Write(b)
	return err
}

// command sends a COM-PORT-OPTION command; IAC in its value is doubled
func (p *rfc2217Port) command(cmd byte, value ...byte) error {
	b := []byte{telIAC, telSB, optComPort, cmd}
	b = appendEscaped(b, value)
	b = append(b, telIAC, telSE)
	if err := p.send(b); err != nil {
		return p.connError()
	}
	return nil
}

func appendEscaped(dst, src []byte) []byte {
	for _, c := range src {
		if c == telIAC {
			dst = append(dst, telIAC)
		}
		dst = append(dst, c)
	}
	return dst
}

func (p *rfc2217Port) Read(b []byte) (int, error) {
	p.rmu.Lock()
	defer p.rmu.Unlock()
	if p.isClosed() {
		return 0, ErrClosed
	}
	for len(p.rest) == 0 {
		select {
		case <-p.done:
			return 0, ErrClosed
		case d, ok := <-p.data:
			if !ok {
				return 0, p.connError()
			}
			p.rest = d
		}
	}
	n := copy(b, p.rest)
	p.rest = p.rest[n:]
	return n, nil
}

func (p *rfc2217Port) Write(b []byte) (int, error) {
	if p.isClosed() {
		return 0, ErrClosed
	}
	if err := p.send(appendEscaped(make([]byte, 0, len(b)+8), b)); err != nil {
		return 0, p.connError()
	}
	return len(b), nil
}

func (p *rfc2217Port) Close() error {
	if !p.setClosed() {
		return nil
	}
	close(p.done)
	return p.conn.Close()
}

func (p *rfc2217Port) SetConfig(c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.RS485 {
		return errors.New("RS-485 mode is not available on a network port")
	}
	if p.isClosed() {
		return ErrClosed
	}
	var baud [4]byte
	binary.BigEndian.PutUint32(baud[:], uint32(c.Baud))
	parity := byte(1)
	switch c.Parity {
	case ParityOdd:
		parity = 2
	case ParityEven:
		parity = 3
	case ParityMark:
		parity = 4
	case ParitySpace:
		parity = 5
	}
	stop := byte(1)
	switch c.StopBits {
	case StopBits2:
		stop = 2
	case StopBits1Half:
		stop = 3
	}
	flow := byte(ctlFlowNone)
	switch c.Flow {
	case FlowXonXoff:
		flow = ctlFlowXonXoff
	case FlowRTSCTS:
		flow = ctlFlowHardware
	}
	p.mu.Lock()
	dtr, rts := p.dtr, p.rts
	p.mu.Unlock()
	cmds := [][]byte{
		append([]byte{cpSetBaudrate}, baud[:]...),
		{cpSetDatasize, byte(c.DataBits)},
		{cpSetParity, parity},
		{cpSetStopsize, stop},
		{cpSetControl, flow},
		{cpSetControl, onOff(dtr, ctlDTROn, ctlDTROff)},
	}
	// With RTS/CTS the server drives RTS itself
	if c.Flow != FlowRTSCTS {
		cmds = append(cmds, []byte{cpSetControl, onOff(rts, ctlRTSOn, ctlRTSOff)})
	}
	for _, cmd := range cmds {
		if err := p.command(cmd[0], cmd[1:]...); err != nil {
			return err
		}
	}
	return nil
}

func onOff(on bool, yes, no byte) byte {
	if on {
		return yes
	}
	return no
}

func (p *rfc2217Port) SetDTR(on bool) error {
	if p.isClosed() {
		return ErrClosed
	}
	p.mu.Lock()
	p.dtr = on
	p.mu.Unlock()
	return p.command(cpSetControl, onOff(on, ctlDTROn, ctlDTROff))
}

func (p *rfc2217Port) SetRTS(on bool) error {
	if p.isClosed() {
		return ErrClosed
	}
	p.mu.Lock()
	p.rts = on
	p.mu.Unlock()
	return p.command(cpSetControl, onOff(on, ctlRTSOn, ctlRTSOff))
}

// Signals is the modem state the server has notified last
func (p *rfc2217Port) Signals() (Signals, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return Signals{}, ErrClosed
	}
	m := p.modem
	return Signals{
		CTS: m&modemCTS != 0,
		DSR: m&modemDSR != 0,
		RI:  m&modemRI != 0,
		DCD: m&modemDCD != 0,
	}, nil
}

func (p *rfc2217Port) Break(d time.Duration) error {
	if p.isClosed() {
		return ErrClosed
	}
	if err := p.command(cpSetControl, ctlBreakOn); err != nil {
		return err
	}
	time.Sleep(d)
	return p.command(cpSetControl, ctlBreakOff)
}
