package text

import (
	"reflect"
	"testing"
)

func TestWidthIgnoresEastAsianEnv(t *testing.T) {
	t.Setenv("RUNEWIDTH_EASTASIAN", "1")
	t.Setenv("LANG", "ja_JP.UTF-8")
	if got := Width("…│─┼▸▾▤"); got != 7 {
		t.Fatalf("Width = %d, want 7", got)
	}
	if got := Width("日本"); got != 4 {
		t.Fatalf("Width(日本) = %d", got)
	}
}

func TestFit(t *testing.T) {
	tests := []struct {
		in   string
		w    int
		want string
	}{
		{"abc", 5, "abc  "},
		{"abcdef", 5, "abcd…"},
		{"a\nb\tc", 6, "a b c "},
		{"XYZ株式会社 東京本社", 11, "XYZ株式会… "},
		{"日本", 1, "…"},
		{"x", 0, ""},
	}
	for _, tt := range tests {
		if got := Fit(tt.in, tt.w); got != tt.want {
			t.Errorf("Fit(%q, %d) = %q, want %q", tt.in, tt.w, got, tt.want)
		}
		if tt.w > 0 && Width(Fit(tt.in, tt.w)) != tt.w {
			t.Errorf("Width(Fit(%q, %d)) != %d", tt.in, tt.w, tt.w)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := Truncate("short", 10); got != "short" {
		t.Errorf("got %q", got)
	}
	if got := Truncate("東京本社ビル", 7); got != "東京本…" {
		t.Errorf("got %q", got)
	}
}

func TestWrap(t *testing.T) {
	got := Wrap("abcdef\n東京本社", 4)
	want := []string{"abcd", "ef", "東京", "本社"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Wrap = %q", got)
	}
	if got := Wrap("", 4); !reflect.DeepEqual(got, []string{""}) {
		t.Fatalf("Wrap(\"\") = %q", got)
	}
}

func TestBlock(t *testing.T) {
	got := Block([]string{"hello world", "x"}, 5, 3)
	if got != "hell…\nx    \n     " {
		t.Fatalf("Block = %q", got)
	}
}

func TestMatcherSmartcase(t *testing.T) {
	lower := Matcher("apple")
	if !lower("Big APPLE") || lower("pear") {
		t.Error("lowercase query must be case-insensitive")
	}
	upper := Matcher("Apple")
	if upper("apple") || !upper("An Apple") {
		t.Error("query with uppercase must be case-sensitive")
	}
}
