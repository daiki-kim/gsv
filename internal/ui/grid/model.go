// Package grid holds the right-pane spreadsheet view.
package grid

import (
	"fmt"
	"strconv"
	"strings"

	"gsv/internal/gsheets"
	"gsv/internal/ui/keymap"
	"gsv/internal/ui/text"
)

// Effect tells the caller what to do after an action.
type Effect int

const (
	None     Effect = iota
	LoadTab         // fetch values for CurrentTab()
	Peek            // show the current cell in a popup
	NotFound        // a repeated search found nothing
	Reopen          // re-fetch the tab list of SpreadsheetID()
)

// LargeSheetCells is the cell count above which the caller warns the user.
const LargeSheetCells = 200000

type Model struct {
	id, title   string
	tabs        []gsheets.Tab
	tab         int
	data        map[int][][]string
	nCols       map[int]int
	loading     map[int]bool
	errs        map[int]error
	openLoading bool
	openErr     error

	curRow, curCol, topRow, leftCol int
	width, height                   int

	query   string
	forward bool
}

func New() *Model {
	return &Model{data: map[int][][]string{}, nCols: map[int]int{}, loading: map[int]bool{}, errs: map[int]error{}}
}

func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
	m.clamp()
}

// Open resets the grid for spreadsheet id and marks it as loading.
func (m *Model) Open(id string) {
	*m = Model{
		id: id, width: m.width, height: m.height, openLoading: true,
		data: map[int][][]string{}, nCols: map[int]int{}, loading: map[int]bool{}, errs: map[int]error{},
	}
}

func (m *Model) SpreadsheetID() string { return m.id }

// Opening reports whether the tab list is still being fetched (between Open and SetTabs/SetOpenError).
func (m *Model) Opening() bool { return m.openLoading }

func (m *Model) SetTabs(title string, tabs []gsheets.Tab) Effect {
	m.title, m.tabs, m.tab, m.openLoading = title, tabs, 0, false
	if len(tabs) == 0 {
		return None
	}
	m.loading[0] = true
	return LoadTab
}

func (m *Model) SetOpenError(err error) {
	m.openLoading, m.openErr = false, err
}

func (m *Model) CurrentTab() int { return m.tab }

func (m *Model) TabTitle(i int) string {
	if i < 0 || i >= len(m.tabs) {
		return ""
	}
	return m.tabs[i].Title
}

func (m *Model) SetTabData(tab int, rows [][]string) {
	n := 0
	for _, r := range rows {
		n = max(n, len(r))
	}
	m.data[tab], m.nCols[tab] = rows, n
	delete(m.loading, tab)
	delete(m.errs, tab)
	m.clamp()
}

func (m *Model) SetTabError(tab int, err error) {
	delete(m.loading, tab)
	m.errs[tab] = err
}

func (m *Model) rows() [][]string { return m.data[m.tab] }

func (m *Model) geom() geometry { return layoutFor(len(m.rows()), m.width, m.height-1) }

func (m *Model) Apply(a keymap.Action) Effect {
	switch a.Kind {
	case keymap.Down:
		m.curRow += a.Count
	case keymap.Up:
		m.curRow -= a.Count
	case keymap.Right:
		m.curCol += a.Count
	case keymap.Left:
		m.curCol -= a.Count
	case keymap.Top:
		m.curRow = 0
		if a.HasCount {
			m.curRow = a.Count - 1
		}
	case keymap.Bottom:
		m.curRow = len(m.rows()) - 1
		if a.HasCount {
			m.curRow = a.Count - 1
		}
	case keymap.LineStart:
		m.curCol = 0
	case keymap.LineEnd:
		if rows := m.rows(); m.curRow < len(rows) {
			m.curCol = len(rows[m.curRow]) - 1
		}
	case keymap.HalfPageDown:
		m.curRow += max(m.geom().visRows/2, 1)
	case keymap.HalfPageUp:
		m.curRow -= max(m.geom().visRows/2, 1)
	case keymap.NextTab:
		if n := len(m.tabs); n > 0 {
			if a.HasCount {
				if a.Count <= n {
					return m.switchTab(a.Count - 1)
				}
				break
			}
			return m.switchTab((m.tab + 1) % n)
		}
	case keymap.PrevTab:
		if n := len(m.tabs); n > 0 {
			return m.switchTab(((m.tab-a.Count)%n + n) % n)
		}
	case keymap.Open, keymap.Peek:
		if _, _, ok := m.CurrentCell(); ok {
			return Peek
		}
	case keymap.Reload:
		if m.id != "" && m.openErr != nil {
			return Reopen
		}
		if len(m.tabs) > 0 {
			m.loading[m.tab] = true
			delete(m.errs, m.tab)
			return LoadTab
		}
	case keymap.NextMatch, keymap.PrevMatch:
		if m.query != "" {
			forward := m.forward == (a.Kind == keymap.NextMatch)
			if !m.find(m.query, forward) {
				return NotFound
			}
		}
	}
	m.clamp()
	return None
}

