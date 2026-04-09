package parsers

import (
	"encoding/json"
	"strings"

	"terraviz/internal/models"
)

// resourceMap is a type alias for the resource lookup map used by analyzers.
type resourceMap = map[string]*models.Resource

// findByProperty searches the resource map for the first resource matching
// the given type and property key/value pair.
func findByProperty(rm resourceMap, rType models.ResourceType, propKey, propValue string) *models.Resource {
	if propValue == "" {
		return nil
	}
	for _, r := range rm {
		if r.Type == rType {
			if v, ok := r.Properties[propKey].(string); ok && v == propValue {
				return r
			}
		}
	}
	return nil
}

// findAllByProperty returns all resources matching the given type and property key/value pair.
func findAllByProperty(rm resourceMap, rType models.ResourceType, propKey, propValue string) []*models.Resource {
	if propValue == "" {
		return nil
	}
	var results []*models.Resource
	for _, r := range rm {
		if r.Type == rType {
			if v, ok := r.Properties[propKey].(string); ok && v == propValue {
				results = append(results, r)
			}
		}
	}
	return results
}

// findAllByType returns all resources of the given type.
func findAllByType(rm resourceMap, rType models.ResourceType) []*models.Resource {
	var results []*models.Resource
	for _, r := range rm {
		if r.Type == rType {
			results = append(results, r)
		}
	}
	return results
}

// findByPropertyContains returns the first resource of the given type where
// the specified string property contains the given substring.
func findByPropertyContains(rm resourceMap, rType models.ResourceType, propKey, substr string) *models.Resource {
	if substr == "" {
		return nil
	}
	for _, r := range rm {
		if r.Type == rType {
			if v, ok := r.Properties[propKey].(string); ok && strings.Contains(v, substr) {
				return r
			}
		}
	}
	return nil
}

// createConnections analyzes resources and creates connections between them.
func (p *TerraformParser) createConnections(diagram *models.Diagram, builder *models.DiagramBuilder) {
	rm := make(resourceMap, len(diagram.Resources))
	for i := range diagram.Resources {
		r := &diagram.Resources[i]
		rm[r.ID] = r
	}

	for i := range diagram.Resources {
		r := &diagram.Resources[i]
		p.analyzeResourceConnections(r, rm, builder)
	}
}

// analyzeResourceConnections dispatches to the appropriate analyzer for a resource.
func (p *TerraformParser) analyzeResourceConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	switch resource.Type {
	case models.ResourceTypeEC2Instance:
		p.analyzeEC2Connections(resource, rm, builder)
	case models.ResourceTypeLoadBalancer:
		p.analyzeLoadBalancerConnections(resource, rm, builder)
	case models.ResourceTypeLBListener:
		p.analyzeLBListenerConnections(resource, rm, builder)
	case models.ResourceTypeRDSInstance:
		p.analyzeRDSConnections(resource, rm, builder)
	case models.ResourceTypeLambdaFunction:
		p.analyzeLambdaConnections(resource, rm, builder)
	case models.ResourceTypeECSService, models.ResourceTypeECSCluster, models.ResourceTypeECSTaskDefinition:
		p.analyzeECSConnections(resource, rm, builder)
	case models.ResourceTypeAPIGateway,
		models.ResourceTypeAPIGatewayResource,
		models.ResourceTypeAPIGatewayMethod,
		models.ResourceTypeAPIGatewayIntegration,
		models.ResourceTypeAPIGatewayStage,
		models.ResourceTypeAPIGatewayDeployment,
		models.ResourceTypeAPIGatewayBasePathMapping,
		models.ResourceTypeAPIGatewayVPCLink:
		p.analyzeAPIGatewayConnections(resource, rm, builder)
	case models.ResourceTypeSNSTopic:
		p.analyzeSNSConnections(resource, rm, builder)
	case models.ResourceTypeSQSQueue:
		p.analyzeSQSConnections(resource, rm, builder)
	case models.ResourceTypeLBTargetGroup:
		p.analyzeTargetGroupConnections(resource, rm, builder)
	case models.ResourceTypeSecurityGroup:
		p.analyzeSecurityGroupConnections(resource, rm, builder)
	case models.ResourceTypeRoute53Record:
		p.analyzeRoute53Connections(resource, rm, builder)
	case models.ResourceTypeACMCertificate:
		p.analyzeACMConnections(resource, rm, builder)
	case models.ResourceTypeCloudWatch:
		p.analyzeCloudWatchConnections(resource, rm, builder)
	case models.ResourceTypeIAMRole, models.ResourceTypeIAMPolicy:
		p.analyzeIAMConnections(resource, rm, builder)
	case models.ResourceTypeServiceDiscoveryService, models.ResourceTypeServiceDiscoveryNamespace:
		p.analyzeServiceDiscoveryConnections(resource, rm, builder)
	case models.ResourceTypeECRRepository:
		p.analyzeECRConnections(resource, rm, builder)
	case models.ResourceTypeWAFWebACL:
		p.analyzeWAFConnections(resource, rm, builder)
	case models.ResourceTypeSecretsManager:
		p.analyzeSecretsManagerConnections(resource, rm, builder)
	}
}

