// Package keymap turns vim-style key sequences into pane-independent actions.
package keymap

import "strconv"

type Kind int

const (
	None Kind = iota
	Up
	Down
	Left
	Right
	Top
	Bottom
	LineStart
	LineEnd
	HalfPageDown
	HalfPageUp
	NextTab
	PrevTab
	PaneLeft
	PaneRight
	PaneNext
	ToggleTree
	Open
	Peek
	Reload
	CommandMode
	SearchForward
	SearchBackward
	NextMatch
	PrevMatch
	Cancel
)

// Action is a completed key sequence. Count is at least 1; HasCount reports
// whether the user typed one explicitly.
type Action struct {
	Kind     Kind
	Count    int
	HasCount bool
}

const maxCount = 99999

var single = map[string]Kind{
	"j": Down, "down": Down,
	"k": Up, "up": Up,
	"h": Left, "left": Left,
	"l": Right, "right": Right,
	"G": Bottom, "0": LineStart, "$": LineEnd,
	"ctrl+d": HalfPageDown, "ctrl+u": HalfPageUp,
	"ctrl+n": ToggleTree,
	"enter":  Open, "K": Peek, "r": Reload,
	":": CommandMode, "/": SearchForward, "?": SearchBackward,
	"n": NextMatch, "N": PrevMatch,
}

// Parser accumulates counts and multi-key prefixes (g, ctrl+w). The zero value is ready to use.
type Parser struct {
	count   int
	pending string
}

// Feed consumes one key and returns the completed action, if any.
func (p *Parser) Feed(key string) (Action, bool) {
	if key == "esc" {
		p.reset()
		return Action{Kind: Cancel, Count: 1}, true
	}
	switch p.pending {
	case "g":
		switch key {
		case "g":
			return p.emit(Top)
		case "t":
			return p.emit(NextTab)
		case "T":
			return p.emit(PrevTab)
		}
		p.reset()
		return Action{}, false
	case "ctrl+w":
		switch key {
		case "h", "left", "ctrl+h":
			return p.emit(PaneLeft)
		case "l", "right", "ctrl+l":
			return p.emit(PaneRight)
		case "w", "ctrl+w":
			return p.emit(PaneNext)
		}
		p.reset()
		return Action{}, false
	}
	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' && (key != "0" || p.count > 0) {
		if next := p.count*10 + int(key[0]-'0'); next <= maxCount {
			p.count = next
		} else {
			p.count = maxCount
		}
		return Action{}, false
	}
	if key == "g" || key == "ctrl+w" {
		p.pending = key
		return Action{}, false
	}
	if k, ok := single[key]; ok {
		return p.emit(k)
	}
	p.reset()
	return Action{}, false
}

// Pending returns the in-progress sequence, e.g. "5g" or "^W".
func (p *Parser) Pending() string {
	s := ""
	if p.count > 0 {
		s = strconv.Itoa(p.count)
	}
	switch p.pending {
	case "g":
		s += "g"
	case "ctrl+w":
		s += "^W"
	}
	return s
}

func (p *Parser) emit(k Kind) (Action, bool) {
	a := Action{Kind: k, Count: max(p.count, 1), HasCount: p.count > 0}
	p.reset()
	return a, true
}

func (p *Parser) reset() {
	p.count = 0
	p.pending = ""
}
