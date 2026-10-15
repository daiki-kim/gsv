// Package config loads gsv's user configuration.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SharedRoot is an extra tree root: a shared drive ID or a folder ID.
type SharedRoot struct {
	Name string `yaml:"name"`
	ID   string `yaml:"id"`
}

type Config struct {
	SharedRoots []SharedRoot `yaml:"shared_roots"`
}

// Dir returns $XDG_CONFIG_HOME/gsv, or ~/.config/gsv when XDG_CONFIG_HOME is unset.
func Dir() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "gsv"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "gsv"), nil
}

// Load reads the YAML config at path. A missing file yields an empty Config.
func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	for i, r := range c.SharedRoots {
		if strings.TrimSpace(r.Name) == "" {
			return Config{}, fmt.Errorf("%s: shared_roots[%d]: name is required", path, i)
		}
		if strings.TrimSpace(r.ID) == "" {
			return Config{}, fmt.Errorf("%s: shared_roots[%d]: id is required", path, i)
		}
	}
	return c, nil
}
