// internal/contract/sidecar.go
//
// Signed-JSON sidecar for envelope.md. The .md is human-readable; the .json
// next to it is the canonical, signed payload used to verify tamper-freedom
// before injecting into the next provider.

package contract

import (
	"encoding/json"
	"fmt"
	"os"
)

// WriteJSON writes the contract as JSON to path (0600).
func WriteJSON(path string, c *ContinuationContract) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("contract: marshal sidecar: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

// ReadJSON reads + parses a JSON contract sidecar.
func ReadJSON(path string) (*ContinuationContract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c ContinuationContract
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("contract: parse sidecar: %w", err)
	}
	return &c, nil
}
