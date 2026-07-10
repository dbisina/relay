// internal/detect/vscdb.go — read VS Code-family state.vscdb (SQLite) stores.
//
// VS Code and its forks (Antigravity, Cursor, …) keep UI/agent state in a
// SQLite db with a single key→value ItemTable. Some agents (Antigravity's
// "Agent Manager") store their trajectory history here rather than in JSON
// files. We read it read-only by copying the db (+WAL) to a temp dir first, so
// a live IDE holding the file open never blocks us.
//
// Reuses the pure-Go modernc.org/sqlite driver already vendored for the graph
// store — no new dependency, no CGO.

package detect

import (
	"database/sql"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// readVSCDBValue returns the raw value for one ItemTable key.
func readVSCDBValue(dbPath, key string) ([]byte, error) {
	tmp, cleanup, err := copyDBForRead(dbPath)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	db, err := sql.Open("sqlite", tmp)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var v []byte
	if err := db.QueryRow("SELECT value FROM ItemTable WHERE key = ?", key).Scan(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// readVSCDBValuesLike returns every ItemTable value whose key matches any of the
// SQL LIKE patterns (e.g. "%trajector%", "%chat%"). Best-effort: a missing db or
// table yields nil. Used to sweep both global and per-workspace stores.
func readVSCDBValuesLike(dbPath string, patterns []string) [][]byte {
	tmp, cleanup, err := copyDBForRead(dbPath)
	if err != nil {
		return nil
	}
	defer cleanup()

	db, err := sql.Open("sqlite", tmp)
	if err != nil {
		return nil
	}
	defer db.Close()

	var out [][]byte
	for _, p := range patterns {
		rows, err := db.Query("SELECT value FROM ItemTable WHERE key LIKE ?", p)
		if err != nil {
			continue
		}
		for rows.Next() {
			var v []byte
			if rows.Scan(&v) == nil && len(v) > 0 {
				out = append(out, v)
			}
		}
		_ = rows.Close()
	}
	return out
}

// vscdbFiles returns the global state.vscdb plus every per-workspace state.vscdb
// for a VS Code-family User dir (globalStorage's parent). Agents stash global
// trajectory summaries in the former and per-conversation chat in the latter.
func vscdbFiles(globalStorage string) []string {
	var dbs []string
	if g := filepath.Join(globalStorage, "state.vscdb"); fileExists(g) {
		dbs = append(dbs, g)
	}
	wsRoot := filepath.Join(filepath.Dir(globalStorage), "workspaceStorage")
	entries, _ := os.ReadDir(wsRoot)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if db := filepath.Join(wsRoot, e.Name(), "state.vscdb"); fileExists(db) {
			dbs = append(dbs, db)
		}
	}
	return dbs
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// copyDBForRead copies the db and its WAL/SHM sidecars to a temp dir, so we can
// open it without contending with a running IDE's write lock.
func copyDBForRead(dbPath string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "relay-vscdb-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	dst := filepath.Join(dir, filepath.Base(dbPath))
	if err := copyFile(dbPath, dst); err != nil {
		cleanup()
		return "", nil, err
	}
	// WAL + SHM are best-effort: absent on a cleanly-closed db.
	for _, ext := range []string{"-wal", "-shm"} {
		_ = copyFile(dbPath+ext, dst+ext)
	}
	return dst, cleanup, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// ─── Antigravity (Gemini's successor) trajectory titles ───────────────────────
//
// Antigravity is a VS Code fork; its agent stores trajectories as base64-wrapped
// protobuf under antigravityUnifiedStateSync.trajectorySummaries. Without the
// .proto we can't decode fields, but the human-readable trajectory titles and
// prompts sit as length-prefixed strings inside, so we surface those
// best-effort — enough to show what the agent has been working on.

// antigravityProducts are the VS Code-family product dirs Antigravity ships under.
// It has appeared as both "Antigravity" and "Antigravity IDE" (and, being a
// Google product, may surface under a Gemini-branded dir), each with its own
// global + per-workspace stores. We sweep them all and de-duplicate.
var antigravityProducts = []string{"Antigravity", "Antigravity IDE", "Gemini", "Antigravity-Gemini"}

// antigravityKeyPatterns are the ItemTable keys that carry agent trajectory /
// chat data: global trajectory summaries plus per-workspace chat blobs.
var antigravityKeyPatterns = []string{"antigravityUnifiedStateSync%", "%trajector%", "%chat%"}

func scanAntigravityTrajectories(home string, maxAge time.Duration) []SessionIntel {
	if home == "" {
		return nil
	}
	cutoff := time.Now().Add(-maxAge)
	set := newOrderedSet()
	var newestDB string
	var newestMs int64

	for _, product := range antigravityProducts {
		for _, db := range vscdbFiles(appGlobalStorage(home, product)) {
			info, err := os.Stat(db)
			if err != nil || info.ModTime().Before(cutoff) {
				continue
			}
			added := false
			for _, val := range readVSCDBValuesLike(db, antigravityKeyPatterns) {
				for _, t := range extractAntigravityTrajectories(val) {
					set.add(t)
					added = true
				}
			}
			if added && info.ModTime().UnixMilli() > newestMs {
				newestMs = info.ModTime().UnixMilli()
				newestDB = db
			}
		}
	}

	titles := set.slice()
	if len(titles) == 0 {
		return nil
	}
	if newestMs == 0 {
		newestMs = time.Now().UnixMilli()
	}
	return []SessionIntel{{
		SessionID:      "antigravity-trajectories",
		TranscriptPath: newestDB,
		InitialPrompt:  titles[0],
		LastActivity:   titles[0],
		Plan:           titles, // recent agent trajectories (best-effort, titles only)
		MessageCount:   len(titles),
		lastActiveMs:   newestMs,
	}}
}

var printableRunRe = regexp.MustCompile(`[\x20-\x7e]{6,}`)
var b64RunRe = regexp.MustCompile(`^[A-Za-z0-9+/=]+$`)
var trajLeadingJunk = "!\"#$%&'()*+,-./:;<=>?@[]^_`{|}~ \t"

// extractAntigravityTrajectories base64-decodes the value, then scans for
// readable strings (and one level of nested base64), returning de-duplicated
// trajectory titles / prompts.
func extractAntigravityTrajectories(value []byte) []string {
	set := newOrderedSet()
	// Antigravity nests base64-wrapped protobuf several levels deep; recurse,
	// collecting readable titles and re-decoding base64 runs as we go.
	var walk func(b []byte, depth int)
	walk = func(b []byte, depth int) {
		for _, m := range printableRunRe.FindAll(b, -1) {
			s := string(m)
			if t := cleanTrajTitle(s); isLikelyTitle(t) {
				set.add(t)
			}
			if depth < 3 {
				t := strings.TrimSpace(s)
				if len(t) >= 16 && b64RunRe.MatchString(t) {
					if dec, err := base64.StdEncoding.DecodeString(padB64(t)); err == nil && len(dec) > 0 {
						walk(dec, depth+1)
					}
				}
			}
		}
	}
	start := value
	if dec, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(value))); err == nil && len(dec) > 0 {
		start = dec
	}
	walk(start, 0)
	return set.slice()
}

func cleanTrajTitle(s string) string {
	return strings.TrimSpace(strings.TrimLeft(s, trajLeadingJunk))
}

// isLikelyTitle keeps human text: a space (which a contiguous base64 blob never
// has, so this alone excludes them) plus enough letters to be real prose.
func isLikelyTitle(s string) bool {
	if len(s) < 6 || !strings.Contains(s, " ") {
		return false
	}
	letters := 0
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			letters++
		}
	}
	return letters >= 8
}

func padB64(s string) string {
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	return s
}
