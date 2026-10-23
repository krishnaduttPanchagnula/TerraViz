// Package exporters renders a diagram into the file formats that CI and
// documentation pipelines need. Every exporter is a pure function: it takes a
// diagram and returns bytes, so it can be tested without a browser or a
// server.
package exporters

import (
	"fmt"
	"strings"

	"terraviz/internal/models"
)

// The formats the export command understands.
const (
	FormatHTML     = "html"
	FormatMermaid  = "mermaid"
	FormatDOT      = "dot"
	FormatDrawIO   = "drawio"
	FormatCSV      = "csv"
	FormatMarkdown = "markdown"
)

// Formats lists every supported export format.
var Formats = []string{FormatHTML, FormatMermaid, FormatDOT, FormatDrawIO, FormatCSV, FormatMarkdown}

// extensions maps a format to the file extension of its output.
var extensions = map[string]string{
	FormatHTML:     ".html",
	FormatMermaid:  ".mmd",
	FormatDOT:      ".dot",
	FormatDrawIO:   ".drawio",
	FormatCSV:      ".csv",
	FormatMarkdown: ".md",
}

// Extension returns the file extension used for a format.
func Extension(format string) string {
	if ext, ok := extensions[format]; ok {
		return ext
	}
	return ".txt"
}

// Export renders the diagram in the given format.
func Export(d *models.Diagram, format string) ([]byte, error) {
	if d == nil {
		return nil, fmt.Errorf("exporting %s: no diagram", format)
	}

	switch format {
	case FormatHTML:
		return HTML(d)
	case FormatMermaid:
		return Mermaid(d)
	case FormatDOT:
		return Dot(d)
	case FormatDrawIO:
		return DrawIO(d)
	case FormatCSV:
		return CSV(d)
	case FormatMarkdown:
		return Markdown(d)
	default:
		return nil, fmt.Errorf("unknown export format %q", format)
	}
}

// diagramName returns the name of the diagram, with a fallback for diagrams
// that were built without one.
func diagramName(d *models.Diagram) string {
	if d != nil && d.Name != "" {
		return d.Name
	}
	return "Infrastructure"
}

// htmlEscape makes a string safe to use as HTML text.
func htmlEscape(text string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(text)
}
