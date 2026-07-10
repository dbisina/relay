// internal/graph/store.go
//
// GraphStore — SQLite WAL-mode knowledge graph.
//
// Schema:
//   nodes(id TEXT PK, nodeType TEXT, payload JSON, sessionId TEXT, weight REAL, createdAt INT, updatedAt INT)
//   edges(fromId TEXT, toId TEXT, edgeType TEXT, weight REAL, createdAt INT, PRIMARY KEY(fromId, toId, edgeType))
//
// WriteQueue: single-writer goroutine via channel (SQLite WAL allows many
// readers + one writer without reader starvation).
//
// getNeighborhood: recursive CTE, 1-3 hops, no raw SQL in application code.

package graph

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite, no CGO required
)

const schema = `
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
PRAGMA synchronous = NORMAL;

CREATE TABLE IF NOT EXISTS nodes (
	id        TEXT PRIMARY KEY,
	nodeType  TEXT NOT NULL,
	payload   TEXT NOT NULL DEFAULT '{}',
	sessionId TEXT NOT NULL,
	weight    REAL NOT NULL DEFAULT 1.0,
	createdAt INTEGER NOT NULL,
	updatedAt INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS edges (
	fromId    TEXT NOT NULL,
	toId      TEXT NOT NULL,
	edgeType  TEXT NOT NULL,
	weight    REAL NOT NULL DEFAULT 1.0,
	createdAt INTEGER NOT NULL,
	PRIMARY KEY (fromId, toId, edgeType)
);

CREATE INDEX IF NOT EXISTS idx_nodes_session ON nodes(sessionId);
CREATE INDEX IF NOT EXISTS idx_nodes_type ON nodes(nodeType);

-- chunks: source-code text snippets keyed by file:line range, optionally
-- paired with a vector embedding for retrieval. Vectors are stored as
-- packed float32 blobs; nil = lexical-only retrieval.
CREATE TABLE IF NOT EXISTS chunks (
	id          TEXT PRIMARY KEY,
	path        TEXT NOT NULL,
	lang        TEXT NOT NULL DEFAULT '',
	startLine   INTEGER NOT NULL DEFAULT 0,
	endLine     INTEGER NOT NULL DEFAULT 0,
	body        TEXT NOT NULL,
	sha         TEXT NOT NULL DEFAULT '',
	embedding   BLOB,
	dim         INTEGER NOT NULL DEFAULT 0,
	createdAt   INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_chunks_path ON chunks(path);

-- Full-text search over chunk bodies. Optional fallback when no embeddings.
CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
	body, path, content='chunks', content_rowid='rowid'
);
CREATE TRIGGER IF NOT EXISTS chunks_ai AFTER INSERT ON chunks BEGIN
	INSERT INTO chunks_fts(rowid, body, path) VALUES (new.rowid, new.body, new.path);
END;
CREATE TRIGGER IF NOT EXISTS chunks_ad AFTER DELETE ON chunks BEGIN
	DELETE FROM chunks_fts WHERE rowid = old.rowid;
END;
CREATE INDEX IF NOT EXISTS idx_edges_from    ON edges(fromId);
CREATE INDEX IF NOT EXISTS idx_edges_to      ON edges(toId);
`

