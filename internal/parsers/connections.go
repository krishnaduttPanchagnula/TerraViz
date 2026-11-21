package parsers

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"

	"terraviz/internal/models"
	"terraviz/internal/relations"
)

// resourceMap is a type alias for the resource lookup map used by analyzers.
type resourceMap = map[string]*models.Resource

// typeIndex maps resource types to slices of resources for O(1) type lookups.
type typeIndex = map[models.ResourceType][]*models.Resource

// externalReferenceSource marks the placeholder nodes that stand for a
// resource referenced outside the state.
const externalReferenceSource = "terraform-reference"

type externalReferenceSpec struct {
	sourceType models.ResourceType
	path       []string
	targetType models.ResourceType
	property   string
}

var externalReferenceSpecs = []externalReferenceSpec{
	{models.ResourceTypeEC2Instance, []string{"vpc_id"}, models.ResourceTypeVPC, "id"},
	{models.ResourceTypeEC2Instance, []string{"subnet_id"}, models.ResourceTypeSubnet, "id"},
	{models.ResourceTypeEC2Instance, []string{"vpc_security_group_ids"}, models.ResourceTypeSecurityGroup, "id"},
	{models.ResourceTypeSubnet, []string{"vpc_id"}, models.ResourceTypeVPC, "id"},
	{models.ResourceTypeSecurityGroup, []string{"vpc_id"}, models.ResourceTypeVPC, "id"},
	{models.ResourceTypeLoadBalancer, []string{"subnets"}, models.ResourceTypeSubnet, "id"},
	{models.ResourceTypeLoadBalancer, []string{"security_groups"}, models.ResourceTypeSecurityGroup, "id"},
	{models.ResourceTypeLBListener, []string{"load_balancer_arn"}, models.ResourceTypeLoadBalancer, "arn"},
	{models.ResourceTypeLBListener, []string{"certificate_arn"}, models.ResourceTypeACMCertificate, "arn"},
	{models.ResourceTypeCloudFront, []string{"viewer_certificate", "acm_certificate_arn"}, models.ResourceTypeACMCertificate, "arn"},
	{models.ResourceTypeRDSInstance, []string{"vpc_security_group_ids"}, models.ResourceTypeSecurityGroup, "id"},
	{models.ResourceTypeLambdaFunction, []string{"vpc_config", "subnet_ids"}, models.ResourceTypeSubnet, "id"},
	{models.ResourceTypeLambdaFunction, []string{"vpc_config", "security_group_ids"}, models.ResourceTypeSecurityGroup, "id"},
	{models.ResourceTypeLambdaFunction, []string{"role"}, models.ResourceTypeIAMRole, "arn"},
	{models.ResourceTypeECSService, []string{"load_balancer", "target_group_arn"}, models.ResourceTypeLBTargetGroup, "arn"},
	{models.ResourceTypeECSService, []string{"cluster"}, models.ResourceTypeECSCluster, "arn"},
	{models.ResourceTypeECSService, []string{"task_definition"}, models.ResourceTypeECSTaskDefinition, "arn"},
	{models.ResourceTypeECSService, []string{"network_configuration", "security_groups"}, models.ResourceTypeSecurityGroup, "id"},
	{models.ResourceTypeECSService, []string{"service_registries", "registry_arn"}, models.ResourceTypeServiceDiscoveryService, "arn"},
	{models.ResourceTypeAPIGatewayIntegration, []string{"connection_id"}, models.ResourceTypeAPIGatewayVPCLink, "id"},
	{models.ResourceTypeAPIGatewayVPCLink, []string{"target_arns"}, models.ResourceTypeLoadBalancer, "arn"},
	{models.ResourceTypeAPIGatewayBasePathMapping, []string{"domain_name"}, models.ResourceTypeAPIGatewayDomainName, "domain_name"},
	{models.ResourceTypeAPIGatewayBasePathMapping, []string{"api_id"}, models.ResourceTypeAPIGateway, "id"},
	{models.ResourceTypeAPIGatewayStage, []string{"rest_api_id"}, models.ResourceTypeAPIGateway, "id"},
	{models.ResourceTypeAPIGatewayDeployment, []string{"rest_api_id"}, models.ResourceTypeAPIGateway, "id"},
	{models.ResourceTypeSNSSubscription, []string{"endpoint"}, models.ResourceTypeSQSQueue, "arn"},
	{models.ResourceTypeSQSQueue, []string{"redrive_policy", "deadLetterTargetArn"}, models.ResourceTypeSQSQueue, "arn"},
	{models.ResourceTypeSecurityGroup, []string{"ingress", "security_groups"}, models.ResourceTypeSecurityGroup, "id"},
	{models.ResourceTypeRoute53Record, []string{"alias", "name"}, models.ResourceTypeLoadBalancer, "dns_name"},
	{models.ResourceTypeServiceDiscoveryService, []string{"namespace_id"}, models.ResourceTypeServiceDiscoveryNamespace, "id"},
	{models.ResourceTypeIAMRolePolicyAttachment, []string{"policy_arn"}, models.ResourceTypeIAMPolicy, "arn"},
}

// buildTypeIndex creates a type-indexed lookup. It walks the resource slice in
// order: ranging over a map would randomize which resource an analyzer finds
// first when several share a type (e.g. a real target vs. a placeholder).
func buildTypeIndex(resources []models.Resource) typeIndex {
	idx := make(typeIndex)
	for i := range resources {
		r := &resources[i]
		idx[r.Type] = append(idx[r.Type], r)
	}
	return idx
}

// findByProperty searches for the first resource matching
// the given type and property key/value pair.
func findByProperty(ti typeIndex, rType models.ResourceType, propKey, propValue string) *models.Resource {
	if propValue == "" {
		return nil
	}
	for _, r := range ti[rType] {
		if v, ok := r.Properties[propKey].(string); ok && v == propValue {
			return r
		}
	}
	return nil
}

// findAllByProperty returns all resources matching the given type and property key/value pair.
func findAllByProperty(ti typeIndex, rType models.ResourceType, propKey, propValue string) []*models.Resource {
	if propValue == "" {
		return nil
	}
	var results []*models.Resource
	for _, r := range ti[rType] {
		if v, ok := r.Properties[propKey].(string); ok && v == propValue {
			results = append(results, r)
		}
	}
	return results
}

