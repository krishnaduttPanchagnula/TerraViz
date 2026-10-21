// Package relations infers the relationships between cloud resources. It is
// shared by the Terraform state parser and the live AWS scanner so that both
// produce the same edges for the same infrastructure.
package relations

import "terraviz/internal/models"

// Edge is a single relationship between two resources.
type Edge struct {
	Source        string
	Target        string
	Type          models.ConnectionType
	Description   string
	Properties    map[string]interface{}
	Bidirectional bool
}

// Collector collects edges and keeps a single edge per unordered resource
// pair. A second edge for the same pair merges into the first one instead of
// duplicating it: the first edge keeps its type and description, the new
// description is appended to the "reasons" property and the new properties
// are merged in.
type Collector struct {
	edges     []Edge
	pairIndex map[string]int // unordered pair -> index in edges
}

// NewCollector creates an empty edge collector.
func NewCollector() *Collector {
	return &Collector{pairIndex: make(map[string]int)}
}

// Add records an edge. Self references and empty ids are ignored.
func (c *Collector) Add(source, target string, connType models.ConnectionType, description string, properties map[string]interface{}) {
	if source == "" || target == "" || source == target {
		return
	}

	key := pairKey(source, target)
	if index, exists := c.pairIndex[key]; exists {
		c.merge(&c.edges[index], source, target, description, properties)
		return
	}

	edge := Edge{
		Source:        source,
		Target:        target,
		Type:          connType,
		Description:   description,
		Properties:    make(map[string]interface{}),
		Bidirectional: directionless(connType),
	}
	addReason(edge.Properties, description)
	mergeProperties(edge.Properties, properties)

	c.pairIndex[key] = len(c.edges)
	c.edges = append(c.edges, edge)
}

// AddDependency records a Terraform dependency edge, but only when the two
// resources are not already connected by a semantic edge in either direction
// (that edge carries more meaning than the raw dependency).
func (c *Collector) AddDependency(source, target string) {
	if source == "" || target == "" || source == target {
		return
	}
	if _, exists := c.pairIndex[pairKey(source, target)]; exists {
		return
	}
	c.Add(source, target, models.ConnectionTypeDependency, "Terraform dependency", nil)
}

// Edges returns the recorded edges in insertion order.
func (c *Collector) Edges() []Edge {
	return c.edges
}

// merge folds a second edge for the same pair into the existing one.
func (c *Collector) merge(edge *Edge, source, target, description string, properties map[string]interface{}) {
	if edge.Source != source || edge.Target != target {
		// The two edges point in opposite directions, so the direction of the
		// pair is arbitrary and the renderer should draw it as an association.
		edge.Bidirectional = true
	}
	addReason(edge.Properties, description)
	mergeProperties(edge.Properties, properties)
}

// addReason appends a description to the reason list of an edge, keeping the
// primary description first and dropping duplicates.
func addReason(properties map[string]interface{}, description string) {
	reasons, _ := properties["reasons"].([]string)
	if len(reasons) == 0 {
		properties["reasons"] = []string{description}
		return
	}
	for _, reason := range reasons {
		if reason == description {
			return
		}
	}
	properties["reasons"] = append(reasons, description)
}

// mergeProperties copies properties that the edge does not have yet.
func mergeProperties(edge, added map[string]interface{}) {
	for key, value := range added {
		if _, exists := edge[key]; !exists {
			edge[key] = value
		}
	}
}

// directionless reports whether the direction of an edge is arbitrary rather
// than a flow, so the renderer can draw arrows on both ends.
func directionless(connType models.ConnectionType) bool {
	return connType == models.ConnectionTypeDependency || connType == models.ConnectionTypeReference
}

// pairKey builds a direction independent key for a resource pair.
func pairKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "\x00" + b
}

// displayNames holds the per resource type name used in edge descriptions.
var displayNames = map[models.ResourceType]string{
	models.ResourceTypeEC2Instance:    "EC2 instance",
	models.ResourceTypeRDSInstance:    "RDS instance",
	models.ResourceTypeLambdaFunction: "Lambda function",
	models.ResourceTypeLoadBalancer:   "Load balancer",
	models.ResourceTypeECSService:     "ECS service",
	models.ResourceTypeSubnet:         "Subnet",
	models.ResourceTypeSecurityGroup:  "Security group",
}

