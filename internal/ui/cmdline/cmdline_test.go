package cmdline

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestInputTypingAndSubmit(t *testing.T) {
	var in Input
	in.Start(Command)
	for _, k := range []tea.KeyMsg{runes("e"), {Type: tea.KeySpace, Runes: []rune{' '}}, runes("ab"), {Type: tea.KeyBackspace}} {
		if r, _ := in.Key(k); r != Pending {
			t.Fatalf("unexpected result %v", r)
		}
	}
	if in.View() != ":e a█" {
		t.Fatalf("View = %q", in.View())
	}
	r, s := in.Key(tea.KeyMsg{Type: tea.KeyEnter})
	if r != Submitted || s != "e a" || in.Active() {
		t.Fatalf("r = %v, s = %q, active = %v", r, s, in.Active())
	}
}

func TestInputPasteKeepsAllRunes(t *testing.T) {
	var in Input
	in.Start(Command)
	in.Key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e https://x/y"), Paste: true})
	if _, s := in.Key(tea.KeyMsg{Type: tea.KeyEnter}); s != "e https://x/y" {
		t.Fatalf("s = %q", s)
	}
}

func TestInputCancel(t *testing.T) {
	var in Input
	in.Start(SearchForward)
	if in.View() != "/█" || in.Mode() != SearchForward {
		t.Fatalf("View = %q", in.View())
	}
	if r, _ := in.Key(tea.KeyMsg{Type: tea.KeyEsc}); r != Cancelled || in.Active() {
		t.Fatal("esc cancels")
	}
	in.Start(SearchBackward)
	if r, _ := in.Key(tea.KeyMsg{Type: tea.KeyBackspace}); r != Cancelled {
		t.Fatal("backspace on empty input cancels")
	}
	in.Start(Command)
	in.Key(runes("abc"))
	in.Key(tea.KeyMsg{Type: tea.KeyCtrlU})
	if in.View() != ":█" {
		t.Fatalf("ctrl+u clears: %q", in.View())
	}
}

func TestParse(t *testing.T) {
	tests := map[string]Cmd{
		"q":              {Name: "q", Raw: "q"},
		"  find  売上 管理 ": {Name: "find", Arg: "売上 管理", Raw: "find  売上 管理"},
		"e https://x":    {Name: "e", Arg: "https://x", Raw: "e https://x"},
		"":               {},
	}
	for in, want := range tests {
		if got := Parse(in); got != want {
			t.Errorf("Parse(%q) = %+v, want %+v", in, got, want)
		}
	}
}

func TestParseSpreadsheetID(t *testing.T) {
	const id = "1AbC-dEf_GhIjKlMnOpQrStUvWxYz0123456789"
	ok := []string{
		"https://docs.google.com/spreadsheets/d/" + id + "/edit#gid=0",
		"https://docs.google.com/spreadsheets/d/" + id + "/edit?usp=sharing",
		"https://docs.google.com/spreadsheets/d/" + id,
		"https://docs.google.com/spreadsheets/u/1/d/" + id + "/edit",
		"  " + id + "  ",
	}
	for _, s := range ok {
		if got, found := ParseSpreadsheetID(s); !found || got != id {
			t.Errorf("ParseSpreadsheetID(%q) = %q, %v", s, got, found)
		}
	}
	if got, found := ParseSpreadsheetID("https://docs.google.com/spreadsheets/d/S1/edit"); !found || got != "S1" {
		t.Errorf("short id in URL = %q, %v", got, found)
	}
	for _, s := range []string{"", "hello", "https://example.com/doc", "has space in it but long enough"} {
		if _, found := ParseSpreadsheetID(s); found {
			t.Errorf("ParseSpreadsheetID(%q) should fail", s)
		}
	}
}