// findAllByType returns all resources of the given type.
func findAllByType(ti typeIndex, rType models.ResourceType) []*models.Resource {
	return ti[rType]
}

// createConnections analyzes resources and creates connections between them.
func (p *TerraformParser) createConnections(diagram *models.Diagram, builder *models.DiagramBuilder) {
	p.addExternalReferences(diagram, builder)

	ti := buildTypeIndex(diagram.Resources)
	edges := relations.NewCollector()

	// The generic relations hold for every source, so the Terraform state and
	// the live AWS scan produce the same edges.
	for _, edge := range relations.AnalyzeGeneric(diagram.Resources) {
		edges.Add(edge.Source, edge.Target, edge.Type, edge.Description, edge.Properties)
	}

	for i := range diagram.Resources {
		r := &diagram.Resources[i]
		p.analyzeResourceConnections(r, ti, edges)
	}

	p.addDependencyConnections(diagram, edges)

	for _, e := range edges.Edges() {
		builder.AddConnectionWithProperties(e.Source, e.Target, e.Type, e.Description, e.Properties, e.Bidirectional)
	}
}

// addDependencyConnections turns the dependency graph recorded by Terraform into
// connections. Resource IDs are Terraform addresses, so a dependency address
// matches a resource ID directly; count/for_each instances are matched on
// their base address (the ID without any "[...]" suffix).
func (p *TerraformParser) addDependencyConnections(diagram *models.Diagram, edges *relations.Collector) {
	byBaseAddress := make(map[string][]*models.Resource, len(diagram.Resources))
	for i := range diagram.Resources {
		r := &diagram.Resources[i]
		base := baseAddress(r.ID)
		byBaseAddress[base] = append(byBaseAddress[base], r)
	}

	for i := range diagram.Resources {
		resource := &diagram.Resources[i]
		for _, dep := range resource.Dependencies {
			for _, target := range byBaseAddress[baseAddress(dep)] {
				edges.AddDependency(resource.ID, target.ID)
			}
		}
	}
}

// baseAddress strips a count/for_each index suffix from a Terraform address.
func baseAddress(address string) string {
	if idx := strings.Index(address, "["); idx >= 0 {
		return address[:idx]
	}
	return address
}

// addExternalReferences creates typed placeholder nodes for recognized resource
// arguments whose targets are not present in the Terraform state.
func (p *TerraformParser) addExternalReferences(diagram *models.Diagram, builder *models.DiagramBuilder) {
	ti := buildTypeIndex(diagram.Resources)
	seen := make(map[string]bool)

	for i := range diagram.Resources {
		resource := &diagram.Resources[i]
		for _, spec := range externalReferenceSpecs {
			if resource.Type != spec.sourceType {
				continue
			}
			if spec.sourceType == models.ResourceTypeAPIGatewayIntegration && resource.Properties["connection_type"] != "VPC_LINK" {
				continue
			}
			if spec.sourceType == models.ResourceTypeSNSSubscription && resource.Properties["protocol"] != "sqs" {
				continue
			}
			for _, value := range referenceValues(resource.Properties, spec.path) {
				if value == "" || findByProperty(ti, spec.targetType, spec.property, value) != nil {
					continue
				}
				// An ECS service may reference a newer revision of a task
				// definition whose family is already in state. That is an
				// in-state dependency, not an external resource.
				if spec.sourceType == models.ResourceTypeECSService && spec.targetType == models.ResourceTypeECSTaskDefinition {
					family := taskDefinitionFamily(value)
					if family != "" {
						inState := false
						for _, td := range findAllByType(ti, models.ResourceTypeECSTaskDefinition) {
							if f, _ := td.Properties["family"].(string); f == family {
								inState = true
								break
							}
						}
						if inState {
							continue
						}
					}
				}
				key := string(spec.targetType) + "\x00" + spec.property + "\x00" + value
				if seen[key] {
					continue
				}
				seen[key] = true

				external := externalResource(spec.targetType, spec.property, value)
				builder.AddResource(external)
				ti[external.Type] = append(ti[external.Type], &diagram.Resources[len(diagram.Resources)-1])
			}
		}
	}
}

func referenceValues(properties map[string]interface{}, path []string) []string {
	var walk func(interface{}, int) []string
	walk = func(value interface{}, depth int) []string {
		if depth == len(path) {
			switch typed := value.(type) {
			case string:
				if len(path) > 0 && path[len(path)-1] == "redrive_policy" {
					var decoded map[string]interface{}
					if json.Unmarshal([]byte(typed), &decoded) == nil {
						return walk(decoded, depth)
					}
				}
				return []string{typed}
			case []interface{}:
				var values []string
				for _, item := range typed {
					values = append(values, walk(item, depth)...)
				}
				return values
			}
			return nil
		}

		switch typed := value.(type) {
		case map[string]interface{}:
			if next, ok := typed[path[depth]]; ok {
				return walk(next, depth+1)
			}
		case []interface{}:
			var values []string
			for _, item := range typed {
				values = append(values, walk(item, depth)...)
			}
			return values
		case string:
			var decoded interface{}
			if json.Unmarshal([]byte(typed), &decoded) == nil {
				return walk(decoded, depth)
			}
		}
		return nil
	}

	return walk(properties, 0)
}

func externalResource(resourceType models.ResourceType, property, value string) models.Resource {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(string(resourceType) + "\x00" + property + "\x00" + value))
	name := value
	if slash := strings.LastIndex(name, "/"); slash >= 0 && slash < len(name)-1 {
		name = name[slash+1:]
	}

	resource := models.Resource{
		ID:          fmt.Sprintf("external-%x", hash.Sum64()),
		Name:        name,
		Type:        resourceType,
		Provider:    "aws",
		State:       models.ResourceStateUnknown,
		Properties:  map[string]interface{}{property: value, "referenced_value": value},
		Tags:        make(map[string]string),
		Source:      externalReferenceSource,
		InStateFile: false,
		Group:       "not-in-state-file",
	}
	if strings.HasPrefix(value, "arn:") {
		parts := strings.Split(value, ":")
		if len(parts) > 4 {
			resource.Region = parts[3]
			resource.Account = parts[4]
		}
	}
	return resource
}

