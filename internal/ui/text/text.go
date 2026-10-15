// Package text measures and shapes terminal text. All width math in gsv goes
// through here so that ambiguous-width runes (…, │, ─) always count as 1.
package text

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

var cond = func() *runewidth.Condition {
	c := runewidth.NewCondition()
	c.EastAsianWidth = false
	return c
}()

var flattener = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ")

func Width(s string) int { return cond.StringWidth(s) }

func Flatten(s string) string { return flattener.Replace(s) }

// Fit flattens s, truncates it with "…" and pads it to exactly w columns.
func Fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return cond.FillRight(Truncate(Flatten(s), w), w)
}

// Truncate shortens s to at most w columns, ending with "…" when cut.
func Truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if cond.StringWidth(s) <= w {
		return s
	}
	return cond.Truncate(s, w, "…")
}

// Wrap splits s on newlines and then breaks each line at w columns.
func Wrap(s string, w int) []string {
	w = max(w, 1)
	var out []string
	for _, para := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		var line strings.Builder
		lw := 0
		for _, r := range para {
			if r == '\t' {
				r = ' '
			}
			rw := cond.RuneWidth(r)
			if lw+rw > w && lw > 0 {
				out = append(out, line.String())
				line.Reset()
				lw = 0
			}
			line.WriteRune(r)
			lw += rw
		}
		out = append(out, line.String())
	}
	return out
}

// Block renders lines as exactly h rows of exactly w columns.
func Block(lines []string, w, h int) string {
	out := make([]string, max(h, 0))
	for i := range out {
		s := ""
		if i < len(lines) {
			s = lines[i]
		}
		out[i] = Fit(s, w)
	}
	return strings.Join(out, "\n")
}

// Matcher reports whether a string contains q, case-insensitively unless q has an uppercase letter.
func Matcher(q string) func(string) bool {
	if strings.ToLower(q) != q {
		return func(s string) bool { return strings.Contains(s, q) }
	}
	return func(s string) bool { return strings.Contains(strings.ToLower(s), q) }
}
