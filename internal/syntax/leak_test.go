package syntax

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"
)

func rssMB(t *testing.T) int {
	t.Helper()
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Fields(line); len(f) > 1 && f[0] == "VmRSS:" {
			kb, _ := strconv.Atoi(f[1])
			return kb / 1024
		}
	}
	return 0
}

// The parser, tree, query and cursor are C memory, so a leak doesn't show in Go's own numbers.
func TestHighlightersDontLeak(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc")
	}
	var src strings.Builder
	for src.Len() < 1<<18-100 {
		fmt.Fprintf(&src, "func f%d(a int) string { return \"x\" + fmt.Sprint(a, 1.5) } // c\n", src.Len())
	}
	text := []byte(src.String())
	cycle := func() {
		h, err := NewHighlighter(ForPath("a.go"))
		if err != nil {
			t.Fatal(err)
		}
		h.Update(text)                         //nolint:errcheck // Checked in other tests.
		h.Update(append([]byte("x"), text...)) //nolint:errcheck // An incremental one too.
		h.Drop()
		h.Update(text) //nolint:errcheck // And a rebuilt one.
		h.Close()
		ReleaseMemory()
	}
	for range 3 {
		cycle()
	}
	debug.FreeOSMemory()
	before := rssMB(t)
	for range 15 {
		cycle()
	}
	debug.FreeOSMemory()
	after := rssMB(t)
	t.Logf("RSS %d MB before, %d MB after 15 more cycles", before, after)
	if after > before+15 {
		t.Fatalf("memory grew from %d MB to %d MB", before, after)
	}
}