// analyzeResourceConnections dispatches to the appropriate analyzer for a resource.
func (p *TerraformParser) analyzeResourceConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	switch resource.Type {
	case models.ResourceTypeCloudFront:
		p.analyzeCloudFrontConnections(resource, ti, edges)
	case models.ResourceTypeLBListener:
		p.analyzeLBListenerConnections(resource, ti, edges)
	case models.ResourceTypeECSService, models.ResourceTypeECSCluster, models.ResourceTypeECSTaskDefinition:
		p.analyzeECSConnections(resource, ti, edges)
	case models.ResourceTypeAPIGateway,
		models.ResourceTypeAPIGatewayResource,
		models.ResourceTypeAPIGatewayMethod,
		models.ResourceTypeAPIGatewayIntegration,
		models.ResourceTypeAPIGatewayStage,
		models.ResourceTypeAPIGatewayDeployment,
		models.ResourceTypeAPIGatewayBasePathMapping,
		models.ResourceTypeAPIGatewayVPCLink:
		p.analyzeAPIGatewayConnections(resource, ti, edges)
	case models.ResourceTypeSNSTopic, models.ResourceTypeSNSSubscription:
		p.analyzeSNSConnections(resource, ti, edges)
	case models.ResourceTypeSQSQueue:
		p.analyzeSQSConnections(resource, ti, edges)
	case models.ResourceTypeLBTargetGroup:
		p.analyzeTargetGroupConnections(resource, ti, edges)
	case models.ResourceTypeLBTargetGroupAttachment:
		p.analyzeTargetGroupAttachmentConnections(resource, ti, edges)
	case models.ResourceTypeLBListenerRule:
		p.analyzeLBListenerRuleConnections(resource, ti, edges)
	case models.ResourceTypeRoute53Record:
		p.analyzeRoute53Connections(resource, ti, edges)
	case models.ResourceTypeACMCertificate:
		p.analyzeACMConnections(resource, ti, edges)
	case models.ResourceTypeCloudWatch:
		p.analyzeCloudWatchConnections(resource, ti, edges)
	case models.ResourceTypeIAMRole, models.ResourceTypeIAMPolicy:
		p.analyzeIAMConnections(resource, ti, edges)
	case models.ResourceTypeServiceDiscoveryService, models.ResourceTypeServiceDiscoveryNamespace:
		p.analyzeServiceDiscoveryConnections(resource, ti, edges)
	case models.ResourceTypeECRRepository:
		p.analyzeECRConnections(resource, ti, edges)
	case models.ResourceTypeWAFWebACL:
		p.analyzeWAFConnections(resource, ti, edges)
	case models.ResourceTypeWAFWebACLAssociation:
		p.analyzeWAFAssociationConnections(resource, ti, edges)
	case models.ResourceTypeSecretsManager:
		p.analyzeSecretsManagerConnections(resource, ti, edges)
	case models.ResourceTypeKMSKey:
		p.analyzeKMSConnections(resource, ti, edges)
	case models.ResourceTypeSQSQueuePolicy, models.ResourceTypeSNSTopicPolicy:
		p.analyzeQueuePolicyConnections(resource, ti, edges)
	case models.ResourceTypeLambdaEventSourceMapping:
		p.analyzeLambdaEventSourceMappingConnections(resource, ti, edges)
	}
}

// connectToSecurityGroups creates connections from a resource to its security groups.
func connectToSecurityGroups(resource *models.Resource, sgIDs []interface{}, ti typeIndex, edges *relations.Collector, description string) {
	seen := make(map[string]bool)
	for _, sgID := range sgIDs {
		sgIDStr, ok := sgID.(string)
		if !ok || seen[sgIDStr] {
			continue
		}
		seen[sgIDStr] = true
		if sg := findByProperty(ti, models.ResourceTypeSecurityGroup, "id", sgIDStr); sg != nil {
			edges.Add(resource.ID, sg.ID, models.ConnectionTypeAccess, description, nil)
		}
	}
}

func (p *TerraformParser) analyzeCloudFrontConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	for _, certificateARN := range referenceValues(resource.Properties, []string{"viewer_certificate", "acm_certificate_arn"}) {
		if certificate := findByProperty(ti, models.ResourceTypeACMCertificate, "arn", certificateARN); certificate != nil {
			edges.Add(resource.ID, certificate.ID, models.ConnectionTypeAccess, "CloudFront distribution uses ACM certificate", nil)
		}
	}

	// S3 origins are referenced by the bucket domain name.
	origins, _ := resource.Properties["origin"].([]interface{})
	for _, originRaw := range origins {
		origin, ok := originRaw.(map[string]interface{})
		if !ok {
			continue
		}
		domain, _ := origin["domain_name"].(string)
		if domain == "" {
			continue
		}
		properties := make(map[string]interface{})
		if originID, ok := origin["origin_id"].(string); ok && originID != "" {
			properties["origin_id"] = originID
		} else if originID, ok := origin["id"].(string); ok && originID != "" {
			properties["origin_id"] = originID
		}
		for _, bucket := range findAllByType(ti, models.ResourceTypeS3Bucket) {
			name := s3BucketName(bucket)
			if name == "" {
				continue
			}
			bucketDomain, _ := bucket.Properties["bucket_domain_name"].(string)
			if domain == name+".s3.amazonaws.com" ||
				(bucketDomain != "" && domain == bucketDomain) ||
				s3BucketNameFromDomain(domain) == name {
				edges.Add(resource.ID, bucket.ID, models.ConnectionTypeReference, "CloudFront distribution serves S3 bucket origin", properties)
			}
		}
	}
}

// analyzeLBListenerConnections analyzes LB Listener connections.
func (p *TerraformParser) analyzeLBListenerConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	if lbArn, ok := resource.Properties["load_balancer_arn"].(string); ok && lbArn != "" {
		if lb := findByProperty(ti, models.ResourceTypeLoadBalancer, "arn", lbArn); lb != nil {
			edges.Add(resource.ID, lb.ID, models.ConnectionTypeDependency, "Listener attached to load balancer", protocolPortProperties(resource))
		}
	}
}

// analyzeECSConnections analyzes ECS resource connections.
func (p *TerraformParser) analyzeECSConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	if resource.Type != models.ResourceTypeECSService {
		return
	}

	p.analyzeECSServiceTargetGroups(resource, ti, edges)
	p.analyzeECSServiceCluster(resource, ti, edges)
	p.analyzeECSServiceTaskDef(resource, ti, edges)
	p.analyzeECSServiceNetworkConfig(resource, ti, edges)
	p.analyzeECSServiceRegistries(resource, ti, edges)
}

