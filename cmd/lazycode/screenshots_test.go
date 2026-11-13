package main

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Juan17la/lazycode/internal/term"
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

// TestScreenshots draws the README's pictures. It runs the real binary on a small demo project
// and saves each screen as an SVG. It only runs when asked:
//
//	LAZYCODE_SCREENSHOTS=$PWD/docs/images go test -run TestScreenshots ./cmd/lazycode
//	for f in docs/images/*.svg; do rsvg-convert -z 1.5 -o "${f%.svg}.png" "$f" && rm "$f"; done
//
// The PNGs use JetBrains Mono when it's installed, as the README's pictures do.
func TestScreenshots(t *testing.T) {
	out := os.Getenv("LAZYCODE_SCREENSHOTS")
	if out == "" {
		t.Skip("set LAZYCODE_SCREENSHOTS to a folder to draw the README's pictures")
	}
	if runtime.GOOS == "windows" {
		t.Skip("the pictures are drawn on Linux and macOS")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "lazycode")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("SHELL", "/bin/sh")
	t.Setenv("PS1", "$ ")
	t.Setenv("ENV", "")

	// A project of two folders: a Go API with a Git history, and a web front end.
	shop := filepath.Join(root, "code")
	api, web := filepath.Join(shop, "shop-api"), filepath.Join(shop, "shop-web")
	writeFiles(t, api, demoAPI)
	writeFiles(t, web, demoWeb)
	gitEnv := []string{"GIT_AUTHOR_NAME=Ana", "GIT_AUTHOR_EMAIL=ana@example.com", "GIT_COMMITTER_NAME=Ana", "GIT_COMMITTER_EMAIL=ana@example.com"}
	when := time.Now().Add(-75 * time.Hour) // Each commit is a while after the one before.
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", api}, args...)...)
		date := when.Format(time.RFC3339)
		cmd.Env = append(os.Environ(), append(gitEnv, "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)...)
		if args[0] == "commit" {
			when = when.Add(26 * time.Hour)
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	git("add", "-A")
	git("commit", "-q", "-m", "First version of the orders API")
	writeFiles(t, api, map[string]string{"orders.go": demoAPI["orders.go"] + demoOrdersMore})
	git("commit", "-qam", "List orders by customer")
	writeFiles(t, api, map[string]string{"README.md": "# shop-api\n\nOrders for the shop.\n\nRun it with `go run .`\n"})
	git("add", "-A")
	git("commit", "-q", "-m", "Add a README")

	config := filepath.Join(root, "config", "lazycode")
	tool, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	writeFiles(t, config, map[string]string{
		"config.toml": fmt.Sprintf(`version = 1
[ui]
theme = "tokyo-night"
[updates]
check = false
[[harness]]
name = "Agent"
command = %q
`, tool),
		"projects.toml": fmt.Sprintf(`[[project]]
name = "shop"
[[project.directory]]
path = %q
[[project.directory]]
path = %q
`, api, web),
	})
	t.Setenv("LAZYCODE_E2E_TOOL", "demo") // The Agent above is this test binary, playing a tool.

	const width, height = 132, 36
	tab, err := term.Start(api, []string{bin}, width, height, func() {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tab.Close)
	d := &driver{t: t, tab: tab}
	shot := func(name string) {
		t.Helper()
		time.Sleep(700 * time.Millisecond) // Let highlighting and the last frame land.
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, name+".svg"), []byte(screenSVG(tab, width, height)), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s:\n%s", name, tab.Text())
	}

	d.wait("shop › shop-api")
	d.wait("orders.go")

	// 1. The editor: a file open, an AI session in the dock changing it, live.
	d.key(tcell.KeyCtrlP, tcell.ModCtrl)
	d.typeText("orders.go")
	d.enter()
	d.wait("func ListOrders")
	d.key(tcell.KeyEsc, 0)
	d.tab.Key(tcell.NewEventKey(tcell.KeyRune, 'A', tcell.ModAlt)) // Alt+Shift+A: new AI session.
	d.wait("Start a session")
	d.typeText("Agent")
	d.enter()
	d.wait("Done.")
	shot("editor")

	// 2. AI mode: every session, by project and folder.
	d.alt('a')
	d.wait("Agent")
	shot("ai-mode")
	d.alt('a')

	// 3. Git: what the agent changed, ready to stage and commit.
	d.key(tcell.KeyCtrlSpace, tcell.ModCtrl)
	time.Sleep(300 * time.Millisecond)
	d.alt('g')
	d.wait("History")
	d.enter() // The diff of the changed file.
	d.wait("countByCustomer")
	shot("git")

	// 4. Search across the folder.
	d.alt('f')
	d.typeText("Customer")
	d.wait("in text")
	shot("search")

	// 5. Every action, grouped by task, and each one can be given a new key.
	d.key(tcell.KeyF1, 0)
	d.wait("Shortcuts")
	shot("shortcuts")
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, text := range files {
		p := filepath.Join(dir, name)
		mkdirs(t, filepath.Dir(p))
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// demoAgent plays an AI coding tool in the pictures: it shows a short task, makes the change it
// describes, and waits.
func demoAgent() {
	const (
		dim, bold, green, cyan, reset = "\x1b[2m", "\x1b[1m", "\x1b[32m", "\x1b[36m", "\x1b[0m"
	)
	say := func(s string) { fmt.Print(s + "\r\n"); time.Sleep(60 * time.Millisecond) }
	say(cyan + "agent" + reset + dim + "  a pretend AI tool, for these pictures" + reset)
	say("")
	say(bold + "> Count each customer's orders in ListOrders" + reset)
	say("")
	say("  Reading orders.go, store.go")
	src, _ := os.ReadFile("orders.go")
	text := strings.Replace(string(src), "\tsortByPlaced(out)\n",
		"\tsortByPlaced(out)\n\tcounts := countByCustomer(out)\n\tlogCounts(counts)\n", 1)
	os.WriteFile("orders.go", []byte(text+demoCount), 0o644) //nolint:errcheck // A picture.
	say("  " + green + "✓" + reset + " Edited orders.go  " + green + "+11" + reset)
	say("  " + green + "✓" + reset + " go test ./...  " + green + "ok" + reset + "  0.42s")
	say("")
	say("  Done. ListOrders now counts orders per customer.")
	say("")
	fmt.Print(bold + "> " + reset)
	select {}
}

// screenSVG draws what the pseudo-terminal shows, cell by cell, with its colours.
func screenSVG(tab *term.Tab, width, height int) string {
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		panic(err)
	}
	defer s.Fini()
	s.SetSize(width, height)
	tab.Draw(s, 0, 0, width, height)
	s.Show()
	cells, _, _ := s.GetContents()

	const cw, ch, pad, bar = 8.4, 18.0, 14.0, 30.0
	bgDefault, fgDefault := "#1a1b26", "#c0caf5"
	hex := func(c tcell.Color, def string) string {
		if c == tcell.ColorDefault || !c.Valid() {
			return def
		}
		r, g, b := c.RGB()
		return fmt.Sprintf("#%02x%02x%02x", r, g, b)
	}
	w, h := float64(width)*cw+2*pad, float64(height)*ch+2*pad+bar
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" font-family="'JetBrains Mono',ui-monospace,SFMono-Regular,Menlo,Consolas,'DejaVu Sans Mono',monospace" font-size="14">`, w, h, w, h)
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" rx="10" fill="%s"/>`, bgDefault)
	for i, c := range []string{"#ff5f57", "#febc2e", "#28c840"} {
		fmt.Fprintf(&b, `<circle cx="%.0f" cy="%.0f" r="6" fill="%s"/>`, pad+6+float64(i)*20, bar/2+2, c)
	}
	fmt.Fprintf(&b, `<g transform="translate(%.0f %.0f)" xml:space="preserve">`, pad, pad+bar-6)

	// Backgrounds first, merged along each row.
	for y := range height {
		for x := 0; x < width; {
			_, bg, _ := cells[y*width+x].Style.Decompose()
			col := hex(bg, bgDefault)
			end := x + 1
			for end < width {
				_, nb, _ := cells[y*width+end].Style.Decompose()
				if hex(nb, bgDefault) != col {
					break
				}
				end++
			}
			if col != bgDefault {
				fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.0f" fill="%s"/>`, float64(x)*cw, float64(y)*ch, float64(end-x)*cw+0.5, ch, col)
			}
			x = end
		}
	}
	// Then text, in runs of one style. A wide character is drawn on its own, over two cells.
	for y := range height {
		for x := 0; x < width; {
			c := cells[y*width+x]
			fg, _, attr := c.Style.Decompose()
			r := ' '
			if len(c.Runes) > 0 {
				r = c.Runes[0]
			}
			if runewidth.RuneWidth(r) == 2 {
				textRun(&b, string(c.Runes), x, y, 2, hex(fg, fgDefault), attr, cw, ch)
				x += 2
				continue
			}
			var run strings.Builder
			end := x
			for end < width {
				n := cells[y*width+end]
				nf, _, na := n.Style.Decompose()
				nr := ' '
				if len(n.Runes) > 0 {
					nr = n.Runes[0]
				}
				if nf != fg || na != attr || runewidth.RuneWidth(nr) == 2 {
					break
				}
				if len(n.Runes) > 0 {
					run.WriteString(string(n.Runes))
				} else {
					run.WriteByte(' ')
				}
				end++
			}
			if strings.TrimSpace(run.String()) != "" {
				textRun(&b, run.String(), x, y, end-x, hex(fg, fgDefault), attr, cw, ch)
			}
			x = end
		}
	}
	b.WriteString("</g></svg>\n")
	return b.String()
}

func textRun(b *strings.Builder, text string, x, y, cells int, fill string, attr tcell.AttrMask, cw, ch float64) {
	extra := ""
	if attr&tcell.AttrBold != 0 {
		extra += ` font-weight="bold"`
	}
	if attr&tcell.AttrItalic != 0 {
		extra += ` font-style="italic"`
	}
	if attr&tcell.AttrDim != 0 {
		extra += ` opacity="0.6"`
	}
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" textLength="%.1f" lengthAdjust="spacingAndGlyphs" fill="%s"%s>%s</text>`,
		float64(x)*cw, float64(y)*ch+ch*0.75, float64(cells)*cw, fill, extra, html.EscapeString(text))
}

var demoAPI = map[string]string{
	"go.mod": "module example.com/shop-api\n\ngo 1.26\n",
	"main.go": `package main

import (
	"log"
	"net/http"
)

func main() {
	store := NewStore()
	http.HandleFunc("/orders", ordersHandler(store))
	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
`,
	"orders.go": `package main

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("order not found")

// ListOrders returns every order, newest first.
func ListOrders(s *Store) ([]Order, error) {
	out := s.All()
	sortByPlaced(out)
	return out, nil
}

// Order is one purchase in the shop.
type Order struct {
	ID       int
	Customer string
	Total    float64
	Placed   time.Time
}
`,
	"store.go": `package main

import "sync"

// Store keeps orders in memory.
type Store struct {
	mu     sync.Mutex
	orders []Order
}

func NewStore() *Store { return &Store{} }

func (s *Store) All() []Order {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Order(nil), s.orders...)
}
`,
	"orders_test.go": `package main

import "testing"

func TestListOrders(t *testing.T) {
	if _, err := ListOrders(NewStore()); err != nil {
		t.Fatal(err)
	}
}
`,
}

const demoOrdersMore = `
// ByCustomer returns one customer's orders.
func ByCustomer(s *Store, customer string) []Order {
	var out []Order
	for _, o := range s.All() {
		if o.Customer == customer {
			out = append(out, o)
		}
	}
	return out
}
`

const demoCount = `
// countByCustomer counts each customer's orders.
func countByCustomer(orders []Order) map[string]int {
	counts := map[string]int{}
	for _, o := range orders {
		counts[o.Customer]++
	}
	return counts
}
`

var demoWeb = map[string]string{
	"package.json": `{ "name": "shop-web", "scripts": { "dev": "vite" } }` + "\n",
	"src/api.ts": `export interface Order {
  id: number;
  customer: string;
  total: number;
}

export async function listOrders(): Promise<Order[]> {
  const res = await fetch("/orders");
  return res.json();
}
`,
	"src/OrdersPage.tsx": `import { listOrders } from "./api";

export async function OrdersPage() {
  const orders = await listOrders();
  return orders.map((o) => o.customer + ": " + o.total);
}
`,
}
