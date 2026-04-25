package parsers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	tfjson "github.com/hashicorp/terraform-json"
	"terraviz/internal/models"
)

// TerraformParser parses Terraform state files
type TerraformParser struct{}

// NewTerraformParser creates a new Terraform parser
func NewTerraformParser() *TerraformParser {
	return &TerraformParser{}
}

// ParseStateFile parses a Terraform state file and returns a diagram
// Supports both raw .tfstate format (v4) and terraform show -json format
func (p *TerraformParser) ParseStateFile(filePath string) (*models.ScanResult, error) {
	// Read the state file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	// Auto-detect format using byte-level checks to avoid a full unmarshal.
	// terraform show -json always has "format_version"; raw v4 has "version" but not "format_version".
	if bytes.Contains(data, []byte(`"format_version"`)) {
		return p.parseTerraformShowJSON(data)
	} else if bytes.Contains(data, []byte(`"version"`)) {
		return p.parseRawStateFile(data, filePath)
	} else {
		return nil, fmt.Errorf("unknown state file format")
	}
}

// parseTerraformShowJSON parses terraform show -json format
func (p *TerraformParser) parseTerraformShowJSON(data []byte) (*models.ScanResult, error) {
	// Parse the JSON state
	var state tfjson.State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	// Create diagram builder
	builder := models.NewDiagramBuilder("Terraform Infrastructure", "terraform")
	builder.Build().Metadata["terraform_version"] = state.TerraformVersion
	if state.Values != nil {
		builder.Build().Metadata["terraform_format_version"] = state.FormatVersion
	}

	var errors []string
	var warnings []string
	resourceCount := 0

	// Process resources from the state
	if state.Values != nil && state.Values.RootModule != nil {
		p.processModule(state.Values.RootModule, builder, "", &errors, &warnings, &resourceCount)
	}

	// Build the final diagram
	diagram := builder.Build()
	diagram.CreatedAt = time.Now()
	diagram.UpdatedAt = time.Now()

	// Create connections based on resource references
	p.createConnections(diagram, builder)

	result := &models.ScanResult{
		Diagram:  *diagram,
		Errors:   errors,
		Warnings: warnings,
		Stats: models.ScanStats{
			ResourceCount:   resourceCount,
			ConnectionCount: len(diagram.Connections),
			ErrorCount:      len(errors),
			WarningCount:    len(warnings),
		},
		ScanTime: time.Now(),
		ScanConfig: models.ScanConfig{
			Source:    "terraform",
			InputPath: "terraform show -json",
		},
	}

	return result, nil
}

// RawStateFile represents the structure of a raw .tfstate file
type RawStateFile struct {
	Version          int                    `json:"version"`
	TerraformVersion string                 `json:"terraform_version"`
	Serial           int                    `json:"serial"`
	Lineage          string                 `json:"lineage"`
	Outputs          map[string]interface{} `json:"outputs"`
	Resources        []RawStateResource     `json:"resources"`
}

// RawStateResource represents a single resource block in a raw .tfstate file (v4).
type RawStateResource struct {
	Module    string                     `json:"module,omitempty"`
	Mode      string                     `json:"mode"`
	Type      string                     `json:"type"`
	Name      string                     `json:"name"`
	Provider  string                     `json:"provider"`
	Instances []RawStateResourceInstance `json:"instances"`
}

// RawStateResourceInstance represents a single instance of a resource in a raw .tfstate file.
type RawStateResourceInstance struct {
	SchemaVersion int                    `json:"schema_version"`
	Attributes    map[string]interface{} `json:"attributes"`
	IndexKey      interface{}            `json:"index_key,omitempty"`
}