// connectToSecurityGroups creates connections from a resource to its security groups.
func connectToSecurityGroups(resource *models.Resource, sgIDs []interface{}, rm resourceMap, builder *models.DiagramBuilder, description string) {
	for _, sgID := range sgIDs {
		sgIDStr, ok := sgID.(string)
		if !ok {
			continue
		}
		if sg := findByProperty(rm, models.ResourceTypeSecurityGroup, "id", sgIDStr); sg != nil {
			builder.AddConnection(resource.ID, sg.ID, models.ConnectionTypeAccess, description)
		}
	}
}

// analyzeEC2Connections analyzes EC2 instance connections.
func (p *TerraformParser) analyzeEC2Connections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	// Connect to VPC.
	if vpcID, ok := resource.Properties["vpc_id"].(string); ok {
		if vpc := findByProperty(rm, models.ResourceTypeVPC, "id", vpcID); vpc != nil {
			builder.AddConnection(resource.ID, vpc.ID, models.ConnectionTypeNetworking, "EC2 instance in VPC")
		}
	}

	// Connect to subnet.
	if subnetID, ok := resource.Properties["subnet_id"].(string); ok {
		if subnet := findByProperty(rm, models.ResourceTypeSubnet, "id", subnetID); subnet != nil {
			builder.AddConnection(resource.ID, subnet.ID, models.ConnectionTypeNetworking, "EC2 instance in subnet")
		}
	}

	// Connect to security groups.
	if sgIDs, ok := resource.Properties["vpc_security_group_ids"].([]interface{}); ok {
		connectToSecurityGroups(resource, sgIDs, rm, builder, "EC2 instance uses security group")
	}
}

// analyzeLoadBalancerConnections analyzes load balancer connections.
func (p *TerraformParser) analyzeLoadBalancerConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	// Connect to subnets.
	if subnets, ok := resource.Properties["subnets"].([]interface{}); ok {
		for _, subnet := range subnets {
			if subnetID, ok := subnet.(string); ok {
				if s := findByProperty(rm, models.ResourceTypeSubnet, "id", subnetID); s != nil {
					builder.AddConnection(resource.ID, s.ID, models.ConnectionTypeNetworking, "Load balancer in subnet")
				}
			}
		}
	}

	// Connect to security groups.
	if sgIDs, ok := resource.Properties["security_groups"].([]interface{}); ok {
		connectToSecurityGroups(resource, sgIDs, rm, builder, "Load balancer uses security group")
	}
}

