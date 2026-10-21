package relations

import (
	"testing"

	"terraviz/internal/models"
)

// resource builds a resource with the given id, type and properties.
func resource(id string, resourceType models.ResourceType, properties map[string]interface{}) models.Resource {
	return models.Resource{
		ID:         id,
		Name:       id,
		Type:       resourceType,
		Properties: properties,
	}
}

// reasons returns the reasons of an edge.
func reasons(t *testing.T, edge Edge) []string {
	t.Helper()
	values, ok := edge.Properties["reasons"].([]string)
	if !ok {
		t.Fatalf("edge %q has no reasons: %#v", edge.Description, edge.Properties)
	}
	return values
}

func TestAddMergesEdgesOfTheSamePair(t *testing.T) {
	collector := NewCollector()
	collector.Add("a", "b", models.ConnectionTypeData, "reads from", nil)
	collector.Add("a", "b", models.ConnectionTypeAccess, "writes to", map[string]interface{}{"port": 443})

	edges := collector.Edges()
	if len(edges) != 1 {
		t.Fatalf("edge count = %d, want 1: %#v", len(edges), edges)
	}

	edge := edges[0]
	if edge.Type != models.ConnectionTypeData {
		t.Errorf("edge type = %q, want the type of the first edge %q", edge.Type, models.ConnectionTypeData)
	}
	if edge.Description != "reads from" {
		t.Errorf("edge description = %q, want %q", edge.Description, "reads from")
	}
	if got := reasons(t, edge); len(got) != 2 || got[0] != "reads from" || got[1] != "writes to" {
		t.Errorf("reasons = %#v, want [reads from writes to]", got)
	}
	if edge.Properties["port"] != 443 {
		t.Errorf("merged property port = %v, want 443", edge.Properties["port"])
	}
}

func TestAddMergesOppositeDirectionAsBidirectional(t *testing.T) {
	collector := NewCollector()
	collector.Add("a", "b", models.ConnectionTypeData, "a to b", nil)
	collector.Add("b", "a", models.ConnectionTypeData, "b to a", nil)

	edges := collector.Edges()
	if len(edges) != 1 {
		t.Fatalf("edge count = %d, want 1: %#v", len(edges), edges)
	}
	if !edges[0].Bidirectional {
		t.Error("edge merged from the opposite direction must be bidirectional")
	}
	if got := reasons(t, edges[0]); len(got) != 2 {
		t.Errorf("reasons = %#v, want two entries", got)
	}
}

func TestAddKeepsExistingProperties(t *testing.T) {
	collector := NewCollector()
	collector.Add("a", "b", models.ConnectionTypeData, "first", map[string]interface{}{"port": 80})
	collector.Add("a", "b", models.ConnectionTypeData, "second", map[string]interface{}{"port": 443, "protocol": "HTTP"})

	edge := collector.Edges()[0]
	if edge.Properties["port"] != 80 {
		t.Errorf("port = %v, want the value of the first edge 80", edge.Properties["port"])
	}
	if edge.Properties["protocol"] != "HTTP" {
		t.Errorf("protocol = %v, want HTTP", edge.Properties["protocol"])
	}
}

func TestAddIgnoresSelfAndEmptyReferences(t *testing.T) {
	collector := NewCollector()
	collector.Add("a", "a", models.ConnectionTypeData, "self", nil)
	collector.Add("", "b", models.ConnectionTypeData, "empty source", nil)
	collector.Add("a", "", models.ConnectionTypeData, "empty target", nil)

	if len(collector.Edges()) != 0 {
		t.Fatalf("edge count = %d, want 0: %#v", len(collector.Edges()), collector.Edges())
	}
}

func TestAddDependencyIsSkippedWhenSemanticEdgeExists(t *testing.T) {
	collector := NewCollector()
	collector.Add("a", "b", models.ConnectionTypeNetworking, "a in b", nil)
	collector.AddDependency("a", "b")

	edges := collector.Edges()
	if len(edges) != 1 {
		t.Fatalf("edge count = %d, want 1: %#v", len(edges), edges)
	}
	if edges[0].Type != models.ConnectionTypeNetworking {
		t.Errorf("edge type = %q, want the semantic type", edges[0].Type)
	}
}

func TestAddDependencyIsSkippedForOppositeDirection(t *testing.T) {
	collector := NewCollector()
	collector.Add("b", "a", models.ConnectionTypeNetworking, "a in b", nil)
	collector.AddDependency("a", "b")

	if len(collector.Edges()) != 1 {
		t.Fatalf("edge count = %d, want 1: %#v", len(collector.Edges()), collector.Edges())
	}
}

func TestAddDependencyIsRecordedWhenPairIsUnconnected(t *testing.T) {
	collector := NewCollector()
	collector.AddDependency("a", "b")

	edges := collector.Edges()
	if len(edges) != 1 {
		t.Fatalf("edge count = %d, want 1: %#v", len(edges), edges)
	}
	if edges[0].Type != models.ConnectionTypeDependency {
		t.Errorf("edge type = %q, want %q", edges[0].Type, models.ConnectionTypeDependency)
	}
	if edges[0].Description != "Terraform dependency" {
		t.Errorf("edge description = %q, want %q", edges[0].Description, "Terraform dependency")
	}
	if !edges[0].Bidirectional {
		t.Error("dependency edges must be bidirectional")
	}
}

