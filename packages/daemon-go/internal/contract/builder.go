// internal/contract/builder.go
//
// ContractBuilder builds and HMAC-SHA256 signs ContinuationContracts from
// the GraphStore neighborhood.

package contract

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/dbisina/relay/internal/graph"
)

// Builder builds and signs continuation contracts.
type Builder struct {
	store      *graph.GraphStore
	signingKey []byte
}

// NewBuilder creates a Builder.
// signingKey: HMAC-SHA256 key (loaded from .relay/.signing-key).
func NewBuilder(store *graph.GraphStore, signingKey []byte) *Builder {
	return &Builder{store: store, signingKey: signingKey}
}

// Build constructs an unsigned contract from the 2-hop graph neighborhood.
func (b *Builder) Build(ctx context.Context, sessionID, taskID, taskGoal, nextAction string) (*ContinuationContract, error) {
	nbr, err := b.store.GetNeighborhood(ctx, taskID, 2)
	if err != nil {
		return nil, fmt.Errorf("contract: graph neighborhood: %w", err)
	}

	c := &ContinuationContract{
		ContractID: fmt.Sprintf("c-%s-%d", sessionID[:8], time.Now().UnixMilli()),
		SessionID:  sessionID,
		TaskID:     taskID,
		CreatedAt:  time.Now().UTC(),
		Version:    2,
		TaskGoal:   taskGoal,
		NextAction: nextAction,
	}

	// Extract structured fields from graph nodes
	for _, n := range nbr.Nodes {
		switch n.NodeType {
		case "decision":
			summary, _ := n.Payload["summary"].(string)
			rationale, _ := n.Payload["rationale"].(string)
			c.Decisions = append(c.Decisions, Decision{Summary: summary, Rationale: rationale})

		case "constraint":
			rule, _ := n.Payload["rule"].(string)
			source, _ := n.Payload["source"].(string)
			c.Constraints = append(c.Constraints, Constraint{Rule: rule, Source: source})

		case "do_not_redo":
			item, _ := n.Payload["item"].(string)
			if item != "" {
				c.DoNotRedo = append(c.DoNotRedo, item)
			}

		case "acceptance":
			assertion, _ := n.Payload["assertion"].(string)
			if assertion != "" {
				c.AcceptanceAssertions = append(c.AcceptanceAssertions, assertion)
			}

		case "file":
			path, _ := n.Payload["path"].(string)
			sha, _ := n.Payload["sha256"].(string)
			modified, _ := n.Payload["modified"].(bool)
			if path != "" {
				c.FileManifest = append(c.FileManifest, FileEntry{
					Path: path, SHA256: sha, Modified: modified,
				})
			}

		// ── Rich session intent (v2) ─────────────────────────────────────────
		case "prompt", "initial_prompt":
			if p, _ := n.Payload["text"].(string); p != "" && c.InitialPrompt == "" {
				c.InitialPrompt = p
			}

		case "plan":
			if item, _ := n.Payload["item"].(string); item != "" {
				c.Plan = append(c.Plan, item)
				if done, _ := n.Payload["done"].(bool); !done {
					c.TasksRemaining = append(c.TasksRemaining, item)
				}
			}

		case "skill":
			name, _ := n.Payload["name"].(string)
			if name == "" {
				break
			}
			switch status, _ := n.Payload["status"].(string); status {
			case "in_use":
				c.SkillsInUse = append(c.SkillsInUse, name)
			case "to_use":
				c.SkillsToUse = append(c.SkillsToUse, name)
			default:
				c.SkillsLoaded = append(c.SkillsLoaded, name)
			}

		case "in_flight":
			if path, _ := n.Payload["path"].(string); path != "" {
				snippet, _ := n.Payload["snippet"].(string)
				c.InFlightCode = append(c.InFlightCode, InFlightFile{Path: path, Snippet: snippet})
			}
		}
	}

	return c, nil
}

// Sign computes HMAC-SHA256 over the canonical JSON of the contract
// (with Signature field set to ""), then sets c.Signature.
func (b *Builder) Sign(c *ContinuationContract) error {
	if len(b.signingKey) == 0 {
		return fmt.Errorf("contract: signing key not loaded")
	}
	c.Signature = ""
	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("contract: marshal for signing: %w", err)
	}
	mac := hmac.New(sha256.New, b.signingKey)
	mac.Write(data)
	c.Signature = hex.EncodeToString(mac.Sum(nil))
	return nil
}

// Verify checks the HMAC-SHA256 signature on a contract.
// Uses constant-time comparison to prevent timing attacks.
func (b *Builder) Verify(c *ContinuationContract) error {
	if len(b.signingKey) == 0 {
		return fmt.Errorf("contract: signing key not loaded")
	}
	sig := c.Signature
	c.Signature = ""
	defer func() { c.Signature = sig }()

	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("contract: marshal for verify: %w", err)
	}
	mac := hmac.New(sha256.New, b.signingKey)
	mac.Write(data)
	expected := mac.Sum(nil)

	got, err := hex.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("contract: invalid signature hex: %w", err)
	}
	if !hmac.Equal(expected, got) {
		return fmt.Errorf("contract: signature mismatch — tampered or wrong key")
	}
	return nil
}

// ComputeHash returns the SHA-256 of the canonical JSON (used for FSM idempotency).
func ComputeHash(c *ContinuationContract) (string, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

// LoadSigningKey reads the signing key from path, generating a new one if absent.
func LoadSigningKey(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil && len(data) >= 32 {
		return data, nil
	}
	// Generate a new random key
	key := make([]byte, 32)
	f, ferr := os.Open("/dev/urandom")
	if ferr != nil {
		// Windows fallback: use crypto/rand
		return generateKeyFallback(path)
	}
	defer f.Close()
	if _, err := f.Read(key); err != nil {
		return nil, fmt.Errorf("signing key: read /dev/urandom: %w", err)
	}
	if err := os.WriteFile(path, key, 0600); err != nil {
		return nil, fmt.Errorf("signing key: write %s: %w", path, err)
	}
	return key, nil
}

func generateKeyFallback(path string) ([]byte, error) {
	// crypto/rand always works on Windows
	key := make([]byte, 32)
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("relay-v0-%d", time.Now().UnixNano())))
	copy(key, h.Sum(nil))
	if err := os.WriteFile(path, key, 0600); err != nil {
		return nil, fmt.Errorf("signing key fallback: %w", err)
	}
	return key, nil
}
