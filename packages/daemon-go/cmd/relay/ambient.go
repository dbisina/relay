// ambient.go — ambient (always-on) agent detection.
//
// Instead of only scanning when the user opens the Detect page, the daemon polls
// the on-disk transcript stores in the background and announces when a NEW active
// agent session appears — turning Relay from a tool you open into background
// infra that notices "you just started Claude in another terminal, want to port
// it?". Polling (not fsnotify) keeps this dependency-free per CLAUDE.md.

package main

import (
	"fmt"
	"time"

	"github.com/dbisina/relay/internal/server"
)

const ambientPollInterval = 20 * time.Second

// startAmbientDetection runs until stop is closed, emitting a system event the
// first time each active agent session is seen. Best-effort: scan errors are
// skipped, never fatal.
func startAmbientDetection(httpServer *server.Server, stop <-chan struct{}) {
	seen := map[string]bool{}
	tick := time.NewTicker(ambientPollInterval)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			agents, err := scanDetected()
			if err != nil {
				continue
			}
			for _, a := range agents {
				if !a.Running || seen[a.ID] {
					continue
				}
				seen[a.ID] = true
				detail := a.DisplayName + " session"
				if a.Session != nil && a.Session.InitialPrompt != "" {
					detail = oneLine(a.Session.InitialPrompt, 60)
				}
				httpServer.PushEvent("system", fmt.Sprintf(
					"◆ active %s detected — %s · adopt: relay detect --adopt %s",
					a.DisplayName, detail, a.ID))
			}
		}
	}
}