func TestAnalyzeGenericInfersNetworkEdges(t *testing.T) {
	resources := []models.Resource{
		resource("vpc", models.ResourceTypeVPC, map[string]interface{}{"id": "vpc-1"}),
		resource("subnet", models.ResourceTypeSubnet, map[string]interface{}{"id": "subnet-1", "vpc_id": "vpc-1"}),
		resource("sg", models.ResourceTypeSecurityGroup, map[string]interface{}{"id": "sg-1", "vpc_id": "vpc-1"}),
		resource("instance", models.ResourceTypeEC2Instance, map[string]interface{}{
			"id":                     "i-1",
			"vpc_id":                 "vpc-1",
			"subnet_id":              "subnet-1",
			"vpc_security_group_ids": []interface{}{"sg-1"},
		}),
	}

	edges := AnalyzeGeneric(resources)
	byPair := make(map[string]Edge, len(edges))
	for _, edge := range edges {
		byPair[edge.Source+"->"+edge.Target] = edge
	}

	cases := []struct {
		source      string
		target      string
		edgeType    models.ConnectionType
		description string
	}{
		{"subnet", "vpc", models.ConnectionTypeNetworking, "Subnet belongs to VPC"},
		{"sg", "vpc", models.ConnectionTypeNetworking, "Security group belongs to VPC"},
		{"instance", "vpc", models.ConnectionTypeNetworking, "EC2 instance in VPC"},
		{"instance", "subnet", models.ConnectionTypeNetworking, "EC2 instance in subnet"},
		{"instance", "sg", models.ConnectionTypeAccess, "EC2 instance uses security group"},
	}

	for _, tc := range cases {
		edge, ok := byPair[tc.source+"->"+tc.target]
		if !ok {
			t.Fatalf("missing edge %s -> %s: %#v", tc.source, tc.target, edges)
		}
		if edge.Type != tc.edgeType {
			t.Errorf("edge %s -> %s type = %q, want %q", tc.source, tc.target, edge.Type, tc.edgeType)
		}
		if edge.Description != tc.description {
			t.Errorf("edge %s -> %s description = %q, want %q", tc.source, tc.target, edge.Description, tc.description)
		}
	}
}

func TestAnalyzeGenericInfersSecurityGroupRuleEdges(t *testing.T) {
	resources := []models.Resource{
		resource("sg-a", models.ResourceTypeSecurityGroup, map[string]interface{}{
			"id":     "sg-a",
			"vpc_id": "vpc-1",
			"ingress": []interface{}{
				map[string]interface{}{
					"protocol":        "tcp",
					"from_port":       float64(443),
					"to_port":         float64(443),
					"cidr_blocks":     []interface{}{"10.0.0.0/16"},
					"security_groups": []interface{}{"sg-b"},
				},
			},
		}),
		resource("sg-b", models.ResourceTypeSecurityGroup, map[string]interface{}{"id": "sg-b"}),
	}

	edges := AnalyzeGeneric(resources)

	var found *Edge
	for i := range edges {
		if edges[i].Source == "sg-a" && edges[i].Target == "sg-b" {
			found = &edges[i]
			break
		}
	}
	if found == nil {
		t.Fatal("missing security group rule edge")
	}
	if found.Type != models.ConnectionTypeAccess {
		t.Errorf("edge type = %q, want %q", found.Type, models.ConnectionTypeAccess)
	}
	if found.Description != "Security group allows traffic from security group" {
		t.Errorf("edge description = %q", found.Description)
	}
	if found.Properties["protocol"] != "tcp" {
		t.Errorf("protocol = %v, want tcp", found.Properties["protocol"])
	}
	if found.Properties["from_port"] != float64(443) {
		t.Errorf("from_port = %v, want 443", found.Properties["from_port"])
	}
	if found.Properties["to_port"] != float64(443) {
		t.Errorf("to_port = %v, want 443", found.Properties["to_port"])
	}
	if cidr, ok := found.Properties["cidr_blocks"].([]interface{}); !ok || len(cidr) != 1 {
		t.Errorf("cidr_blocks = %v, want one entry", found.Properties["cidr_blocks"])
	}
}

func TestAnalyzeGenericHandlesStringListSecurityGroups(t *testing.T) {
	resources := []models.Resource{
		resource("sg", models.ResourceTypeSecurityGroup, map[string]interface{}{"id": "sg-1"}),
		resource("instance", models.ResourceTypeEC2Instance, map[string]interface{}{
			"id":                     "i-1",
			"vpc_security_group_ids": []string{"sg-1"},
		}),
	}

	var found bool
	for _, edge := range AnalyzeGeneric(resources) {
		if edge.Source == "instance" && edge.Target == "sg" {
			found = true
			if edge.Description != "EC2 instance uses security group" {
				t.Errorf("edge description = %q", edge.Description)
			}
		}
	}
	if !found {
		t.Fatal("missing instance -> security group edge")
	}
}

func TestAnalyzeGenericDoesNotModifyInput(t *testing.T) {
	resources := []models.Resource{
		resource("sg", models.ResourceTypeSecurityGroup, map[string]interface{}{"id": "sg-1"}),
		resource("instance", models.ResourceTypeEC2Instance, map[string]interface{}{
			"id":                     "i-1",
			"vpc_security_group_ids": []string{"sg-1"},
		}),
	}

	AnalyzeGeneric(resources)

	if len(resources[0].Properties) != 1 {
		t.Fatalf("input was modified: %#v", resources[0].Properties)
	}
	if len(resources[1].Properties) != 2 {
		t.Fatalf("input was modified: %#v", resources[1].Properties)
	}
}
