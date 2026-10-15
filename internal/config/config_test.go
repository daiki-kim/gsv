package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(c.SharedRoots) != 0 {
		t.Fatalf("roots = %v", c.SharedRoots)
	}
}

func TestLoadEmptyFileIsEmpty(t *testing.T) {
	c, err := Load(write(t, ""))
	if err != nil || len(c.SharedRoots) != 0 {
		t.Fatalf("c = %v, err = %v", c, err)
	}
}

func TestLoadValid(t *testing.T) {
	c, err := Load(write(t, "shared_roots:\n  - name: 営業部\n    id: 0AbC\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []SharedRoot{{Name: "営業部", ID: "0AbC"}}
	if len(c.SharedRoots) != 1 || c.SharedRoots[0] != want[0] {
		t.Fatalf("roots = %v", c.SharedRoots)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name, body, wantSub string
	}{
		{"syntax", "shared_roots:\n  - name: [\n", "line"},
		{"missing name", "shared_roots:\n  - id: X\n", "shared_roots[0]: name is required"},
		{"missing id", "shared_roots:\n  - name: X\n", "shared_roots[0]: id is required"},
		{"unknown field", "shared_root:\n  - name: X\n", "field shared_root not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(write(t, tt.body))
			if err == nil || !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantSub)
			}
		})
	}
}

func TestDirUsesXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	d, err := Dir()
	if err != nil || d != "/tmp/xdg/gsv" {
		t.Fatalf("Dir() = %q, %v", d, err)
	}
}

func TestDirDefault(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/me")
	d, err := Dir()
	if err != nil || d != "/home/me/.config/gsv" {
		t.Fatalf("Dir() = %q, %v", d, err)
	}
}
