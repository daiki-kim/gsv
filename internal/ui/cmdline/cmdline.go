// Package cmdline implements the bottom-line ":" / "/" / "?" input and command parsing.
package cmdline

import (
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type Mode int

const (
	Command Mode = iota
	SearchForward
	SearchBackward
)

type Result int

const (
	Pending Result = iota
	Submitted
	Cancelled
)

type Input struct {
	active bool
	mode   Mode
	buf    []rune
}

func (in *Input) Start(mode Mode) { in.active, in.mode, in.buf = true, mode, nil }
func (in *Input) Active() bool    { return in.active }
func (in *Input) Mode() Mode      { return in.mode }

// Key edits the line. On Submitted it returns the typed text without the prefix.
func (in *Input) Key(msg tea.KeyMsg) (Result, string) {
	switch msg.Type {
	case tea.KeyEnter:
		in.active = false
		return Submitted, string(in.buf)
	case tea.KeyEsc, tea.KeyCtrlC:
		in.active = false
		return Cancelled, ""
	case tea.KeyBackspace:
		if len(in.buf) == 0 {
			in.active = false
			return Cancelled, ""
		}
		in.buf = in.buf[:len(in.buf)-1]
	case tea.KeyCtrlU:
		in.buf = nil
	case tea.KeySpace:
		in.buf = append(in.buf, ' ')
	case tea.KeyRunes:
		in.buf = append(in.buf, msg.Runes...)
	}
	return Pending, ""
}

func (in *Input) View() string {
	prefix := ":"
	switch in.mode {
	case SearchForward:
		prefix = "/"
	case SearchBackward:
		prefix = "?"
	}
	return prefix + string(in.buf) + "█"
}

// Cmd is a parsed ex command: Name is the first word, Arg the trimmed rest, Raw the whole trimmed line.
type Cmd struct {
	Name, Arg, Raw string
}

func Parse(s string) Cmd {
	raw := strings.TrimSpace(s)
	name, arg, _ := strings.Cut(raw, " ")
	return Cmd{Name: name, Arg: strings.TrimSpace(arg), Raw: raw}
}

var (
	urlID  = regexp.MustCompile(`/spreadsheets/(?:u/\d+/)?d/([A-Za-z0-9_-]+)`)
	bareID = regexp.MustCompile(`^[A-Za-z0-9_-]{20,}$`)
)

// ParseSpreadsheetID accepts a Sheets URL or a bare spreadsheet ID.
func ParseSpreadsheetID(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if m := urlID.FindStringSubmatch(s); m != nil {
		return m[1], true
	}
	if bareID.MatchString(s) {
		return s, true
	}
	return "", false
}
