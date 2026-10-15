package grid

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"gsv/internal/gsheets"
	"gsv/internal/ui/keymap"
)

func act(k keymap.Kind) keymap.Action { return keymap.Action{Kind: k, Count: 1} }
func actN(k keymap.Kind, n int) keymap.Action {
	return keymap.Action{Kind: k, Count: n, HasCount: true}
}

func table(nRows, nCols int) [][]string {
	rows := make([][]string, nRows)
	for r := range rows {
		rows[r] = make([]string, nCols)
		for c := range rows[r] {
			rows[r][c] = fmt.Sprintf("r%dc%d", r, c)
		}
	}
	return rows
}

var threeTabs = []gsheets.Tab{{Title: "Sheet1"}, {Title: "Sheet2"}, {Title: "Sheet3"}}

// loaded returns a 60x13 grid (body 12 rows → 5 data rows, 4 columns visible).
func loaded(rows [][]string) *Model {
	m := New()
	m.SetSize(60, 13)
	m.Open("ID")
	m.SetTabs("売上管理", threeTabs)
	m.SetTabData(0, rows)
	return m
}

func TestMovementClamps(t *testing.T) {
	m := loaded(table(50, 30))
	m.Apply(actN(keymap.Down, 100))
	m.Apply(actN(keymap.Right, 100))
	if m.curRow != 49 || m.curCol != 29 {
		t.Fatalf("cur = %d,%d", m.curRow, m.curCol)
	}
	m.Apply(actN(keymap.Up, 500))
	m.Apply(act(keymap.LineStart))
	if m.curRow != 0 || m.curCol != 0 {
		t.Fatalf("cur = %d,%d", m.curRow, m.curCol)
	}
}

func TestTopBottomWithCount(t *testing.T) {
	m := loaded(table(50, 3))
	m.Apply(act(keymap.Bottom))
	if m.curRow != 49 {
		t.Fatalf("G -> %d", m.curRow)
	}
	m.Apply(actN(keymap.Top, 10))
	if m.curRow != 9 {
		t.Fatalf("10gg -> %d", m.curRow)
	}
	m.Apply(act(keymap.Top))
	if m.curRow != 0 {
		t.Fatalf("gg -> %d", m.curRow)
	}
	m.Apply(actN(keymap.Bottom, 3))
	if m.curRow != 2 {
		t.Fatalf("3G -> %d", m.curRow)
	}
}

func TestScrollFollowsCursor(t *testing.T) {
	m := loaded(table(50, 30))
	m.Apply(actN(keymap.Down, 7))
	if m.topRow != 3 {
		t.Fatalf("topRow = %d", m.topRow)
	}
	m.Apply(actN(keymap.Right, 6))
	if m.leftCol != 3 {
		t.Fatalf("leftCol = %d", m.leftCol)
	}
	m.Apply(act(keymap.Top))
	if m.topRow != 0 {
		t.Fatalf("topRow = %d", m.topRow)
	}
	m.Apply(act(keymap.HalfPageDown))
	if m.curRow != 2 {
		t.Fatalf("ctrl+d -> %d", m.curRow)
	}
}

func TestRaggedRows(t *testing.T) {
	m := loaded([][]string{{"a", "b", "c"}, {"d"}, {}})
	m.Apply(act(keymap.Down))
	m.Apply(act(keymap.LineEnd))
	if m.curCol != 0 {
		t.Fatalf("$ on short row -> %d", m.curCol)
	}
	m.Apply(actN(keymap.Right, 5))
	addr, v, ok := m.CurrentCell()
	if !ok || addr != "C2" || v != "" {
		t.Fatalf("cell = %q %q %v", addr, v, ok)
	}
	m.Apply(act(keymap.Down))
	if _, v, ok := m.CurrentCell(); !ok || v != "" {
		t.Fatalf("empty row cell = %q %v", v, ok)
	}
	if strings.Count(m.View(true), "\n") != 12 {
		t.Fatal("view must be exactly 13 lines")
	}
}

func TestEmptyTab(t *testing.T) {
	m := loaded(nil)
	m.Apply(actN(keymap.Down, 3))
	m.Apply(act(keymap.LineEnd))
	if _, _, ok := m.CurrentCell(); ok {
		t.Fatal("empty tab has no current cell")
	}
	if m.Apply(act(keymap.Open)) != None {
		t.Fatal("Enter on empty tab must not open a popup")
	}
	if !strings.Contains(m.View(true), "(空のシート)") {
		t.Fatal("view should say the sheet is empty")
	}
}

