package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/chun37/knowledge-graph/graph"
)

const usage = `kg - a small knowledge graph CLI

Usage:
  kg <command> [args...]

Commands:
  add-node <id> [--label L]... [--prop k=v]...
      Add or update a node. Repeated --label and --prop are accumulated.

  add-edge <from> <relation> <to> [--prop k=v]...
      Add a directed edge (triple). Creates endpoints if missing.

  add-triple <subject> <predicate> <object>
      Alias for add-edge.

  delete-node <id>
      Delete a node and all its incident edges.

  delete-edge <from> <relation> <to>
      Delete a single edge.

  show <id>
      Print a node with its labels, properties, outgoing and incoming edges.

  list-nodes [--label L]
      List nodes, optionally filtered by label.

  list-edges [--relation R]
      List edges, optionally filtered by relation.

  query [--subject S] [--predicate P] [--object O]
      SPARQL-lite triple query. Omit a flag to treat it as wildcard.

  neighbors <id> [--direction out|in|both]
      Show edges touching a node. Default: both.

  path <from> <to>
      Shortest path between two nodes (undirected BFS).

  stats
      Print counts of nodes, edges, and relations.

  export [--format json|triples]
      Dump the whole graph to stdout. Default: json.

  compact
      Rewrite the JSONL log from the current state. Removes tombstones and
      overwritten records, shrinking the file. Replay time also drops.

  help
      Show this help.

Storage:
  Each mutation appends one JSON object to a log file (JSONL). Reads replay
  the log into memory. Run "kg compact" to collapse the log to a minimal
  snapshot when it grows large.

Environment:
  KG_DATA   Path to the JSONL log. If unset, kg uses ./kg.jsonl when it
            exists in the current directory, otherwise ~/.kg/log.jsonl.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		fmt.Print(usage)
		return
	}

	path := graph.DefaultDataPath()
	s, err := graph.Open(path)
	if err != nil {
		die("open: %v", err)
	}

	switch cmd {
	case "add-node":
		cmdAddNode(s, args)
	case "add-edge", "add-triple":
		cmdAddEdge(s, args)
	case "delete-node":
		cmdDeleteNode(s, args)
	case "delete-edge":
		cmdDeleteEdge(s, args)
	case "show":
		cmdShow(s.G, args)
	case "list-nodes":
		cmdListNodes(s.G, args)
	case "list-edges":
		cmdListEdges(s.G, args)
	case "query":
		cmdQuery(s.G, args)
	case "neighbors":
		cmdNeighbors(s.G, args)
	case "path":
		cmdPath(s.G, args)
	case "stats":
		fmt.Println(s.G.Stats())
	case "export":
		cmdExport(s.G, args)
	case "compact":
		cmdCompact(s, args)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", a...)
	os.Exit(1)
}

// parseFlags walks args and extracts repeated --label / --prop and named flags.
// Positional args are returned in order. Unknown flags are an error.
func parseFlags(args []string, spec map[string]flagKind) ([]string, map[string][]string, error) {
	pos := []string{}
	got := map[string][]string{}
	i := 0
	for i < len(args) {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			pos = append(pos, a)
			i++
			continue
		}
		name := strings.TrimPrefix(a, "--")
		var value string
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			value = name[eq+1:]
			name = name[:eq]
		} else {
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("flag --%s needs a value", name)
			}
			value = args[i+1]
			i++
		}
		kind, ok := spec[name]
		if !ok {
			return nil, nil, fmt.Errorf("unknown flag --%s", name)
		}
		if kind == flagSingle && len(got[name]) > 0 {
			return nil, nil, fmt.Errorf("flag --%s repeated", name)
		}
		got[name] = append(got[name], value)
		i++
	}
	return pos, got, nil
}

type flagKind int

const (
	flagSingle flagKind = iota
	flagRepeated
)

func parseProps(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := map[string]string{}
	for _, v := range values {
		eq := strings.IndexByte(v, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("bad --prop %q (want key=value)", v)
		}
		out[v[:eq]] = v[eq+1:]
	}
	return out, nil
}

func cmdAddNode(s *graph.Store, args []string) {
	pos, flags, err := parseFlags(args, map[string]flagKind{
		"label": flagRepeated,
		"prop":  flagRepeated,
	})
	if err != nil {
		die("%v", err)
	}
	if len(pos) != 1 {
		die("add-node needs exactly one <id>")
	}
	props, err := parseProps(flags["prop"])
	if err != nil {
		die("%v", err)
	}
	n, created, err := s.AddNode(pos[0], flags["label"], props)
	if err != nil {
		die("write log: %v", err)
	}
	verb := "updated"
	if created {
		verb = "added"
	}
	fmt.Printf("%s: %s\n", verb, n)
}

func cmdAddEdge(s *graph.Store, args []string) {
	pos, flags, err := parseFlags(args, map[string]flagKind{
		"prop": flagRepeated,
	})
	if err != nil {
		die("%v", err)
	}
	if len(pos) != 3 {
		die("add-edge needs <from> <relation> <to>")
	}
	props, err := parseProps(flags["prop"])
	if err != nil {
		die("%v", err)
	}
	e, err := s.AddEdge(pos[0], pos[1], pos[2], props)
	if err != nil {
		die("%v", err)
	}
	fmt.Printf("added: %s\n", e)
}

func cmdDeleteNode(s *graph.Store, args []string) {
	if len(args) != 1 {
		die("delete-node needs <id>")
	}
	ok, err := s.DeleteNode(args[0])
	if err != nil {
		die("write log: %v", err)
	}
	if !ok {
		die("no such node: %s", args[0])
	}
	fmt.Printf("deleted node: %s\n", args[0])
}

func cmdDeleteEdge(s *graph.Store, args []string) {
	if len(args) != 3 {
		die("delete-edge needs <from> <relation> <to>")
	}
	ok, err := s.DeleteEdge(args[0], args[1], args[2])
	if err != nil {
		die("write log: %v", err)
	}
	if !ok {
		die("no such edge")
	}
	fmt.Printf("deleted edge: (%s) -[%s]-> (%s)\n", args[0], args[1], args[2])
}

func cmdShow(g *graph.Graph, args []string) {
	if len(args) != 1 {
		die("show needs <id>")
	}
	n, ok := g.Nodes[args[0]]
	if !ok {
		die("no such node: %s", args[0])
	}
	fmt.Println(n)
	outs := g.Neighbors(n.ID, graph.DirOut)
	ins := g.Neighbors(n.ID, graph.DirIn)
	if len(outs) > 0 {
		fmt.Println("  outgoing:")
		for _, e := range outs {
			fmt.Printf("    -[%s]-> %s\n", e.Relation, e.To)
		}
	}
	if len(ins) > 0 {
		fmt.Println("  incoming:")
		for _, e := range ins {
			fmt.Printf("    %s -[%s]->\n", e.From, e.Relation)
		}
	}
}

func cmdListNodes(g *graph.Graph, args []string) {
	_, flags, err := parseFlags(args, map[string]flagKind{"label": flagSingle})
	if err != nil {
		die("%v", err)
	}
	want := ""
	if v := flags["label"]; len(v) > 0 {
		want = v[0]
	}
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		n := g.Nodes[id]
		if want != "" {
			ok := false
			for _, l := range n.Labels {
				if l == want {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		fmt.Println(n)
	}
}

func cmdListEdges(g *graph.Graph, args []string) {
	_, flags, err := parseFlags(args, map[string]flagKind{"relation": flagSingle})
	if err != nil {
		die("%v", err)
	}
	want := ""
	if v := flags["relation"]; len(v) > 0 {
		want = v[0]
	}
	for _, e := range g.Edges {
		if want != "" && e.Relation != want {
			continue
		}
		fmt.Println(e)
	}
}

func cmdQuery(g *graph.Graph, args []string) {
	_, flags, err := parseFlags(args, map[string]flagKind{
		"subject":   flagSingle,
		"predicate": flagSingle,
		"object":    flagSingle,
	})
	if err != nil {
		die("%v", err)
	}
	first := func(k string) string {
		if v := flags[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	res := g.Query(first("subject"), first("predicate"), first("object"))
	if len(res) == 0 {
		fmt.Println("(no matches)")
		return
	}
	for _, e := range res {
		fmt.Println(e)
	}
}

func cmdNeighbors(g *graph.Graph, args []string) {
	pos, flags, err := parseFlags(args, map[string]flagKind{"direction": flagSingle})
	if err != nil {
		die("%v", err)
	}
	if len(pos) != 1 {
		die("neighbors needs <id>")
	}
	dir := graph.DirBoth
	if v := flags["direction"]; len(v) > 0 {
		switch v[0] {
		case "out":
			dir = graph.DirOut
		case "in":
			dir = graph.DirIn
		case "both":
			dir = graph.DirBoth
		default:
			die("--direction must be one of: out, in, both")
		}
	}
	if _, ok := g.Nodes[pos[0]]; !ok {
		die("no such node: %s", pos[0])
	}
	res := g.Neighbors(pos[0], dir)
	if len(res) == 0 {
		fmt.Println("(no neighbors)")
		return
	}
	for _, e := range res {
		fmt.Println(e)
	}
}

func cmdPath(g *graph.Graph, args []string) {
	if len(args) != 2 {
		die("path needs <from> <to>")
	}
	p := g.FindPath(args[0], args[1])
	if p == nil {
		fmt.Println("(no path)")
		return
	}
	fmt.Println(strings.Join(p, " -> "))
}

func cmdExport(g *graph.Graph, args []string) {
	_, flags, err := parseFlags(args, map[string]flagKind{"format": flagSingle})
	if err != nil {
		die("%v", err)
	}
	format := "json"
	if v := flags["format"]; len(v) > 0 {
		format = v[0]
	}
	switch format {
	case "json":
		b, err := json.MarshalIndent(g, "", "  ")
		if err != nil {
			die("marshal: %v", err)
		}
		fmt.Println(string(b))
	case "triples":
		for _, e := range g.Edges {
			fmt.Printf("%s\t%s\t%s\n", e.From, e.Relation, e.To)
		}
	default:
		die("unknown format: %s (want json or triples)", format)
	}
}

func cmdCompact(s *graph.Store, args []string) {
	if len(args) != 0 {
		die("compact takes no arguments")
	}
	oldSize, newSize, err := s.Compact()
	if err != nil {
		die("compact: %v", err)
	}
	if oldSize > 0 {
		ratio := 100.0 * float64(newSize) / float64(oldSize)
		fmt.Printf("compacted: %d -> %d bytes (%.1f%% of original)\n", oldSize, newSize, ratio)
	} else {
		fmt.Printf("compacted: %d bytes (no prior file)\n", newSize)
	}
}
