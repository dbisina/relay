package config

import (
	"reflect"
	"testing"
)

func TestTopoOrderRespectsDeps(t *testing.T) {
	p := Pipeline{
		Name: "build",
		Nodes: []PipelineNode{
			{ID: "design", Provider: "claude"},
			{ID: "impl", Provider: "codex", DependsOn: []string{"design"}},
			{ID: "test", Provider: "claude", DependsOn: []string{"impl"}},
			{ID: "docs", Provider: "claude", DependsOn: []string{"design"}},
		},
	}
	order, err := p.TopoOrder()
	if err != nil {
		t.Fatal(err)
	}
	pos := map[string]int{}
	for i, id := range order {
		pos[id] = i
	}
	if pos["design"] > pos["impl"] || pos["impl"] > pos["test"] || pos["design"] > pos["docs"] {
		t.Fatalf("topo order violates deps: %v", order)
	}
}

func TestCycleDetected(t *testing.T) {
	p := Pipeline{
		Name: "loop",
		Nodes: []PipelineNode{
			{ID: "a", Provider: "claude", DependsOn: []string{"b"}},
			{ID: "b", Provider: "codex", DependsOn: []string{"a"}},
		},
	}
	if _, err := p.TopoOrder(); err == nil {
		t.Fatal("expected cycle error")
	}
	if err := p.Validate(); err == nil {
		t.Fatal("Validate should reject a cyclic pipeline")
	}
}

func TestValidateRejectsUnknownDep(t *testing.T) {
	p := Pipeline{
		Name:  "x",
		Nodes: []PipelineNode{{ID: "a", Provider: "claude", DependsOn: []string{"ghost"}}},
	}
	if err := p.Validate(); err == nil {
		t.Fatal("Validate should reject a dependency on an unknown node")
	}
}

func TestNodePriorityDedup(t *testing.T) {
	n := PipelineNode{Provider: "claude", Fallback: []string{"claude", "codex", "ollama"}}
	got := n.Priority()
	want := []string{"claude", "codex", "ollama"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Priority() = %v, want %v", got, want)
	}
}

func TestPipelinesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ps := []Pipeline{{
		Name: "build",
		Nodes: []PipelineNode{
			{ID: "design", Provider: "claude", Task: "design the API"},
			{ID: "impl", Provider: "codex", Task: "implement", DependsOn: []string{"design"}, Fallback: []string{"claude"}},
		},
	}}
	if err := SavePipelines(dir, ps); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPipelines(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "build" || len(got[0].Nodes) != 2 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if _, ok := FindPipeline(got, "build"); !ok {
		t.Fatal("FindPipeline missed a present pipeline")
	}
}

func TestSaveRejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	bad := []Pipeline{{Name: "bad", Nodes: []PipelineNode{
		{ID: "a", DependsOn: []string{"a"}}, // self-dependency
	}}}
	if err := SavePipelines(dir, bad); err == nil {
		t.Fatal("SavePipelines must reject an invalid pipeline")
	}
}