// Node represents a knowledge graph node.
type Node struct {
	ID        string
	NodeType  string
	Payload   map[string]interface{}
	SessionID string
	Weight    float64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Edge represents a directed edge between nodes.
type Edge struct {
	FromID    string
	ToID      string
	EdgeType  string
	Weight    float64
	CreatedAt time.Time
}

// Neighborhood is the result of a recursive CTE hop query.
type Neighborhood struct {
	Nodes []Node
	Edges []Edge
}

// Chunk — a retrievable source-code snippet.
type Chunk struct {
	ID        string
	Path      string
	Lang      string
	StartLine int
	EndLine   int
	Body      string
	SHA       string
	// Embedding is float32 packed little-endian. Empty = lexical-only.
	Embedding []byte
	Dim       int
}

// UpsertChunk inserts or replaces a chunk by ID. Triggers update chunks_fts.
func (s *GraphStore) UpsertChunk(c Chunk) error {
	if c.ID == "" || c.Body == "" {
		return fmt.Errorf("chunk: missing id or body")
	}
	_, err := s.db.Exec(`
		INSERT INTO chunks (id, path, lang, startLine, endLine, body, sha, embedding, dim, createdAt)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			path=excluded.path, lang=excluded.lang,
			startLine=excluded.startLine, endLine=excluded.endLine,
			body=excluded.body, sha=excluded.sha,
			embedding=excluded.embedding, dim=excluded.dim
	`,
		c.ID, c.Path, c.Lang, c.StartLine, c.EndLine, c.Body, c.SHA,
		c.Embedding, c.Dim, time.Now().Unix(),
	)
	return err
}

// SearchChunks runs FTS5 over chunk bodies. limit defaults to 20.
// Caller can refine by embedding similarity afterwards if it wishes.
func (s *GraphStore) SearchChunks(ctx context.Context, query string, limit int) ([]Chunk, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.path, c.lang, c.startLine, c.endLine, c.body, c.sha, c.embedding, c.dim
		FROM chunks_fts JOIN chunks c ON chunks_fts.rowid = c.rowid
		WHERE chunks_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.ID, &c.Path, &c.Lang, &c.StartLine, &c.EndLine, &c.Body, &c.SHA, &c.Embedding, &c.Dim); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CountChunks — quick stats endpoint.
func (s *GraphStore) CountChunks(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM chunks").Scan(&n)
	return n, err
}

// writeOp is a queued write operation.
type writeOp struct {
	fn   func(*sql.Tx) error
	done chan error
}

// GraphStore is a thread-safe SQLite knowledge graph.
type GraphStore struct {
	db      *sql.DB
	writeCh chan writeOp
	stopCh  chan struct{}
}

// Open opens (or creates) the graph database at dbPath.
func Open(dbPath string) (*GraphStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("graph: open %s: %w", dbPath, err)
	}

	// Single write connection — avoids SQLITE_BUSY under WAL
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("graph: schema init: %w", err)
	}

	g := &GraphStore{
		db:      db,
		writeCh: make(chan writeOp, 256),
		stopCh:  make(chan struct{}),
	}
	go g.writeLoop()
	return g, nil
}

// writeLoop is the single-writer goroutine.
func (g *GraphStore) writeLoop() {
	for {
		select {
		case op := <-g.writeCh:
			tx, err := g.db.Begin()
			if err != nil {
				op.done <- fmt.Errorf("graph: begin tx: %w", err)
				continue
			}
			if err := op.fn(tx); err != nil {
				tx.Rollback()
				op.done <- err
				continue
			}
			op.done <- tx.Commit()
		case <-g.stopCh:
			return
		}
	}
}

// write enqueues a write operation and blocks until it completes.
func (g *GraphStore) write(fn func(*sql.Tx) error) error {
	done := make(chan error, 1)
	g.writeCh <- writeOp{fn: fn, done: done}
	return <-done
}

// UpsertNode inserts or updates a node.
func (g *GraphStore) UpsertNode(n Node) error {
	payloadJSON, err := json.Marshal(n.Payload)
	if err != nil {
		return fmt.Errorf("graph: marshal payload: %w", err)
	}
	now := time.Now().UnixMilli()
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now()
	}

	return g.write(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO nodes (id, nodeType, payload, sessionId, weight, createdAt, updatedAt)
			 VALUES (?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(id) DO UPDATE SET
			   payload   = excluded.payload,
			   weight    = excluded.weight,
			   updatedAt = excluded.updatedAt`,
			n.ID, n.NodeType, string(payloadJSON), n.SessionID, n.Weight,
			n.CreatedAt.UnixMilli(), now,
		)
		return err
	})
}

// UpsertEdge inserts or updates an edge.
func (g *GraphStore) UpsertEdge(e Edge) error {
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	return g.write(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO edges (fromId, toId, edgeType, weight, createdAt)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(fromId, toId, edgeType) DO UPDATE SET weight = excluded.weight`,
			e.FromID, e.ToID, e.EdgeType, e.Weight, e.CreatedAt.UnixMilli(),
		)
		return err
	})
}

