package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"gsv/internal/gdrive"
	"gsv/internal/gsheets"
)

type fakeDrive struct {
	children map[string][]gdrive.Item
	search   map[string][]gdrive.Item
	err      error
}

func (f *fakeDrive) ListChildren(_ context.Context, id string) ([]gdrive.Item, error) {
	return f.children[id], f.err
}
func (f *fakeDrive) ListSharedWithMe(context.Context) ([]gdrive.Item, error) { return nil, f.err }
func (f *fakeDrive) SearchSpreadsheets(_ context.Context, q string) ([]gdrive.Item, error) {
	return f.search[q], f.err
}

type book struct {
	title  string
	tabs   []gsheets.Tab
	values map[string][][]string
}

type fakeSheets struct{ books map[string]book }

func (f *fakeSheets) GetTabs(_ context.Context, id string) (string, []gsheets.Tab, error) {
	b, ok := f.books[id]
	if !ok {
		return "", nil, errors.New("404 not found")
	}
	return b.title, b.tabs, nil
}
func (f *fakeSheets) GetValues(_ context.Context, id, tab string) ([][]string, error) {
	return f.books[id].values[tab], nil
}

func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+w":
		return tea.KeyMsg{Type: tea.KeyCtrlW}
	case "ctrl+n":
		return tea.KeyMsg{Type: tea.KeyCtrlN}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

// drain runs cmd synchronously, feeding results back into m. It reports whether tea.Quit was produced.
func drain(m *Model, cmd tea.Cmd) (quit bool) {
	if cmd == nil {
		return false
	}
	switch msg := cmd().(type) {
	case tea.QuitMsg:
		return true
	case tea.BatchMsg:
		for _, c := range msg {
			quit = drain(m, c) || quit
		}
		return quit
	case nil:
		return false
	default:
		_, next := m.Update(msg)
		return drain(m, next)
	}
}

func press(m *Model, keys ...string) (quit bool) {
	for _, k := range keys {
		_, cmd := m.Update(keyMsg(k))
		quit = drain(m, cmd) || quit
	}
	return quit
}

// typeLine types s (starting with ":" or "/") and presses enter.
func typeLine(m *Model, s string) bool {
	press(m, s[:1])
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s[1:])})
	drain(m, cmd)
	return press(m, "enter")
}

func statusLine(m *Model) string {
	lines := strings.Split(m.View(), "\n")
	return lines[len(lines)-1]
}

func newApp(t *testing.T) (*Model, *fakeDrive) {
	t.Helper()
	d := &fakeDrive{
		children: map[string][]gdrive.Item{"root": {
			{ID: "F1", Name: "仕事", Kind: gdrive.KindFolder},
			{ID: "S1", Name: "売上管理", Kind: gdrive.KindSpreadsheet},
		}},
		search: map[string][]gdrive.Item{"売上": {{ID: "S1", Name: "売上管理", Kind: gdrive.KindSpreadsheet}}},
	}
	big := make([][]string, 1000)
	for i := range big {
		big[i] = make([]string, 201)
	}
	s := &fakeSheets{books: map[string]book{
		"S1": {title: "売上管理", tabs: []gsheets.Tab{{Title: "Sheet1"}, {Title: "Sheet2"}}, values: map[string][][]string{
			"Sheet1": {{"日付", "顧客"}, {"10/03", "XYZ株式会社 東京本社"}},
			"Sheet2": {{"予算"}},
		}},
		"BIG": {title: "big", tabs: []gsheets.Tab{{Title: "T"}}, values: map[string][][]string{"T": big}},
	}}
	m := New(Deps{Drive: d, Sheets: s})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	drain(m, m.Init())
	return m, d
}

func TestStartupExpandsMyDrive(t *testing.T) {
	m, _ := newApp(t)
	v := m.View()
	if !strings.Contains(v, "▾ My Drive") || !strings.Contains(v, "▤ 売上管理") {
		t.Fatalf("view:\n%s", v)
	}
	if got := len(strings.Split(v, "\n")); got != 30 {
		t.Fatalf("view has %d lines, want 30", got)
	}
}

func TestOpenSheetFromTree(t *testing.T) {
	m, _ := newApp(t)
	press(m, "j", "j", "l")
	if m.focus != focusGrid {
		t.Fatal("opening a sheet focuses the grid")
	}
	v := m.View()
	if !strings.Contains(v, "日付") || !strings.Contains(v, "[Sheet1]") {
		t.Fatalf("view:\n%s", v)
	}
	if s := statusLine(m); !strings.Contains(s, "売上管理 / Sheet1") || !strings.Contains(s, "A1") || !strings.Contains(s, "日付") {
		t.Fatalf("status = %q", s)
	}
	press(m, "g", "t")
	if !strings.Contains(m.View(), "予算") {
		t.Fatal("gt should load Sheet2")
	}
}