// analyzeLBListenerConnections analyzes LB Listener connections.
func (p *TerraformParser) analyzeLBListenerConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	if lbArn, ok := resource.Properties["load_balancer_arn"].(string); ok && lbArn != "" {
		if lb := findByProperty(rm, models.ResourceTypeLoadBalancer, "arn", lbArn); lb != nil {
			builder.AddConnection(resource.ID, lb.ID, models.ConnectionTypeDependency, "Listener attached to load balancer")
		}
	}
}

// analyzeRDSConnections analyzes RDS instance connections.
func (p *TerraformParser) analyzeRDSConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	if sgIDs, ok := resource.Properties["vpc_security_group_ids"].([]interface{}); ok {
		connectToSecurityGroups(resource, sgIDs, rm, builder, "RDS instance uses security group")
	}
}

// analyzeLambdaConnections analyzes Lambda function connections.
func (p *TerraformParser) analyzeLambdaConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	vpcConfig, ok := resource.Properties["vpc_config"].([]interface{})
	if !ok || len(vpcConfig) == 0 {
		return
	}

	config, ok := vpcConfig[0].(map[string]interface{})
	if !ok {
		return
	}

	// Connect to subnets.
	if subnets, ok := config["subnet_ids"].([]interface{}); ok {
		for _, subnet := range subnets {
			if subnetID, ok := subnet.(string); ok {
				if s := findByProperty(rm, models.ResourceTypeSubnet, "id", subnetID); s != nil {
					builder.AddConnection(resource.ID, s.ID, models.ConnectionTypeNetworking, "Lambda function in subnet")
				}
			}
		}
	}

	// Connect to security groups.
	if sgIDs, ok := config["security_group_ids"].([]interface{}); ok {
		connectToSecurityGroups(resource, sgIDs, rm, builder, "Lambda function uses security group")
	}
}

// analyzeECSConnections analyzes ECS resource connections.
func (p *TerraformParser) analyzeECSConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	if resource.Type != models.ResourceTypeECSService {
		return
	}

	p.analyzeECSServiceTargetGroups(resource, rm, builder)
	p.analyzeECSServiceCluster(resource, rm, builder)
	p.analyzeECSServiceTaskDef(resource, rm, builder)
	p.analyzeECSServiceNetworkConfig(resource, rm, builder)
	p.analyzeECSServiceRegistries(resource, rm, builder)
}

func (p *TerraformParser) analyzeECSServiceTargetGroups(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
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
			if tg := findByProperty(rm, models.ResourceTypeLBTargetGroup, "arn", tgArn); tg != nil {
				builder.AddConnection(resource.ID, tg.ID, models.ConnectionTypeData, "ECS service uses target group")
			}
		}
	}
}

func (p *TerraformParser) analyzeECSServiceCluster(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	clusterArn, ok := resource.Properties["cluster"].(string)
	if !ok || clusterArn == "" {
		return
	}
	// Match by ARN or by ID.
	if cluster := findByProperty(rm, models.ResourceTypeECSCluster, "arn", clusterArn); cluster != nil {
		builder.AddConnection(resource.ID, cluster.ID, models.ConnectionTypeDependency, "ECS service runs in cluster")
	} else if cluster := findByProperty(rm, models.ResourceTypeECSCluster, "id", clusterArn); cluster != nil {
		builder.AddConnection(resource.ID, cluster.ID, models.ConnectionTypeDependency, "ECS service runs in cluster")
	}
}

func (p *TerraformParser) analyzeECSServiceTaskDef(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	taskDefArn, ok := resource.Properties["task_definition"].(string)
	if !ok || taskDefArn == "" {
		return
	}
	for _, other := range findAllByType(rm, models.ResourceTypeECSTaskDefinition) {
		if arn, _ := other.Properties["arn"].(string); arn != "" && strings.Contains(taskDefArn, arn) {
			builder.AddConnection(resource.ID, other.ID, models.ConnectionTypeDependency, "ECS service uses task definition")
			return
		}
		if family, _ := other.Properties["family"].(string); family != "" && strings.HasPrefix(taskDefArn, family) {
			builder.AddConnection(resource.ID, other.ID, models.ConnectionTypeDependency, "ECS service uses task definition")
			return
		}
	}
}

