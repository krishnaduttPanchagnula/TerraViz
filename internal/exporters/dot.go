package exporters

import (
	"fmt"
	"strings"

	"terraviz/internal/models"
)

// dotEscaper escapes a string so it can live inside a quoted Graphviz string.
var dotEscaper = strings.NewReplacer(
	`\`, `\\`,
	`"`, `\"`,
	"\r\n", `\n`,
	"\n", `\n`,
	"\r", `\n`,
)

// Dot renders the diagram as a Graphviz DOT digraph: one box per resource
// (label = name and type) and one labelled edge per connection.
func Dot(d *models.Diagram) ([]byte, error) {
	if d == nil {
		return nil, fmt.Errorf("building DOT: no diagram")
	}

	var b strings.Builder
	b.WriteString("digraph \"architecture\" {\n")

	known := make(map[string]bool, len(d.Resources))
	for _, resource := range d.Resources {
		known[resource.ID] = true

		name := resource.Name
		if name == "" {
			name = resource.ID
		}
		label := dotEscaper.Replace(name) + "\\n" + dotEscaper.Replace(string(resource.Type))
		fmt.Fprintf(&b, "  \"%s\" [label=\"%s\", shape=box];\n", dotEscaper.Replace(resource.ID), label)
	}

	for _, connection := range d.Connections {
		if !known[connection.SourceID] || !known[connection.TargetID] {
			continue
		}
		label := string(connection.Type)
		if label == "" {
			label = "default"
		}
		fmt.Fprintf(&b, "  \"%s\" -> \"%s\" [label=\"%s\"];\n",
			dotEscaper.Replace(connection.SourceID),
			dotEscaper.Replace(connection.TargetID),
			dotEscaper.Replace(label),
		)
	}

	b.WriteString("}\n")
	return []byte(b.String()), nil
}