func (p *TerraformParser) analyzeECSServiceTargetGroups(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	loadBalancers, ok := resource.Properties["load_balancer"].([]interface{})
	if !ok {
		return
	}
	for _, lb := range loadBalancers {
		lbConfig, ok := lb.(map[string]interface{})
		if !ok {
			continue
		}
		if tgArn, ok := lbConfig["target_group_arn"].(string); ok {
			if tg := findByProperty(ti, models.ResourceTypeLBTargetGroup, "arn", tgArn); tg != nil {
				edges.Add(resource.ID, tg.ID, models.ConnectionTypeData, "ECS service uses target group", nil)
			}
		}
	}
}

func (p *TerraformParser) analyzeECSServiceCluster(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	clusterArn, ok := resource.Properties["cluster"].(string)
	if !ok || clusterArn == "" {
		return
	}
	// Match by ARN or by ID.
	if cluster := findByProperty(ti, models.ResourceTypeECSCluster, "arn", clusterArn); cluster != nil {
		edges.Add(resource.ID, cluster.ID, models.ConnectionTypeDependency, "ECS service runs in cluster", nil)
	} else if cluster := findByProperty(ti, models.ResourceTypeECSCluster, "id", clusterArn); cluster != nil {
		edges.Add(resource.ID, cluster.ID, models.ConnectionTypeDependency, "ECS service runs in cluster", nil)
	}
}

func (p *TerraformParser) analyzeECSServiceTaskDef(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	taskDefArn, ok := resource.Properties["task_definition"].(string)
	if !ok || taskDefArn == "" {
		return
	}
	family := taskDefinitionFamily(taskDefArn)
	for _, other := range findAllByType(ti, models.ResourceTypeECSTaskDefinition) {
		if arn, _ := other.Properties["arn"].(string); arn != "" && arn == taskDefArn {
			edges.Add(resource.ID, other.ID, models.ConnectionTypeDependency, "ECS service uses task definition", nil)
			return
		}
		if otherFamily, _ := other.Properties["family"].(string); otherFamily != "" && otherFamily == family {
			edges.Add(resource.ID, other.ID, models.ConnectionTypeDependency, "ECS service uses task definition", nil)
			return
		}
	}
}

func (p *TerraformParser) analyzeECSServiceNetworkConfig(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	netConf, ok := resource.Properties["network_configuration"].([]interface{})
	if !ok {
		return
	}
	for _, nc := range netConf {
		ncMap, ok := nc.(map[string]interface{})
		if !ok {
			continue
		}
		if sgIDs, ok := ncMap["security_groups"].([]interface{}); ok {
			connectToSecurityGroups(resource, sgIDs, ti, edges, "ECS service uses security group")
		}
	}
}

func (p *TerraformParser) analyzeECSServiceRegistries(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	registries, ok := resource.Properties["service_registries"].([]interface{})
	if !ok {
		return
	}
	for _, reg := range registries {
		regMap, ok := reg.(map[string]interface{})
		if !ok {
			continue
		}
		if sdArn, ok := regMap["registry_arn"].(string); ok && sdArn != "" {
			if sd := findByProperty(ti, models.ResourceTypeServiceDiscoveryService, "arn", sdArn); sd != nil {
				edges.Add(resource.ID, sd.ID, models.ConnectionTypeReference, "ECS service registered in service discovery", nil)
			}
		}
	}
}

// analyzeAPIGatewayConnections analyzes API Gateway connections.
func (p *TerraformParser) analyzeAPIGatewayConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	switch resource.Type {
	case models.ResourceTypeAPIGatewayMethod:
		p.analyzeAPIGatewayMethodConnections(resource, ti, edges)
	case models.ResourceTypeAPIGatewayResource:
		p.connectToRestAPI(resource, ti, edges, "rest_api_id", "API resource belongs to REST API")
	case models.ResourceTypeAPIGatewayIntegration:
		p.analyzeAPIGatewayIntegrationConnections(resource, ti, edges)
	case models.ResourceTypeAPIGatewayVPCLink:
		p.analyzeAPIGatewayVPCLinkConnections(resource, ti, edges)
	case models.ResourceTypeAPIGatewayStage:
		p.connectToRestAPI(resource, ti, edges, "rest_api_id", "Stage belongs to REST API")
	case models.ResourceTypeAPIGatewayDeployment:
		p.connectToRestAPI(resource, ti, edges, "rest_api_id", "Deployment belongs to REST API")
	case models.ResourceTypeAPIGatewayBasePathMapping:
		p.analyzeAPIGatewayBasePathMappingConnections(resource, ti, edges)
	}
}

func (p *TerraformParser) analyzeAPIGatewayMethodConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	restAPIID, _ := resource.Properties["rest_api_id"].(string)
	resourceIDProp, _ := resource.Properties["resource_id"].(string)
	if restAPIID == "" || resourceIDProp == "" {
		return
	}

	for _, other := range findAllByType(ti, models.ResourceTypeAPIGatewayIntegration) {
		otherRestAPI, _ := other.Properties["rest_api_id"].(string)
		otherResourceID, _ := other.Properties["resource_id"].(string)
		if otherRestAPI == restAPIID && otherResourceID == resourceIDProp {
			edges.Add(resource.ID, other.ID, models.ConnectionTypeData, "API method uses integration", nil)
		}
	}
}

func (p *TerraformParser) analyzeAPIGatewayIntegrationConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	connType, _ := resource.Properties["connection_type"].(string)
	if connType != "VPC_LINK" {
		return
	}
	connID, ok := resource.Properties["connection_id"].(string)
	if !ok || connID == "" {
		return
	}
	if vpcLink := findByProperty(ti, models.ResourceTypeAPIGatewayVPCLink, "id", connID); vpcLink != nil {
		edges.Add(resource.ID, vpcLink.ID, models.ConnectionTypeNetworking, "API integration uses VPC Link", nil)
	}
}

