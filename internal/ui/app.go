// Package ui wires the tree, grid and command line into the gsv TUI.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gsv/internal/config"
	"gsv/internal/gdrive"
	"gsv/internal/gsheets"
	"gsv/internal/ui/cmdline"
	"gsv/internal/ui/grid"
	"gsv/internal/ui/keymap"
	"gsv/internal/ui/text"
	"gsv/internal/ui/tree"
)

type Deps struct {
	Drive  gdrive.Client
	Sheets gsheets.Client
	Shared []config.SharedRoot
}

type focus int

const (
	focusTree focus = iota
	focusGrid
)

const apiTimeout = 60 * time.Second

var errStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

type childrenMsg struct {
	node  *tree.Node
	items []gdrive.Item
	err   error
}

type tabsMsg struct {
	id, title string
	tabs      []gsheets.Tab
	err       error
}

type valuesMsg struct {
	id   string
	tab  int
	rows [][]string
	err  error
}

type Model struct {
	deps       Deps
	tree       *tree.Model
	grid       *grid.Model
	keys       keymap.Parser
	input      cmdline.Input
	popup      *popup
	focus      focus
	treeHidden bool
	width      int
	height     int
	msg        string
	msgErr     bool
}

func New(d Deps) *Model {
	return &Model{deps: d, tree: tree.New(d.Shared), grid: grid.New()}
}

// Init expands My Drive, which starts selected.
func (m *Model) Init() tea.Cmd {
	return m.treeEffect(m.tree.Apply(keymap.Action{Kind: keymap.Open, Count: 1}))
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
	case tea.KeyMsg:
		return m, m.handleKey(msg)
	case childrenMsg:
		m.tree.SetChildren(msg.node, msg.items, msg.err)
		if msg.err != nil {
			m.setErr("Drive: " + msg.err.Error())
		}
	case tabsMsg:
		if msg.id != m.grid.SpreadsheetID() || !m.grid.Opening() {
			break
		}
		if msg.err != nil {
			m.grid.SetOpenError(msg.err)
			m.setErr("Sheets: " + msg.err.Error())
			break
		}
		if m.grid.SetTabs(msg.title, msg.tabs) == grid.LoadTab {
			return m, m.loadValues()
		}
	case valuesMsg:
		if msg.id != m.grid.SpreadsheetID() {
			break
		}
		if msg.err != nil {
			m.grid.SetTabError(msg.tab, msg.err)
			m.setErr("Sheets: " + msg.err.Error())
			break
		}
		m.grid.SetTabData(msg.tab, msg.rows)
		if n := len(msg.rows) * maxLen(msg.rows); n > grid.LargeSheetCells {
			m.msg, m.msgErr = fmt.Sprintf("警告: %d セルの大きなシートです（表示が重くなる場合があります）", n), false
		}
	}
	return m, nil
}

func maxLen(rows [][]string) int {
	n := 0
	for _, r := range rows {
		n = max(n, len(r))
	}
	return n
}

func (m *Model) setErr(s string) { m.msg, m.msgErr = s, true }

func (m *Model) resize() {
	treeW := 0
	if !m.treeHidden {
		treeW = 30
		if m.width < 80 {
			treeW = m.width / 3
		}
	}
	gridW := m.width
	if !m.treeHidden {
		gridW = m.width - treeW - 1
	}
	paneH := max(m.height-1, 0)
	m.tree.SetSize(treeW, paneH)
	m.grid.SetSize(gridW, paneH)
}

func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	if m.popup != nil {
		switch msg.String() {
		case "q", "esc", "enter", "K":
			m.popup = nil
		case "j", "down":
			m.popup.scroll(1, m.height-1)
		case "k", "up":
			m.popup.scroll(-1, m.height-1)
		case "ctrl+c":
			return tea.Quit
		}
		return nil
	}
	if m.input.Active() {
		mode := m.input.Mode()
		res, s := m.input.Key(msg)
		if res != cmdline.Submitted {
			return nil
		}
		return m.submit(mode, s)
	}
	if msg.String() == "ctrl+c" {
		return tea.Quit
	}
	m.msg = ""
	a, ok := m.keys.Feed(msg.String())
	if !ok {
		return nil
	}
	switch a.Kind {
	case keymap.CommandMode:
		m.input.Start(cmdline.Command)
	case keymap.SearchForward:
		m.input.Start(cmdline.SearchForward)
	case keymap.SearchBackward:
		m.input.Start(cmdline.SearchBackward)
	case keymap.PaneLeft:
		if !m.treeHidden {
			m.focus = focusTree
		}
	case keymap.PaneRight:
		m.focus = focusGrid
	case keymap.PaneNext:
		if !m.treeHidden {
			m.focus = 1 - m.focus
		}
	case keymap.ToggleTree:
		m.treeHidden = !m.treeHidden
		if m.treeHidden {
			m.focus = focusGrid
		}
		m.resize()
	case keymap.Cancel:
	default:
		if m.focus == focusTree {
			return m.treeEffect(m.tree.Apply(a))
		}
		return m.gridEffect(m.grid.Apply(a))
	}
	return nil
}

