package exporters

import (
	"fmt"
	"strings"

	"terraviz/internal/models"
)

// Markdown renders a GitHub ready summary: the diagram as a Mermaid block, the
// resources that are referenced but missing from the state file, and a table
// of every resource. It is what a pull request comment carries.
func Markdown(d *models.Diagram) ([]byte, error) {
	if d == nil {
		return nil, fmt.Errorf("building Markdown: no diagram")
	}

	mermaid, err := Mermaid(d)
	if err != nil {
		return nil, fmt.Errorf("building the Mermaid diagram: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", markdownText(diagramName(d)))
	fmt.Fprintf(&b, "**Resources:** %d  \n", len(d.Resources))
	fmt.Fprintf(&b, "**Connections:** %d\n\n", len(d.Connections))

	b.WriteString("## Diagram\n\n```mermaid\n")
	b.Write(mermaid)
	b.WriteString("```\n\n")

	missing := make([]models.Resource, 0)
	for _, resource := range d.Resources {
		if !resource.InStateFile {
			missing = append(missing, resource)
		}
	}

	b.WriteString("## Referenced but missing from the state file\n\n")
	if len(missing) == 0 {
		b.WriteString("None. Every referenced resource is present in the state file.\n\n")
	} else {
		b.WriteString("| Resource | Type | Referenced value |\n|---|---|---|\n")
		for _, resource := range missing {
			fmt.Fprintf(&b, "| %s | %s | %s |\n",
				markdownCell(resource.Name),
				markdownCell(string(resource.Type)),
				markdownCell(referencedValue(resource)),
			)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Resources\n\n")
	b.WriteString("| Resource | Type | Provider | Region |\n|---|---|---|---|\n")
	for _, resource := range d.Resources {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n",
			markdownCell(resource.Name),
			markdownCell(string(resource.Type)),
			markdownCell(resource.Provider),
			markdownCell(resource.Region),
		)
	}

	return []byte(b.String()), nil
}

// referencedValue returns the value a placeholder resource was created for.
func referencedValue(resource models.Resource) string {
	if value, ok := resource.Properties["referenced_value"].(string); ok {
		return value
	}
	return ""
}

// markdownCell makes a value safe to put in a table cell.
func markdownCell(text string) string {
	text = strings.ReplaceAll(text, "|", `\|`)
	text = strings.ReplaceAll(text, "\r\n", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	return strings.TrimSpace(text)
}

// markdownText makes a value safe to put in a heading or a paragraph.
func markdownText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	return strings.TrimSpace(text)
}