func (p *TerraformParser) analyzeAPIGatewayVPCLinkConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	targetArns, ok := resource.Properties["target_arns"].([]interface{})
	if !ok {
		return
	}
	for _, arnRaw := range targetArns {
		if arn, ok := arnRaw.(string); ok {
			if lb := findByProperty(ti, models.ResourceTypeLoadBalancer, "arn", arn); lb != nil {
				edges.Add(resource.ID, lb.ID, models.ConnectionTypeNetworking, "VPC Link targets NLB", nil)
			}
		}
	}
}

func (p *TerraformParser) analyzeAPIGatewayBasePathMappingConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	if domainName, ok := resource.Properties["domain_name"].(string); ok && domainName != "" {
		if domain := findByProperty(ti, models.ResourceTypeAPIGatewayDomainName, "domain_name", domainName); domain != nil {
			edges.Add(resource.ID, domain.ID, models.ConnectionTypeDependency, "Base path mapping on domain", nil)
		}
	}
	p.connectToRestAPI(resource, ti, edges, "api_id", "Base path mapping targets REST API")
}

// connectToRestAPI creates a connection from a resource to a REST API by matching a property key.
func (p *TerraformParser) connectToRestAPI(resource *models.Resource, ti typeIndex, edges *relations.Collector, propKey, description string) {
	restAPIID, ok := resource.Properties[propKey].(string)
	if !ok || restAPIID == "" {
		return
	}
	if api := findByProperty(ti, models.ResourceTypeAPIGateway, "id", restAPIID); api != nil {
		edges.Add(resource.ID, api.ID, models.ConnectionTypeDependency, description, nil)
	}
}

// analyzeSNSConnections analyzes SNS topic connections.
func (p *TerraformParser) analyzeSNSConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	if resource.Type == models.ResourceTypeSNSSubscription {
		protocol, _ := resource.Properties["protocol"].(string)
		endpoint, _ := resource.Properties["endpoint"].(string)
		if protocol == "sqs" {
			if queue := findByProperty(ti, models.ResourceTypeSQSQueue, "arn", endpoint); queue != nil {
				edges.Add(resource.ID, queue.ID, models.ConnectionTypeTrigger, "SNS subscription delivers to SQS queue", nil)
			}
		}
		return
	}

	topicArn, _ := resource.Properties["arn"].(string)
	if topicArn == "" {
		return
	}

	for _, sub := range findAllByProperty(ti, models.ResourceTypeSNSSubscription, "topic_arn", topicArn) {
		edges.Add(resource.ID, sub.ID, models.ConnectionTypeTrigger, "SNS topic sends to subscription", nil)

		// If the subscription targets an SQS queue, connect Subscription -> Queue.
		protocol, _ := sub.Properties["protocol"].(string)
		endpoint, _ := sub.Properties["endpoint"].(string)
		if protocol == "sqs" && endpoint != "" {
			if queue := findByProperty(ti, models.ResourceTypeSQSQueue, "arn", endpoint); queue != nil {
				edges.Add(sub.ID, queue.ID, models.ConnectionTypeTrigger, "SNS subscription delivers to SQS queue", nil)
			}
		}
	}
}

// analyzeSQSConnections analyzes SQS queue connections.
func (p *TerraformParser) analyzeSQSConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	redriveRaw, ok := resource.Properties["redrive_policy"]
	if !ok {
		return
	}

	var redriveMap map[string]interface{}
	switch v := redriveRaw.(type) {
	case map[string]interface{}:
		redriveMap = v
	case string:
		if v != "" {
			_ = json.Unmarshal([]byte(v), &redriveMap)
		}
	}

	if redriveMap == nil {
		return
	}

	dlqArn, _ := redriveMap["deadLetterTargetArn"].(string)
	if dlqArn == "" {
		return
	}

	for _, other := range findAllByProperty(ti, models.ResourceTypeSQSQueue, "arn", dlqArn) {
		if other.ID != resource.ID {
			edges.Add(resource.ID, other.ID, models.ConnectionTypeData, "SQS queue sends failures to DLQ", nil)
		}
	}
}

// analyzeTargetGroupConnections analyzes Target Group connections.
func (p *TerraformParser) analyzeTargetGroupConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	targetGroupArn, _ := resource.Properties["arn"].(string)
	if targetGroupArn == "" {
		return
	}

	for _, listener := range findAllByType(ti, models.ResourceTypeLBListener) {
		defaultActions, ok := listener.Properties["default_action"].([]interface{})
		if !ok {
			continue
		}
		for _, action := range defaultActions {
			actionMap, ok := action.(map[string]interface{})
			if !ok {
				continue
			}
			if tgArn, _ := actionMap["target_group_arn"].(string); tgArn == targetGroupArn {
				edges.Add(listener.ID, resource.ID, models.ConnectionTypeData, "Load balancer listener forwards to target group", protocolPortProperties(listener))
			}
		}
	}
}

// analyzeRoute53Connections analyzes Route53 connections.
func (p *TerraformParser) analyzeRoute53Connections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	zoneID, _ := resource.Properties["zone_id"].(string)
	if zone := findByProperty(ti, models.ResourceTypeRoute53Zone, "id", zoneID); zone != nil {
		edges.Add(resource.ID, zone.ID, models.ConnectionTypeDependency, "Route53 record belongs to hosted zone", nil)
	}

	alias, ok := resource.Properties["alias"].(map[string]interface{})
	if !ok {
		return
	}
	dnsName, _ := alias["name"].(string)
	if dnsName == "" {
		return
	}
	if lb := findByProperty(ti, models.ResourceTypeLoadBalancer, "dns_name", dnsName); lb != nil {
		edges.Add(resource.ID, lb.ID, models.ConnectionTypeReference, "Route53 record points to load balancer", nil)
	}
}

// analyzeACMConnections analyzes ACM Certificate connections.
func (p *TerraformParser) analyzeACMConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	certArn, _ := resource.Properties["arn"].(string)
	if certArn == "" {
		return
	}

	for _, listener := range findAllByProperty(ti, models.ResourceTypeLBListener, "certificate_arn", certArn) {
		edges.Add(listener.ID, resource.ID, models.ConnectionTypeAccess, "Load balancer listener uses SSL certificate", nil)
	}

	for _, domain := range findAllByProperty(ti, models.ResourceTypeAPIGatewayDomainName, "certificate_arn", certArn) {
		edges.Add(domain.ID, resource.ID, models.ConnectionTypeAccess, "API Gateway domain uses SSL certificate", nil)
	}
}

