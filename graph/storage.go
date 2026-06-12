package graph

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func DefaultDataPath() string {
	if p := os.Getenv("KG_DATA"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "kg-data.json"
	}
	return filepath.Join(home, ".kg", "data.json")
}

func Load(path string) (*Graph, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return New(), nil
		}
		return nil, err
	}
	g := New()
	if len(b) == 0 {
		return g, nil
	}
	if err := json.Unmarshal(b, g); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if g.Nodes == nil {
		g.Nodes = map[string]*Node{}
	}
	return g, nil
}

func Save(path string, g *Graph) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