func (m *Model) switchTab(i int) Effect {
	m.tab, m.curRow, m.curCol, m.topRow, m.leftCol = i, 0, 0, 0, 0
	m.clamp()
	if _, ok := m.data[i]; !ok && !m.loading[i] {
		m.loading[i] = true
		delete(m.errs, i)
		return LoadTab
	}
	return None
}

// clamp keeps the cursor inside the data and scrolls it into view.
func (m *Model) clamp() {
	nRows, nCols := len(m.rows()), m.nCols[m.tab]
	m.curRow = min(max(m.curRow, 0), max(nRows-1, 0))
	m.curCol = min(max(m.curCol, 0), max(nCols-1, 0))
	g := m.geom()
	if m.curRow < m.topRow {
		m.topRow = m.curRow
	}
	if m.curRow >= m.topRow+g.visRows {
		m.topRow = m.curRow - g.visRows + 1
	}
	if m.curCol < m.leftCol {
		m.leftCol = m.curCol
	}
	if m.curCol >= m.leftCol+g.visCols {
		m.leftCol = m.curCol - g.visCols + 1
	}
}

// Search remembers q for n/N and jumps to the next matching cell in row-major order.
func (m *Model) Search(q string, forward bool) bool {
	m.query, m.forward = q, forward
	return m.find(q, forward)
}

func (m *Model) Query() string { return m.query }

func (m *Model) find(q string, forward bool) bool {
	rows, nCols := m.rows(), m.nCols[m.tab]
	total := len(rows) * nCols
	if total == 0 {
		return false
	}
	match := text.Matcher(q)
	start := m.curRow*nCols + m.curCol
	for i := 1; i <= total; i++ {
		step := i
		if !forward {
			step = -i
		}
		p := ((start+step)%total + total) % total
		if r, c := p/nCols, p%nCols; match(cellAt(rows, r, c)) {
			m.curRow, m.curCol = r, c
			m.clamp()
			return true
		}
	}
	return false
}

// CurrentCell returns the cursor's A1 address and raw value; ok is false when no data is shown.
func (m *Model) CurrentCell() (addr, value string, ok bool) {
	rows := m.rows()
	if len(rows) == 0 || m.nCols[m.tab] == 0 {
		return "", "", false
	}
	return ColumnName(m.curCol) + strconv.Itoa(m.curRow+1), cellAt(rows, m.curRow, m.curCol), true
}

func (m *Model) Location() string {
	if m.id == "" || m.title == "" {
		return ""
	}
	if t := m.TabTitle(m.tab); t != "" {
		return m.title + " / " + t
	}
	return m.title
}

func (m *Model) titleLine() string {
	if m.id == "" {
		return ""
	}
	parts := []string{m.title}
	for i, t := range m.tabs {
		if i == m.tab {
			parts = append(parts, "["+t.Title+"]")
		} else {
			parts = append(parts, t.Title)
		}
	}
	return strings.Join(parts, "  ")
}

// View renders the title line plus the body, exactly height lines.
func (m *Model) View(focused bool) string {
	if m.height <= 0 {
		return ""
	}
	title := text.Fit(m.titleLine(), m.width)
	bodyH := m.height - 1
	var body string
	switch {
	case m.id == "":
		body = text.Block([]string{"シートが開かれていません。", "左のツリーで l / Enter、または :e <URL>"}, m.width, bodyH)
	case m.openLoading:
		body = text.Block([]string{"⟳ loading…"}, m.width, bodyH)
	case m.openErr != nil:
		body = text.Block([]string{"! 読み込み失敗: " + m.openErr.Error() + " (r で再試行)"}, m.width, bodyH)
	case m.errs[m.tab] != nil:
		body = text.Block([]string{fmt.Sprintf("! 読み込み失敗: %v (r で再試行)", m.errs[m.tab])}, m.width, bodyH)
	case m.loading[m.tab] || (len(m.tabs) > 0 && !m.loaded(m.tab)):
		body = text.Block([]string{"⟳ loading…"}, m.width, bodyH)
	default:
		body = render(frame{
			rows: m.rows(), nCols: m.nCols[m.tab],
			curRow: m.curRow, curCol: m.curCol, topRow: m.topRow, leftCol: m.leftCol,
			width: m.width, height: bodyH, focused: focused,
		})
	}
	if bodyH <= 0 {
		return title
	}
	return title + "\n" + body
}

func (m *Model) loaded(tab int) bool {
	_, ok := m.data[tab]
	return ok
}
