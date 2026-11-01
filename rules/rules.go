// Package rules finds the lines the user cares about: a rule is a text, a
// regular expression or hex bytes, and what to do when a line has it —
// color the line, beep, show a notification, answer the device, pause the
// screen, stop the log
package rules

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/ipoluianov/altserialport/proto"
)

// Rule is a pattern and its actions
type Rule struct {
	Enabled bool   `json:"enabled"`
	Pattern string `json:"pattern"`
	Regex   bool   `json:"regex,omitempty"`
	Hex     bool   `json:"hex,omitempty"`  // the pattern is bytes: "0D 0A"
	Case    bool   `json:"case,omitempty"` // case-sensitive
	Color   string `json:"color,omitempty"`
	Beep    bool   `json:"beep,omitempty"`
	Notify  bool   `json:"notify,omitempty"`
	Pause   bool   `json:"pause,omitempty"`
	StopLog bool   `json:"stop_log,omitempty"`
	// Answer is sent to the device, with escapes (\r\n), when a line matches
	Answer string `json:"answer,omitempty"`
}

// Triggers tells whether the rule does more than coloring
func (r Rule) Triggers() bool {
	return r.Beep || r.Notify || r.Pause || r.StopLog || r.Answer != ""
}

// LogLevels are the rules of the usual log levels
func LogLevels() []Rule {
	return []Rule{
		{Enabled: true, Pattern: `\b(ERROR|ERR|FATAL|PANIC|E \(\d+\))`, Regex: true, Case: true, Color: "#f85149"},
		{Enabled: true, Pattern: `\b(WARN|WARNING|W \(\d+\))`, Regex: true, Case: true, Color: "#d29922"},
		{Enabled: true, Pattern: `\b(DEBUG|DBG|D \(\d+\))`, Regex: true, Case: true, Color: "#8b949e"},
		{Enabled: true, Pattern: `Guru Meditation|Backtrace:|HardFault|assert failed|Kernel panic`, Regex: true, Case: true, Color: "#ff7b72", Notify: true},
	}
}

type compiled struct {
	index int
	rule  Rule
	re    *regexp.Regexp
	text  string // lower case unless Case
	hex   []byte
}

// Set is the enabled rules, ready to match
type Set struct {
	rules []compiled
}

// Compile prepares the enabled rules; the broken ones are left out and
// returned as errors by their index
func Compile(list []Rule) (*Set, map[int]error) {
	s := &Set{}
	errs := map[int]error{}
	for i, r := range list {
		if !r.Enabled || r.Pattern == "" {
			continue
		}
		c := compiled{index: i, rule: r}
		switch {
		case r.Hex:
			b, err := proto.ParseHex(r.Pattern)
			if err != nil || len(b) == 0 {
				errs[i] = err
				continue
			}
			c.hex = b
		case r.Regex:
			p := r.Pattern
			if !r.Case {
				p = "(?i)" + p
			}
			re, err := regexp.Compile(p)
			if err != nil {
				errs[i] = err
				continue
			}
			c.re = re
		default:
			c.text = r.Pattern
			if !r.Case {
				c.text = strings.ToLower(c.text)
			}
		}
		s.rules = append(s.rules, c)
	}
	return s, errs
}

// Empty tells whether there is nothing to match
func (s *Set) Empty() bool { return s == nil || len(s.rules) == 0 }

// Match calls f with each rule that the line matches, by its index in the
// list given to Compile; text is the line as shown, raw its bytes
func (s *Set) Match(text string, raw []byte, f func(index int, r Rule)) {
	if s.Empty() {
		return
	}
	var lower string
	for _, c := range s.rules {
		ok := false
		switch {
		case c.hex != nil:
			ok = bytes.Contains(raw, c.hex)
		case c.re != nil:
			ok = c.re.MatchString(text)
		case c.rule.Case:
			ok = strings.Contains(text, c.text)
		default:
			if lower == "" {
				lower = strings.ToLower(text)
			}
			ok = strings.Contains(lower, c.text)
		}
		if ok {
			f(c.index, c.rule)
		}
	}
}

// Color returns the color of the first coloring rule the line matches
func (s *Set) Color(text string, raw []byte) string {
	color := ""
	s.Match(text, raw, func(_ int, r Rule) {
		if color == "" && r.Color != "" {
			color = r.Color
		}
	})
	return color
}

// HasTriggers tells whether any rule does more than coloring
func (s *Set) HasTriggers() bool {
	if s == nil {
		return false
	}
	for _, c := range s.rules {
		if c.rule.Triggers() {
			return true
		}
	}
	return false
}
