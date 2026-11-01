// Package plot finds the numbers in the lines of a device for the live
// chart, with no setup: "23.4", "23.4 41 1013", "t=23.4 h=41",
// "temp:23.4,hum:41" (the format of the Arduino plotter), "1.5;2.5"
package plot

import (
	"strconv"
	"strings"
)

// Value is a number of a line and the name of its series
type Value struct {
	Name string
	V    float64
}

func isSep(r rune) bool {
	return r == ' ' || r == '\t' || r == ',' || r == ';' || r == '|'
}

// Parse returns the values of the line. A line of bare numbers gives the
// series "1", "2"...; a line with name=value or name:value pairs gives
// the pairs and ignores the rest, so a log line with a reading in it
// ("I (1234) sensor: t=23.4") gives only t. Other lines give nothing.
func Parse(line string) []Value {
	fields := strings.FieldsFunc(line, isSep)
	if len(fields) == 0 {
		return nil
	}
	var named, bare []Value
	allNumbers := true
	for k := 0; k < len(fields); k++ {
		f := fields[k]
		if i := strings.IndexAny(f, "=:"); i > 0 {
			name, val := f[:i], f[i+1:]
			// "temp: 23.4": the value is the next field
			if val == "" && k+1 < len(fields) {
				if v, ok := number(fields[k+1]); ok && validName(name) {
					named = append(named, Value{name, v})
					k++
					continue
				}
			}
			if v, ok := number(val); ok && validName(name) {
				named = append(named, Value{name, v})
				continue
			}
			allNumbers = false
			continue
		}
		if v, ok := number(f); ok {
			bare = append(bare, Value{strconv.Itoa(len(bare) + 1), v})
			continue
		}
		allNumbers = false
	}
	if len(named) > 0 {
		return named
	}
	if allNumbers {
		return bare
	}
	return nil
}

// number reads a number with an optional unit stuck to it: "23.4C", "12ms"
func number(s string) (float64, bool) {
	end := len(s)
	for end > 0 && !(s[end-1] >= '0' && s[end-1] <= '9' || s[end-1] == '.') {
		end--
	}
	if end == 0 || len(s)-end > 4 {
		return 0, false
	}
	v, err := strconv.ParseFloat(s[:end], 64)
	return v, err == nil
}

func validName(s string) bool {
	if len(s) > 32 {
		return false
	}
	for _, r := range s {
		if !(r == '_' || r == '-' || r == '.' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}