func (p *TerraformParser) analyzeECSServiceNetworkConfig(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
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
			connectToSecurityGroups(resource, sgIDs, rm, builder, "ECS service uses security group")
		}
	}
}

func (p *TerraformParser) analyzeECSServiceRegistries(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
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
			if sd := findByProperty(rm, models.ResourceTypeServiceDiscoveryService, "arn", sdArn); sd != nil {
				builder.AddConnection(resource.ID, sd.ID, models.ConnectionTypeReference, "ECS service registered in service discovery")
			}
		}
	}
}

// analyzeAPIGatewayConnections analyzes API Gateway connections.
func (p *TerraformParser) analyzeAPIGatewayConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	switch resource.Type {
	case models.ResourceTypeAPIGatewayMethod:
		p.analyzeAPIGatewayMethodConnections(resource, rm, builder)
	case models.ResourceTypeAPIGatewayIntegration:
		p.analyzeAPIGatewayIntegrationConnections(resource, rm, builder)
	case models.ResourceTypeAPIGatewayVPCLink:
		p.analyzeAPIGatewayVPCLinkConnections(resource, rm, builder)
	case models.ResourceTypeAPIGatewayStage:
		p.connectToRestAPI(resource, rm, builder, "rest_api_id", "Stage belongs to REST API")
	case models.ResourceTypeAPIGatewayDeployment:
		p.connectToRestAPI(resource, rm, builder, "rest_api_id", "Deployment belongs to REST API")
	case models.ResourceTypeAPIGatewayBasePathMapping:
		p.analyzeAPIGatewayBasePathMappingConnections(resource, rm, builder)
	}
}

func (p *TerraformParser) analyzeAPIGatewayMethodConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	restAPIID, _ := resource.Properties["rest_api_id"].(string)
	resourceIDProp, _ := resource.Properties["resource_id"].(string)
	if restAPIID == "" || resourceIDProp == "" {
		return
	}

	for _, other := range findAllByType(rm, models.ResourceTypeAPIGatewayIntegration) {
		otherRestAPI, _ := other.Properties["rest_api_id"].(string)
		otherResourceID, _ := other.Properties["resource_id"].(string)
		if otherRestAPI == restAPIID && otherResourceID == resourceIDProp {
			builder.AddConnection(resource.ID, other.ID, models.ConnectionTypeData, "API method uses integration")
		}
	}
}

func (p *TerraformParser) analyzeAPIGatewayIntegrationConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	connType, _ := resource.Properties["connection_type"].(string)
	if connType != "VPC_LINK" {
		return
	}
	connID, ok := resource.Properties["connection_id"].(string)
	if !ok || connID == "" {
		return
	}
	if vpcLink := findByProperty(rm, models.ResourceTypeAPIGatewayVPCLink, "id", connID); vpcLink != nil {
		builder.AddConnection(resource.ID, vpcLink.ID, models.ConnectionTypeNetworking, "API integration uses VPC Link")
	}
}

func (p *TerraformParser) analyzeAPIGatewayVPCLinkConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	targetArns, ok := resource.Properties["target_arns"].([]interface{})
	if !ok {
		return
	}
	for _, arnRaw := range targetArns {
		if arn, ok := arnRaw.(string); ok {
			if lb := findByProperty(rm, models.ResourceTypeLoadBalancer, "arn", arn); lb != nil {
				builder.AddConnection(resource.ID, lb.ID, models.ConnectionTypeNetworking, "VPC Link targets NLB")
			}
		}
	}
}

