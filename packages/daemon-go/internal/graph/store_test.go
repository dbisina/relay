// internal/graph/store_test.go — knowledge-graph store behaviour.
package graph

import (
	"context"
	"path/filepath"
	"testing"
)

// TestRecentIncludesCrossTypeEdges guards the fix for the Graph page rendering
// an edgeless dust cloud: edges are frequently cross-type (module→symbol
// "defines"), so an edge often straddles the recent-node window. Recent must
// still return those edges and pull in the missing endpoint node, rather than
// dropping every edge whose other end fell outside the window.
func TestRecentIncludesCrossTypeEdges(t *testing.T) {
	dir := t.TempDir()
	g, err := Open(filepath.Join(dir, "graph.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer g.Close()

	// One module node that defines many symbol nodes. The symbols are inserted
	// last so they dominate the "most recent" window; the module falls outside
	// a small recent limit.
	mod := "module:pkg/foo.go"
	if err := g.UpsertNode(Node{ID: mod, NodeType: "module", SessionID: "s", Weight: 1}); err != nil {
		t.Fatalf("upsert module: %v", err)
	}
	symbols := []string{"symbol:pkg/foo.go:A", "symbol:pkg/foo.go:B", "symbol:pkg/foo.go:C"}
	for _, s := range symbols {
		if err := g.UpsertNode(Node{ID: s, NodeType: "symbol", SessionID: "s", Weight: 0.8}); err != nil {
			t.Fatalf("upsert symbol: %v", err)
		}
		if err := g.UpsertEdge(Edge{FromID: mod, ToID: s, EdgeType: "defines", Weight: 1}); err != nil {
			t.Fatalf("upsert edge: %v", err)
		}
	}

	// Recent window smaller than the total node count, so the module is not in
	// the initial recent set — only the symbols are.
	nb, err := g.Recent(context.Background(), 3)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}

	if len(nb.Edges) != len(symbols) {
		t.Fatalf("expected %d edges, got %d (cross-type edges dropped?)", len(symbols), len(nb.Edges))
	}

	// The missing module endpoint must be pulled into the node set so the edges
	// have both endpoints to draw between.
	var haveModule bool
	for _, n := range nb.Nodes {
		if n.ID == mod {
			haveModule = true
		}
	}
	if !haveModule {
		t.Fatalf("module endpoint not pulled into neighborhood; edges would dangle")
	}
}
