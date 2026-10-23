package exporters

import (
	"fmt"
	"regexp"
	"strings"

	"terraviz/internal/models"
)

var (
	mermaidWhitespace = regexp.MustCompile(`\s+`)
	mermaidUnsafe     = regexp.MustCompile(`[&"#;{}()|\[\]]`)
	mermaidIDUnsafe   = regexp.MustCompile(`[^A-Za-z0-9_]`)
)

// mermaidEscapes maps the characters that would break out of a Mermaid node or
// edge label to the entity Mermaid understands.
var mermaidEscapes = map[string]string{
	"&": "&amp;",
	`"`: "#quot;",
	"#": "#35;",
	";": "#59;",
	"{": "#123;",
	"}": "#125;",
	"[": "#91;",
	"]": "#93;",
	"(": "#40;",
	")": "#41;",
	"|": "#124;",
}

// escapeMermaidLabel makes a label safe inside a quoted Mermaid string. It
// mirrors escapeMermaidLabel in the browser so both exports look the same.
func escapeMermaidLabel(text string) string {
	collapsed := strings.TrimSpace(mermaidWhitespace.ReplaceAllString(text, " "))
	return mermaidUnsafe.ReplaceAllStringFunc(collapsed, func(ch string) string {
		return mermaidEscapes[ch]
	})
}

// mermaidNodeID builds a Mermaid node id. Mermaid only allows alphanumerics
// and underscores, and every id has to be unique.
func mermaidNodeID(resourceID string, used map[string]bool) string {
	base := "n_" + mermaidIDUnsafe.ReplaceAllString(resourceID, "_")
	id := base
	for suffix := 2; used[id]; suffix++ {
		id = fmt.Sprintf("%s_%d", base, suffix)
	}
	used[id] = true
	return id
}

// Mermaid renders the diagram as a Mermaid flowchart: one node per resource,
// one edge per connection, the connection type as the link label.
func Mermaid(d *models.Diagram) ([]byte, error) {
	if d == nil {
		return nil, fmt.Errorf("building Mermaid: no diagram")
	}

	var b strings.Builder
	b.WriteString("graph TD\n")

	used := map[string]bool{}
	ids := make(map[string]string, len(d.Resources))
	for _, resource := range d.Resources {
		id := mermaidNodeID(resource.ID, used)
		ids[resource.ID] = id

		label := resource.Name
		if label == "" {
			label = resource.ID
		}
		fmt.Fprintf(&b, "  %s[\"%s\"]\n", id, escapeMermaidLabel(label))
	}

	for _, connection := range d.Connections {
		from, to := ids[connection.SourceID], ids[connection.TargetID]
		if from == "" || to == "" {
			continue
		}
		label := string(connection.Type)
		if label == "" {
			label = "default"
		}
		fmt.Fprintf(&b, "  %s -->|\"%s\"| %s\n", from, escapeMermaidLabel(label), to)
	}

	return []byte(b.String()), nil
}
