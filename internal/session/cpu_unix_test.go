//go:build unix

package session

import (
	"testing"
	"time"
)

func TestSumTreeCountsDescendantsOnly(t *testing.T) {
	table := `
    1     0 00:10:00
  100     1 00:00:05
  101   100 00:00:10
  102   101 01:00:00
  200     1 05:00:00
  300   200 1-00:00:01
  bad  line here
`
	if got, want := sumTree(table, 100), time.Hour+15*time.Second; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := sumTree(table, 999); got != 0 {
		t.Fatalf("an unknown process: %v", got)
	}
}

func TestParseCPU(t *testing.T) {
	for text, want := range map[string]time.Duration{
		"00:00":      0,
		"01:30":      90 * time.Second,
		"01:30.50":   90*time.Second + 500*time.Millisecond,
		"02:00:01":   2*time.Hour + time.Second,
		"1-00:00:01": 24*time.Hour + time.Second,
	} {
		if got := parseCPU(text); got != want {
			t.Errorf("%s: %v, want %v", text, got, want)
		}
	}
}