func (p *TerraformParser) analyzeAPIGatewayBasePathMappingConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	if domainName, ok := resource.Properties["domain_name"].(string); ok && domainName != "" {
		if domain := findByProperty(rm, models.ResourceTypeAPIGatewayDomainName, "domain_name", domainName); domain != nil {
			builder.AddConnection(resource.ID, domain.ID, models.ConnectionTypeDependency, "Base path mapping on domain")
		}
	}
	p.connectToRestAPI(resource, rm, builder, "api_id", "Base path mapping targets REST API")
}

// connectToRestAPI creates a connection from a resource to a REST API by matching a property key.
func (p *TerraformParser) connectToRestAPI(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder, propKey, description string) {
	restAPIID, ok := resource.Properties[propKey].(string)
	if !ok || restAPIID == "" {
		return
	}
	if api := findByProperty(rm, models.ResourceTypeAPIGateway, "id", restAPIID); api != nil {
		builder.AddConnection(resource.ID, api.ID, models.ConnectionTypeDependency, description)
	}
}

// analyzeSNSConnections analyzes SNS topic connections.
func (p *TerraformParser) analyzeSNSConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	topicArn, _ := resource.Properties["arn"].(string)
	if topicArn == "" {
		return
	}

	for _, sub := range findAllByProperty(rm, models.ResourceTypeSNSSubscription, "topic_arn", topicArn) {
		builder.AddConnection(resource.ID, sub.ID, models.ConnectionTypeTrigger, "SNS topic sends to subscription")

		// If the subscription targets an SQS queue, connect Subscription -> Queue.
		protocol, _ := sub.Properties["protocol"].(string)
		endpoint, _ := sub.Properties["endpoint"].(string)
		if protocol == "sqs" && endpoint != "" {
			if queue := findByProperty(rm, models.ResourceTypeSQSQueue, "arn", endpoint); queue != nil {
				builder.AddConnection(sub.ID, queue.ID, models.ConnectionTypeTrigger, "SNS subscription delivers to SQS queue")
			}
		}
	}
}

// analyzeSQSConnections analyzes SQS queue connections.
func (p *TerraformParser) analyzeSQSConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
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

	for _, other := range findAllByProperty(rm, models.ResourceTypeSQSQueue, "arn", dlqArn) {
		if other.ID != resource.ID {
			builder.AddConnection(resource.ID, other.ID, models.ConnectionTypeData, "SQS queue sends failures to DLQ")
		}
	}
}

// analyzeTargetGroupConnections analyzes Target Group connections.
func (p *TerraformParser) analyzeTargetGroupConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	targetGroupArn, _ := resource.Properties["arn"].(string)
	if targetGroupArn == "" {
		return
	}

	for _, listener := range findAllByType(rm, models.ResourceTypeLBListener) {
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
				builder.AddConnection(listener.ID, resource.ID, models.ConnectionTypeData, "Load balancer listener forwards to target group")
			}
		}
	}
}

// analyzeSecurityGroupConnections analyzes Security Group connections.
func (p *TerraformParser) analyzeSecurityGroupConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	ingress, ok := resource.Properties["ingress"].([]interface{})
	if !ok {
		return
	}

	for _, rule := range ingress {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}
		securityGroups, ok := ruleMap["security_groups"].([]interface{})
		if !ok {
			continue
		}
		for _, sg := range securityGroups {
			sgID, ok := sg.(string)
			if !ok {
				continue
			}
			if other := findByProperty(rm, models.ResourceTypeSecurityGroup, "id", sgID); other != nil && other.ID != resource.ID {
				builder.AddConnection(resource.ID, other.ID, models.ConnectionTypeAccess, "Security group allows traffic from security group")
			}
		}
	}
}

// analyzeRoute53Connections analyzes Route53 connections.
func (p *TerraformParser) analyzeRoute53Connections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	alias, ok := resource.Properties["alias"].(map[string]interface{})
	if !ok {
		return
	}
	dnsName, _ := alias["name"].(string)
	if dnsName == "" {
		return
	}
	if lb := findByProperty(rm, models.ResourceTypeLoadBalancer, "dns_name", dnsName); lb != nil {
		builder.AddConnection(resource.ID, lb.ID, models.ConnectionTypeReference, "Route53 record points to load balancer")
	}
}

