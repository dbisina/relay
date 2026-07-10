// internal/audit/logger.go
//
// AuditLogger — append-only JSONL with SHA-256 hash-chain.
// SEC-10: fd opened O_APPEND, fsync on every write.
// Each entry: { seq, ts, event, data, prevHash, hash }
// AuditLogger.Verify(path) replays the chain to detect tampering.

package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Entry is one line in the audit log.
type Entry struct {
	Seq       int64          `json:"seq"`
	Timestamp time.Time      `json:"ts"`
	Event     string         `json:"event"`
	Data      map[string]any `json:"data,omitempty"`
	PrevHash  string         `json:"prevHash"`
	Hash      string         `json:"hash"` // SHA-256 of this entry with Hash=""
}

// Logger writes hash-chained audit entries to a JSONL file.
type Logger struct {
	mu       sync.Mutex
	fd       *os.File
	seq      int64
	prevHash string
}

// Open opens (or creates) the audit log at path.
// Resumes the chain from the last entry if the file exists.
func Open(path string) (*Logger, error) {
	fd, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("audit: open %s: %w", path, err)
	}

	l := &Logger{fd: fd}
	// Bootstrap: replay existing file to get last seq + hash
	l.seq, l.prevHash = resumeChain(path)
	return l, nil
}

// Log appends an audit entry.
func (l *Logger) Log(event string, data map[string]any) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.seq++
	entry := Entry{
		Seq:       l.seq,
		Timestamp: time.Now().UTC(),
		Event:     event,
		Data:      data,
		PrevHash:  l.prevHash,
	}

	// Compute hash with Hash="" (canonical)
	entry.Hash = computeEntryHash(entry)
	l.prevHash = entry.Hash

	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("audit: marshal: %w", err)
	}
	line = append(line, '\n')

	if _, err := l.fd.Write(line); err != nil {
		return fmt.Errorf("audit: write: %w", err)
	}
	// SEC-10: fsync every write
	return l.fd.Sync()
}

// Close flushes and closes the log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.fd != nil {
		_ = l.fd.Sync()
		err := l.fd.Close()
		l.fd = nil
		return err
	}
	return nil
}

// Verify replays the hash chain in path and reports any broken links.
// Used by `relay audit verify`.
func Verify(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("audit verify: read %s: %w", path, err)
	}

	var prevHash string
	var seq int64

	lines := splitLines(data)
	for i, line := range lines {
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			return fmt.Errorf("audit verify: line %d: parse: %w", i+1, err)
		}
		if e.Seq != seq+1 {
			return fmt.Errorf("audit verify: line %d: seq gap %d→%d", i+1, seq, e.Seq)
		}
		if e.PrevHash != prevHash {
			return fmt.Errorf("audit verify: line %d seq=%d: prevHash mismatch", i+1, e.Seq)
		}
		expectedHash := computeEntryHash(e)
		if e.Hash != expectedHash {
			return fmt.Errorf("audit verify: line %d seq=%d: hash mismatch — TAMPERED", i+1, e.Seq)
		}
		prevHash = e.Hash
		seq = e.Seq
	}
	return nil
}

// Tail returns the last n parsed entries (oldest→newest) for the time-machine
// history view. A missing log is an empty slice, not an error.
func Tail(path string, n int) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []Entry
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var e Entry
		if json.Unmarshal(line, &e) == nil {
			entries = append(entries, e)
		}
	}
	if n > 0 && len(entries) > n {
		entries = entries[len(entries)-n:]
	}
	return entries, nil
}

// computeEntryHash returns SHA-256(JSON(entry with Hash="")).
func computeEntryHash(e Entry) string {
	e.Hash = ""
	data, _ := json.Marshal(e)
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// resumeChain reads the last line of an existing log to get seq + hash.
func resumeChain(path string) (int64, string) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return 0, ""
	}
	lines := splitLines(data)
	// Find last non-empty line
	for i := len(lines) - 1; i >= 0; i-- {
		if len(lines[i]) == 0 {
			continue
		}
		var e Entry
		if json.Unmarshal(lines[i], &e) == nil {
			return e.Seq, e.Hash
		}
	}
	return 0, ""
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
