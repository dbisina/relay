// internal/config/pipeline.go — multi-agent pipeline DAGs (pillar 4).
//
// A profile is a single linear provider chain. A pipeline is a DAG: each node is
// one agent doing one part of the task, edges encode ordering (dependsOn), and a
// per-node fallback list gives provider failover on a snag. Pipelines live in
// .relay/pipelines.json so the designer can edit them without touching relay.toml.

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PipelineNode is one agent + task in a pipeline DAG.
type PipelineNode struct {
	ID        string   `json:"id"`                  // unique within the pipeline
	Provider  string   `json:"provider"`            // primary provider for this part
	Task      string   `json:"task"`                // what this agent should do
	DependsOn []string `json:"dependsOn,omitempty"` // node ids that must finish first
	Fallback  []string `json:"fallback,omitempty"`  // providers to try if the primary snags
	Verify    []string `json:"verify,omitempty"`    // acceptance commands; all must pass (verifier gate)
}

// Pipeline is a named DAG of agent nodes.
type Pipeline struct {
	Name  string         `json:"name"`
	Nodes []PipelineNode `json:"nodes"`
}

// Priority returns the provider order for a node: its primary then its fallbacks,
// de-duplicated. This maps directly onto the orchestrator's provider priority, so
// a node's "fallback on snag" reuses the existing quota-breach handoff chain.
func (n PipelineNode) Priority() []string {
	out := make([]string, 0, 1+len(n.Fallback))
	seen := map[string]bool{}
	for _, p := range append([]string{n.Provider}, n.Fallback...) {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// Validate checks the pipeline is well-formed: non-empty unique node ids, every
// dependency references a real node, and the graph is acyclic.
func (p Pipeline) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("pipeline: name required")
	}
	ids := map[string]bool{}
	for _, n := range p.Nodes {
		if n.ID == "" {
			return fmt.Errorf("pipeline %q: a node has an empty id", p.Name)
		}
		if ids[n.ID] {
			return fmt.Errorf("pipeline %q: duplicate node id %q", p.Name, n.ID)
		}
		ids[n.ID] = true
	}
	for _, n := range p.Nodes {
		for _, dep := range n.DependsOn {
			if !ids[dep] {
				return fmt.Errorf("pipeline %q: node %q depends on unknown node %q", p.Name, n.ID, dep)
			}
			if dep == n.ID {
				return fmt.Errorf("pipeline %q: node %q depends on itself", p.Name, n.ID)
			}
		}
	}
	if _, err := p.TopoOrder(); err != nil {
		return err
	}
	return nil
}

// TopoOrder returns node ids in dependency order (a node appears after every node
// it depends on) using Kahn's algorithm. It errors if the graph has a cycle.
// Ties are broken by the node's position in p.Nodes for a stable, predictable run.
func (p Pipeline) TopoOrder() ([]string, error) {
	indeg := map[string]int{}
	order := make([]string, 0, len(p.Nodes))
	pos := map[string]int{}
	for i, n := range p.Nodes {
		indeg[n.ID] = 0
		pos[n.ID] = i
	}
	// edges: dep -> node (node depends on dep)
	adj := map[string][]string{}
	for _, n := range p.Nodes {
		for _, dep := range n.DependsOn {
			adj[dep] = append(adj[dep], n.ID)
			indeg[n.ID]++
		}
	}
	// ready set = indeg 0, drained in stable p.Nodes order each round
	for len(order) < len(p.Nodes) {
		next := ""
		for _, n := range p.Nodes {
			if indeg[n.ID] == 0 {
				next = n.ID
				break
			}
		}
		if next == "" {
			return nil, fmt.Errorf("pipeline %q: dependency cycle detected", p.Name)
		}
		order = append(order, next)
		indeg[next] = -1 // consumed
		for _, m := range adj[next] {
			if indeg[m] > 0 {
				indeg[m]--
			}
		}
	}
	return order, nil
}

const pipelinesFile = "pipelines.json"

// LoadPipelines reads .relay/pipelines.json. A missing file yields an empty list.
func LoadPipelines(stateDir string) ([]Pipeline, error) {
	data, err := os.ReadFile(filepath.Join(stateDir, pipelinesFile))
	if os.IsNotExist(err) {
		return []Pipeline{}, nil
	}
	if err != nil {
		return nil, err
	}
	var ps []Pipeline
	if err := json.Unmarshal(data, &ps); err != nil {
		return nil, fmt.Errorf("pipelines: parse: %w", err)
	}
	return ps, nil
}

// SavePipelines writes the full pipeline list, validating each first so a bad
// edit never persists.
func SavePipelines(stateDir string, ps []Pipeline) error {
	for _, p := range ps {
		if err := p.Validate(); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stateDir, pipelinesFile), data, 0o600)
}

// FindPipeline returns the named pipeline, or false.
func FindPipeline(ps []Pipeline, name string) (Pipeline, bool) {
	for _, p := range ps {
		if p.Name == name {
			return p, true
		}
	}
	return Pipeline{}, false
}
