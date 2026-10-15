package keymap

import (
	"reflect"
	"testing"
)

func feed(p *Parser, keys ...string) []Action {
	var out []Action
	for _, k := range keys {
		if a, ok := p.Feed(k); ok {
			out = append(out, a)
		}
	}
	return out
}

func one(k Kind) Action { return Action{Kind: k, Count: 1} }

func TestFeed(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want []Action
	}{
		{"j", []string{"j"}, []Action{one(Down)}},
		{"count", []string{"5", "j"}, []Action{{Kind: Down, Count: 5, HasCount: true}}},
		{"count with zero", []string{"1", "0", "k"}, []Action{{Kind: Up, Count: 10, HasCount: true}}},
		{"zero is line start", []string{"0"}, []Action{one(LineStart)}},
		{"gg", []string{"g", "g"}, []Action{one(Top)}},
		{"5gg", []string{"5", "g", "g"}, []Action{{Kind: Top, Count: 5, HasCount: true}}},
		{"G", []string{"G"}, []Action{one(Bottom)}},
		{"gt", []string{"g", "t"}, []Action{one(NextTab)}},
		{"3gt", []string{"3", "g", "t"}, []Action{{Kind: NextTab, Count: 3, HasCount: true}}},
		{"gT", []string{"g", "T"}, []Action{one(PrevTab)}},
		{"g then unknown resets", []string{"g", "x", "j"}, []Action{one(Down)}},
		{"ctrl+w l", []string{"ctrl+w", "l"}, []Action{one(PaneRight)}},
		{"ctrl+w h", []string{"ctrl+w", "h"}, []Action{one(PaneLeft)}},
		{"ctrl+w w", []string{"ctrl+w", "w"}, []Action{one(PaneNext)}},
		{"ctrl+w ctrl+w", []string{"ctrl+w", "ctrl+w"}, []Action{one(PaneNext)}},
		{"esc clears count", []string{"5", "esc", "j"}, []Action{one(Cancel), one(Down)}},
		{"unknown key clears count", []string{"5", "x", "j"}, []Action{one(Down)}},
		{"arrows", []string{"down", "up", "left", "right"}, []Action{one(Down), one(Up), one(Left), one(Right)}},
		{"hl", []string{"h", "l"}, []Action{one(Left), one(Right)}},
		{"singles",
			[]string{"$", "ctrl+d", "ctrl+u", "ctrl+n", "enter", "K", "r", ":", "/", "?", "n", "N"},
			[]Action{one(LineEnd), one(HalfPageDown), one(HalfPageUp), one(ToggleTree), one(Open), one(Peek),
				one(Reload), one(CommandMode), one(SearchForward), one(SearchBackward), one(NextMatch), one(PrevMatch)}},
		{"huge count is capped", []string{"9", "9", "9", "9", "9", "9", "9", "j"}, []Action{{Kind: Down, Count: 99999, HasCount: true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p Parser
			got := feed(&p, tt.keys...)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPending(t *testing.T) {
	var p Parser
	p.Feed("5")
	p.Feed("g")
	if got := p.Pending(); got != "5g" {
		t.Fatalf("Pending = %q", got)
	}
	p.Feed("g")
	if got := p.Pending(); got != "" {
		t.Fatalf("Pending after gg = %q", got)
	}
	p.Feed("ctrl+w")
	if got := p.Pending(); got != "^W" {
		t.Fatalf("Pending = %q", got)
	}
}
