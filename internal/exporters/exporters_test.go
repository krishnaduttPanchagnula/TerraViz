package exporters

import (
	"strings"
	"testing"

	"terraviz/internal/models"
)

func sampleDiagram() *models.Diagram {
	return &models.Diagram{
		Name: "Test Diagram",
		Resources: []models.Resource{
			{ID: "aws_lb_listener.main", Name: `Listener "prod" {x}`, Type: models.ResourceTypeLBListener, Provider: "aws", Properties: map[string]interface{}{}, IconURL: "/icons/test.svg", InStateFile: true},
			{ID: "aws_instance.app", Name: "app", Type: models.ResourceTypeEC2Instance, Provider: "aws", Properties: map[string]interface{}{}, IconURL: "/icons/test.svg", InStateFile: true},
		},
		Connections: []models.Connection{
			{ID: "c1", SourceID: "aws_lb_listener.main", TargetID: "aws_instance.app", Type: models.ConnectionTypeData, Description: "Listener forwards to instance", Properties: map[string]interface{}{"reasons": []interface{}{"dependency", "reference"}}},
			{ID: "c2", SourceID: "aws_instance.app", TargetID: "gone", Type: models.ConnectionTypeNetworking},
		},
	}
}

func TestMermaidEscaping(t *testing.T) {
	out, err := Mermaid(sampleDiagram())
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	// raw ids must be sanitized
	if strings.Contains(s, "aws_lb_listener.main") {
		t.Error("raw id leaked into mermaid output")
	}
	if !strings.Contains(s, "n_aws_lb_listener_main") {
		t.Error("sanitized node id missing")
	}
	// quotes become #quot; and braces become #123;/#125;
	if !strings.Contains(s, "#quot;") {
		t.Error("double quotes not escaped")
	}
	if !strings.Contains(s, "#123;") || !strings.Contains(s, "#125;") {
		t.Error("braces not escaped")
	}
	if strings.Contains(s, "\"prod\"") {
		t.Error("raw quotes survive")
	}
	// connection type is the link label
	if !strings.Contains(s, `-->|"data"|`) {
		t.Error(`connection type not used as label (expected -->|"data"|)`)
	}
	// edge to missing resource dropped
	if strings.Count(s, "-->") != 1 {
		t.Errorf("expected 1 edge (drop dangling), got %d", strings.Count(s, "-->"))
	}
}

func TestMarkdownHasMermaidFence(t *testing.T) {
	out, err := Markdown(sampleDiagram())
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "```mermaid") || !strings.Contains(s, "```") {
		t.Error("mermaid fence missing")
	}
}