// analyzeCloudWatchConnections analyzes CloudWatch log group connections.
func (p *TerraformParser) analyzeCloudWatchConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	logGroupName, _ := resource.Properties["name"].(string)
	if logGroupName == "" {
		return
	}

	// Find ECS task definitions that log to this log group.
	for _, taskDef := range findAllByType(ti, models.ResourceTypeECSTaskDefinition) {
		for _, group := range ecsLogGroups(taskDef.Properties["container_definitions"]) {
			if group == logGroupName {
				edges.Add(taskDef.ID, resource.ID, models.ConnectionTypeData, "ECS task logs to CloudWatch", nil)
				break
			}
		}
	}

	// Find Lambda functions that log to this group. Lambda writes to the
	// conventionally named group "/aws/lambda/<function name>".
	for _, fn := range findAllByType(ti, models.ResourceTypeLambdaFunction) {
		functionName, _ := fn.Properties["function_name"].(string)
		if functionName == "" {
			continue
		}
		if logGroupName == "/aws/lambda/"+functionName {
			edges.Add(fn.ID, resource.ID, models.ConnectionTypeData, "Lambda function logs to CloudWatch", nil)
		}
	}
}

// analyzeServiceDiscoveryConnections analyzes Service Discovery connections.
func (p *TerraformParser) analyzeServiceDiscoveryConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	if resource.Type != models.ResourceTypeServiceDiscoveryService {
		return
	}
	nsID, ok := resource.Properties["namespace_id"].(string)
	if !ok || nsID == "" {
		return
	}
	if ns := findByProperty(ti, models.ResourceTypeServiceDiscoveryNamespace, "id", nsID); ns != nil {
		edges.Add(resource.ID, ns.ID, models.ConnectionTypeDependency, "Service registered in namespace", nil)
	}
}

// analyzeECRConnections analyzes ECR repository connections.
func (p *TerraformParser) analyzeECRConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	if resource.Type != models.ResourceTypeECRRepository {
		return
	}

	repoURL, _ := resource.Properties["repository_url"].(string)
	if repoURL == "" {
		return
	}

	for _, taskDef := range findAllByType(ti, models.ResourceTypeECSTaskDefinition) {
		for _, image := range containerImages(taskDef.Properties["container_definitions"]) {
			if stripImageTag(image) == repoURL {
				edges.Add(taskDef.ID, resource.ID, models.ConnectionTypeReference, "ECS task uses ECR image", nil)
				break
			}
		}
	}
}

// analyzeWAFConnections analyzes WAF Web ACL connections.
func (p *TerraformParser) analyzeWAFConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	if resource.Type != models.ResourceTypeWAFWebACL {
		return
	}

	referenced := collectStrings(decodeJSONValue(resource.Properties["rule"]))
	if len(referenced) == 0 {
		return
	}

	for _, ipSet := range findAllByType(ti, models.ResourceTypeWAFIPSet) {
		id, _ := ipSet.Properties["id"].(string)
		arn, _ := ipSet.Properties["arn"].(string)
		if (id != "" && containsString(referenced, id)) || (arn != "" && containsString(referenced, arn)) {
			edges.Add(resource.ID, ipSet.ID, models.ConnectionTypeReference, "WAF ACL references IP set", nil)
		}
	}
}

// analyzeSecretsManagerConnections analyzes Secrets Manager connections.
func (p *TerraformParser) analyzeSecretsManagerConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	if resource.Type != models.ResourceTypeSecretsManager {
		return
	}

	secretArn, _ := resource.Properties["arn"].(string)
	secretName, _ := resource.Properties["name"].(string)
	if secretArn == "" && secretName == "" {
		return
	}

	for _, taskDef := range findAllByType(ti, models.ResourceTypeECSTaskDefinition) {
		for _, ref := range containerSecretReferences(taskDef.Properties["container_definitions"]) {
			if (secretArn != "" && ref == secretArn) ||
				(secretName != "" && ref == secretName) ||
				(secretName != "" && secretNameFromARN(ref) == secretName) {
				edges.Add(taskDef.ID, resource.ID, models.ConnectionTypeAccess, "ECS task accesses secret", nil)
				break
			}
		}
	}
}

// analyzeIAMConnections analyzes IAM role and policy connections.
func (p *TerraformParser) analyzeIAMConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	if resource.Type == models.ResourceTypeIAMRole {
		p.analyzeIAMRoleConnections(resource, ti, edges)
	}
	if resource.Type == models.ResourceTypeIAMPolicy {
		p.analyzeIAMPolicyConnections(resource, ti, edges)
	}
}

func (p *TerraformParser) analyzeIAMRoleConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	roleArn, _ := resource.Properties["arn"].(string)
	if roleArn == "" {
		return
	}

	// ECS task definitions that use this role.
	for _, taskDef := range findAllByType(ti, models.ResourceTypeECSTaskDefinition) {
		if taskRoleArn, _ := taskDef.Properties["task_role_arn"].(string); taskRoleArn == roleArn {
			edges.Add(taskDef.ID, resource.ID, models.ConnectionTypeAccess, "ECS task uses IAM role", nil)
		}
		if execRoleArn, _ := taskDef.Properties["execution_role_arn"].(string); execRoleArn == roleArn {
			edges.Add(taskDef.ID, resource.ID, models.ConnectionTypeAccess, "ECS task execution uses IAM role", nil)
		}
	}

	// Lambda functions that use this role.
	for _, fn := range findAllByProperty(ti, models.ResourceTypeLambdaFunction, "role", roleArn) {
		edges.Add(fn.ID, resource.ID, models.ConnectionTypeAccess, "Lambda function uses IAM role", nil)
	}

	// IAM role policy attachments.
	for _, attachment := range findAllByProperty(ti, models.ResourceTypeIAMRolePolicyAttachment, "role", roleArn) {
		edges.Add(resource.ID, attachment.ID, models.ConnectionTypeAccess, "IAM role has policy attachment", nil)
	}
}

func (p *TerraformParser) analyzeIAMPolicyConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	policyArn, _ := resource.Properties["arn"].(string)
	if policyArn == "" {
		return
	}

	for _, attachment := range findAllByProperty(ti, models.ResourceTypeIAMRolePolicyAttachment, "policy_arn", policyArn) {
		edges.Add(resource.ID, attachment.ID, models.ConnectionTypeAccess, "IAM policy attached to role", nil)
	}
}

