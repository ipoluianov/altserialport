package proto

import (
	"bytes"
	"testing"
)

func TestParseEscapes(t *testing.T) {
	cases := map[string]string{
		`AT\r\n`:       "AT\r\n",
		`\x01\x7F\xff`: "\x01\x7f\xff",
		`a\\b`:         `a\b`,
		`\e[0m`:        "\x1b[0m",
		`C:\dir\path`:  `C:\dir\path`,
		`\u00e9`:       "é",
		`end\`:         `end\`,
		`\x4`:          `\x4`,
		`tab\there\0`:  "tab\there\x00",
	}
	for in, want := range cases {
		if got := string(ParseEscapes(in)); got != want {
			t.Errorf("%s: got %q, want %q", in, got, want)
		}
	}
}

func TestParseHex(t *testing.T) {
	ok := map[string][]byte{
		"01 03 00 0A":     {1, 3, 0, 10},
		"0x01,0x03, 0xff": {1, 3, 255},
		"01030A":          {1, 3, 10},
		"de:ad-BE;ef":     {0xde, 0xad, 0xbe, 0xef},
		"1 2 a":           {1, 2, 10},
		"":                nil,
	}
	for in, want := range ok {
		got, err := ParseHex(in)
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%q: got %v %v, want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"123", "zz", "0x"} {
		if _, err := ParseHex(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
}

func TestChecksums(t *testing.T) {
	modbus := SumModbus.Append([]byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x0A})
	if FormatHex(modbus[6:]) != "C5 CD" {
		t.Fatal("modbus", FormatHex(modbus))
	}
	check := []byte("123456789") // the check values of the catalog of CRCs
	want := map[Checksum]string{
		SumCCITT:  "29 B1",
		SumXModem: "31 C3",
		SumCRC8:   "F4",
		SumCRC32:  "26 39 F4 CB",
		SumModbus: "37 4B",
	}
	for sum, w := range want {
		got := sum.Append(append([]byte(nil), check...))
		if FormatHex(got[len(check):]) != w {
			t.Errorf("%s: %s, want %s", sum.Name(), FormatHex(got[len(check):]), w)
		}
	}
	if got := SumXOR.Append([]byte("GPGLL,5300.97914,N,00259.98174,E,125926,A")); got[len(got)-1] != 0x28 {
		t.Errorf("xor %02X", got[len(got)-1])
	}
	if got := SumLRC.Append([]byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x0A}); got[len(got)-1] != 0xF2 {
		t.Errorf("lrc %02X", got[len(got)-1])
	}
}

func TestFormatCArray(t *testing.T) {
	if got := FormatCArray([]byte{1, 2}); got != "{\n    0x01, 0x02\n}" {
		t.Fatalf("%q", got)
	}
}
