package exporters

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"terraviz/internal/models"
)

// csvHeaders are the columns of the CSV export. They match the browser
// export so both files can be diffed against each other.
var csvHeaders = []string{
	"id", "name", "type", "provider", "region", "account", "state", "source",
	"in_state_file", "created_at", "updated_at", "icon_url", "x", "y", "hidden",
	"tags", "properties", "incoming_connections", "outgoing_connections",
}

// csvConnection is a connection plus the resource at its other end, the same
// shape the browser export writes.
type csvConnection struct {
	models.Connection
	ConnectedResourceID string `json:"connected_resource_id"`
}

// CSV renders one row per resource, with the connections that point at it and
// the connections that leave it.
func CSV(d *models.Diagram) ([]byte, error) {
	if d == nil {
		return nil, fmt.Errorf("building CSV: no diagram")
	}

	var b strings.Builder
	b.WriteString(strings.Join(csvHeaders, ","))
	b.WriteString("\r\n")

	for _, resource := range d.Resources {
		incoming := []csvConnection{}
		outgoing := []csvConnection{}
		for _, connection := range d.Connections {
			switch {
			case connection.TargetID == resource.ID:
				incoming = append(incoming, csvConnection{
					Connection:          connection,
					ConnectedResourceID: connection.SourceID,
				})
			case connection.SourceID == resource.ID:
				outgoing = append(outgoing, csvConnection{
					Connection:          connection,
					ConnectedResourceID: connection.TargetID,
				})
			}
		}

		tags := resource.Tags
		if tags == nil {
			tags = map[string]string{}
		}
		properties := resource.Properties
		if properties == nil {
			properties = map[string]interface{}{}
		}

		fields := []string{
			resource.ID,
			resource.Name,
			string(resource.Type),
			resource.Provider,
			resource.Region,
			resource.Account,
			string(resource.State),
			resource.Source,
			strconv.FormatBool(resource.InStateFile),
			formatTime(resource.CreatedAt),
			formatTime(resource.UpdatedAt),
			resource.IconURL,
			strconv.FormatFloat(resource.X, 'f', -1, 64),
			strconv.FormatFloat(resource.Y, 'f', -1, 64),
			strconv.FormatBool(resource.Hidden),
		}
		fields = append(fields,
			marshalJSON(tags),
			marshalJSON(properties),
			marshalJSON(incoming),
			marshalJSON(outgoing),
		)

		for i, field := range fields {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(csvField(field))
		}
		b.WriteString("\r\n")
	}

	return []byte(b.String()), nil
}

// csvField quotes a value the way the browser export does.
func csvField(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

// marshalJSON renders a value as JSON, falling back to an empty object so a
// broken value cannot break the row.
func marshalJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// formatTime renders a timestamp the way the diagram JSON holds it.
func formatTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