// analyzeTargetGroupAttachmentConnections analyzes target group attachments.
// Traffic flows target group -> attachment -> target.
func (p *TerraformParser) analyzeTargetGroupAttachmentConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	tgArn, _ := resource.Properties["target_group_arn"].(string)
	if tg := findByProperty(ti, models.ResourceTypeLBTargetGroup, "arn", tgArn); tg != nil {
		edges.Add(tg.ID, resource.ID, models.ConnectionTypeData, "Target group attaches target", protocolPortProperties(tg))
	}

	targetID, _ := resource.Properties["target_id"].(string)
	if targetID == "" {
		return
	}
	properties := make(map[string]interface{})
	if port, ok := numberProperty(resource.Properties, "port"); ok {
		properties["port"] = port
	}
	if instance := findByProperty(ti, models.ResourceTypeEC2Instance, "id", targetID); instance != nil {
		edges.Add(resource.ID, instance.ID, models.ConnectionTypeData, "Target group attaches EC2 instance", properties)
		return
	}
	if service := findByProperty(ti, models.ResourceTypeECSService, "arn", targetID); service != nil {
		edges.Add(resource.ID, service.ID, models.ConnectionTypeData, "Target group attaches ECS service", properties)
	}
}

// analyzeLBListenerRuleConnections analyzes listener rules.
// Traffic flows listener -> rule -> target group.
func (p *TerraformParser) analyzeLBListenerRuleConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	listenerArn, _ := resource.Properties["listener_arn"].(string)
	if listener := findByProperty(ti, models.ResourceTypeLBListener, "arn", listenerArn); listener != nil {
		edges.Add(listener.ID, resource.ID, models.ConnectionTypeData, "Listener has rule", nil)
	}

	tgArn, _ := resource.Properties["target_group_arn"].(string)
	if tg := findByProperty(ti, models.ResourceTypeLBTargetGroup, "arn", tgArn); tg != nil {
		edges.Add(resource.ID, tg.ID, models.ConnectionTypeData, "Listener rule forwards to target group", nil)
	}
}

// analyzeWAFAssociationConnections analyzes WAF web ACL associations.
// Traffic flows load balancer -> association -> web ACL.
func (p *TerraformParser) analyzeWAFAssociationConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	lbArn, _ := resource.Properties["resource_arn"].(string)
	if lb := findByProperty(ti, models.ResourceTypeLoadBalancer, "arn", lbArn); lb != nil {
		edges.Add(lb.ID, resource.ID, models.ConnectionTypeData, "Load balancer protected by WAF association", nil)
	}

	aclArn, _ := resource.Properties["web_acl_arn"].(string)
	if acl := findByProperty(ti, models.ResourceTypeWAFWebACL, "arn", aclArn); acl != nil {
		edges.Add(resource.ID, acl.ID, models.ConnectionTypeData, "WAF association applies web ACL", nil)
	}
}

// analyzeLambdaEventSourceMappingConnections analyzes Lambda event source
// mappings. Traffic flows event source -> mapping -> function.
func (p *TerraformParser) analyzeLambdaEventSourceMappingConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	functionName, _ := resource.Properties["function_name"].(string)
	if functionName != "" {
		fn := findByProperty(ti, models.ResourceTypeLambdaFunction, "function_name", functionName)
		if fn == nil {
			fn = findByProperty(ti, models.ResourceTypeLambdaFunction, "arn", functionName)
		}
		if fn != nil {
			properties := make(map[string]interface{})
			if batchSize, ok := numberProperty(resource.Properties, "batch_size"); ok {
				properties["batch_size"] = batchSize
			}
			edges.Add(resource.ID, fn.ID, models.ConnectionTypeTrigger, "Lambda event source mapping triggers function", properties)
		}
	}

	eventSourceArn, _ := resource.Properties["event_source_arn"].(string)
	if eventSourceArn == "" {
		return
	}
	if queue := findByProperty(ti, models.ResourceTypeSQSQueue, "arn", eventSourceArn); queue != nil {
		edges.Add(queue.ID, resource.ID, models.ConnectionTypeTrigger, "SQS queue triggers Lambda event source mapping", nil)
		return
	}
	if topic := findByProperty(ti, models.ResourceTypeSNSTopic, "arn", eventSourceArn); topic != nil {
		edges.Add(topic.ID, resource.ID, models.ConnectionTypeTrigger, "SNS topic triggers Lambda event source mapping", nil)
	}
}

// analyzeQueuePolicyConnections analyzes SQS and SNS policy resources.
func (p *TerraformParser) analyzeQueuePolicyConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	if resource.Type == models.ResourceTypeSNSTopicPolicy {
		topicArn, _ := resource.Properties["arn"].(string)
		if topic := findByProperty(ti, models.ResourceTypeSNSTopic, "arn", topicArn); topic != nil {
			edges.Add(resource.ID, topic.ID, models.ConnectionTypeAccess, "SNS topic policy applies to topic", nil)
		}
		return
	}

	queueURL, _ := resource.Properties["queue_url"].(string)
	if queueURL == "" {
		return
	}
	queueName := lastPathSegment(queueURL)
	if queueName == "" {
		return
	}
	for _, queue := range findAllByType(ti, models.ResourceTypeSQSQueue) {
		name, _ := queue.Properties["name"].(string)
		if name == "" {
			if id, _ := queue.Properties["id"].(string); id != "" {
				name = lastPathSegment(id)
			}
		}
		if name == queueName {
			edges.Add(resource.ID, queue.ID, models.ConnectionTypeAccess, "SQS queue policy applies to queue", nil)
		}
	}
}

