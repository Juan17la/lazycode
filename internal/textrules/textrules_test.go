package textrules

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func spans(t *testing.T, rules []Rule, src string) []string {
	t.Helper()
	s, err := Compile(rules)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	s.Find([]byte(src), func(a, b int, class string) { out = append(out, class+":"+src[a:b]) })
	return out
}

func TestLeftmostRuleWinsAndGroupsDontConfuseIt(t *testing.T) {
	got := spans(t, []Rule{
		{"comment", `//.*$`},
		{"string", `"(a|b|[^"])*"`}, // A group of its own.
		{"keyword", `\b(let|fn)\b`},
		{"comment", `(?s)/\*.*?\*/`},
	}, "let x = \"// no\" // yes\n/* two\nlines */ fn\n")
	want := "[keyword:let string:\"// no\" comment:// yes comment:/* two\nlines */ keyword:fn]"
	if fmt.Sprint(got) != want {
		t.Fatalf("got %v", got)
	}
}

func TestBadRulesAreRefused(t *testing.T) {
	for _, rules := range [][]Rule{nil, {{"pink", "a"}}, {{"string", "("}}, {{"string", ""}}, {{"string", strings.Repeat("a", maxPattern+1)}}, make([]Rule, maxRules+1)} {
		if _, err := Compile(rules); err == nil {
			t.Errorf("accepted %v", len(rules))
		}
	}
}

func TestEmptyMatchesColourNothing(t *testing.T) {
	if got := spans(t, []Rule{{"string", `x*`}}, "ab"); len(got) != 0 {
		t.Fatal(got)
	}
}

// A pattern that takes exponential time in a backtracking engine.
func TestPathologicalPatternStaysFast(t *testing.T) {
	start := time.Now()
	spans(t, []Rule{{"string", `(a+)+$`}}, strings.Repeat("a", 1<<20)+"!")
	if d := time.Since(start); d > 30*time.Second {
		t.Fatalf("took %v", d)
	}
}
