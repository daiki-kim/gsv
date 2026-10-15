package tree

import (
	"errors"
	"strings"
	"testing"

	"gsv/internal/config"
	"gsv/internal/gdrive"
	"gsv/internal/ui/keymap"
)

func act(k keymap.Kind) keymap.Action { return keymap.Action{Kind: k, Count: 1} }

func folder(id, name string) gdrive.Item {
	return gdrive.Item{ID: id, Name: name, Kind: gdrive.KindFolder}
}
func sheet(id, name string) gdrive.Item {
	return gdrive.Item{ID: id, Name: name, Kind: gdrive.KindSpreadsheet}
}

func names(m *Model) []string {
	var out []string
	for _, n := range m.visible() {
		out = append(out, n.Name)
	}
	return out
}

func newTree() *Model {
	m := New([]config.SharedRoot{{Name: "営業部", ID: "0AbC"}})
	m.SetSize(30, 20)
	return m
}

func TestRoots(t *testing.T) {
	m := newTree()
	if got := strings.Join(names(m), ","); got != "My Drive,Shared with me,営業部" {
		t.Fatalf("roots = %s", got)
	}
	if r := m.Selected(); r.Root != MyDrive || r.ID != "root" {
		t.Fatalf("selected = %+v", r)
	}
}

func TestExpandLoadsLazilyOnce(t *testing.T) {
	m := newTree()
	e := m.Apply(act(keymap.Right))
	if e.Kind != LoadChildren || e.Node.Root != MyDrive || !e.Node.Loading {
		t.Fatalf("effect = %+v", e)
	}
	if !strings.Contains(m.View(true), "⟳") {
		t.Fatal("loading marker expected")
	}
	m.SetChildren(e.Node, []gdrive.Item{folder("F1", "仕事"), sheet("S1", "売上管理")}, nil)
	if got := strings.Join(names(m), ","); got != "My Drive,仕事,売上管理,Shared with me,営業部" {
		t.Fatalf("visible = %s", got)
	}
	m.Apply(act(keymap.Left))
	m.Apply(act(keymap.Right))
	if e := m.Apply(act(keymap.Open)); e.Kind != None {
		t.Fatalf("re-expanding a loaded folder must not reload: %+v", e)
	}
}

func TestOpenSheetAndParentNavigation(t *testing.T) {
	m := newTree()
	e := m.Apply(act(keymap.Open))
	m.SetChildren(e.Node, []gdrive.Item{folder("F1", "仕事"), sheet("S1", "売上管理")}, nil)
	m.Apply(keymap.Action{Kind: keymap.Down, Count: 2, HasCount: true})
	if e := m.Apply(act(keymap.Right)); e.Kind != OpenSheet || e.Node.ID != "S1" {
		t.Fatalf("effect = %+v", e)
	}
	m.Apply(act(keymap.Left))
	if m.Selected().Root != MyDrive {
		t.Fatalf("h on a leaf should go to parent, got %q", m.Selected().Name)
	}
	m.Apply(act(keymap.Left))
	if got := len(m.visible()); got != 3 {
		t.Fatalf("h on expanded folder should collapse, visible = %d", got)
	}
}

func TestLoadErrorThenRetry(t *testing.T) {
	m := newTree()
	e := m.Apply(act(keymap.Open))
	m.SetChildren(e.Node, nil, errors.New("offline"))
	if !strings.Contains(m.View(true), "!") || e.Node.Loading {
		t.Fatal("failed node should be marked")
	}
	if e2 := m.Apply(act(keymap.Reload)); e2.Kind != LoadChildren || e2.Node != e.Node || e.Node.Err != nil {
		t.Fatalf("r should retry: %+v", e2)
	}
}

func TestReloadOnSheetReloadsParent(t *testing.T) {
	m := newTree()
	e := m.Apply(act(keymap.Open))
	m.SetChildren(e.Node, []gdrive.Item{sheet("S1", "a")}, nil)
	m.Apply(act(keymap.Down))
	if e2 := m.Apply(act(keymap.Reload)); e2.Kind != LoadChildren || e2.Node.Root != MyDrive {
		t.Fatalf("effect = %+v", e2)
	}
}

func TestBeginSearchReplacesPreviousResults(t *testing.T) {
	m := newTree()
	e := m.BeginSearch("売上")
	if e.Kind != LoadChildren || e.Node.Root != SearchResults || e.Node.ID != "売上" || m.Selected() != e.Node {
		t.Fatalf("effect = %+v", e)
	}
	m.SetChildren(e.Node, []gdrive.Item{sheet("S1", "売上管理")}, nil)
	m.BeginSearch("予算")
	got := names(m)
	if got[0] != "Search: 予算" || strings.Contains(strings.Join(got, ","), "Search: 売上") {
		t.Fatalf("visible = %v", got)
	}
}

func TestSearchExpandsCollapsedLoadedNodes(t *testing.T) {
	m := newTree()
	e := m.Apply(act(keymap.Open))
	m.SetChildren(e.Node, []gdrive.Item{folder("F1", "仕事")}, nil)
	m.Apply(act(keymap.Down))
	e = m.Apply(act(keymap.Open))
	m.SetChildren(e.Node, []gdrive.Item{sheet("S1", "Budget 2025")}, nil)
	m.Apply(act(keymap.Left)) // collapse 仕事
	m.Apply(act(keymap.Top))
	if !m.Search("budget", true) || m.Selected().ID != "S1" {
		t.Fatalf("selected = %+v", m.Selected())
	}
	if m.Search("nothing", true) {
		t.Fatal("should not find")
	}
	if m.Apply(act(keymap.NextMatch)).Kind != NotFound {
		t.Fatal("n after failed search -> NotFound")
	}
}

func TestViewHeightAndCursor(t *testing.T) {
	m := newTree()
	m.SetSize(30, 4)
	v := m.View(true)
	lines := strings.Split(v, "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[0], "Drive") || !strings.Contains(lines[1], "▸ My Drive") {
		t.Fatalf("view = %q", v)
	}
	m.Apply(act(keymap.Bottom))
	lines = strings.Split(m.View(true), "\n")
	if !strings.Contains(lines[3], "営業部") {
		t.Fatalf("cursor must stay visible: %q", lines)
	}
}
