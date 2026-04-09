package models

import (
	"time"
)

// ResourceType represents different types of cloud resources
type ResourceType string

const (
	// AWS Resource Types
	ResourceTypeEC2Instance    ResourceType = "aws:ec2:instance"
	ResourceTypeS3Bucket       ResourceType = "aws:s3:bucket"
	ResourceTypeRDSInstance    ResourceType = "aws:rds:instance"
	ResourceTypeLambdaFunction ResourceType = "aws:lambda:function"
	ResourceTypeLoadBalancer   ResourceType = "aws:elb:loadbalancer"
	ResourceTypeVPC            ResourceType = "aws:ec2:vpc"
	ResourceTypeSubnet         ResourceType = "aws:ec2:subnet"
	ResourceTypeSecurityGroup  ResourceType = "aws:ec2:securitygroup"
	ResourceTypeAPIGateway     ResourceType = "aws:apigateway:restapi"
	ResourceTypeCloudFront     ResourceType = "aws:cloudfront:distribution"
	ResourceTypeRoute53Zone    ResourceType = "aws:route53:hostedzone"
	ResourceTypeIAMRole        ResourceType = "aws:iam:role"
	ResourceTypeIAMUser        ResourceType = "aws:iam:user"
	ResourceTypeIAMPolicy      ResourceType = "aws:iam:policy"
	ResourceTypeCloudWatch     ResourceType = "aws:logs:loggroup"
	ResourceTypeKMSKey         ResourceType = "aws:kms:key"
	ResourceTypeSecretsManager ResourceType = "aws:secretsmanager:secret"

	// ECS / Container
	ResourceTypeECSCluster        ResourceType = "aws:ecs:cluster"
	ResourceTypeECSService        ResourceType = "aws:ecs:service"
	ResourceTypeECSTaskDefinition ResourceType = "aws:ecs:taskdefinition"
	ResourceTypeECRRepository     ResourceType = "aws:ecr:repository"

	// Messaging
	ResourceTypeSNSTopic        ResourceType = "aws:sns:topic"
	ResourceTypeSQSQueue        ResourceType = "aws:sqs:queue"
	ResourceTypeSNSSubscription ResourceType = "aws:sns:subscription"

	// Networking / DNS
	ResourceTypeRoute53Record             ResourceType = "aws:route53:record"
	ResourceTypeLBListener                ResourceType = "aws:elb:listener"
	ResourceTypeLBTargetGroup             ResourceType = "aws:elb:targetgroup"
	ResourceTypeAPIGatewayResource        ResourceType = "aws:apigateway:resource"
	ResourceTypeAPIGatewayStage           ResourceType = "aws:apigateway:stage"
	ResourceTypeAPIGatewayDeployment      ResourceType = "aws:apigateway:deployment"
	ResourceTypeAPIGatewayIntegration     ResourceType = "aws:apigateway:integration"
	ResourceTypeAPIGatewayDomainName      ResourceType = "aws:apigateway:domainname"
	ResourceTypeAPIGatewayUsagePlan       ResourceType = "aws:apigateway:usageplan"
	ResourceTypeAPIGatewayVPCLink         ResourceType = "aws:apigateway:vpclink"
	ResourceTypeAPIGatewayMethod          ResourceType = "aws:apigateway:method"
	ResourceTypeAPIGatewayMethodResponse  ResourceType = "aws:apigateway:methodresponse"
	ResourceTypeAPIGatewayIntegrationResp ResourceType = "aws:apigateway:integrationresponse"
	ResourceTypeAPIGatewayBasePathMapping ResourceType = "aws:apigateway:basepathmapping"

	// Security
	ResourceTypeACMCertificate ResourceType = "aws:acm:certificate"
	ResourceTypeWAFWebACL      ResourceType = "aws:waf:webacl"
	ResourceTypeWAFIPSet       ResourceType = "aws:waf:ipset"

	// IAM additional
	ResourceTypeIAMRolePolicyAttachment ResourceType = "aws:iam:rolepolicyattachment"

	// Service Discovery
	ResourceTypeServiceDiscoveryNamespace ResourceType = "aws:servicediscovery:namespace"
	ResourceTypeServiceDiscoveryService   ResourceType = "aws:servicediscovery:service"

	// Additional ELB types
	ResourceTypeLBListenerRule          ResourceType = "aws:elb:listenerrule"
	ResourceTypeLBTargetGroupAttachment ResourceType = "aws:elb:targetgroupattachment"

	// Additional SQS / SNS types
	ResourceTypeSQSQueuePolicy ResourceType = "aws:sqs:queuepolicy"
	ResourceTypeSNSTopicPolicy ResourceType = "aws:sns:topicpolicy"

	// Additional ACM types
	ResourceTypeACMCertificateValidation ResourceType = "aws:acm:certificatevalidation"

	// Additional WAF types
	ResourceTypeWAFWebACLAssociation ResourceType = "aws:waf:webaclassociation"

	// Generic Types
	ResourceTypeUnknown ResourceType = "unknown"
)

