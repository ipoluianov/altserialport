package rules

import (
	"testing"

	"github.com/ipoluianov/altserialport/proto"
)

func TestMatch(t *testing.T) {
	list := []Rule{
		{Enabled: true, Pattern: "error", Color: "#f00"},
		{Enabled: true, Pattern: `temp=(\d+)`, Regex: true, Beep: true},
		{Enabled: true, Pattern: "0D 0A", Hex: true, Answer: "ok"},
		{Enabled: false, Pattern: "x", Color: "#0f0"},
		{Enabled: true, Pattern: "(", Regex: true},
	}
	s, errs := Compile(list)
	if len(errs) != 1 || errs[4] == nil {
		t.Fatal(errs)
	}
	var got []int
	s.Match("ERROR temp=12", []byte("ERROR temp=12\r\n"), func(i int, _ Rule) { got = append(got, i) })
	if len(got) != 3 || got[0] != 0 || got[1] != 1 || got[2] != 2 {
		t.Fatal(got)
	}
	if s.Color("an Error", nil) != "#f00" || s.Color("fine", nil) != "" {
		t.Fatal("color")
	}
	if !s.HasTriggers() {
		t.Fatal("triggers")
	}
	ll, errs := Compile(LogLevels())
	if len(errs) != 0 || ll.Color("E (1530) sensor: CRC", nil) == "" {
		t.Fatal(errs)
	}
}

func TestLines(t *testing.T) {
	var l proto.Lines
	var got []string
	for _, part := range []string{"a\r", "\nb\nc", "\r\r\nd"} {
		l.Feed([]byte(part), func(line []byte) { got = append(got, string(line)) })
	}
	if len(got) != 4 || got[0] != "a" || got[1] != "b" || got[2] != "c" || got[3] != "" {
		t.Fatalf("%q", got)
	}
}
