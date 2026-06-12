package graph

import (
	"os"
	"path/filepath"
)

// DefaultDataPath returns the JSONL log path used when KG_DATA is unset.
// Resolution order:
//  1. ./kg.jsonl in the current working directory, if it exists
//  2. ~/.kg/log.jsonl, or legacy ~/.kg/data.json if that is the only one present
func DefaultDataPath() string {
	if p := os.Getenv("KG_DATA"); p != "" {
		return p
	}
	const cwdName = "kg.jsonl"
	if _, err := os.Stat(cwdName); err == nil {
		return cwdName
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return cwdName
	}
	newPath := filepath.Join(home, ".kg", "log.jsonl")
	if _, err := os.Stat(newPath); err == nil {
		return newPath
	}
	legacy := filepath.Join(home, ".kg", "data.json")
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return newPath
}