// ResourceState represents the current state of a resource
type ResourceState string

const (
	ResourceStateActive     ResourceState = "active"
	ResourceStateInactive   ResourceState = "inactive"
	ResourceStatePending    ResourceState = "pending"
	ResourceStateFailed     ResourceState = "failed"
	ResourceStateTerminated ResourceState = "terminated"
	ResourceStateUnknown    ResourceState = "unknown"
)

// ConnectionType represents different types of connections between resources
type ConnectionType string

const (
	ConnectionTypeNetworking ConnectionType = "networking"
	ConnectionTypeAccess     ConnectionType = "access"
	ConnectionTypeData       ConnectionType = "data"
	ConnectionTypeTrigger    ConnectionType = "trigger"
	ConnectionTypeDependency ConnectionType = "dependency"
	ConnectionTypeReference  ConnectionType = "reference"
)

// Resource represents a cloud infrastructure resource
type Resource struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Type       ResourceType           `json:"type"`
	Provider   string                 `json:"provider"` // aws, azure, gcp
	Region     string                 `json:"region"`
	Account    string                 `json:"account"`
	State      ResourceState          `json:"state"`
	Properties map[string]interface{} `json:"properties"`
	Tags       map[string]string      `json:"tags"`
	CreatedAt  *time.Time             `json:"created_at,omitempty"`
	UpdatedAt  *time.Time             `json:"updated_at,omitempty"`
	Source     string                 `json:"source"`   // terraform, cloudformation, live, repo
	IconURL    string                 `json:"icon_url"` // URL to service icon

	// UI Properties
	X      float64 `json:"x"`      // X coordinate in diagram
	Y      float64 `json:"y"`      // Y coordinate in diagram
	Hidden bool    `json:"hidden"` // Whether resource is hidden in UI
}

// Connection represents a relationship between two resources
type Connection struct {
	ID            string                 `json:"id"`
	SourceID      string                 `json:"source_id"`
	TargetID      string                 `json:"target_id"`
	Type          ConnectionType         `json:"type"`
	Description   string                 `json:"description"`
	Properties    map[string]interface{} `json:"properties"`
	Bidirectional bool                   `json:"bidirectional"`

	// UI Properties
	Hidden bool   `json:"hidden"` // Whether connection is hidden in UI
	Color  string `json:"color"`  // Line color
	Style  string `json:"style"`  // Line style (solid, dashed, dotted)
}