func (m *Model) submit(mode cmdline.Mode, s string) tea.Cmd {
	if mode == cmdline.Command {
		return m.execute(cmdline.Parse(s))
	}
	forward := mode == cmdline.SearchForward
	var found bool
	if m.focus == focusTree {
		if s == "" {
			s = m.tree.Query()
		}
		found = s != "" && m.tree.Search(s, forward)
	} else {
		if s == "" {
			s = m.grid.Query()
		}
		found = s != "" && m.grid.Search(s, forward)
	}
	if !found {
		m.setErr("E486: Pattern not found: " + s)
	}
	return nil
}

func (m *Model) execute(c cmdline.Cmd) tea.Cmd {
	switch c.Name {
	case "":
		return nil
	case "q", "q!", "qa", "qa!", "quit", "qall":
		return tea.Quit
	case "e", "edit":
		if c.Arg == "" {
			m.setErr("E471: Argument required")
			return nil
		}
		id, ok := cmdline.ParseSpreadsheetID(c.Arg)
		if !ok {
			m.setErr("スプレッドシートの URL / ID ではありません: " + c.Arg)
			return nil
		}
		return m.open(id)
	case "find":
		if c.Arg == "" {
			m.setErr("E471: Argument required")
			return nil
		}
		if m.treeHidden {
			m.treeHidden = false
			m.resize()
		}
		m.focus = focusTree
		return m.treeEffect(m.tree.BeginSearch(c.Arg))
	}
	m.setErr("E492: Not an editor command: " + c.Raw)
	return nil
}

func (m *Model) treeEffect(e tree.Effect) tea.Cmd {
	switch e.Kind {
	case tree.LoadChildren:
		return m.loadChildren(e.Node)
	case tree.OpenSheet:
		return m.open(e.Node.ID)
	case tree.NotFound:
		m.setErr("E486: Pattern not found: " + m.tree.Query())
	}
	return nil
}

func (m *Model) gridEffect(e grid.Effect) tea.Cmd {
	switch e {
	case grid.LoadTab:
		return m.loadValues()
	case grid.Reopen:
		return m.open(m.grid.SpreadsheetID())
	case grid.Peek:
		if addr, v, ok := m.grid.CurrentCell(); ok {
			m.popup = newPopup(addr, v, m.width)
		}
	case grid.NotFound:
		m.setErr("E486: Pattern not found: " + m.grid.Query())
	}
	return nil
}

func (m *Model) open(id string) tea.Cmd {
	m.grid.Open(id)
	m.focus = focusGrid
	sheets := m.deps.Sheets
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
		defer cancel()
		title, tabs, err := sheets.GetTabs(ctx, id)
		return tabsMsg{id: id, title: title, tabs: tabs, err: err}
	}
}

func (m *Model) loadValues() tea.Cmd {
	id, tab := m.grid.SpreadsheetID(), m.grid.CurrentTab()
	title := m.grid.TabTitle(tab)
	sheets := m.deps.Sheets
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
		defer cancel()
		rows, err := sheets.GetValues(ctx, id, title)
		return valuesMsg{id: id, tab: tab, rows: rows, err: err}
	}
}

func (m *Model) loadChildren(n *tree.Node) tea.Cmd {
	drive := m.deps.Drive
	root, id := n.Root, n.ID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
		defer cancel()
		var items []gdrive.Item
		var err error
		switch root {
		case tree.SharedWithMe:
			items, err = drive.ListSharedWithMe(ctx)
		case tree.SearchResults:
			items, err = drive.SearchSpreadsheets(ctx, id)
		default:
			items, err = drive.ListChildren(ctx, id)
		}
		return childrenMsg{node: n, items: items, err: err}
	}
}

func (m *Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	paneH := m.height - 1
	var main string
	switch {
	case m.popup != nil:
		main = m.popup.View(m.width, paneH)
	case m.treeHidden:
		main = m.grid.View(m.focus == focusGrid)
	default:
		divider := strings.TrimSuffix(strings.Repeat("│\n", paneH), "\n")
		main = lipgloss.JoinHorizontal(lipgloss.Top,
			m.tree.View(m.focus == focusTree), divider, m.grid.View(m.focus == focusGrid))
	}
	if paneH <= 0 {
		return m.statusLine()
	}
	return main + "\n" + m.statusLine()
}

func (m *Model) statusLine() string {
	if m.input.Active() {
		return text.Truncate(text.Flatten(m.input.View()), m.width)
	}
	if m.msg != "" {
		s := text.Truncate(text.Flatten(m.msg), m.width)
		if m.msgErr {
			return errStyle.Render(s)
		}
		return s
	}
	parts := []string{"NORMAL"}
	if loc := m.grid.Location(); loc != "" {
		parts = append(parts, loc)
	}
	if addr, v, ok := m.grid.CurrentCell(); ok {
		parts = append(parts, addr, text.Flatten(v))
	}
	if p := m.keys.Pending(); p != "" {
		parts = append(parts, p)
	}
	return text.Truncate(text.Flatten(strings.Join(parts, "  ")), m.width)
}
