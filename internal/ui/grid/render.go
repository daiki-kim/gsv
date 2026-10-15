package grid

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"gsv/internal/ui/text"
)

// CellWidth is the fixed display width of every column: one space of padding plus 11 columns of content.
const CellWidth = 12

var (
	cursorFocused = lipgloss.NewStyle().Reverse(true)
	cursorBlurred = lipgloss.NewStyle().Underline(true)
)

// ColumnName converts a 0-based column index to A, B, …, Z, AA, ….
func ColumnName(i int) string {
	name := ""
	for i++; i > 0; i = (i - 1) / 26 {
		name = string(rune('A'+(i-1)%26)) + name
	}
	return name
}

type geometry struct {
	gutter  int // width of the row-number column
	visCols int
	visRows int
}

// layoutFor computes how many columns and data rows fit in a body of width×height.
func layoutFor(nRows, width, height int) geometry {
	g := geometry{gutter: max(len(strconv.Itoa(nRows)), 2)}
	g.visCols = max((width-g.gutter-1)/(CellWidth+1), 1)
	g.visRows = max((height-2)/2, 1)
	return g
}

func cellAt(rows [][]string, r, c int) string {
	if r < 0 || r >= len(rows) || c < 0 || c >= len(rows[r]) {
		return ""
	}
	return rows[r][c]
}

type frame struct {
	rows                            [][]string
	nCols                           int
	curRow, curCol, topRow, leftCol int
	width, height                   int
	focused                         bool
}

func separator(gutter, n int) string {
	return strings.Repeat("─", gutter) + "┼" +
		strings.Repeat(strings.Repeat("─", CellWidth)+"┼", n-1) +
		strings.Repeat("─", CellWidth) + "┤"
}

// render draws the ruled grid body: a column header, then each row followed by a separator line.
func render(f frame) string {
	if f.height <= 0 {
		return ""
	}
	g := layoutFor(len(f.rows), f.width, f.height)
	if f.width < g.gutter+CellWidth+2 {
		return text.Block([]string{"(幅が狭すぎます)"}, f.width, f.height)
	}
	if len(f.rows) == 0 || f.nCols == 0 {
		return text.Block([]string{"(空のシート)"}, f.width, f.height)
	}
	f.leftCol = min(max(f.leftCol, 0), f.nCols-1)
	f.topRow = min(max(f.topRow, 0), len(f.rows)-1)
	lastCol := min(f.leftCol+g.visCols, f.nCols)
	cursor := cursorBlurred
	if f.focused {
		cursor = cursorFocused
	}

	var lines []string
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", g.gutter) + "│")
	for c := f.leftCol; c < lastCol; c++ {
		b.WriteString(" " + text.Fit(ColumnName(c), CellWidth-1) + "│")
	}
	sep := separator(g.gutter, lastCol-f.leftCol)
	lines = append(lines, b.String(), sep)

	for r := f.topRow; r < min(f.topRow+g.visRows, len(f.rows)); r++ {
		b.Reset()
		b.WriteString(fmt.Sprintf("%*d│", g.gutter, r+1))
		for c := f.leftCol; c < lastCol; c++ {
			cell := " " + text.Fit(cellAt(f.rows, r, c), CellWidth-1)
			if r == f.curRow && c == f.curCol {
				cell = cursor.Render(cell)
			}
			b.WriteString(cell + "│")
		}
		lines = append(lines, b.String(), sep)
	}
	for len(lines) < f.height {
		lines = append(lines, "")
	}
	return strings.Join(lines[:f.height], "\n")
}
