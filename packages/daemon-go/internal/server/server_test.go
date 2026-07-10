package server

import (
	"testing"
	"time"
)

// TryStartSession is the single guarded entry point for starting work (POST
// /api/run and the detect→adopt flow both go through it). The guard must hold the
// single-session invariant and forward the request — including a provider override
// — to the run callback verbatim.
func TestTryStartSessionGuards(t *testing.T) {
	s := New(0) // construct only; do not Start() a listener

	// No run callback wired yet → not ready, nothing starts.
	if err := s.TryStartSession(RunRequest{Task: "x"}); err == nil {
		t.Fatal("want error when runCB is nil")
	}

	got := make(chan RunRequest, 1)
	release := make(chan struct{})
	s.OnRun(func(req RunRequest) {
		got <- req
		<-release // hold the session open so the guard trips on a second start
	})

	// Empty task → rejected.
	if err := s.TryStartSession(RunRequest{Task: ""}); err == nil {
		t.Fatal("want error for empty task")
	}

	// First start succeeds and forwards the request, provider override included.
	if err := s.TryStartSession(RunRequest{Task: "go", Providers: []string{"codex"}}); err != nil {
		t.Fatalf("first start: %v", err)
	}
	select {
	case req := <-got:
		if req.Task != "go" || len(req.Providers) != 1 || req.Providers[0] != "codex" {
			t.Fatalf("runCB got %+v, want task=go providers=[codex]", req)
		}
	case <-time.After(time.Second):
		t.Fatal("runCB not invoked")
	}

	// Second start while the first is still active → rejected.
	if err := s.TryStartSession(RunRequest{Task: "again"}); err == nil {
		t.Fatal("want 'session already running' error")
	}

	// Release the first session; the slot frees and a fresh start works.
	close(release)
	deadline := time.Now().Add(time.Second)
	for s.sessionActive.Load() {
		if time.Now().After(deadline) {
			t.Fatal("session slot never freed after callback returned")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := s.TryStartSession(RunRequest{Task: "third"}); err != nil {
		t.Fatalf("third start after release: %v", err)
	}
}