// analyzeACMConnections analyzes ACM Certificate connections.
func (p *TerraformParser) analyzeACMConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	certArn, _ := resource.Properties["arn"].(string)
	if certArn == "" {
		return
	}

	for _, listener := range findAllByProperty(rm, models.ResourceTypeLBListener, "certificate_arn", certArn) {
		builder.AddConnection(listener.ID, resource.ID, models.ConnectionTypeAccess, "Load balancer listener uses SSL certificate")
	}

	for _, domain := range findAllByProperty(rm, models.ResourceTypeAPIGatewayDomainName, "certificate_arn", certArn) {
		builder.AddConnection(domain.ID, resource.ID, models.ConnectionTypeAccess, "API Gateway domain uses SSL certificate")
	}
}

// analyzeCloudWatchConnections analyzes CloudWatch log group connections.
func (p *TerraformParser) analyzeCloudWatchConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	logGroupName, _ := resource.Properties["name"].(string)
	if logGroupName == "" {
		return
	}

	// Find ECS task definitions that log to this log group.
	for _, taskDef := range findAllByType(rm, models.ResourceTypeECSTaskDefinition) {
		if containerDefs, ok := taskDef.Properties["container_definitions"].(string); ok {
			if strings.Contains(containerDefs, logGroupName) {
				builder.AddConnection(taskDef.ID, resource.ID, models.ConnectionTypeData, "ECS task logs to CloudWatch")
			}
		}
	}

	// Find Lambda functions that log to this group.
	for _, fn := range findAllByType(rm, models.ResourceTypeLambdaFunction) {
		if functionName, ok := fn.Properties["function_name"].(string); ok {
			if strings.Contains(logGroupName, functionName) {
				builder.AddConnection(fn.ID, resource.ID, models.ConnectionTypeData, "Lambda function logs to CloudWatch")
			}
		}
	}
}

// analyzeServiceDiscoveryConnections analyzes Service Discovery connections.
func (p *TerraformParser) analyzeServiceDiscoveryConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	if resource.Type != models.ResourceTypeServiceDiscoveryService {
		return
	}
	nsID, ok := resource.Properties["namespace_id"].(string)
	if !ok || nsID == "" {
		return
	}
	if ns := findByProperty(rm, models.ResourceTypeServiceDiscoveryNamespace, "id", nsID); ns != nil {
		builder.AddConnection(resource.ID, ns.ID, models.ConnectionTypeDependency, "Service registered in namespace")
	}
}

// analyzeECRConnections analyzes ECR repository connections.
func (p *TerraformParser) analyzeECRConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	if resource.Type != models.ResourceTypeECRRepository {
		return
	}

	repoURL, _ := resource.Properties["repository_url"].(string)
	repoName, _ := resource.Properties["name"].(string)
	if repoURL == "" && repoName == "" {
		return
	}

	for _, taskDef := range findAllByType(rm, models.ResourceTypeECSTaskDefinition) {
		containerDefs, ok := taskDef.Properties["container_definitions"].(string)
		if !ok {
			continue
		}
		if (repoURL != "" && strings.Contains(containerDefs, repoURL)) ||
			(repoName != "" && strings.Contains(containerDefs, repoName)) {
			builder.AddConnection(taskDef.ID, resource.ID, models.ConnectionTypeReference, "ECS task uses ECR image")
		}
	}
}

