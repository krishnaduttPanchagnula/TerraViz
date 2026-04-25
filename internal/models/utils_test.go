package models

import (
	"fmt"
	"testing"
)

// buildBenchDiagram creates a Diagram with n resources of mixed types.
func buildBenchDiagram(n int) *Diagram {
	types := []ResourceType{
		ResourceTypeEC2Instance, ResourceTypeS3Bucket, ResourceTypeLambdaFunction,
		ResourceTypeVPC, ResourceTypeSubnet, ResourceTypeSecurityGroup,
		ResourceTypeIAMRole, ResourceTypeCloudWatch, ResourceTypeRDSInstance,
		ResourceTypeLoadBalancer,
	}
	providers := []string{"aws", "aws", "aws", "aws", "aws"}
	states := []ResourceState{ResourceStateActive, ResourceStateInactive, ResourceStatePending}

	d := &Diagram{
		ID:          "bench",
		Name:        "benchmark",
		Resources:   make([]Resource, n),
		Connections: make([]Connection, 0),
	}

	for i := 0; i < n; i++ {
		d.Resources[i] = Resource{
			ID:       fmt.Sprintf("res-%04d", i),
			Name:     fmt.Sprintf("resource-%d", i),
			Type:     types[i%len(types)],
			Provider: providers[i%len(providers)],
			Region:   fmt.Sprintf("us-east-%d", i%3+1),
			State:    states[i%len(states)],
			Tags:     map[string]string{"Name": fmt.Sprintf("resource-%d", i), "env": "prod"},
			Source:   "test",
		}
	}

	return d
}

// ─── GetResourceStats benchmarks (issue #7: fmt.Sprintf overhead) ────────────

func BenchmarkGetResourceStats_100(b *testing.B) {
	d := buildBenchDiagram(100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.GetResourceStats()
	}
}

func BenchmarkGetResourceStats_1000(b *testing.B) {
	d := buildBenchDiagram(1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.GetResourceStats()
	}
}

func BenchmarkGetResourceStats_5000(b *testing.B) {
	d := buildBenchDiagram(5000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.GetResourceStats()
	}
}

// ─── FilterResources benchmarks ──────────────────────────────────────────────

func BenchmarkFilterResources_1000(b *testing.B) {
	d := buildBenchDiagram(1000)
	filters := map[string]string{"type": string(ResourceTypeEC2Instance)}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.FilterResources(filters)
	}
}

func BenchmarkFilterResources_5000(b *testing.B) {
	d := buildBenchDiagram(5000)
	filters := map[string]string{"type": string(ResourceTypeEC2Instance), "state": "active"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.FilterResources(filters)
	}
}

// ─── SearchResources benchmarks ──────────────────────────────────────────────

func BenchmarkSearchResources_1000(b *testing.B) {
	d := buildBenchDiagram(1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.SearchResources("resource-42")
	}
}

func BenchmarkSearchResources_5000(b *testing.B) {
	d := buildBenchDiagram(5000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.SearchResources("resource-42")
	}
}

// ─── Connection ID collision test (issue #2) ─────────────────────────────────

func TestConnectionIDCollision(t *testing.T) {
	builder := NewDiagramBuilder("test", "test")
	builder.AddResource(Resource{ID: "a", Name: "A", Type: ResourceTypeEC2Instance})
	builder.AddResource(Resource{ID: "b", Name: "B", Type: ResourceTypeS3Bucket})

	// Two connections between the same resources, same type, different descriptions
	ok1 := builder.AddConnection("a", "b", ConnectionTypeData, "reads from")
	ok2 := builder.AddConnection("a", "b", ConnectionTypeData, "writes to")

	if !ok1 || !ok2 {
		t.Fatal("AddConnection returned false unexpectedly")
	}

	diagram := builder.Build()
	if len(diagram.Connections) != 2 {
		t.Fatalf("expected 2 connections, got %d", len(diagram.Connections))
	}

	id1 := diagram.Connections[0].ID
	id2 := diagram.Connections[1].ID

	if id1 == id2 {
		t.Errorf("connection ID collision: both connections have ID %q (descriptions: %q vs %q)",
			id1, diagram.Connections[0].Description, diagram.Connections[1].Description)
	}
}
