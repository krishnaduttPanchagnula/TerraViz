package parsers

import (
	"encoding/json"
	"testing"

	"terraviz/internal/models"
)

// rawResource builds a raw .tfstate resource block for tests.
func rawResource(resType, name string, deps []string, attributes map[string]interface{}) map[string]interface{} {
	instance := map[string]interface{}{"attributes": attributes}
	if len(deps) > 0 {
		instance["dependencies"] = deps
	}
	resource := map[string]interface{}{
		"mode":     "managed",
		"type":     resType,
		"name":     name,
		"provider": `provider["registry.terraform.io/hashicorp/aws"]`,
		"instances": []interface{}{
			instance,
		},
	}
	if len(deps) > 0 {
		resource["dependencies"] = deps
	}
	return resource
}

// parseRawState parses a raw .tfstate built from the given resources.
func parseRawState(t *testing.T, resources ...map[string]interface{}) *models.ScanResult {
	t.Helper()
	data, err := json.Marshal(map[string]interface{}{
		"version":   4,
		"resources": resources,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewTerraformParser().ParseStateData(data, "test.tfstate")
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// hasConnection reports whether a connection between the two resources exists.
func hasConnection(diagram *models.Diagram, src, dst string) bool {
	for _, conn := range diagram.Connections {
		if conn.SourceID == src && conn.TargetID == dst {
			return true
		}
	}
	return false
}

// countConnections returns the number of connections between the two resources.
func countConnections(diagram *models.Diagram, src, dst string) int {
	count := 0
	for _, conn := range diagram.Connections {
		if conn.SourceID == src && conn.TargetID == dst {
			count++
		}
	}
	return count
}

func TestDependencyFromStateProducesConnection(t *testing.T) {
	result := parseRawState(t,
		rawResource("aws_vpc", "main", nil, map[string]interface{}{"id": "vpc-1"}),
		rawResource("aws_instance", "app", []string{"aws_vpc.main"}, map[string]interface{}{"id": "i-1"}),
	)

	if !hasConnection(&result.Diagram, "aws_instance.app", "aws_vpc.main") {
		t.Fatalf("expected dependency connection, got %#v", result.Diagram.Connections)
	}
	for _, conn := range result.Diagram.Connections {
		if conn.SourceID == "aws_instance.app" && conn.TargetID == "aws_vpc.main" {
			if conn.Type != models.ConnectionTypeDependency {
				t.Fatalf("connection type = %q, want %q", conn.Type, models.ConnectionTypeDependency)
			}
			if conn.Description != "Terraform dependency" {
				t.Fatalf("connection description = %q", conn.Description)
			}
		}
	}
}

func TestDependencyConnectionNotDuplicatedWhenSemanticEdgeExists(t *testing.T) {
	result := parseRawState(t,
		rawResource("aws_vpc", "main", nil, map[string]interface{}{"id": "vpc-1"}),
		rawResource("aws_instance", "app", []string{"aws_vpc.main"}, map[string]interface{}{
			"id": "i-1", "vpc_id": "vpc-1",
		}),
	)

	if got := countConnections(&result.Diagram, "aws_instance.app", "aws_vpc.main"); got != 1 {
		t.Fatalf("connection count = %d, want 1 (connections: %#v)", got, result.Diagram.Connections)
	}
}

func TestUnmappedResourceTypeIsRetained(t *testing.T) {
	result := parseRawState(t,
		rawResource("aws_dynamodb_table", "items", nil, map[string]interface{}{"id": "items"}),
	)

	var found *models.Resource
	for i := range result.Diagram.Resources {
		if result.Diagram.Resources[i].ID == "aws_dynamodb_table.items" {
			found = &result.Diagram.Resources[i]
		}
	}
	if found == nil {
		t.Fatalf("unmapped resource was dropped: %#v", result.Diagram.Resources)
	}
	if found.Type != models.ResourceTypeUnknown {
		t.Fatalf("resource type = %q, want %q", found.Type, models.ResourceTypeUnknown)
	}
	if found.Properties["terraform_type"] != "aws_dynamodb_table" {
		t.Fatalf("terraform_type = %v", found.Properties["terraform_type"])
	}
	if !found.InStateFile || found.Source != "terraform" || found.State != models.ResourceStateActive {
		t.Fatalf("unexpected resource metadata: %#v", found)
	}
	if found.Name != "items" {
		t.Fatalf("resource name = %q, want %q", found.Name, "items")
	}
}

func TestNonAWSResourceTypeIsStillDropped(t *testing.T) {
	result := parseRawState(t,
		map[string]interface{}{
			"mode":     "managed",
			"type":     "null_resource",
			"name":     "noop",
			"provider": `provider["registry.terraform.io/hashicorp/null"]`,
			"instances": []interface{}{
				map[string]interface{}{"attributes": map[string]interface{}{"id": "noop"}},
			},
		},
	)

	if got := len(result.Diagram.Resources); got != 0 {
		t.Fatalf("resource count = %d, want 0", got)
	}
}

func TestTargetGroupAttachmentConnectsToTargetGroupAndInstance(t *testing.T) {
	result := parseRawState(t,
		rawResource("aws_lb_target_group", "app", nil, map[string]interface{}{
			"id":  "tg-1",
			"arn": "arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/app/abc",
		}),
		rawResource("aws_instance", "app", nil, map[string]interface{}{"id": "i-1"}),
		rawResource("aws_lb_target_group_attachment", "app", nil, map[string]interface{}{
			"id":               "att-1",
			"target_group_arn": "arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/app/abc",
			"target_id":        "i-1",
		}),
	)

	if !hasConnection(&result.Diagram, "aws_lb_target_group.app", "aws_lb_target_group_attachment.app") {
		t.Fatalf("expected target group -> attachment connection: %#v", result.Diagram.Connections)
	}
	if !hasConnection(&result.Diagram, "aws_lb_target_group_attachment.app", "aws_instance.app") {
		t.Fatalf("expected attachment -> instance connection: %#v", result.Diagram.Connections)
	}
}

func TestCloudFrontConnectsToS3BucketOrigin(t *testing.T) {
	result := parseRawState(t,
		rawResource("aws_s3_bucket", "assets", nil, map[string]interface{}{
			"id":     "assets",
			"bucket": "my-assets",
		}),
		rawResource("aws_cloudfront_distribution", "web", nil, map[string]interface{}{
			"id": "dist-1",
			"origin": []interface{}{
				map[string]interface{}{"domain_name": "my-assets.s3.amazonaws.com"},
			},
		}),
	)

	if !hasConnection(&result.Diagram, "aws_cloudfront_distribution.web", "aws_s3_bucket.assets") {
		t.Fatalf("expected CloudFront -> S3 origin connection: %#v", result.Diagram.Connections)
	}
}

func TestCloudWatchLogGroupMatchesFunctionNameExactly(t *testing.T) {
	result := parseRawState(t,
		rawResource("aws_lambda_function", "app", nil, map[string]interface{}{
			"id":            "lambda-1",
			"function_name": "app",
		}),
		rawResource("aws_cloudwatch_log_group", "other", nil, map[string]interface{}{
			"id":   "/aws/lambda/app-prod",
			"name": "/aws/lambda/app-prod",
		}),
		rawResource("aws_cloudwatch_log_group", "own", nil, map[string]interface{}{
			"id":   "/aws/lambda/app",
			"name": "/aws/lambda/app",
		}),
	)

	if hasConnection(&result.Diagram, "aws_lambda_function.app", "aws_cloudwatch_log_group.other") {
		t.Fatal("log group /aws/lambda/app-prod must not match function app")
	}
	if !hasConnection(&result.Diagram, "aws_lambda_function.app", "aws_cloudwatch_log_group.own") {
		t.Fatalf("expected Lambda -> log group connection: %#v", result.Diagram.Connections)
	}
}

func TestECSTaskDefinitionMatchesFamilyExactly(t *testing.T) {
	result := parseRawState(t,
		rawResource("aws_ecs_task_definition", "app", nil, map[string]interface{}{
			"id":     "td-1",
			"family": "app",
			"arn":    "arn:aws:ecs:us-east-1:123456789012:task-definition/app:1",
		}),
		rawResource("aws_ecs_service", "svc", nil, map[string]interface{}{
			"id":              "svc-1",
			"task_definition": "arn:aws:ecs:us-east-1:123456789012:task-definition/app:3",
		}),
	)

	if !hasConnection(&result.Diagram, "aws_ecs_service.svc", "aws_ecs_task_definition.app") {
		t.Fatalf("expected ECS service -> task definition connection: %#v", result.Diagram.Connections)
	}
}

func TestECSTaskDefinitionDoesNotMatchSimilarFamily(t *testing.T) {
	result := parseRawState(t,
		rawResource("aws_ecs_task_definition", "app", nil, map[string]interface{}{
			"id":     "td-1",
			"family": "app",
			"arn":    "arn:aws:ecs:us-east-1:123456789012:task-definition/app:1",
		}),
		rawResource("aws_ecs_service", "svc", nil, map[string]interface{}{
			"id":              "svc-1",
			"task_definition": "arn:aws:ecs:us-east-1:123456789012:task-definition/application:3",
		}),
	)

	if hasConnection(&result.Diagram, "aws_ecs_service.svc", "aws_ecs_task_definition.app") {
		t.Fatal("task definition family must match exactly")
	}
}

// findConnection returns the connection between the two resources.
func findConnection(diagram *models.Diagram, src, dst string) *models.Connection {
	for i := range diagram.Connections {
		if diagram.Connections[i].SourceID == src && diagram.Connections[i].TargetID == dst {
			return &diagram.Connections[i]
		}
	}
	return nil
}

func TestGenericRelationsCoverEC2Instance(t *testing.T) {
	result := parseRawState(t,
		rawResource("aws_vpc", "main", nil, map[string]interface{}{"id": "vpc-1"}),
		rawResource("aws_subnet", "app", nil, map[string]interface{}{"id": "subnet-1", "vpc_id": "vpc-1"}),
		rawResource("aws_security_group", "app", nil, map[string]interface{}{"id": "sg-1", "vpc_id": "vpc-1"}),
		rawResource("aws_instance", "app", nil, map[string]interface{}{
			"id":                     "i-1",
			"vpc_id":                 "vpc-1",
			"subnet_id":              "subnet-1",
			"vpc_security_group_ids": []interface{}{"sg-1"},
		}),
	)

	cases := []struct {
		src, dst    string
		edgeType    models.ConnectionType
		description string
	}{
		{"aws_instance.app", "aws_vpc.main", models.ConnectionTypeNetworking, "EC2 instance in VPC"},
		{"aws_instance.app", "aws_subnet.app", models.ConnectionTypeNetworking, "EC2 instance in subnet"},
		{"aws_instance.app", "aws_security_group.app", models.ConnectionTypeAccess, "EC2 instance uses security group"},
		{"aws_subnet.app", "aws_vpc.main", models.ConnectionTypeNetworking, "Subnet belongs to VPC"},
		{"aws_security_group.app", "aws_vpc.main", models.ConnectionTypeNetworking, "Security group belongs to VPC"},
	}

	for _, tc := range cases {
		conn := findConnection(&result.Diagram, tc.src, tc.dst)
		if conn == nil {
			t.Fatalf("missing connection %s -> %s: %#v", tc.src, tc.dst, result.Diagram.Connections)
		}
		if conn.Type != tc.edgeType {
			t.Errorf("connection %s -> %s type = %q, want %q", tc.src, tc.dst, conn.Type, tc.edgeType)
		}
		if conn.Description != tc.description {
			t.Errorf("connection %s -> %s description = %q, want %q", tc.src, tc.dst, conn.Description, tc.description)
		}
	}
}

func TestSecurityGroupRuleEdgeCarriesRuleProperties(t *testing.T) {
	result := parseRawState(t,
		rawResource("aws_security_group", "web", nil, map[string]interface{}{
			"id": "sg-web",
			"ingress": []interface{}{
				map[string]interface{}{
					"protocol":        "tcp",
					"from_port":       443,
					"to_port":         443,
					"cidr_blocks":     []interface{}{"10.0.0.0/16"},
					"security_groups": []interface{}{"sg-db"},
				},
			},
		}),
		rawResource("aws_security_group", "db", nil, map[string]interface{}{"id": "sg-db"}),
	)

	conn := findConnection(&result.Diagram, "aws_security_group.web", "aws_security_group.db")
	if conn == nil {
		t.Fatalf("missing security group rule connection: %#v", result.Diagram.Connections)
	}
	if conn.Type != models.ConnectionTypeAccess {
		t.Errorf("connection type = %q, want %q", conn.Type, models.ConnectionTypeAccess)
	}
	if conn.Properties["protocol"] != "tcp" {
		t.Errorf("protocol = %v, want tcp", conn.Properties["protocol"])
	}
	if conn.Properties["from_port"] != float64(443) {
		t.Errorf("from_port = %v, want 443", conn.Properties["from_port"])
	}
	if conn.Properties["to_port"] != float64(443) {
		t.Errorf("to_port = %v, want 443", conn.Properties["to_port"])
	}
}

func TestMergedEdgesProduceSingleConnection(t *testing.T) {
	result := parseRawState(t,
		rawResource("aws_iam_role", "app", nil, map[string]interface{}{
			"id":  "role-1",
			"arn": "arn:aws:iam::123456789012:role/app",
		}),
		rawResource("aws_ecs_task_definition", "app", nil, map[string]interface{}{
			"id":                 "td-1",
			"task_role_arn":      "arn:aws:iam::123456789012:role/app",
			"execution_role_arn": "arn:aws:iam::123456789012:role/app",
		}),
	)

	if got := countConnections(&result.Diagram, "aws_ecs_task_definition.app", "aws_iam_role.app"); got != 1 {
		t.Fatalf("connection count = %d, want 1 (connections: %#v)", got, result.Diagram.Connections)
	}

	conn := findConnection(&result.Diagram, "aws_ecs_task_definition.app", "aws_iam_role.app")
	merged, ok := conn.Properties["reasons"].([]string)
	if !ok {
		t.Fatalf("connection has no reasons: %#v", conn.Properties)
	}
	if len(merged) != 2 {
		t.Fatalf("reasons = %#v, want two entries", merged)
	}
	if merged[0] != "ECS task uses IAM role" {
		t.Errorf("primary reason = %q", merged[0])
	}
}

func TestDependencyAndReferenceEdgesAreBidirectional(t *testing.T) {
	result := parseRawState(t,
		rawResource("aws_route53_zone", "main", nil, map[string]interface{}{"id": "Z123"}),
		rawResource("aws_route53_record", "app", nil, map[string]interface{}{
			"id":      "rec-1",
			"zone_id": "Z123",
		}),
	)

	conn := findConnection(&result.Diagram, "aws_route53_record.app", "aws_route53_zone.main")
	if conn == nil {
		t.Fatalf("missing route53 connection: %#v", result.Diagram.Connections)
	}
	if conn.Type != models.ConnectionTypeDependency {
		t.Fatalf("connection type = %q, want %q", conn.Type, models.ConnectionTypeDependency)
	}
	if !conn.Bidirectional {
		t.Error("dependency connections must be bidirectional")
	}
}