// GetNeighborhood returns nodes and edges within `hops` of `rootID`.
// Uses a recursive CTE — no raw SQL from callers.
func (g *GraphStore) GetNeighborhood(ctx context.Context, rootID string, hops int) (Neighborhood, error) {
	if hops < 1 {
		hops = 1
	}
	if hops > 3 {
		hops = 3 // safety cap
	}

	// Recursive CTE: expand edges up to `hops` levels
	query := `
	WITH RECURSIVE reachable(id, depth) AS (
		SELECT ?, 0
		UNION
		SELECT e.toId, r.depth + 1
		FROM   edges e
		JOIN   reachable r ON e.fromId = r.id
		WHERE  r.depth < ?
	)
	SELECT DISTINCT n.id, n.nodeType, n.payload, n.sessionId, n.weight, n.createdAt, n.updatedAt
	FROM   nodes n
	JOIN   reachable r ON n.id = r.id
	`

	rows, err := g.db.QueryContext(ctx, query, rootID, hops)
	if err != nil {
		return Neighborhood{}, fmt.Errorf("graph: neighborhood query: %w", err)
	}
	defer rows.Close()

	var nodes []Node
	nodeSet := map[string]bool{}
	for rows.Next() {
		var n Node
		var payloadStr string
		var createdMS, updatedMS int64
		if err := rows.Scan(&n.ID, &n.NodeType, &payloadStr, &n.SessionID, &n.Weight, &createdMS, &updatedMS); err != nil {
			return Neighborhood{}, err
		}
		_ = json.Unmarshal([]byte(payloadStr), &n.Payload)
		n.CreatedAt = time.UnixMilli(createdMS)
		n.UpdatedAt = time.UnixMilli(updatedMS)
		nodes = append(nodes, n)
		nodeSet[n.ID] = true
	}
	if err := rows.Err(); err != nil {
		return Neighborhood{}, err
	}

	// Fetch edges between the identified nodes
	edges, err := g.edgesBetween(ctx, nodeSet)
	if err != nil {
		return Neighborhood{}, err
	}

	return Neighborhood{Nodes: nodes, Edges: edges}, nil
}