// parseRawStateFile parses raw .tfstate format (version 4)
func (p *TerraformParser) parseRawStateFile(data []byte, filePath string) (*models.ScanResult, error) {
	var state RawStateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse raw state file: %w", err)
	}

	// Create diagram builder
	builder := models.NewDiagramBuilder("Terraform Infrastructure", "terraform")
	builder.Build().Metadata["terraform_version"] = state.TerraformVersion
	builder.Build().Metadata["state_version"] = state.Version
	builder.Build().Metadata["serial"] = state.Serial

	var errors []string
	var warnings []string
	resourceCount := 0

	// Process all resources
	for _, rawResource := range state.Resources {
		// Skip data sources
		if rawResource.Mode == "data" {
			continue
		}

		// Process each instance of the resource
		for instanceIdx, instance := range rawResource.Instances {
			resource, err := p.convertRawTerraformResource(rawResource, instance, instanceIdx)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Error processing resource %s.%s: %v", rawResource.Type, rawResource.Name, err))
				continue
			}

			if resource != nil {
				builder.AddResource(*resource)
				resourceCount++
			} else {
				warnings = append(warnings, fmt.Sprintf("Unsupported resource type: %s", rawResource.Type))
			}
		}
	}

	// Build the final diagram
	diagram := builder.Build()
	diagram.CreatedAt = time.Now()
	diagram.UpdatedAt = time.Now()

	// Create connections based on resource references
	p.createConnections(diagram, builder)

	result := &models.ScanResult{
		Diagram:  *diagram,
		Errors:   errors,
		Warnings: warnings,
		Stats: models.ScanStats{
			ResourceCount:   resourceCount,
			ConnectionCount: len(diagram.Connections),
			ErrorCount:      len(errors),
			WarningCount:    len(warnings),
		},
		ScanTime: time.Now(),
		ScanConfig: models.ScanConfig{
			Source:    "terraform",
			InputPath: filePath,
		},
	}

	return result, nil
}
func (p *TerraformParser) processModule(module *tfjson.StateModule, builder *models.DiagramBuilder, modulePrefix string, errors *[]string, warnings *[]string, resourceCount *int) {
	// Process resources
	for _, resource := range module.Resources {
		tfResource, err := p.convertTerraformResource(resource, modulePrefix)
		if err != nil {
			*errors = append(*errors, fmt.Sprintf("Error processing resource %s: %v", resource.Address, err))
			continue
		}

		if tfResource != nil {
			builder.AddResource(*tfResource)
			*resourceCount++
		} else {
			*warnings = append(*warnings, fmt.Sprintf("Unsupported resource type: %s", resource.Type))
		}
	}

	// Process child modules
	for _, childModule := range module.ChildModules {
		childPrefix := modulePrefix
		if modulePrefix != "" {
			childPrefix += "."
		}
		childPrefix += childModule.Address
		p.processModule(childModule, builder, childPrefix, errors, warnings, resourceCount)
	}
}

// convertRawTerraformResource converts a raw .tfstate resource to our internal model
func (p *TerraformParser) convertRawTerraformResource(rawResource RawStateResource, instance RawStateResourceInstance, instanceIdx int) (*models.Resource, error) {
	// Map Terraform resource types to our internal types
	resourceType := p.mapTerraformType(rawResource.Type)
	if resourceType == models.ResourceTypeUnknown {
		return nil, nil // Skip unknown types
	}

	// Build resource ID from module path and resource address
	var resourceID string
	if rawResource.Module != "" {
		resourceID = fmt.Sprintf("%s.%s.%s", rawResource.Module, rawResource.Type, rawResource.Name)
	} else {
		resourceID = fmt.Sprintf("%s.%s", rawResource.Type, rawResource.Name)
	}

	// If this is a multi-instance resource, add index
	if len(rawResource.Instances) > 1 {
		if instance.IndexKey != nil {
			resourceID += fmt.Sprintf("[%v]", instance.IndexKey)
		} else {
			resourceID += fmt.Sprintf("[%d]", instanceIdx)
		}
	}

	resource := &models.Resource{
		ID:         resourceID,
		Name:       p.extractRawResourceName(rawResource, instance.Attributes),
		Type:       resourceType,
		Provider:   p.extractProvider(rawResource.Provider),
		State:      models.ResourceStateActive, // Assume active if in state
		Properties: make(map[string]interface{}),
		Tags:       p.extractTags(instance.Attributes),
		Source:     "terraform",
	}

	// Extract common attributes
	if region, ok := instance.Attributes["region"].(string); ok {
		resource.Region = region
	}
	if accountID, ok := instance.Attributes["account_id"].(string); ok {
		resource.Account = accountID
	} else if arn, ok := instance.Attributes["arn"].(string); ok {
		// Extract account ID from ARN format: arn:aws:service:region:account:resource
		parts := strings.Split(arn, ":")
		if len(parts) >= 5 {
			resource.Account = parts[4]
		}
	}

	// Copy all attribute values to properties
	for key, value := range instance.Attributes {
		resource.Properties[key] = value
	}

	return resource, nil
}

