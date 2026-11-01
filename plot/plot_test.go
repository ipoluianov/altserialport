package plot

import (
	"fmt"
	"testing"
)

func TestParse(t *testing.T) {
	cases := map[string]string{
		"23.4":                        "[{1 23.4}]",
		"23.4 41 -5":                  "[{1 23.4} {2 41} {3 -5}]",
		"1.5,2.5;3":                   "[{1 1.5} {2 2.5} {3 3}]",
		"t=23.4 h=41":                 "[{t 23.4} {h 41}]",
		"temp:23.4,hum:41":            "[{temp 23.4} {hum 41}]",
		"temp: 23.4 hum: 41":          "[{temp 23.4} {hum 41}]",
		"I (1234) sensor: t=23.4C":    "[{t 23.4}]",
		"I (1234) wifi: connected":    "[]",
		"OK":                          "[]",
		"":                            "[]",
		"rssi=-54 channel=6 ssid=lab": "[{rssi -54} {channel 6}]",
		"x=1e3":                       "[{x 1000}]",
	}
	for in, want := range cases {
		got := fmt.Sprint(Parse(in))
		if got == "[]" && Parse(in) == nil {
			got = "[]"
		}
		if got != want {
			t.Errorf("%q: %s, want %s", in, got, want)
		}
	}
}
