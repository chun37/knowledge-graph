package main

import (
	"fmt"
	"sort"
	"strings"
)

type Node struct {
	ID         string            `json:"id"`
	Labels     []string          `json:"labels,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

type Edge struct {
	From       string            `json:"from"`
	Relation   string            `json:"relation"`
	To         string            `json:"to"`
	Properties map[string]string `json:"properties,omitempty"`
}

type Graph struct {
	Nodes map[string]*Node `json:"nodes"`
	Edges []*Edge          `json:"edges"`
}

func NewGraph() *Graph {
	return &Graph{Nodes: map[string]*Node{}, Edges: []*Edge{}}
}

func (g *Graph) AddNode(id string, labels []string, props map[string]string) (*Node, bool) {
	if n, ok := g.Nodes[id]; ok {
		if len(labels) > 0 {
			n.Labels = mergeUnique(n.Labels, labels)
		}
		for k, v := range props {
			if n.Properties == nil {
				n.Properties = map[string]string{}
			}
			n.Properties[k] = v
		}
		return n, false
	}
	n := &Node{ID: id, Labels: labels, Properties: props}
	g.Nodes[id] = n
	return n, true
}

func (g *Graph) AddEdge(from, relation, to string, props map[string]string) (*Edge, error) {
	if _, ok := g.Nodes[from]; !ok {
		g.AddNode(from, nil, nil)
	}
	if _, ok := g.Nodes[to]; !ok {
		g.AddNode(to, nil, nil)
	}
	for _, e := range g.Edges {
		if e.From == from && e.Relation == relation && e.To == to {
			for k, v := range props {
				if e.Properties == nil {
					e.Properties = map[string]string{}
				}
				e.Properties[k] = v
			}
			return e, nil
		}
	}
	e := &Edge{From: from, Relation: relation, To: to, Properties: props}
	g.Edges = append(g.Edges, e)
	return e, nil
}

func (g *Graph) DeleteNode(id string) bool {
	if _, ok := g.Nodes[id]; !ok {
		return false
	}
	delete(g.Nodes, id)
	kept := g.Edges[:0]
	for _, e := range g.Edges {
		if e.From != id && e.To != id {
			kept = append(kept, e)
		}
	}
	g.Edges = kept
	return true
}

func (g *Graph) DeleteEdge(from, relation, to string) bool {
	for i, e := range g.Edges {
		if e.From == from && e.Relation == relation && e.To == to {
			g.Edges = append(g.Edges[:i], g.Edges[i+1:]...)
			return true
		}
	}
	return false
}

// Query returns triples matching the (subject, predicate, object) pattern.
// Empty string in any position acts as a wildcard.
func (g *Graph) Query(subject, predicate, object string) []*Edge {
	var out []*Edge
	for _, e := range g.Edges {
		if subject != "" && e.From != subject {
			continue
		}
		if predicate != "" && e.Relation != predicate {
			continue
		}
		if object != "" && e.To != object {
			continue
		}
		out = append(out, e)
	}
	return out
}

type Direction int

const (
	DirOut Direction = iota
	DirIn
	DirBoth
)

func (g *Graph) Neighbors(id string, dir Direction) []*Edge {
	var out []*Edge
	for _, e := range g.Edges {
		switch dir {
		case DirOut:
			if e.From == id {
				out = append(out, e)
			}
		case DirIn:
			if e.To == id {
				out = append(out, e)
			}
		case DirBoth:
			if e.From == id || e.To == id {
				out = append(out, e)
			}
		}
	}
	return out
}

// FindPath returns the shortest path (sequence of node IDs) from `from` to `to`,
// treating edges as undirected. Returns nil if no path exists.
func (g *Graph) FindPath(from, to string) []string {
	if _, ok := g.Nodes[from]; !ok {
		return nil
	}
	if _, ok := g.Nodes[to]; !ok {
		return nil
	}
	if from == to {
		return []string{from}
	}
	adj := map[string][]string{}
	for _, e := range g.Edges {
		adj[e.From] = append(adj[e.From], e.To)
		adj[e.To] = append(adj[e.To], e.From)
	}
	prev := map[string]string{from: ""}
	queue := []string{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, nxt := range adj[cur] {
			if _, seen := prev[nxt]; seen {
				continue
			}
			prev[nxt] = cur
			if nxt == to {
				path := []string{nxt}
				for p := cur; p != ""; p = prev[p] {
					path = append([]string{p}, path...)
				}
				return path
			}
			queue = append(queue, nxt)
		}
	}
	return nil
}

func (g *Graph) Stats() string {
	rels := map[string]int{}
	for _, e := range g.Edges {
		rels[e.Relation]++
	}
	var rs []string
	for k, v := range rels {
		rs = append(rs, fmt.Sprintf("%s=%d", k, v))
	}
	sort.Strings(rs)
	return fmt.Sprintf("nodes=%d edges=%d relations={%s}", len(g.Nodes), len(g.Edges), strings.Join(rs, ", "))
}

func mergeUnique(a, b []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, x := range append(a, b...) {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func (n *Node) String() string {
	var parts []string
	parts = append(parts, n.ID)
	if len(n.Labels) > 0 {
		parts = append(parts, ":"+strings.Join(n.Labels, ":"))
	}
	if len(n.Properties) > 0 {
		var kv []string
		keys := make([]string, 0, len(n.Properties))
		for k := range n.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			kv = append(kv, fmt.Sprintf("%s=%q", k, n.Properties[k]))
		}
		parts = append(parts, "{"+strings.Join(kv, ", ")+"}")
	}
	return strings.Join(parts, " ")
}

func (e *Edge) String() string {
	s := fmt.Sprintf("(%s) -[%s]-> (%s)", e.From, e.Relation, e.To)
	if len(e.Properties) > 0 {
		var kv []string
		keys := make([]string, 0, len(e.Properties))
		for k := range e.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			kv = append(kv, fmt.Sprintf("%s=%q", k, e.Properties[k]))
		}
		s += " {" + strings.Join(kv, ", ") + "}"
	}
	return s
}
