// Package mcp exposes a TerraViz diagram to AI agents over the Model Context
// Protocol. It speaks JSON-RPC 2.0 as newline-delimited messages on stdio, so
// it needs no network port and no credentials.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"terraviz/internal/exporters"
	"terraviz/internal/models"
)

const protocolVersion = "2024-11-05"

// Server answers MCP requests against a loaded scan result.
type Server struct {
	result *models.ScanResult
}

// NewServer creates a server for a scan result.
func NewServer(result *models.ScanResult) *Server {
	return &Server{result: result}
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type toolResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Serve reads requests from in until EOF and writes responses to out.
// Notifications (requests with no id) do not get a response.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	enc := json.NewEncoder(out)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			if err := enc.Encode(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: -32700, Message: "parse error"}}); err != nil {
				return fmt.Errorf("writing response: %w", err)
			}
			continue
		}

		// A missing id means a notification; there is nothing to reply to.
		if len(req.ID) == 0 {
			continue
		}

		result, rpcErr := s.dispatch(req)
		resp := response{JSONRPC: "2.0", ID: req.ID, Result: result}
		if rpcErr != nil {
			resp.Result = nil
			resp.Error = rpcErr
		}
		if err := enc.Encode(resp); err != nil {
			return fmt.Errorf("writing response: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading requests: %w", err)
	}
	return nil
}

func (s *Server) dispatch(req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "terraviz", "version": "1.0.0"},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": tools()}, nil
	case "tools/call":
		var call toolCall
		if err := json.Unmarshal(req.Params, &call); err != nil {
			return nil, &rpcError{Code: -32602, Message: "invalid params"}
		}
		return s.callTool(call), nil
	default:
		return nil, &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}
}

func (s *Server) callTool(call toolCall) toolResult {
	data, err := s.runTool(call)
	if err != nil {
		return toolResult{Content: []toolContent{{Type: "text", Text: err.Error()}}, IsError: true}
	}
	return toolResult{Content: []toolContent{{Type: "text", Text: data}}}
}

func (s *Server) runTool(call toolCall) (string, error) {
	if s.result == nil {
		return "", fmt.Errorf("no diagram loaded")
	}
	diagram := s.result.Diagram
	args := call.Arguments

	switch call.Name {
	case "get_diagram":
		providers := map[string]int{}
		for _, r := range diagram.Resources {
			providers[r.Provider]++
		}
		return jsonString(map[string]any{
			"name": diagram.Name, "source": diagram.Source,
			"resources": len(diagram.Resources), "connections": len(diagram.Connections),
			"providers": providers,
		})

	case "list_resources":
		filters := map[string]string{}
		for _, key := range []string{"type", "provider", "region"} {
			if v, _ := args[key].(string); v != "" {
				filters[key] = v
			}
		}
		list := diagram.FilterResources(filters)
		if name, _ := args["name"].(string); name != "" {
			list = diagram.SearchResources(name)
		}
		limit := intFrom(args, "limit", 200)
		if limit < len(list) {
			list = list[:limit]
		}
		type compact struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Type     string `json:"type"`
			Provider string `json:"provider"`
			Region   string `json:"region"`
			State    string `json:"state"`
		}
		out := make([]compact, 0, len(list))
		for _, r := range list {
			out = append(out, compact{r.ID, r.Name, string(r.Type), r.Provider, r.Region, string(r.State)})
		}
		return jsonString(out)

	case "get_resource":
		id, _ := args["id"].(string)
		for i := range diagram.Resources {
			if diagram.Resources[i].ID == id {
				return jsonString(diagram.Resources[i])
			}
		}
		return "", fmt.Errorf("resource %q not found", id)

	case "list_connections":
		type compact struct {
			ID          string `json:"id"`
			SourceID    string `json:"source_id"`
			TargetID    string `json:"target_id"`
			Type        string `json:"type"`
			Description string `json:"description,omitempty"`
		}
		out := make([]compact, 0, len(diagram.Connections))
		for _, c := range diagram.Connections {
			if want, _ := args["resource_id"].(string); want != "" && c.SourceID != want && c.TargetID != want {
				continue
			}
			if want, _ := args["type"].(string); want != "" && string(c.Type) != want {
				continue
			}
			out = append(out, compact{c.ID, c.SourceID, c.TargetID, string(c.Type), c.Description})
		}
		return jsonString(out)

	case "get_stats":
		return jsonString(diagram.GetResourceStats())

	case "get_missing":
		missing := s.result.ReferencedMissing
		if missing == nil {
			missing = []models.ReferencedMissing{}
		}
		return jsonString(missing)

	case "export_diagram":
		format, _ := args["format"].(string)
		if format == "" {
			format = exporters.FormatMermaid
		}
		data, err := exporters.Export(&diagram, format)
		if err != nil {
			return "", err
		}
		return string(data), nil

	default:
		return "", fmt.Errorf("unknown tool: %s", call.Name)
	}
}

func jsonString(v any) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encoding result: %w", err)
	}
	return string(data), nil
}

func intFrom(args map[string]any, key string, fallback int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return fallback
	}
}

func tools() []map[string]any {
	schema := func(props map[string]any, required []string) map[string]any {
		s := map[string]any{"type": "object", "properties": props}
		if len(required) > 0 {
			s["required"] = required
		}
		return s
	}
	str := map[string]any{"type": "string"}
	return []map[string]any{
		{"name": "get_diagram", "description": "Overview of the loaded diagram: name, source and resource/connection counts.", "inputSchema": schema(map[string]any{}, nil)},
		{"name": "list_resources", "description": "List resources, optionally filtered by type, provider, region or name.", "inputSchema": schema(map[string]any{
			"type": str, "provider": str, "region": str, "name": str, "limit": map[string]any{"type": "integer"},
		}, nil)},
		{"name": "get_resource", "description": "Get the full properties of one resource by its ID.", "inputSchema": schema(map[string]any{"id": str}, []string{"id"})},
		{"name": "list_connections", "description": "List typed edges, optionally filtered by resource_id or type.", "inputSchema": schema(map[string]any{"resource_id": str, "type": str}, nil)},
		{"name": "get_stats", "description": "Resource counts grouped by type, provider, region and state.", "inputSchema": schema(map[string]any{}, nil)},
		{"name": "get_missing", "description": "Resources referenced by the diagram that are not part of it.", "inputSchema": schema(map[string]any{}, nil)},
		{"name": "export_diagram", "description": "Export the diagram as mermaid, dot, csv, markdown or drawio text.", "inputSchema": schema(map[string]any{
			"format": map[string]any{"type": "string", "enum": []string{"mermaid", "dot", "csv", "markdown", "drawio"}},
		}, []string{"format"})},
	}
}