func TestStatusShowsFullValueAndPopup(t *testing.T) {
	m, _ := newApp(t)
	press(m, "j", "j", "l", "j", "l")
	if s := statusLine(m); !strings.Contains(s, "B2") || !strings.Contains(s, "XYZ株式会社 東京本社") {
		t.Fatalf("status = %q", s)
	}
	press(m, "enter")
	if m.popup == nil || !strings.Contains(m.View(), "XYZ株式会社 東京本社") {
		t.Fatal("enter opens popup with full text")
	}
	press(m, "j") // ignored by grid while popup is open
	press(m, "q")
	if m.popup != nil {
		t.Fatal("q closes popup")
	}
	if addr, _, _ := m.grid.CurrentCell(); addr != "B2" {
		t.Fatalf("keys leaked to grid while popup open: %s", addr)
	}
}

func TestPaneSwitching(t *testing.T) {
	m, _ := newApp(t)
	press(m, "j", "j", "l")
	press(m, "ctrl+w", "h")
	if m.focus != focusTree {
		t.Fatal("ctrl+w h focuses tree")
	}
	press(m, "ctrl+w", "l")
	if m.focus != focusGrid {
		t.Fatal("ctrl+w l focuses grid")
	}
	press(m, "ctrl+n")
	if !m.treeHidden || strings.Contains(m.View(), "My Drive") {
		t.Fatal("ctrl+n hides tree")
	}
	press(m, "ctrl+w", "h")
	if m.focus != focusGrid {
		t.Fatal("cannot focus hidden tree")
	}
	press(m, "ctrl+n")
	if m.treeHidden {
		t.Fatal("ctrl+n shows tree again")
	}
}

func TestCommands(t *testing.T) {
	m, _ := newApp(t)
	typeLine(m, ":frobnicate now")
	if s := statusLine(m); !strings.Contains(s, "E492: Not an editor command: frobnicate now") {
		t.Fatalf("status = %q", s)
	}
	typeLine(m, ":e")
	if s := statusLine(m); !strings.Contains(s, "E471: Argument required") {
		t.Fatalf("status = %q", s)
	}
	typeLine(m, ":e https://docs.google.com/spreadsheets/d/S1/edit#gid=0")
	if !strings.Contains(m.View(), "日付") || m.focus != focusGrid {
		t.Fatal(":e opens the sheet")
	}
	typeLine(m, ":find 売上")
	if !strings.Contains(m.View(), "Search: 売上") || m.focus != focusTree {
		t.Fatalf("view:\n%s", m.View())
	}
	if !typeLine(m, ":q") {
		t.Fatal(":q quits")
	}
}

func TestSearchNotFoundMessage(t *testing.T) {
	m, _ := newApp(t)
	press(m, "j", "j", "l")
	typeLine(m, "/zzz")
	if s := statusLine(m); !strings.Contains(s, "E486: Pattern not found: zzz") {
		t.Fatalf("status = %q", s)
	}
	typeLine(m, "/xyz")
	if addr, _, _ := m.grid.CurrentCell(); addr != "B2" {
		t.Fatalf("search moved to %s", addr)
	}
}

func TestStaleResponsesIgnored(t *testing.T) {
	m, _ := newApp(t)
	m.grid.Open("S2")
	m.Update(tabsMsg{id: "S1", title: "売上管理", tabs: []gsheets.Tab{{Title: "Sheet1"}}})
	m.Update(valuesMsg{id: "S1", tab: 0, rows: [][]string{{"stale"}}})
	if strings.Contains(m.View(), "stale") || m.grid.Location() != "" {
		t.Fatalf("stale response leaked:\n%s", m.View())
	}
	if m.grid.SpreadsheetID() != "S2" {
		t.Fatal("grid must stay on S2")
	}
}

func TestErrorsShowInStatus(t *testing.T) {
	m, d := newApp(t)
	typeLine(m, ":e https://docs.google.com/spreadsheets/d/NOPE/edit")
	if s := statusLine(m); !strings.Contains(s, "404") {
		t.Fatalf("status = %q", s)
	}
	d.err = errors.New("network down")
	press(m, "ctrl+w", "h", "g", "g", "r")
	if s := statusLine(m); !strings.Contains(s, "network down") {
		t.Fatalf("status = %q", s)
	}
}

func TestLargeSheetWarning(t *testing.T) {
	m, _ := newApp(t)
	typeLine(m, ":e https://docs.google.com/spreadsheets/d/BIG/edit")
	if s := statusLine(m); !strings.Contains(s, "201000") {
		t.Fatalf("status = %q", s)
	}
}

func TestCtrlCQuitsOutsideInput(t *testing.T) {
	m, _ := newApp(t)
	press(m, ":")
	if press(m, "ctrl+c") {
		t.Fatal("ctrl+c in command line only cancels it")
	}
	if !press(m, "ctrl+c") {
		t.Fatal("ctrl+c quits in normal mode")
	}
}

func TestStatusNeutralizesControlChars(t *testing.T) {
	m, _ := newApp(t)
	m.setErr("Drive: bad \x1b[2J response")
	s := statusLine(m)
	if strings.Contains(s, "\x1b") || !strings.Contains(s, "bad") {
		t.Fatalf("status = %q", s)
	}
}
