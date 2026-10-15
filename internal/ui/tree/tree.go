// Package tree holds the left-pane Drive tree.
package tree

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"gsv/internal/config"
	"gsv/internal/gdrive"
	"gsv/internal/ui/keymap"
	"gsv/internal/ui/text"
)

type RootKind int

const (
	NotRoot RootKind = iota
	MyDrive
	SharedWithMe
	Custom
	SearchResults
)

// Node is a tree entry. ID and Root never change after creation, so loaders may read them concurrently.
type Node struct {
	ID       string // Drive file ID; "root" for My Drive; the query for search results
	Name     string
	Kind     gdrive.Kind
	Root     RootKind
	Children []*Node
	Expanded bool
	Loaded   bool
	Loading  bool
	Err      error

	parent *Node
	depth  int
}

func (n *Node) IsSheet() bool { return n.Kind == gdrive.KindSpreadsheet }

type EffectKind int

const (
	None         EffectKind = iota
	LoadChildren            // fetch Node's children
	OpenSheet               // open Node in the grid
	NotFound                // a repeated search found nothing
)

type Effect struct {
	Kind EffectKind
	Node *Node
}

var (
	cursorFocused = lipgloss.NewStyle().Reverse(true)
	cursorBlurred = lipgloss.NewStyle().Bold(true)
)

type Model struct {
	roots         []*Node
	cursor, top   int
	width, height int
	query         string
	forward       bool
}

func New(shared []config.SharedRoot) *Model {
	m := &Model{roots: []*Node{
		{ID: "root", Name: "My Drive", Kind: gdrive.KindFolder, Root: MyDrive},
		{Name: "Shared with me", Kind: gdrive.KindFolder, Root: SharedWithMe},
	}}
	for _, s := range shared {
		m.roots = append(m.roots, &Node{ID: s.ID, Name: s.Name, Kind: gdrive.KindFolder, Root: Custom})
	}
	return m
}

func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
	m.clamp()
}

func (m *Model) visible() []*Node {
	var out []*Node
	var walk func(ns []*Node)
	walk = func(ns []*Node) {
		for _, n := range ns {
			out = append(out, n)
			if n.Expanded {
				walk(n.Children)
			}
		}
	}
	walk(m.roots)
	return out
}

func (m *Model) Selected() *Node {
	vis := m.visible()
	if m.cursor < 0 || m.cursor >= len(vis) {
		return nil
	}
	return vis[m.cursor]
}

func (m *Model) indexOf(n *Node) int {
	for i, v := range m.visible() {
		if v == n {
			return i
		}
	}
	return m.cursor
}

func (m *Model) bodyHeight() int { return max(m.height-1, 1) }

func (m *Model) Apply(a keymap.Action) Effect {
	sel := m.Selected()
	switch a.Kind {
	case keymap.Down:
		m.cursor += a.Count
	case keymap.Up:
		m.cursor -= a.Count
	case keymap.Top:
		m.cursor = 0
		if a.HasCount {
			m.cursor = a.Count - 1
		}
	case keymap.Bottom:
		m.cursor = len(m.visible()) - 1
		if a.HasCount {
			m.cursor = a.Count - 1
		}
	case keymap.HalfPageDown:
		m.cursor += max(m.bodyHeight()/2, 1)
	case keymap.HalfPageUp:
		m.cursor -= max(m.bodyHeight()/2, 1)
	case keymap.Right, keymap.Open:
		if sel == nil {
			break
		}
		if sel.IsSheet() {
			return Effect{Kind: OpenSheet, Node: sel}
		}
		return m.expand(sel)
	case keymap.Left:
		if sel == nil {
			break
		}
		if sel.Expanded {
			sel.Expanded = false
		} else if sel.parent != nil {
			m.cursor = m.indexOf(sel.parent)
		}
	case keymap.Reload:
		target := sel
		if sel != nil && sel.IsSheet() {
			target = sel.parent
		}
		if target != nil {
			target.Expanded, target.Loading, target.Err = true, true, nil
			return Effect{Kind: LoadChildren, Node: target}
		}
	case keymap.NextMatch, keymap.PrevMatch:
		if m.query != "" {
			forward := m.forward == (a.Kind == keymap.NextMatch)
			if !m.find(m.query, forward) {
				return Effect{Kind: NotFound}
			}
		}
	}
	m.clamp()
	return Effect{}
}