// extractRawResourceName extracts a meaningful name from a raw state resource
func (p *TerraformParser) extractRawResourceName(rawResource RawStateResource, attributes map[string]interface{}) string {
	// Try common name attributes
	if name, ok := attributes["name"].(string); ok && name != "" {
		return name
	}
	if id, ok := attributes["id"].(string); ok && id != "" {
		// For some resources like SQS, the ID is a URL - extract just the name part
		if strings.Contains(id, "/") {
			parts := strings.Split(id, "/")
			lastPart := parts[len(parts)-1]
			if lastPart != "" {
				return lastPart
			}
		}
		return id
	}
	if tags, ok := attributes["tags"].(map[string]interface{}); ok {
		if name, ok := tags["Name"].(string); ok && name != "" {
			return name
		}
	}

	// Fall back to the Terraform resource name
	return rawResource.Name
}
func (p *TerraformParser) convertTerraformResource(tfResource *tfjson.StateResource, modulePrefix string) (*models.Resource, error) {
	// Skip data sources for now
	if tfResource.Mode == tfjson.DataResourceMode {
		return nil, nil
	}

	// Create resource ID with module prefix if applicable
	resourceID := tfResource.Address
	if modulePrefix != "" {
		resourceID = modulePrefix + "." + resourceID
	}

	// Map Terraform resource types to our internal types
	resourceType := p.mapTerraformType(tfResource.Type)
	if resourceType == models.ResourceTypeUnknown {
		return nil, nil // Skip unknown types
	}

	resource := &models.Resource{
		ID:         resourceID,
		Name:       p.extractResourceName(tfResource),
		Type:       resourceType,
		Provider:   p.extractProvider(tfResource.ProviderName),
		State:      models.ResourceStateActive, // Assume active if in state
		Properties: make(map[string]interface{}),
		Tags:       p.extractTags(tfResource.AttributeValues),
		Source:     "terraform",
	}

	// Extract common attributes
	if region, ok := tfResource.AttributeValues["region"].(string); ok {
		resource.Region = region
	}
	if accountID, ok := tfResource.AttributeValues["account_id"].(string); ok {
		resource.Account = accountID
	}

	// Copy all attribute values to properties
	for key, value := range tfResource.AttributeValues {
		resource.Properties[key] = value
	}

	return resource, nil
}