func (g *GraphStore) edgesBetween(ctx context.Context, nodeSet map[string]bool) ([]Edge, error) {
	// Build IN clause from node set
	ids := make([]interface{}, 0, len(nodeSet))
	placeholders := make([]byte, 0, len(nodeSet)*2)
	first := true
	for id := range nodeSet {
		ids = append(ids, id)
		if !first {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		first = false
	}
	if len(ids) == 0 {
		return nil, nil
	}

	q := `SELECT fromId, toId, edgeType, weight, createdAt FROM edges
	      WHERE fromId IN (` + string(placeholders) + `)
	        AND toId   IN (` + string(placeholders) + `)`

	args := append(ids, ids...)
	rows, err := g.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("graph: edgesBetween: %w", err)
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		var e Edge
		var createdMS int64
		if err := rows.Scan(&e.FromID, &e.ToID, &e.EdgeType, &e.Weight, &createdMS); err != nil {
			return nil, err
		}
		e.CreatedAt = time.UnixMilli(createdMS)
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// Recent returns the latest nodes and edges between them for dashboard views.
func (g *GraphStore) Recent(ctx context.Context, limit int) (Neighborhood, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := g.db.QueryContext(ctx,
		`SELECT id, nodeType, payload, sessionId, weight, createdAt, updatedAt
		 FROM nodes
		 ORDER BY updatedAt DESC
		 LIMIT ?`, limit)
	if err != nil {
		return Neighborhood{}, fmt.Errorf("graph: recent nodes: %w", err)
	}
	defer rows.Close()

	var nodes []Node
	nodeSet := map[string]bool{}
	for rows.Next() {
		var n Node
		var payloadStr string
		var createdMS, updatedMS int64
		if err := rows.Scan(&n.ID, &n.NodeType, &payloadStr, &n.SessionID, &n.Weight, &createdMS, &updatedMS); err != nil {
			return Neighborhood{}, err
		}
		_ = json.Unmarshal([]byte(payloadStr), &n.Payload)
		n.CreatedAt = time.UnixMilli(createdMS)
		n.UpdatedAt = time.UnixMilli(updatedMS)
		nodes = append(nodes, n)
		nodeSet[n.ID] = true
	}
	if err := rows.Err(); err != nil {
		return Neighborhood{}, err
	}

	// Edges are frequently cross-type (e.g. module→symbol "defines"), so an
	// edge often has one endpoint inside the recent window and one outside it.
	// Restricting to edges whose BOTH endpoints are recent would drop nearly
	// every connection and render an edgeless dust cloud. Instead we take every
	// edge that TOUCHES a recent node, then pull in the missing endpoints so the
	// neighborhood stays connected.
	edges, err := g.edgesTouching(ctx, nodeSet)
	if err != nil {
		return Neighborhood{}, err
	}

	missing := map[string]bool{}
	for _, e := range edges {
		if !nodeSet[e.FromID] {
			missing[e.FromID] = true
		}
		if !nodeSet[e.ToID] {
			missing[e.ToID] = true
		}
	}
	if len(missing) > 0 {
		extra, err := g.nodesByIDs(ctx, missing)
		if err != nil {
			return Neighborhood{}, err
		}
		nodes = append(nodes, extra...)
	}
	return Neighborhood{Nodes: nodes, Edges: edges}, nil
}

// edgesTouching returns every edge with at least one endpoint in nodeSet.
func (g *GraphStore) edgesTouching(ctx context.Context, nodeSet map[string]bool) ([]Edge, error) {
	ids := make([]interface{}, 0, len(nodeSet))
	placeholders := make([]byte, 0, len(nodeSet)*2)
	first := true
	for id := range nodeSet {
		ids = append(ids, id)
		if !first {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		first = false
	}
	if len(ids) == 0 {
		return nil, nil
	}

	q := `SELECT fromId, toId, edgeType, weight, createdAt FROM edges
	      WHERE fromId IN (` + string(placeholders) + `)
	         OR toId   IN (` + string(placeholders) + `)`

	args := append(ids, ids...)
	rows, err := g.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("graph: edgesTouching: %w", err)
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		var e Edge
		var createdMS int64
		if err := rows.Scan(&e.FromID, &e.ToID, &e.EdgeType, &e.Weight, &createdMS); err != nil {
			return nil, err
		}
		e.CreatedAt = time.UnixMilli(createdMS)
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// nodesByIDs fetches full node records for the given id set.
func (g *GraphStore) nodesByIDs(ctx context.Context, idSet map[string]bool) ([]Node, error) {
	if len(idSet) == 0 {
		return nil, nil
	}
	ids := make([]interface{}, 0, len(idSet))
	placeholders := make([]byte, 0, len(idSet)*2)
	first := true
	for id := range idSet {
		ids = append(ids, id)
		if !first {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		first = false
	}

	q := `SELECT id, nodeType, payload, sessionId, weight, createdAt, updatedAt
	      FROM nodes WHERE id IN (` + string(placeholders) + `)`
	rows, err := g.db.QueryContext(ctx, q, ids...)
	if err != nil {
		return nil, fmt.Errorf("graph: nodesByIDs: %w", err)
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		var n Node
		var payloadStr string
		var createdMS, updatedMS int64
		if err := rows.Scan(&n.ID, &n.NodeType, &payloadStr, &n.SessionID, &n.Weight, &createdMS, &updatedMS); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(payloadStr), &n.Payload)
		n.CreatedAt = time.UnixMilli(createdMS)
		n.UpdatedAt = time.UnixMilli(updatedMS)
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// RecentForProject returns the latest nodes and edges for a specific project.
func (g *GraphStore) RecentForProject(ctx context.Context, project string, limit int) (Neighborhood, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := g.db.QueryContext(ctx,
		`SELECT id, nodeType, payload, sessionId, weight, createdAt, updatedAt
		 FROM nodes
		 WHERE json_extract(payload, '$.project') = ?
		 ORDER BY updatedAt DESC
		 LIMIT ?`, project, limit)
	if err != nil {
		return Neighborhood{}, fmt.Errorf("graph: recent nodes for project: %w", err)
	}
	defer rows.Close()

	var nodes []Node
	nodeSet := map[string]bool{}
	for rows.Next() {
		var n Node
		var payloadStr string
		var createdMS, updatedMS int64
		if err := rows.Scan(&n.ID, &n.NodeType, &payloadStr, &n.SessionID, &n.Weight, &createdMS, &updatedMS); err != nil {
			return Neighborhood{}, err
		}
		_ = json.Unmarshal([]byte(payloadStr), &n.Payload)
		n.CreatedAt = time.UnixMilli(createdMS)
		n.UpdatedAt = time.UnixMilli(updatedMS)
		nodes = append(nodes, n)
		nodeSet[n.ID] = true
	}
	if err := rows.Err(); err != nil {
		return Neighborhood{}, err
	}

	edges, err := g.edgesBetween(ctx, nodeSet)
	if err != nil {
		return Neighborhood{}, err
	}
	return Neighborhood{Nodes: nodes, Edges: edges}, nil
}

// Stats returns node and edge counts.
func (g *GraphStore) Stats(ctx context.Context) (nodes, edges int, err error) {
	row := g.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes`)
	if err = row.Scan(&nodes); err != nil {
		return
	}
	row = g.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM edges`)
	err = row.Scan(&edges)
	return
}

// FlushAndSync waits for the write queue to drain and then checkpoints WAL.
// Called before safe-pause ACK.
func (g *GraphStore) FlushAndSync() error {
	// Drain write queue by sending a no-op
	if err := g.write(func(tx *sql.Tx) error { return nil }); err != nil {
		return err
	}
	// WAL checkpoint
	_, err := g.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return err
}

// Close stops the write loop and closes the database.
func (g *GraphStore) Close() error {
	close(g.stopCh)
	return g.db.Close()
}
