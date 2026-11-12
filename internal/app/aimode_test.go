package app

import (
	"strings"
	"testing"
	"time"

	"github.com/Juan17la/lazycode/internal/project"
	"github.com/Juan17la/lazycode/internal/session"
	"github.com/gdamore/tcell/v2"
)

func TestAIModeTogglesAndComesBack(t *testing.T) {
	u := withTool(t, "stay", map[string]string{"a.txt": "x"})
	u.startFake()

	u.alt('a') // From inside the tool, into AI mode, still in the tool.
	u.wait("AI sessions")
	u.sendLine("hello")
	u.wait("got: hello")

	u.alt('a') // Out to the editor, with the tool back in the dock.
	u.wait("Editor")
	u.waitGone("AI sessions")

	u.alt('a') // And back, into the same tool, ready to type.
	u.wait("AI sessions")
	u.sendLine("again")
	u.wait("got: again")

	u.key(tcell.KeyCtrlBackslash) // Out to the tree, and Ctrl+Space back in.
	u.key(tcell.KeyCtrlSpace)
	u.sendLine("again")
	u.wait("got: again")
}

func TestAIModeWithNoSessions(t *testing.T) {
	u := withTool(t, "stay", nil)
	u.alt('a')
	u.wait("No sessions yet. n starts one")
	u.press("n")
	u.wait("Start a session")
	u.press("Fake")
	u.key(tcell.KeyEnter)
	u.wait("tool args: --session-id")
	u.wait("AI sessions") // Still in AI mode, showing the new one.
}

func TestAITreeGroupsByProjectAndFolder(t *testing.T) {
	a, first, _ := sessionApp(t)
	dir := a.projects[0].Dirs[0]
	a.projects = append(a.projects, project.Project{Name: "web", Dirs: []string{"/w/front", "/w/api"}})
	add := func(name, dir string, age time.Duration) *sessionEntry {
		e := &sessionEntry{Session: session.Session{Name: name, Dir: dir, Created: time.Now().Add(-age)}}
		a.sessions.entries = append(a.sessions.entries, e)
		return e
	}
	first.Created = time.Now().Add(-time.Hour)
	add("newer", dir, time.Minute)
	add("front", "/w/front/src", time.Minute)
	add("api", "/w/api", time.Minute)
	add("lost", "/elsewhere", time.Minute)
	add("gone", "/elsewhere", 0).Hidden = true

	var got []string
	for _, r := range a.aiRows() {
		got = append(got, strings.Repeat(" ", r.depth)+r.label)
	}
	want := []string{"p", " newer", " Fake 1", "web", " front", "  front", " api", "  api", "Other folders", " lost"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("tree:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	a.fold("p:web", true)
	if rows := a.aiRows(); len(rows) != 6 {
		t.Fatalf("folding web should leave 6 rows, got %d", len(rows))
	}
	a.selectEntry(a.sessions.entries[2]) // front, folded away: it unfolds.
	if row, _ := a.selected(a.aiRows()); row.label != "front" || row.e == nil {
		t.Fatalf("selected %+v", row)
	}
}