// analyzeKMSConnections analyzes KMS key usage. The key points at the
// resources it encrypts.
func (p *TerraformParser) analyzeKMSConnections(resource *models.Resource, ti typeIndex, edges *relations.Collector) {
	keyArn, _ := resource.Properties["arn"].(string)
	if keyArn == "" {
		return
	}

	for _, bucket := range findAllByType(ti, models.ResourceTypeS3Bucket) {
		if kmsKeyID, _ := bucket.Properties["kms_key_id"].(string); kmsKeyID == keyArn {
			edges.Add(resource.ID, bucket.ID, models.ConnectionTypeData, "KMS key encrypts S3 bucket", nil)
		}
	}
	for _, rds := range findAllByType(ti, models.ResourceTypeRDSInstance) {
		if kmsKeyID, _ := rds.Properties["kms_key_id"].(string); kmsKeyID == keyArn {
			edges.Add(resource.ID, rds.ID, models.ConnectionTypeData, "KMS key encrypts RDS instance", nil)
		}
	}
	for _, secret := range findAllByType(ti, models.ResourceTypeSecretsManager) {
		if kmsKeyID, _ := secret.Properties["kms_key_id"].(string); kmsKeyID == keyArn {
			edges.Add(resource.ID, secret.ID, models.ConnectionTypeData, "KMS key encrypts secret", nil)
		}
	}
}

// protocolPortProperties returns the protocol and port of a load balancer
// listener or target group, when they are known.
func protocolPortProperties(resource *models.Resource) map[string]interface{} {
	properties := make(map[string]interface{})
	if protocol, ok := resource.Properties["protocol"].(string); ok && protocol != "" {
		properties["protocol"] = protocol
	}
	if port, ok := numberProperty(resource.Properties, "port"); ok {
		properties["port"] = port
	}
	return properties
}

// numberProperty returns a numeric property of a resource.
func numberProperty(properties map[string]interface{}, key string) (int64, bool) {
	switch value := properties[key].(type) {
	case float64:
		return int64(value), true
	case int:
		return int64(value), true
	case int64:
		return value, true
	case json.Number:
		if number, err := value.Int64(); err == nil {
			return number, true
		}
	}
	return 0, false
}

// decodeJSONValue decodes a JSON encoded attribute into a generic value.
// Values that are already structured are returned unchanged.
func decodeJSONValue(raw interface{}) interface{} {
	if encoded, ok := raw.(string); ok {
		var decoded interface{}
		if err := json.Unmarshal([]byte(encoded), &decoded); err == nil {
			return decoded
		}
	}
	return raw
}

// collectStrings returns every string leaf value of a decoded JSON value.
func collectStrings(value interface{}) []string {
	var values []string
	var walk func(interface{})
	walk = func(current interface{}) {
		switch typed := current.(type) {
		case string:
			values = append(values, typed)
		case []interface{}:
			for _, item := range typed {
				walk(item)
			}
		case map[string]interface{}:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(value)
	return values
}

// containsString reports whether values contains the given string.
func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// containerDefinitions decodes the container_definitions attribute of an ECS
// task definition into its list of container definitions.
func containerDefinitions(raw interface{}) []map[string]interface{} {
	decoded := decodeJSONValue(raw)
	list, ok := decoded.([]interface{})
	if !ok {
		return nil
	}
	containers := make([]map[string]interface{}, 0, len(list))
	for _, item := range list {
		if container, ok := item.(map[string]interface{}); ok {
			containers = append(containers, container)
		}
	}
	return containers
}

// containerImages returns the container images of an ECS task definition.
func containerImages(raw interface{}) []string {
	var images []string
	for _, container := range containerDefinitions(raw) {
		if image, ok := container["image"].(string); ok && image != "" {
			images = append(images, image)
		}
	}
	return images
}

// ecsLogGroups returns the CloudWatch log groups an ECS task definition writes
// to through the awslogs log driver.
func ecsLogGroups(raw interface{}) []string {
	var groups []string
	for _, container := range containerDefinitions(raw) {
		logConfig, ok := container["logConfiguration"].(map[string]interface{})
		if !ok {
			continue
		}
		options, ok := logConfig["options"].(map[string]interface{})
		if !ok {
			continue
		}
		if group, ok := options["awslogs-group"].(string); ok && group != "" {
			groups = append(groups, group)
		}
	}
	return groups
}

// containerSecretReferences returns the secrets an ECS task definition reads
// through environment variables or "secrets" blocks.
func containerSecretReferences(raw interface{}) []string {
	var refs []string
	for _, container := range containerDefinitions(raw) {
		if env, ok := container["environment"].([]interface{}); ok {
			for _, item := range env {
				variable, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				if value, ok := variable["value"].(string); ok && value != "" {
					refs = append(refs, value)
				}
			}
		}
		if secrets, ok := container["secrets"].([]interface{}); ok {
			for _, item := range secrets {
				secret, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				if value, ok := secret["valueFrom"].(string); ok && value != "" {
					refs = append(refs, value)
				}
			}
		}
	}
	return refs
}

// stripImageTag removes the tag or digest suffix of a container image.
func stripImageTag(image string) string {
	if idx := strings.Index(image, "@"); idx >= 0 {
		image = image[:idx]
	}
	if idx := strings.LastIndex(image, ":"); idx > strings.LastIndex(image, "/") {
		image = image[:idx]
	}
	return image
}

// secretNameFromARN returns the secret name of a Secrets Manager ARN.
func secretNameFromARN(arn string) string {
	idx := strings.LastIndex(arn, ":")
	if idx < 0 {
		return ""
	}
	return arn[idx+1:]
}

// taskDefinitionFamily returns the family of an ECS task definition ARN
// (arn:aws:ecs:<region>:<account>:task-definition/<family>:<revision>).
func taskDefinitionFamily(arn string) string {
	idx := strings.LastIndex(arn, "/")
	if idx < 0 {
		return ""
	}
	name := arn[idx+1:]
	if colon := strings.LastIndex(name, ":"); colon > 0 {
		name = name[:colon]
	}
	return name
}

// lastPathSegment returns the last segment of a slash separated path.
func lastPathSegment(path string) string {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

// s3BucketName returns the name of an S3 bucket resource.
func s3BucketName(bucket *models.Resource) string {
	if name, _ := bucket.Properties["bucket"].(string); name != "" {
		return name
	}
	if name, _ := bucket.Properties["id"].(string); name != "" && !strings.HasPrefix(name, "arn:") {
		return name
	}
	if arn, _ := bucket.Properties["arn"].(string); arn != "" {
		return lastPathSegment(arn)
	}
	return ""
}

// s3BucketNameFromDomain extracts the bucket name of an S3 domain name such
// as "my-bucket.s3.us-east-1.amazonaws.com". It returns "" for other domains.
func s3BucketNameFromDomain(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) < 3 || parts[1] != "s3" {
		return ""
	}
	return parts[0]
}