func TestTabSwitching(t *testing.T) {
	m := loaded(table(5, 5))
	m.Apply(actN(keymap.Down, 3))
	if e := m.Apply(act(keymap.NextTab)); e != LoadTab || m.CurrentTab() != 1 || m.curRow != 0 {
		t.Fatalf("gt: effect %v tab %d row %d", e, m.CurrentTab(), m.curRow)
	}
	if !strings.Contains(m.View(true), "loading") {
		t.Fatal("unloaded tab should show loading")
	}
	m.SetTabData(1, table(2, 2))
	if e := m.Apply(act(keymap.PrevTab)); e != None || m.CurrentTab() != 0 {
		t.Fatalf("gT: effect %v tab %d", e, m.CurrentTab())
	}
	if e := m.Apply(act(keymap.PrevTab)); e != LoadTab || m.CurrentTab() != 2 {
		t.Fatalf("gT wraps: effect %v tab %d", e, m.CurrentTab())
	}
	if e := m.Apply(actN(keymap.NextTab, 2)); e != None || m.CurrentTab() != 1 {
		t.Fatalf("2gt: effect %v tab %d", e, m.CurrentTab())
	}
	if e := m.Apply(actN(keymap.NextTab, 9)); e != None || m.CurrentTab() != 1 {
		t.Fatalf("9gt out of range must be ignored: effect %v tab %d", e, m.CurrentTab())
	}
	if !strings.Contains(m.View(true), "[Sheet2]") {
		t.Fatal("title should bracket current tab")
	}
}

func TestReloadAndErrors(t *testing.T) {
	m := loaded(table(2, 2))
	if e := m.Apply(act(keymap.Reload)); e != LoadTab {
		t.Fatalf("r -> %v", e)
	}
	m.SetTabError(0, errors.New("boom"))
	v := m.View(true)
	if !strings.Contains(v, "読み込み失敗") || !strings.Contains(v, "boom") {
		t.Fatalf("view = %q", v)
	}
	m2 := New()
	m2.SetSize(60, 13)
	m2.Open("X")
	m2.SetOpenError(errors.New("404"))
	if !strings.Contains(m2.View(true), "404") {
		t.Fatal("open error should be shown")
	}
}

func TestSearch(t *testing.T) {
	m := loaded([][]string{{"Apple", "x"}, {"pear", "apple pie"}, {"APPLE", "y"}})
	if !m.Search("apple", true) {
		t.Fatal("should find")
	}
	if addr, _, _ := m.CurrentCell(); addr != "B2" {
		t.Fatalf("first hit = %s", addr)
	}
	m.Apply(act(keymap.NextMatch))
	if addr, _, _ := m.CurrentCell(); addr != "A3" {
		t.Fatalf("n -> %s", addr)
	}
	m.Apply(act(keymap.NextMatch))
	if addr, _, _ := m.CurrentCell(); addr != "A1" {
		t.Fatalf("n wraps -> %s", addr)
	}
	m.Apply(act(keymap.PrevMatch))
	if addr, _, _ := m.CurrentCell(); addr != "A3" {
		t.Fatalf("N -> %s", addr)
	}
	if !m.Search("Apple", false) {
		t.Fatal("case-sensitive backward search should find A1")
	}
	if addr, _, _ := m.CurrentCell(); addr != "A1" {
		t.Fatalf("?Apple -> %s", addr)
	}
	if m.Search("banana", true) {
		t.Fatal("should not find")
	}
	if m.Apply(act(keymap.NextMatch)) != NotFound || m.Query() != "banana" {
		t.Fatal("n after failed search -> NotFound")
	}
}

func TestPeekAndLocation(t *testing.T) {
	m := loaded([][]string{{"長い\n文章"}})
	if m.Apply(act(keymap.Peek)) != Peek || m.Apply(act(keymap.Open)) != Peek {
		t.Fatal("K / Enter should peek")
	}
	if m.Location() != "売上管理 / Sheet1" {
		t.Fatalf("Location = %q", m.Location())
	}
	if _, v, _ := m.CurrentCell(); v != "長い\n文章" {
		t.Fatalf("value keeps newlines: %q", v)
	}
}

func TestViewBeforeOpen(t *testing.T) {
	m := New()
	m.SetSize(40, 5)
	if !strings.Contains(m.View(false), ":e") || m.Location() != "" {
		t.Fatal("empty grid should explain how to open a sheet")
	}
}

func TestReloadAfterOpenErrorReopens(t *testing.T) {
	m := New()
	m.SetSize(60, 13)
	m.Open("X")
	if !m.Opening() {
		t.Fatal("should be opening")
	}
	m.SetOpenError(errors.New("503"))
	if m.Opening() {
		t.Fatal("not opening after error")
	}
	if got := m.Apply(keymap.Action{Kind: keymap.Reload, Count: 1}); got != Reopen {
		t.Fatalf("effect = %v, want Reopen", got)
	}
	if !strings.Contains(m.View(true), "r で再試行") {
		t.Fatal("hint missing")
	}
}