// analyzeWAFConnections analyzes WAF Web ACL connections.
func (p *TerraformParser) analyzeWAFConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	if resource.Type != models.ResourceTypeWAFWebACL {
		return
	}

	rulesRaw := resource.Properties["rule"]
	rulesJSON, err := json.Marshal(rulesRaw)
	if err != nil {
		return
	}
	rulesStr := string(rulesJSON)

	for _, ipSet := range findAllByType(rm, models.ResourceTypeWAFIPSet) {
		if id, _ := ipSet.Properties["id"].(string); id != "" && strings.Contains(rulesStr, id) {
			builder.AddConnection(resource.ID, ipSet.ID, models.ConnectionTypeReference, "WAF ACL references IP set")
			continue
		}
		if arn, _ := ipSet.Properties["arn"].(string); arn != "" && strings.Contains(rulesStr, arn) {
			builder.AddConnection(resource.ID, ipSet.ID, models.ConnectionTypeReference, "WAF ACL references IP set")
		}
	}
}

// analyzeSecretsManagerConnections analyzes Secrets Manager connections.
func (p *TerraformParser) analyzeSecretsManagerConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	if resource.Type != models.ResourceTypeSecretsManager {
		return
	}

	secretArn, _ := resource.Properties["arn"].(string)
	secretName, _ := resource.Properties["name"].(string)
	if secretArn == "" && secretName == "" {
		return
	}

	for _, taskDef := range findAllByType(rm, models.ResourceTypeECSTaskDefinition) {
		containerDefs, ok := taskDef.Properties["container_definitions"].(string)
		if !ok {
			continue
		}
		if (secretArn != "" && strings.Contains(containerDefs, secretArn)) ||
			(secretName != "" && strings.Contains(containerDefs, secretName)) {
			builder.AddConnection(taskDef.ID, resource.ID, models.ConnectionTypeAccess, "ECS task accesses secret")
		}
	}
}

// analyzeIAMConnections analyzes IAM role and policy connections.
func (p *TerraformParser) analyzeIAMConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	if resource.Type == models.ResourceTypeIAMRole {
		p.analyzeIAMRoleConnections(resource, rm, builder)
	}
	if resource.Type == models.ResourceTypeIAMPolicy {
		p.analyzeIAMPolicyConnections(resource, rm, builder)
	}
}

func (p *TerraformParser) analyzeIAMRoleConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	roleArn, _ := resource.Properties["arn"].(string)
	if roleArn == "" {
		return
	}

	// ECS task definitions that use this role.
	for _, taskDef := range findAllByType(rm, models.ResourceTypeECSTaskDefinition) {
		if taskRoleArn, _ := taskDef.Properties["task_role_arn"].(string); taskRoleArn == roleArn {
			builder.AddConnection(taskDef.ID, resource.ID, models.ConnectionTypeAccess, "ECS task uses IAM role")
		}
		if execRoleArn, _ := taskDef.Properties["execution_role_arn"].(string); execRoleArn == roleArn {
			builder.AddConnection(taskDef.ID, resource.ID, models.ConnectionTypeAccess, "ECS task execution uses IAM role")
		}
	}

	// Lambda functions that use this role.
	for _, fn := range findAllByProperty(rm, models.ResourceTypeLambdaFunction, "role", roleArn) {
		builder.AddConnection(fn.ID, resource.ID, models.ConnectionTypeAccess, "Lambda function uses IAM role")
	}

	// IAM role policy attachments.
	for _, attachment := range findAllByProperty(rm, models.ResourceTypeIAMRolePolicyAttachment, "role", roleArn) {
		builder.AddConnection(resource.ID, attachment.ID, models.ConnectionTypeAccess, "IAM role has policy attachment")
	}
}

func (p *TerraformParser) analyzeIAMPolicyConnections(resource *models.Resource, rm resourceMap, builder *models.DiagramBuilder) {
	policyArn, _ := resource.Properties["arn"].(string)
	if policyArn == "" {
		return
	}

	for _, attachment := range findAllByProperty(rm, models.ResourceTypeIAMRolePolicyAttachment, "policy_arn", policyArn) {
		builder.AddConnection(resource.ID, attachment.ID, models.ConnectionTypeAccess, "IAM policy attached to role")
	}
}
