package models

import (
	"fmt"
	"hash/fnv"
	"strings"
)

const (
	// DiagramVersion is the current version of the diagram format.
	DiagramVersion = "1.0"
)

// DiagramBuilder helps build diagrams incrementally.
type DiagramBuilder struct {
	diagram     *Diagram
	resourceMap map[string]*Resource
}

// NewDiagramBuilder creates a new diagram builder.
func NewDiagramBuilder(name, source string) *DiagramBuilder {
	return &DiagramBuilder{
		diagram: &Diagram{
			ID:          generateID(name + source),
			Name:        name,
			Resources:   make([]Resource, 0),
			Connections: make([]Connection, 0),
			Metadata:    make(map[string]interface{}),
			Source:      source,
			Version:     DiagramVersion,
		},
		resourceMap: make(map[string]*Resource),
	}
}

// AddResource adds a resource to the diagram.
func (db *DiagramBuilder) AddResource(resource Resource) {
	if resource.ID == "" {
		resource.ID = generateID(resource.Name + string(resource.Type) + resource.Provider)
	}

	// Populate IconURL from the canonical icon map when not already set.
	if resource.IconURL == "" {
		resource.IconURL = resource.GetResourceIcon()
	}

	db.diagram.Resources = append(db.diagram.Resources, resource)
	// Point to the element within the slice to avoid stale pointers after reallocation.
	db.resourceMap[resource.ID] = &db.diagram.Resources[len(db.diagram.Resources)-1]
}

// AddConnection adds a connection between two resources.
// Returns false if either the source or target resource does not exist.
func (db *DiagramBuilder) AddConnection(sourceID, targetID string, connType ConnectionType, description string) bool {
	if _, exists := db.resourceMap[sourceID]; !exists {
		return false
	}
	if _, exists := db.resourceMap[targetID]; !exists {
		return false
	}

	conn := Connection{
		ID:          generateID(sourceID + targetID + string(connType)),
		SourceID:    sourceID,
		TargetID:    targetID,
		Type:        connType,
		Description: description,
		Properties:  make(map[string]interface{}),
	}

	db.diagram.Connections = append(db.diagram.Connections, conn)
	return true
}

// GetResource returns a resource by ID.
func (db *DiagramBuilder) GetResource(id string) (*Resource, bool) {
	resource, exists := db.resourceMap[id]
	return resource, exists
}

// Build returns the completed diagram.
func (db *DiagramBuilder) Build() *Diagram {
	return db.diagram
}

// FilterResources filters resources based on criteria.
func (d *Diagram) FilterResources(filters map[string]string) []Resource {
	var filtered []Resource

	for _, resource := range d.Resources {
		if matchesFilters(resource, filters) {
			filtered = append(filtered, resource)
		}
	}

	return filtered
}

// matchesFilters checks if a resource matches all the given filter criteria.
func matchesFilters(resource Resource, filters map[string]string) bool {
	for key, value := range filters {
		switch key {
		case "type":
			if string(resource.Type) != value {
				return false
			}
		case "provider":
			if resource.Provider != value {
				return false
			}
		case "region":
			if resource.Region != value {
				return false
			}
		case "state":
			if string(resource.State) != value {
				return false
			}
		case "tag":
			parts := strings.SplitN(value, ":", 2)
			if len(parts) == 2 {
				if tagValue, exists := resource.Tags[parts[0]]; !exists || tagValue != parts[1] {
					return false
				}
			}
		}
	}
	return true
}

// SearchResources searches for resources by name or ID.
func (d *Diagram) SearchResources(query string) []Resource {
	var results []Resource
	query = strings.ToLower(query)

	for _, resource := range d.Resources {
		if strings.Contains(strings.ToLower(resource.Name), query) ||
			strings.Contains(strings.ToLower(resource.ID), query) {
			results = append(results, resource)
		}
	}

	return results
}