// terraformTypeMap maps Terraform resource types to internal types.
// Hoisted to package level to avoid rebuilding on every call.
var terraformTypeMap = map[string]models.ResourceType{
	// Compute
	"aws_instance":            models.ResourceTypeEC2Instance,
	"aws_ecs_cluster":         models.ResourceTypeECSCluster,
	"aws_ecs_service":         models.ResourceTypeECSService,
	"aws_ecs_task_definition": models.ResourceTypeECSTaskDefinition,
	"aws_lambda_function":     models.ResourceTypeLambdaFunction,

	// Storage
	"aws_s3_bucket":      models.ResourceTypeS3Bucket,
	"aws_db_instance":    models.ResourceTypeRDSInstance,
	"aws_ecr_repository": models.ResourceTypeECRRepository,

	// Networking
	"aws_vpc":             models.ResourceTypeVPC,
	"aws_subnet":          models.ResourceTypeSubnet,
	"aws_security_group":  models.ResourceTypeSecurityGroup,
	"aws_lb":              models.ResourceTypeLoadBalancer,
	"aws_alb":             models.ResourceTypeLoadBalancer,
	"aws_elb":             models.ResourceTypeLoadBalancer,
	"aws_lb_listener":     models.ResourceTypeLBListener,
	"aws_lb_target_group": models.ResourceTypeLBTargetGroup,

	// API Gateway
	"aws_api_gateway_rest_api":             models.ResourceTypeAPIGateway,
	"aws_api_gateway_resource":             models.ResourceTypeAPIGatewayResource,
	"aws_api_gateway_method":               models.ResourceTypeAPIGatewayMethod,
	"aws_api_gateway_integration":          models.ResourceTypeAPIGatewayIntegration,
	"aws_api_gateway_method_response":      models.ResourceTypeAPIGatewayMethodResponse,
	"aws_api_gateway_integration_response": models.ResourceTypeAPIGatewayIntegrationResp,
	"aws_api_gateway_deployment":           models.ResourceTypeAPIGatewayDeployment,
	"aws_api_gateway_stage":                models.ResourceTypeAPIGatewayStage,
	"aws_api_gateway_domain_name":          models.ResourceTypeAPIGatewayDomainName,
	"aws_api_gateway_base_path_mapping":    models.ResourceTypeAPIGatewayBasePathMapping,
	"aws_api_gateway_usage_plan":           models.ResourceTypeAPIGatewayUsagePlan,
	"aws_api_gateway_vpc_link":             models.ResourceTypeAPIGatewayVPCLink,

	// Messaging
	"aws_sns_topic":              models.ResourceTypeSNSTopic,
	"aws_sns_topic_subscription": models.ResourceTypeSNSSubscription,
	"aws_sqs_queue":              models.ResourceTypeSQSQueue,

	// DNS & CDN
	"aws_route53_zone":            models.ResourceTypeRoute53Zone,
	"aws_route53_record":          models.ResourceTypeRoute53Record,
	"aws_cloudfront_distribution": models.ResourceTypeCloudFront,

	// Security & IAM
	"aws_iam_role":                   models.ResourceTypeIAMRole,
	"aws_iam_user":                   models.ResourceTypeIAMUser,
	"aws_iam_policy":                 models.ResourceTypeIAMPolicy,
	"aws_iam_role_policy_attachment": models.ResourceTypeIAMRolePolicyAttachment,
	"aws_acm_certificate":            models.ResourceTypeACMCertificate,
	"aws_wafv2_web_acl":              models.ResourceTypeWAFWebACL,
	"aws_wafv2_ip_set":               models.ResourceTypeWAFIPSet,

	// Monitoring & Logging
	"aws_cloudwatch_log_group":  models.ResourceTypeCloudWatch,
	"aws_kms_key":               models.ResourceTypeKMSKey,
	"aws_secretsmanager_secret": models.ResourceTypeSecretsManager,

	// Service Discovery
	"aws_service_discovery_private_dns_namespace": models.ResourceTypeServiceDiscoveryNamespace,
	"aws_service_discovery_service":               models.ResourceTypeServiceDiscoveryService,

	// Additional ELB types
	"aws_lb_listener_rule":           models.ResourceTypeLBListenerRule,
	"aws_lb_target_group_attachment": models.ResourceTypeLBTargetGroupAttachment,

	// Additional SQS / SNS types
	"aws_sqs_queue_policy": models.ResourceTypeSQSQueuePolicy,
	"aws_sns_topic_policy": models.ResourceTypeSNSTopicPolicy,

	// Additional ACM types
	"aws_acm_certificate_validation": models.ResourceTypeACMCertificateValidation,

	// Additional WAF types
	"aws_wafv2_web_acl_association": models.ResourceTypeWAFWebACLAssociation,
}

// mapTerraformType maps a Terraform resource type string to an internal ResourceType.
func (p *TerraformParser) mapTerraformType(tfType string) models.ResourceType {
	if rt, ok := terraformTypeMap[tfType]; ok {
		return rt
	}
	return models.ResourceTypeUnknown
}

// extractProvider extracts the provider name from the provider string
func (p *TerraformParser) extractProvider(providerName string) string {
	// Provider name format is usually "registry.terraform.io/hashicorp/aws" or just "aws"
	parts := strings.Split(providerName, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return providerName
}

// extractResourceName extracts a meaningful name from the resource
func (p *TerraformParser) extractResourceName(tfResource *tfjson.StateResource) string {
	// Try common name attributes
	if name, ok := tfResource.AttributeValues["name"].(string); ok && name != "" {
		return name
	}
	if id, ok := tfResource.AttributeValues["id"].(string); ok && id != "" {
		return id
	}
	if tags, ok := tfResource.AttributeValues["tags"].(map[string]interface{}); ok {
		if name, ok := tags["Name"].(string); ok && name != "" {
			return name
		}
	}

	// Fall back to the Terraform resource name
	parts := strings.Split(tfResource.Address, ".")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}

	return tfResource.Address
}

// extractTags extracts tags from resource attributes
func (p *TerraformParser) extractTags(attributes map[string]interface{}) map[string]string {
	tags := make(map[string]string)

	if tagMap, ok := attributes["tags"].(map[string]interface{}); ok {
		for key, value := range tagMap {
			if strValue, ok := value.(string); ok {
				tags[key] = strValue
			}
		}
	}

	return tags
}
