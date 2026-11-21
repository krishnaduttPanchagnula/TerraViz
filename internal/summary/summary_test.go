package summary

import (
	"context"
	"strings"
	"testing"

	"terraviz/internal/models"
)

func diagram() *models.Diagram {
	return &models.Diagram{
		Name: "prod",
		Resources: []models.Resource{
			{ID: "aws_vpc.main", Name: "main", Type: models.ResourceTypeVPC, Provider: "aws", Region: "us-east-1"},
			{ID: "aws_instance.app", Name: "app", Type: models.ResourceTypeEC2Instance, Provider: "aws", Region: "us-east-1"},
		},
		Connections: []models.Connection{
			{ID: "c1", SourceID: "aws_instance.app", TargetID: "aws_vpc.main", Type: models.ConnectionTypeNetworking, Description: "in VPC"},
		},
	}
}

func TestBuildOffline(t *testing.T) {
	text := Build(diagram())
	for _, want := range []string{"prod", "2 resources, 1 connections", "aws:ec2:instance", "aws:ec2:vpc", "in VPC"} {
		if !strings.Contains(text, want) {
			t.Errorf("summary missing %q:\n%s", want, text)
		}
	}
	if Build(nil) == "" {
		t.Error("nil diagram should still produce text")
	}
}

func TestGenerateDefaultsToLocal(t *testing.T) {
	text, source, err := Generate(context.Background(), diagram(), "")
	if err != nil || source != "local" {
		t.Fatalf("source=%s err=%v", source, err)
	}
	if !strings.Contains(text, "2 resources") {
		t.Error("offline summary missing")
	}
}

func TestRejectsRemoteEndpoint(t *testing.T) {
	_, source, err := Generate(context.Background(), diagram(), "https://example.com/v1/chat/completions")
	if err == nil {
		t.Fatal("expected remote endpoint to be rejected")
	}
	if source != "local" {
		t.Errorf("should fall back to local, got %s", source)
	}
}