// Diagram represents the complete infrastructure diagram
type Diagram struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Resources   []Resource             `json:"resources"`
	Connections []Connection           `json:"connections"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Source      string                 `json:"source"`
	Version     string                 `json:"version"`
}

// ScanResult represents the result of scanning infrastructure
type ScanResult struct {
	Diagram    Diagram    `json:"diagram"`
	Errors     []string   `json:"errors,omitempty"`
	Warnings   []string   `json:"warnings,omitempty"`
	Stats      ScanStats  `json:"stats"`
	ScanTime   time.Time  `json:"scan_time"`
	ScanConfig ScanConfig `json:"scan_config"`
}

// ScanStats provides statistics about the scan
type ScanStats struct {
	ResourceCount   int `json:"resource_count"`
	ConnectionCount int `json:"connection_count"`
	ErrorCount      int `json:"error_count"`
	WarningCount    int `json:"warning_count"`
	ScanDurationMs  int `json:"scan_duration_ms"`
}

// ScanConfig contains configuration for the scan
type ScanConfig struct {
	Source     string            `json:"source"`     // terraform, cloudformation, aws, repo
	InputPath  string            `json:"input_path"` // path to state file, template, or repo
	Regions    []string          `json:"regions,omitempty"`
	Accounts   []string          `json:"accounts,omitempty"`
	Filters    map[string]string `json:"filters,omitempty"`
	IncludeAll bool              `json:"include_all"` // include all resource types
}

// ComparisonResult represents the result of comparing two diagrams
type ComparisonResult struct {
	BaseVersion        string            `json:"base_version"`
	CompareVersion     string            `json:"compare_version"`
	Added              []Resource        `json:"added"`
	Removed            []Resource        `json:"removed"`
	Modified           []ResourceDiff    `json:"modified"`
	ConnectionsAdded   []Connection      `json:"connections_added"`
	ConnectionsRemoved []Connection      `json:"connections_removed"`
	Summary            ComparisonSummary `json:"summary"`
	GeneratedAt        time.Time         `json:"generated_at"`
}

// ResourceDiff represents changes to a resource
type ResourceDiff struct {
	ResourceID string                    `json:"resource_id"`
	Changes    map[string]PropertyChange `json:"changes"`
}

// PropertyChange represents a change to a property
type PropertyChange struct {
	OldValue interface{} `json:"old_value"`
	NewValue interface{} `json:"new_value"`
}

// ComparisonSummary provides high-level statistics about the comparison
type ComparisonSummary struct {
	AddedCount     int `json:"added_count"`
	RemovedCount   int `json:"removed_count"`
	ModifiedCount  int `json:"modified_count"`
	UnchangedCount int `json:"unchanged_count"`
}

// DefaultResourceIcon is the fallback icon for unknown resource types.
const DefaultResourceIcon = "/icons/General-Icons/Marketplace_Dark.svg"

// resourceIconMap maps resource types to their icon URLs.
// All paths point to the embedded AWS icon pack served at /icons/.
var resourceIconMap = map[ResourceType]string{
	// Compute
	ResourceTypeEC2Instance:    "/icons/Compute/EC2.svg",
	ResourceTypeLambdaFunction: "/icons/Compute/Lambda.svg",

	// Containers
	ResourceTypeECSCluster:        "/icons/Containers/Elastic-Container-Service.svg",
	ResourceTypeECSService:        "/icons/Containers/Elastic-Container-Service.svg",
	ResourceTypeECSTaskDefinition: "/icons/Containers/Elastic-Container-Service.svg",
	ResourceTypeECRRepository:     "/icons/Containers/Elastic-Container-Registry.svg",

	// Storage
	ResourceTypeS3Bucket: "/icons/Storage/Simple-Storage-Service.svg",

	// Database
	ResourceTypeRDSInstance: "/icons/Database/RDS.svg",

	// Networking & Content Delivery
	ResourceTypeVPC:                       "/icons/Networking-Content-Delivery/Virtual-Private-Cloud.svg",
	ResourceTypeSubnet:                    "/icons/Networking-Content-Delivery/Virtual-Private-Cloud.svg",
	ResourceTypeSecurityGroup:             "/icons/Networking-Content-Delivery/Virtual-Private-Cloud.svg",
	ResourceTypeLoadBalancer:              "/icons/Networking-Content-Delivery/Elastic-Load-Balancing.svg",
	ResourceTypeLBListener:                "/icons/Networking-Content-Delivery/Elastic-Load-Balancing.svg",
	ResourceTypeLBTargetGroup:             "/icons/Networking-Content-Delivery/Elastic-Load-Balancing.svg",
	ResourceTypeLBListenerRule:            "/icons/Networking-Content-Delivery/Elastic-Load-Balancing.svg",
	ResourceTypeLBTargetGroupAttachment:   "/icons/Networking-Content-Delivery/Elastic-Load-Balancing.svg",
	ResourceTypeRoute53Zone:               "/icons/Networking-Content-Delivery/Route-53.svg",
	ResourceTypeRoute53Record:             "/icons/Networking-Content-Delivery/Route-53.svg",
	ResourceTypeCloudFront:                "/icons/Networking-Content-Delivery/CloudFront.svg",
	ResourceTypeServiceDiscoveryNamespace: "/icons/Networking-Content-Delivery/Cloud-Map.svg",
	ResourceTypeServiceDiscoveryService:   "/icons/Networking-Content-Delivery/Cloud-Map.svg",

	// App Integration
	ResourceTypeAPIGateway:                "/icons/App-Integration/API-Gateway.svg",
	ResourceTypeAPIGatewayResource:        "/icons/App-Integration/API-Gateway.svg",
	ResourceTypeAPIGatewayStage:           "/icons/App-Integration/API-Gateway.svg",
	ResourceTypeAPIGatewayDeployment:      "/icons/App-Integration/API-Gateway.svg",
	ResourceTypeAPIGatewayIntegration:     "/icons/App-Integration/API-Gateway.svg",
	ResourceTypeAPIGatewayDomainName:      "/icons/App-Integration/API-Gateway.svg",
	ResourceTypeAPIGatewayUsagePlan:       "/icons/App-Integration/API-Gateway.svg",
	ResourceTypeAPIGatewayVPCLink:         "/icons/App-Integration/API-Gateway.svg",
	ResourceTypeAPIGatewayMethod:          "/icons/App-Integration/API-Gateway.svg",
	ResourceTypeAPIGatewayMethodResponse:  "/icons/App-Integration/API-Gateway.svg",
	ResourceTypeAPIGatewayIntegrationResp: "/icons/App-Integration/API-Gateway.svg",
	ResourceTypeAPIGatewayBasePathMapping: "/icons/App-Integration/API-Gateway.svg",
	ResourceTypeSNSTopic:                  "/icons/App-Integration/Simple-Notification-Service.svg",
	ResourceTypeSNSSubscription:           "/icons/App-Integration/Simple-Notification-Service.svg",
	ResourceTypeSNSTopicPolicy:            "/icons/App-Integration/Simple-Notification-Service.svg",
	ResourceTypeSQSQueue:                  "/icons/App-Integration/Simple-Queue-Service.svg",
	ResourceTypeSQSQueuePolicy:            "/icons/App-Integration/Simple-Queue-Service.svg",

	// Security, Identity & Compliance
	ResourceTypeIAMRole:                  "/icons/Security-Identity-Compliance/Identity-and-Access-Management.svg",
	ResourceTypeIAMUser:                  "/icons/Security-Identity-Compliance/Identity-and-Access-Management.svg",
	ResourceTypeIAMPolicy:                "/icons/Security-Identity-Compliance/Identity-and-Access-Management.svg",
	ResourceTypeIAMRolePolicyAttachment:  "/icons/Security-Identity-Compliance/Identity-and-Access-Management.svg",
	ResourceTypeACMCertificate:           "/icons/Security-Identity-Compliance/Certificate-Manager.svg",
	ResourceTypeACMCertificateValidation: "/icons/Security-Identity-Compliance/Certificate-Manager.svg",
	ResourceTypeWAFWebACL:                "/icons/Security-Identity-Compliance/WAF.svg",
	ResourceTypeWAFIPSet:                 "/icons/Security-Identity-Compliance/WAF.svg",
	ResourceTypeWAFWebACLAssociation:     "/icons/Security-Identity-Compliance/WAF.svg",
	ResourceTypeSecretsManager:           "/icons/Security-Identity-Compliance/Secrets-Manager.svg",
	ResourceTypeKMSKey:                   "/icons/Security-Identity-Compliance/Key-Management-Service.svg",

	// Management & Governance
	ResourceTypeCloudWatch: "/icons/Management-Governance/CloudWatch.svg",
}

// GetResourceIcon returns the appropriate icon URL for a resource type.
func (r *Resource) GetResourceIcon() string {
	if r.IconURL != "" {
		return r.IconURL
	}

	if icon, exists := resourceIconMap[r.Type]; exists {
		return icon
	}

	return DefaultResourceIcon
}
