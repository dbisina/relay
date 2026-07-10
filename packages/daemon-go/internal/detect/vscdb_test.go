package detect

import (
	"database/sql"
	"encoding/base64"
	"path/filepath"
	"testing"
)

func TestReadVSCDBValue(t *testing.T) {
	dir := t.TempDir()
	dbp := filepath.Join(dir, "state.vscdb")

	db, err := sql.Open("sqlite", dbp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE ItemTable(key TEXT PRIMARY KEY, value BLOB)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO ItemTable(key,value) VALUES('k','hello world')"); err != nil {
		t.Fatal(err)
	}
	db.Close()

	v, err := readVSCDBValue(dbp, "k")
	if err != nil {
		t.Fatal(err)
	}
	if string(v) != "hello world" {
		t.Errorf("readVSCDBValue = %q, want %q", v, "hello world")
	}
	if _, err := readVSCDBValue(dbp, "missing"); err == nil {
		t.Error("expected error for missing key")
	}
}

func TestExtractAntigravityTrajectories(t *testing.T) {
	// \x12 framing bytes mimic protobuf length-prefixed string fields.
	raw := []byte("\x12\x14Fixing the login bug\x12\x13Adding dark mode now\x12\x04junk")
	enc := base64.StdEncoding.EncodeToString(raw)

	got := extractAntigravityTrajectories([]byte(enc))
	if !contains(got, "Fixing the login bug") || !contains(got, "Adding dark mode now") {
		t.Errorf("extracted titles = %v", got)
	}
	if contains(got, "junk") {
		t.Errorf("short/no-space noise should be filtered, got %v", got)
	}
}
