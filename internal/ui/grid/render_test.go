package grid

import (
	"strings"
	"testing"
)

func TestColumnName(t *testing.T) {
	for i, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 27: "AB", 701: "ZZ", 702: "AAA"} {
		if got := ColumnName(i); got != want {
			t.Errorf("ColumnName(%d) = %q, want %q", i, got, want)
		}
	}
}

func TestLayoutFor(t *testing.T) {
	g := layoutFor(50, 60, 12)
	if g.gutter != 2 || g.visCols != 4 || g.visRows != 5 {
		t.Fatalf("g = %+v", g)
	}
	g = layoutFor(1234, 20, 3)
	if g.gutter != 4 || g.visCols != 1 || g.visRows != 1 {
		t.Fatalf("g = %+v", g)
	}
}

func TestRenderGolden(t *testing.T) {
	rows := [][]string{{"日付", "顧客"}, {"10/03", "XYZ株式会社 東京本社"}}
	got := render(frame{rows: rows, nCols: 2, curRow: 1, curCol: 1, width: 40, height: 6})
	want := strings.Join([]string{
		"  │ A          │ B          │",
		"──┼────────────┼────────────┤",
		" 1│ 日付       │ 顧客       │",
		"──┼────────────┼────────────┤",
		" 2│ 10/03      │ XYZ株式会… │",
		"──┼────────────┼────────────┤",
	}, "\n")
	if got != want {
		t.Fatalf("render =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderScrolledAndPadded(t *testing.T) {
	rows := [][]string{{"a", "b", "c"}, {"d"}, {"g", "h", "i"}}
	got := render(frame{rows: rows, nCols: 3, topRow: 1, leftCol: 2, width: 20, height: 8})
	lines := strings.Split(got, "\n")
	if len(lines) != 8 {
		t.Fatalf("lines = %d", len(lines))
	}
	if lines[0] != "  │ C          │" || lines[2] != " 2│            │" || lines[4] != " 3│ i          │" || lines[7] != "" {
		t.Fatalf("render =\n%s", got)
	}
}

func TestRenderTooNarrow(t *testing.T) {
	got := render(frame{rows: [][]string{{"a"}}, nCols: 1, width: 10, height: 2})
	if !strings.Contains(got, "(幅が") {
		t.Fatalf("render = %q", got)
	}
}