// AnalyzeGeneric infers the relationships that hold for both Terraform state
// and live AWS scans. It only looks at normalized property keys, so the same
// infrastructure yields the same edges no matter where it was read from. The
// input is not modified.
func AnalyzeGeneric(resources []models.Resource) []Edge {
	index := newResourceIndex(resources)
	edges := make([]Edge, 0)

	for i := range resources {
		resource := &resources[i]
		name := displayName(resource)

		if vpcID, ok := stringProperty(resource.Properties, "vpc_id"); ok {
			if vpc := index.find(models.ResourceTypeVPC, vpcID); vpc != nil && vpc.ID != resource.ID {
				edges = append(edges, Edge{
					Source:      resource.ID,
					Target:      vpc.ID,
					Type:        models.ConnectionTypeNetworking,
					Description: vpcDescription(resource, name),
					Properties:  map[string]interface{}{},
				})
			}
		}

		for _, subnetID := range subnetIDs(resource) {
			if subnet := index.find(models.ResourceTypeSubnet, subnetID); subnet != nil && subnet.ID != resource.ID {
				edges = append(edges, Edge{
					Source:      resource.ID,
					Target:      subnet.ID,
					Type:        models.ConnectionTypeNetworking,
					Description: name + " in subnet",
					Properties:  map[string]interface{}{},
				})
			}
		}

		for _, sgID := range securityGroupIDs(resource) {
			if sg := index.find(models.ResourceTypeSecurityGroup, sgID); sg != nil && sg.ID != resource.ID {
				edges = append(edges, Edge{
					Source:      resource.ID,
					Target:      sg.ID,
					Type:        models.ConnectionTypeAccess,
					Description: name + " uses security group",
					Properties:  map[string]interface{}{},
				})
			}
		}

		if resource.Type != models.ResourceTypeSecurityGroup {
			continue
		}
		for _, rule := range ingressRules(resource) {
			for _, sgID := range stringList(rule["security_groups"]) {
				sg := index.find(models.ResourceTypeSecurityGroup, sgID)
				if sg == nil || sg.ID == resource.ID {
					continue
				}
				edges = append(edges, Edge{
					Source:      resource.ID,
					Target:      sg.ID,
					Type:        models.ConnectionTypeAccess,
					Description: "Security group allows traffic from security group",
					Properties:  ruleProperties(rule),
				})
			}
		}
	}

	return edges
}

// displayName returns the name used for a resource in edge descriptions.
func displayName(resource *models.Resource) string {
	if name, ok := displayNames[resource.Type]; ok {
		return name
	}
	return resource.Name
}

// vpcDescription returns the description of the edge between a resource and
// its VPC.
func vpcDescription(resource *models.Resource, name string) string {
	switch resource.Type {
	case models.ResourceTypeSubnet:
		return "Subnet belongs to VPC"
	case models.ResourceTypeSecurityGroup:
		return "Security group belongs to VPC"
	default:
		return name + " in VPC"
	}
}

// subnetIDs returns the ids of the subnets a resource belongs to.
func subnetIDs(resource *models.Resource) []string {
	var ids []string
	if id, ok := stringProperty(resource.Properties, "subnet_id"); ok {
		ids = append(ids, id)
	}
	ids = append(ids, stringList(resource.Properties["subnets"])...)
	for _, config := range mapList(resource.Properties["vpc_config"]) {
		ids = append(ids, stringList(config["subnet_ids"])...)
	}
	return unique(ids)
}

// securityGroupIDs returns the ids of the security groups a resource uses.
func securityGroupIDs(resource *models.Resource) []string {
	var ids []string
	ids = append(ids, stringList(resource.Properties["vpc_security_group_ids"])...)
	ids = append(ids, stringList(resource.Properties["security_groups"])...)
	for _, config := range mapList(resource.Properties["vpc_config"]) {
		ids = append(ids, stringList(config["security_group_ids"])...)
	}
	for _, config := range mapList(resource.Properties["network_configuration"]) {
		ids = append(ids, stringList(config["security_groups"])...)
	}
	return unique(ids)
}

// unique removes duplicate values while keeping the order.
func unique(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

// ingressRules returns the ingress rules of a security group.
func ingressRules(resource *models.Resource) []map[string]interface{} {
	return mapList(resource.Properties["ingress"])
}

// ruleProperties returns the known properties of a security group rule.
func ruleProperties(rule map[string]interface{}) map[string]interface{} {
	properties := make(map[string]interface{})
	for _, key := range []string{"protocol", "from_port", "to_port", "cidr_blocks"} {
		if value, exists := rule[key]; exists {
			properties[key] = value
		}
	}
	return properties
}

// resourceIndex looks resources up by their type and their "id" property.
type resourceIndex map[models.ResourceType]map[string]*models.Resource

func newResourceIndex(resources []models.Resource) resourceIndex {
	index := make(resourceIndex)
	for i := range resources {
		resource := &resources[i]
		id, ok := stringProperty(resource.Properties, "id")
		if !ok {
			continue
		}
		byID, exists := index[resource.Type]
		if !exists {
			byID = make(map[string]*models.Resource)
			index[resource.Type] = byID
		}
		if _, exists := byID[id]; !exists {
			byID[id] = resource
		}
	}
	return index
}

// find returns the resource of the given type whose "id" matches.
func (index resourceIndex) find(resourceType models.ResourceType, id string) *models.Resource {
	return index[resourceType][id]
}

// stringProperty returns a string property of a resource.
func stringProperty(properties map[string]interface{}, key string) (string, bool) {
	value, ok := properties[key].(string)
	if !ok || value == "" {
		return "", false
	}
	return value, true
}

// stringList returns the string values of a list property. Both
// []interface{} and []string are accepted.
func stringList(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && text != "" {
				values = append(values, text)
			}
		}
		return values
	default:
		return nil
	}
}

// mapList returns the map values of a list property.
func mapList(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		return typed
	case []interface{}:
		values := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if entry, ok := item.(map[string]interface{}); ok {
				values = append(values, entry)
			}
		}
		return values
	default:
		return nil
	}
}
