package graph

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

const (
	OpNode    = "node"
	OpEdge    = "edge"
	OpDelNode = "del-node"
	OpDelEdge = "del-edge"
)

// Op is one entry in the append-only JSONL log. Exactly one Op corresponds
// to one user-visible mutation.
type Op struct {
	Op         string            `json:"op"`
	ID         string            `json:"id,omitempty"`
	Labels     []string          `json:"labels,omitempty"`
	From       string            `json:"from,omitempty"`
	Relation   string            `json:"relation,omitempty"`
	To         string            `json:"to,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// Apply executes op against g. Used both for live writes (after appending to
// the log) and for replay during Open.
func (g *Graph) Apply(op Op) {
	switch op.Op {
	case OpNode:
		g.AddNode(op.ID, op.Labels, op.Properties)
	case OpEdge:
		g.AddEdge(op.From, op.Relation, op.To, op.Properties)
	case OpDelNode:
		g.DeleteNode(op.ID)
	case OpDelEdge:
		g.DeleteEdge(op.From, op.Relation, op.To)
	}
}

// Store wraps an in-memory Graph and an append-only JSONL log on disk.
// Mutating methods both update G and append a single op to the log.
type Store struct {
	path string
	G    *Graph
}

// Open reads the append-only log at path. If the file is a legacy whole-file
// JSON document (the pre-JSONL format), it is migrated in place: the previous
// contents are saved to path+".bak" and a fresh JSONL log is written.
func Open(path string) (*Store, error) {
	s := &Store{path: path, G: New()}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return s, nil
	}
	legacy, err := looksLikeLegacyJSON(b)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if legacy {
		if err := s.migrate(b); err != nil {
			return nil, fmt.Errorf("migrate %s: %w", path, err)
		}
		fmt.Fprintf(os.Stderr, "kg: migrated %s from JSON to JSONL (backup: %s.bak)\n", path, path)
		return s, nil
	}
	if err := s.replay(b); err != nil {
		return nil, fmt.Errorf("replay %s: %w", path, err)
	}
	return s, nil
}

// looksLikeLegacyJSON returns true if b is the old whole-file Graph JSON,
// false if it is JSONL (or empty), and an error if neither.
func looksLikeLegacyJSON(b []byte) (bool, error) {
	nl := bytes.IndexByte(b, '\n')
	firstLine := bytes.TrimSpace(b)
	if nl >= 0 {
		firstLine = bytes.TrimSpace(b[:nl])
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(firstLine, &probe); err != nil {
		// The first line alone may not be valid (pretty-printed JSON starts
		// with `{` on its own line). Fall back to parsing the whole file.
		if err2 := json.Unmarshal(b, &probe); err2 != nil {
			return false, fmt.Errorf("not JSONL and not legacy JSON: %w", err2)
		}
	}
	if _, ok := probe["op"]; ok {
		return false, nil
	}
	if _, ok := probe["nodes"]; ok {
		return true, nil
	}
	return false, fmt.Errorf("unrecognized JSON document")
}

func (s *Store) replay(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	for {
		var op Op
		err := dec.Decode(&op)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		s.G.Apply(op)
	}
	return nil
}

func (s *Store) migrate(b []byte) error {
	var legacy struct {
		Nodes map[string]*Node `json:"nodes"`
		Edges []*Edge          `json:"edges"`
	}
	if err := json.Unmarshal(b, &legacy); err != nil {
		return err
	}
	ids := make([]string, 0, len(legacy.Nodes))
	for id := range legacy.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		n := legacy.Nodes[id]
		s.G.AddNode(id, n.Labels, n.Properties)
	}
	for _, e := range legacy.Edges {
		s.G.AddEdge(e.From, e.Relation, e.To, e.Properties)
	}
	if err := os.Rename(s.path, s.path+".bak"); err != nil {
		return err
	}
	_, _, err := s.Compact()
	return err
}

// append serializes op and appends it to the log with fsync.
func (s *Store) append(op Op) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(op)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if _, err := f.Write(b); err != nil {
		return err
	}
	return f.Sync()
}

func (s *Store) AddNode(id string, labels []string, props map[string]string) (*Node, bool, error) {
	n, created := s.G.AddNode(id, labels, props)
	if err := s.append(Op{Op: OpNode, ID: id, Labels: labels, Properties: props}); err != nil {
		return n, created, err
	}
	return n, created, nil
}

func (s *Store) AddEdge(from, relation, to string, props map[string]string) (*Edge, error) {
	e, err := s.G.AddEdge(from, relation, to, props)
	if err != nil {
		return nil, err
	}
	return e, s.append(Op{Op: OpEdge, From: from, Relation: relation, To: to, Properties: props})
}

func (s *Store) DeleteNode(id string) (bool, error) {
	if !s.G.DeleteNode(id) {
		return false, nil
	}
	return true, s.append(Op{Op: OpDelNode, ID: id})
}

func (s *Store) DeleteEdge(from, relation, to string) (bool, error) {
	if !s.G.DeleteEdge(from, relation, to) {
		return false, nil
	}
	return true, s.append(Op{Op: OpDelEdge, From: from, Relation: relation, To: to})
}

// Compact rewrites the log from the current in-memory state. The new file
// contains one OpNode per node and one OpEdge per edge, with no del-* records,
// so its size reflects the live graph rather than the accumulated edit history.
// The rewrite is atomic via tmp file + rename.
func (s *Store) Compact() (oldSize, newSize int64, err error) {
	if info, statErr := os.Stat(s.path); statErr == nil {
		oldSize = info.Size()
	}
	if err = os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return
	}
	tmp := s.path + ".compact"
	f, err := os.Create(tmp)
	if err != nil {
		return
	}
	cleanup := func() {
		f.Close()
		os.Remove(tmp)
	}
	enc := json.NewEncoder(f)
	ids := make([]string, 0, len(s.G.Nodes))
	for id := range s.G.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		n := s.G.Nodes[id]
		if err = enc.Encode(Op{Op: OpNode, ID: n.ID, Labels: n.Labels, Properties: n.Properties}); err != nil {
			cleanup()
			return
		}
	}
	for _, e := range s.G.Edges {
		if err = enc.Encode(Op{Op: OpEdge, From: e.From, Relation: e.Relation, To: e.To, Properties: e.Properties}); err != nil {
			cleanup()
			return
		}
	}
	if err = f.Sync(); err != nil {
		cleanup()
		return
	}
	if err = f.Close(); err != nil {
		os.Remove(tmp)
		return
	}
	if err = os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return
	}
	if info, statErr := os.Stat(s.path); statErr == nil {
		newSize = info.Size()
	}
	return
}
