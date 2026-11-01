package proto

import "hash/crc32"

// Checksum is added after the data of a command, before the line ending
type Checksum uint8

const (
	SumNone   Checksum = iota
	SumModbus          // CRC-16/MODBUS, low byte first
	SumCCITT           // CRC-16/CCITT-FALSE, high byte first
	SumXModem          // CRC-16/XMODEM, high byte first
	SumCRC8            // CRC-8/SMBUS (poly 0x07)
	SumCRC32           // CRC-32 (IEEE), low byte first
	SumXOR             // XOR of the bytes (NMEA, many simple protocols)
	SumAdd8            // sum of the bytes, modulo 256
	SumLRC             // LRC of Modbus ASCII: the two's complement of the sum
)

// Checksums are the checksums offered, with their names
var Checksums = []struct {
	Sum  Checksum
	Name string
}{
	{SumNone, "None"},
	{SumModbus, "CRC-16 Modbus"},
	{SumCCITT, "CRC-16 CCITT-FALSE"},
	{SumXModem, "CRC-16 XMODEM"},
	{SumCRC8, "CRC-8"},
	{SumCRC32, "CRC-32"},
	{SumXOR, "XOR"},
	{SumAdd8, "Sum 8"},
	{SumLRC, "LRC"},
}

// Name returns the name of the checksum
func (c Checksum) Name() string {
	for _, x := range Checksums {
		if x.Sum == c {
			return x.Name
		}
	}
	return "None"
}

// Append adds the checksum of data to its end
func (c Checksum) Append(data []byte) []byte {
	switch c {
	case SumModbus:
		v := crc16(data, 0xA001, 0xFFFF, true)
		return append(data, byte(v), byte(v>>8))
	case SumCCITT:
		v := crc16(data, 0x1021, 0xFFFF, false)
		return append(data, byte(v>>8), byte(v))
	case SumXModem:
		v := crc16(data, 0x1021, 0x0000, false)
		return append(data, byte(v>>8), byte(v))
	case SumCRC8:
		var crc byte
		for _, b := range data {
			crc ^= b
			for i := 0; i < 8; i++ {
				if crc&0x80 != 0 {
					crc = crc<<1 ^ 0x07
				} else {
					crc <<= 1
				}
			}
		}
		return append(data, crc)
	case SumCRC32:
		v := crc32.ChecksumIEEE(data)
		return append(data, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
	case SumXOR:
		var x byte
		for _, b := range data {
			x ^= b
		}
		return append(data, x)
	case SumAdd8:
		var s byte
		for _, b := range data {
			s += b
		}
		return append(data, s)
	case SumLRC:
		var s byte
		for _, b := range data {
			s += b
		}
		return append(data, -s)
	}
	return data
}

// crc16 computes a CRC-16; reflected is the bit order of Modbus (the
// polynomial given reversed)
func crc16(data []byte, poly, init uint16, reflected bool) uint16 {
	crc := init
	for _, b := range data {
		if reflected {
			crc ^= uint16(b)
			for i := 0; i < 8; i++ {
				if crc&1 != 0 {
					crc = crc>>1 ^ poly
				} else {
					crc >>= 1
				}
			}
			continue
		}
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ poly
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
