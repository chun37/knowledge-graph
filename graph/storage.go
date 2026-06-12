package graph

import (
	"os"
	"path/filepath"
)

// DefaultDataPath returns the JSONL log path used when KG_DATA is unset.
// If the new log.jsonl does not yet exist but a legacy data.json does, the
// legacy path is returned so Open can migrate it on first access.
func DefaultDataPath() string {
	if p := os.Getenv("KG_DATA"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "kg-log.jsonl"
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
