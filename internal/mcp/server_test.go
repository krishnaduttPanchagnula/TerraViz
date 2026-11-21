package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"terraviz/internal/models"
)

func testServer() *Server {
	d := &models.Diagram{
		Name:   "Test",
		Source: "terraform",
		Resources: []models.Resource{
			{ID: "aws_instance.app", Name: "app", Type: models.ResourceTypeEC2Instance, Provider: "aws", Region: "us-east-1", State: models.ResourceStateActive, Properties: map[string]interface{}{"id": "i-1"}},
		},
		Connections: []models.Connection{
			{ID: "c1", SourceID: "aws_instance.app", TargetID: "aws_s3_bucket.logs", Type: models.ConnectionTypeData},
		},
	}
	return NewServer(&models.ScanResult{Diagram: *d})
}

// exchange sends one JSON-RPC message and returns the decoded response.
func exchange(t *testing.T, s *Server, msg string) map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := s.Serve(context.Background(), strings.NewReader(msg+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) == "" {
		return nil
	}
	var resp map[string]any
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("bad response %q: %v", out.String(), err)
	}
	return resp
}

func TestInitializeAndToolsList(t *testing.T) {
	s := testServer()
	init := exchange(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	result := init["result"].(map[string]any)
	if result["protocolVersion"] != protocolVersion {
		t.Errorf("unexpected protocol version: %v", result["protocolVersion"])
	}
	if _, ok := result["capabilities"].(map[string]any)["tools"]; !ok {
		t.Error("tools capability missing")
	}

	list := exchange(t, s, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	tools := list["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 7 {
		t.Errorf("expected 7 tools, got %d", len(tools))
	}
}

func TestGetResource(t *testing.T) {
	s := testServer()
	resp := exchange(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_resource","arguments":{"id":"aws_instance.app"}}}`)
	result := resp["result"].(map[string]any)
	if result["isError"] == true {
		t.Fatalf("unexpected tool error: %v", result)
	}
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "i-1") {
		t.Errorf("resource properties missing: %s", text)
	}

	missing := exchange(t, s, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_resource","arguments":{"id":"nope"}}}`)
	if missing["result"].(map[string]any)["isError"] != true {
		t.Error("expected isError for missing resource")
	}
}

func TestExportTool(t *testing.T) {
	s := testServer()
	resp := exchange(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"export_diagram","arguments":{"format":"mermaid"}}}`)
	text := resp["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "graph TD") {
		t.Errorf("mermaid output missing: %s", text)
	}
}

func TestErrorsAndNotifications(t *testing.T) {
	s := testServer()
	bad := exchange(t, s, `{"jsonrpc":"2.0","id":1,"method":"nope"}`)
	if bad["error"].(map[string]any)["code"] != float64(-32601) {
		t.Errorf("expected method-not-found error, got %v", bad)
	}

	parse := exchange(t, s, `not json`)
	if parse["error"].(map[string]any)["code"] != float64(-32700) {
		t.Errorf("expected parse error, got %v", parse)
	}

	var out bytes.Buffer
	msg := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	if err := s.Serve(context.Background(), strings.NewReader(msg+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("notification produced a response: %s", out.String())
	}
}
