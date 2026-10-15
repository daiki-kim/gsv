package ui

import "gsv/internal/ui/text"

// popup shows one cell's full value, wrapped to the pane width.
type popup struct {
	title string
	lines []string
	off   int
}

func newPopup(addr, value string, width int) *popup {
	return &popup{title: addr, lines: text.Wrap(value, max(width-2, 1))}
}

func (p *popup) scroll(d, height int) {
	p.off = min(max(p.off+d, 0), max(len(p.lines)-(height-1), 0))
}

func (p *popup) View(width, height int) string {
	out := []string{"── " + p.title + " ──  (j/k でスクロール、q / Esc で閉じる)"}
	for i := p.off; i < len(p.lines) && len(out) < height; i++ {
		out = append(out, " "+p.lines[i])
	}
	return text.Block(out, width, height)
}
