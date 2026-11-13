// Package textrules colours code with regular expressions, for language packs (doc 09). A pack is
// text that a reviewer can read, unlike a compiled grammar. It's less exact than a parser, since it
// doesn't understand nesting. Go's regexp runs in time linear in the input, so a badly written
// pattern can't hang on a file the way a backtracking engine can.
package textrules

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Classes are the colours a rule can ask for.
var Classes = []string{"keyword", "string", "number", "constant", "function", "type", "variable", "property", "operator", "tag", "comment"}

const (
	maxRules   = 64
	maxPattern = 1000
)

// Rule colours whatever Pattern matches.
type Rule struct {
	Class   string `toml:"class"`
	Pattern string `toml:"pattern"`
}

// Set is compiled rules. It's safe to share between goroutines.
type Set struct {
	re      *regexp.Regexp
	classes []string // Of each rule.
	groups  []int    // The capture group of each rule.
}

// Compile checks rules and joins them into one expression, so that one pass finds the leftmost
// match of any rule: // inside a string stays part of the string. At the same place, the rule
// listed first wins. ^ and $ match at each line.
func Compile(rules []Rule) (*Set, error) {
	if len(rules) > maxRules {
		return nil, fmt.Errorf("%d rules is more than the %d allowed", len(rules), maxRules)
	}
	if len(rules) == 0 {
		return nil, errors.New("there are no rules")
	}
	s := &Set{}
	parts := make([]string, 0, len(rules))
	group := 1
	for i, r := range rules {
		if !slices.Contains(Classes, r.Class) {
			return nil, fmt.Errorf("rule %d: unknown class %q, which must be one of %s", i+1, r.Class, strings.Join(Classes, ", "))
		}
		if r.Pattern == "" || len(r.Pattern) > maxPattern {
			return nil, fmt.Errorf("rule %d: a pattern has 1 to %d characters", i+1, maxPattern)
		}
		one, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, fmt.Errorf("rule %d: %w", i+1, err)
		}
		s.classes, s.groups = append(s.classes, r.Class), append(s.groups, group)
		parts = append(parts, "("+r.Pattern+")")
		group += 1 + one.NumSubexp()
	}
	re, err := regexp.Compile("(?m)" + strings.Join(parts, "|"))
	if err != nil {
		return nil, err
	}
	s.re = re
	return s, nil
}

// Find calls found for each match, in order and without overlaps.
func (s *Set) Find(src []byte, found func(start, end int, class string)) {
	for _, m := range s.re.FindAllSubmatchIndex(src, -1) {
		if m[0] == m[1] {
			continue
		}
		for i, g := range s.groups {
			if m[2*g] >= 0 {
				found(m[0], m[1], s.classes[i])
				break
			}
		}
	}
}
