// Package summary turns a dependency graph into a short architecture brief.
// The deterministic builder is the default and never leaves the machine. An
// optional OpenAI-compatible endpoint (Ollama by default) can rewrite it, but
// only loopback addresses are accepted so the "nothing leaves the machine"
// promise holds.
package summary

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"terraviz/internal/models"
)

const defaultModel = "llama3.2"

// Build renders a deterministic, offline summary of the diagram.
func Build(d *models.Diagram) string {
	if d == nil || len(d.Resources) == 0 {
		return "No resources in this diagram."
	}

	byType := map[string]int{}
	byProvider := map[string]int{}
	byRegion := map[string]int{}
	byConnection := map[string]int{}
	for _, r := range d.Resources {
		byType[string(r.Type)]++
		if r.Provider != "" {
			byProvider[r.Provider]++
		}
		if r.Region != "" {
			byRegion[r.Region]++
		}
	}
	for _, c := range d.Connections {
		byConnection[string(c.Type)]++
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Architecture summary: %s\n\n", diagramName(d.Name))
	fmt.Fprintf(&b, "%d resources, %d connections.", len(d.Resources), len(d.Connections))
	if len(byProvider) > 0 {
		fmt.Fprintf(&b, " Providers: %s.", joinCounts(byProvider))
	}
	if len(byRegion) > 0 {
		fmt.Fprintf(&b, " Regions: %s.", joinCounts(byRegion))
	}
	b.WriteString("\n\nResource types:\n")
	for _, k := range sortedKeys(byType) {
		fmt.Fprintf(&b, "  - %s: %d\n", k, byType[k])
	}

	if len(d.Connections) > 0 {
		b.WriteString("\nConnections by type:\n")
		for _, k := range sortedKeys(byConnection) {
			fmt.Fprintf(&b, "  - %s: %d\n", k, byConnection[k])
		}
	}

	// Show a bounded, ordered set of edges so the brief is reproducible.
	edges := make([]string, 0, len(d.Connections))
	for _, c := range d.Connections {
		if c.SourceID == "" || c.TargetID == "" {
			continue
		}
		edges = append(edges, fmt.Sprintf("  - %s -> %s [%s] %s",
			c.SourceID, c.TargetID, c.Type, strings.TrimSpace(c.Description)))
	}
	sort.Strings(edges)
	if len(edges) > 0 {
		b.WriteString("\nNotable relationships:\n")
		for i, e := range edges {
			if i == 12 {
				fmt.Fprintf(&b, "  ... and %d more\n", len(edges)-i)
				break
			}
			b.WriteString(e)
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// Generate returns an offline summary, optionally rewritten by a local LLM.
// source is "local" or "llm".
func Generate(ctx context.Context, d *models.Diagram, endpoint string) (text, source string, err error) {
	base := Build(d)
	if strings.TrimSpace(endpoint) == "" {
		return base, "local", nil
	}
	if err := validateLocal(endpoint); err != nil {
		return base, "local", err
	}

	prompt := "You are a cloud architect. Rewrite this infrastructure summary into a " +
		"concise brief for a reviewer. Keep every resource and connection fact accurate; " +
		"do not invent resources.\n\n" + base

	body, err := json.Marshal(map[string]any{
		"model": defaultModel,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"stream": false,
	})
	if err != nil {
		return base, "local", fmt.Errorf("encoding prompt: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return base, "local", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return base, "local", fmt.Errorf("calling local model: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return base, "local", fmt.Errorf("local model returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}

	// Ollama's native /api/generate and the OpenAI-compatible chat shape both
	// expose message content; accept either.
	var parsed struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Response string `json:"response"`
		Choices  []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return base, "local", fmt.Errorf("reading model response: %w", err)
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return base, "local", fmt.Errorf("decoding model response: %w", err)
	}
	answer := parsed.Message.Content
	if answer == "" {
		answer = parsed.Response
	}
	if answer == "" && len(parsed.Choices) > 0 {
		answer = parsed.Choices[0].Message.Content
	}
	if strings.TrimSpace(answer) == "" {
		return base, "local", fmt.Errorf("local model returned no text")
	}
	return strings.TrimSpace(answer) + "\n", "llm", nil
}

// validateLocal rejects non-loopback endpoints so a configured LLM cannot be
// used to exfiltrate the graph to a remote host.
func validateLocal(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid LLM endpoint: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("LLM endpoint must be http(s)")
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() {
			return nil
		}
		return fmt.Errorf("LLM endpoint must be on localhost (got %s)", host)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	return fmt.Errorf("LLM endpoint must be on localhost (got %s)", host)
}

func diagramName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "Infrastructure"
	}
	return name
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func joinCounts(m map[string]int) string {
	parts := make([]string, 0, len(m))
	for _, k := range sortedKeys(m) {
		parts = append(parts, fmt.Sprintf("%s (%d)", k, m[k]))
	}
	return strings.Join(parts, ", ")
}