func (m *Model) expand(n *Node) Effect {
	n.Expanded = true
	if !n.Loaded && !n.Loading {
		n.Loading, n.Err = true, nil
		return Effect{Kind: LoadChildren, Node: n}
	}
	return Effect{}
}

func (m *Model) SetChildren(n *Node, items []gdrive.Item, err error) {
	n.Loading = false
	if err != nil {
		n.Err = err
		m.clamp()
		return
	}
	n.Err, n.Loaded = nil, true
	n.Children = make([]*Node, 0, len(items))
	for _, it := range items {
		n.Children = append(n.Children, &Node{ID: it.ID, Name: it.Name, Kind: it.Kind, parent: n, depth: n.depth + 1})
	}
	m.clamp()
}

// BeginSearch puts a fresh "Search: query" root at the top and selects it.
func (m *Model) BeginSearch(query string) Effect {
	roots := m.roots[:0:0]
	for _, r := range m.roots {
		if r.Root != SearchResults {
			roots = append(roots, r)
		}
	}
	n := &Node{ID: query, Name: "Search: " + query, Kind: gdrive.KindFolder, Root: SearchResults,
		Expanded: true, Loading: true}
	m.roots = append([]*Node{n}, roots...)
	m.cursor = 0
	m.clamp()
	return Effect{Kind: LoadChildren, Node: n}
}

func (m *Model) Search(q string, forward bool) bool {
	m.query, m.forward = q, forward
	return m.find(q, forward)
}

func (m *Model) Query() string { return m.query }

// find searches every loaded node (expanded or not) in tree order and reveals the hit.
func (m *Model) find(q string, forward bool) bool {
	var all []*Node
	var walk func(ns []*Node)
	walk = func(ns []*Node) {
		for _, n := range ns {
			all = append(all, n)
			walk(n.Children)
		}
	}
	walk(m.roots)
	start := 0
	for i, n := range all {
		if n == m.Selected() {
			start = i
		}
	}
	match := text.Matcher(q)
	for i := 1; i <= len(all); i++ {
		step := i
		if !forward {
			step = -i
		}
		n := all[((start+step)%len(all)+len(all))%len(all)]
		if match(n.Name) {
			for p := n.parent; p != nil; p = p.parent {
				p.Expanded = true
			}
			m.cursor = m.indexOf(n)
			m.clamp()
			return true
		}
	}
	return false
}

func (m *Model) clamp() {
	n := len(m.visible())
	m.cursor = min(max(m.cursor, 0), max(n-1, 0))
	h := m.bodyHeight()
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+h {
		m.top = m.cursor - h + 1
	}
}

func label(n *Node) string {
	icon := "▤"
	if !n.IsSheet() {
		icon = "▸"
		if n.Expanded {
			icon = "▾"
		}
	}
	s := strings.Repeat("  ", n.depth) + icon + " " + n.Name
	switch {
	case n.Loading:
		s += " ⟳ loading…"
	case n.Err != nil:
		s += " ! 読み込み失敗"
	}
	return s
}

// View renders "Drive" and the visible nodes, exactly height lines of width columns.
func (m *Model) View(focused bool) string {
	if m.height <= 0 || m.width <= 0 {
		return ""
	}
	cursor := cursorBlurred
	if focused {
		cursor = cursorFocused
	}
	lines := []string{text.Fit("Drive", m.width)}
	vis := m.visible()
	for i := m.top; i < min(m.top+m.bodyHeight(), len(vis)); i++ {
		line := text.Fit(label(vis[i]), m.width)
		if i == m.cursor {
			line = cursor.Render(line)
		}
		lines = append(lines, line)
	}
	for len(lines) < m.height {
		lines = append(lines, strings.Repeat(" ", m.width))
	}
	return strings.Join(lines[:m.height], "\n")
}