// GetResourceStats returns statistics about the diagram in a single pass.
func (d *Diagram) GetResourceStats() map[string]int {
	stats := make(map[string]int)

	for _, resource := range d.Resources {
		stats[fmt.Sprintf("type:%s", resource.Type)]++
		stats[fmt.Sprintf("provider:%s", resource.Provider)]++
		stats[fmt.Sprintf("state:%s", resource.State)]++
	}

	stats["total_resources"] = len(d.Resources)
	stats["total_connections"] = len(d.Connections)

	return stats
}

// CompareDiagrams compares two diagrams and returns the differences.
func CompareDiagrams(base, compare *Diagram) *ComparisonResult {
	result := &ComparisonResult{
		BaseVersion:    base.ID,
		CompareVersion: compare.ID,
		Added:          make([]Resource, 0),
		Removed:        make([]Resource, 0),
		Modified:       make([]ResourceDiff, 0),
	}

	baseResources := indexResources(base.Resources)
	compareResources := indexResources(compare.Resources)

	// Find added resources.
	for _, resource := range compare.Resources {
		if _, exists := baseResources[resource.ID]; !exists {
			result.Added = append(result.Added, resource)
		}
	}

	// Find removed resources.
	for _, resource := range base.Resources {
		if _, exists := compareResources[resource.ID]; !exists {
			result.Removed = append(result.Removed, resource)
		}
	}

	// Find modified resources.
	for id, compareResource := range compareResources {
		if baseResource, exists := baseResources[id]; exists {
			changes := diffResources(baseResource, compareResource)
			if len(changes) > 0 {
				result.Modified = append(result.Modified, ResourceDiff{
					ResourceID: id,
					Changes:    changes,
				})
			}
		}
	}

	result.ConnectionsAdded, result.ConnectionsRemoved = diffConnections(base.Connections, compare.Connections)

	result.Summary = ComparisonSummary{
		AddedCount:     len(result.Added),
		RemovedCount:   len(result.Removed),
		ModifiedCount:  len(result.Modified),
		UnchangedCount: len(baseResources) - len(result.Removed) - len(result.Modified),
	}

	return result
}

// indexResources creates a lookup map from resource ID to resource.
func indexResources(resources []Resource) map[string]Resource {
	m := make(map[string]Resource, len(resources))
	for _, r := range resources {
		m[r.ID] = r
	}
	return m
}

// diffResources compares two resources and returns their differences.
func diffResources(base, compare Resource) map[string]PropertyChange {
	changes := make(map[string]PropertyChange)

	if base.Name != compare.Name {
		changes["name"] = PropertyChange{base.Name, compare.Name}
	}
	if base.State != compare.State {
		changes["state"] = PropertyChange{base.State, compare.State}
	}
	if base.Region != compare.Region {
		changes["region"] = PropertyChange{base.Region, compare.Region}
	}

	if !tagsEqual(base.Tags, compare.Tags) {
		changes["tags"] = PropertyChange{base.Tags, compare.Tags}
	}

	return changes
}

// tagsEqual reports whether two tag maps are equal.
func tagsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if bValue, exists := b[key]; !exists || bValue != value {
			return false
		}
	}
	return true
}

// diffConnections finds added and removed connections between two slices.
func diffConnections(base, compare []Connection) (added, removed []Connection) {
	baseMap := make(map[string]struct{}, len(base))
	for _, conn := range base {
		baseMap[conn.ID] = struct{}{}
	}

	compareMap := make(map[string]struct{}, len(compare))
	for _, conn := range compare {
		compareMap[conn.ID] = struct{}{}
	}

	for _, conn := range compare {
		if _, exists := baseMap[conn.ID]; !exists {
			added = append(added, conn)
		}
	}

	for _, conn := range base {
		if _, exists := compareMap[conn.ID]; !exists {
			removed = append(removed, conn)
		}
	}

	return added, removed
}

// generateID generates a consistent ID based on input using FNV-1a hash.
func generateID(input string) string {
	h := fnv.New64a()
	h.Write([]byte(input))
	return fmt.Sprintf("%016x", h.Sum64())[:12]
}
