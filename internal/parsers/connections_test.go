package parsers

import (
	"fmt"
	"testing"

	"terraviz/internal/models"
)

// buildTestDiagram creates a diagram with n resources of mixed types
// that reference each other, exercising connection analyzers.
func buildTestDiagram(n int) (*models.Diagram, *models.DiagramBuilder) {
	builder := models.NewDiagramBuilder("bench", "test")

	// Resource type distribution (10 types cycling)
	typeSpecs := []struct {
		rType models.ResourceType
		props func(i, n int) map[string]interface{}
	}{
		{models.ResourceTypeVPC, func(i, n int) map[string]interface{} {
			return map[string]interface{}{
				"id":         fmt.Sprintf("vpc-%04d", i),
				"cidr_block": "10.0.0.0/16",
			}
		}},
		{models.ResourceTypeSubnet, func(i, n int) map[string]interface{} {
			return map[string]interface{}{
				"id":     fmt.Sprintf("subnet-%04d", i),
				"vpc_id": fmt.Sprintf("vpc-%04d", i%max(n/10, 1)),
			}
		}},
		{models.ResourceTypeSecurityGroup, func(i, n int) map[string]interface{} {
			return map[string]interface{}{
				"id":     fmt.Sprintf("sg-%04d", i),
				"vpc_id": fmt.Sprintf("vpc-%04d", i%max(n/10, 1)),
			}
		}},
		{models.ResourceTypeEC2Instance, func(i, n int) map[string]interface{} {
			return map[string]interface{}{
				"id":                     fmt.Sprintf("i-%04d", i),
				"subnet_id":              fmt.Sprintf("subnet-%04d", i%max(n/5, 1)),
				"vpc_security_group_ids": fmt.Sprintf("[\"sg-%04d\"]", i%max(n/10, 1)),
				"iam_instance_profile":   fmt.Sprintf("profile-%d", i%max(n/20, 1)),
			}
		}},
		{models.ResourceTypeLambdaFunction, func(i, n int) map[string]interface{} {
			return map[string]interface{}{
				"function_name": fmt.Sprintf("fn-%d", i),
				"role":          fmt.Sprintf("arn:aws:iam::123456789012:role/role-%d", i%max(n/20, 1)),
			}
		}},
		{models.ResourceTypeIAMRole, func(i, n int) map[string]interface{} {
			return map[string]interface{}{
				"name": fmt.Sprintf("role-%d", i),
				"arn":  fmt.Sprintf("arn:aws:iam::123456789012:role/role-%d", i),
			}
		}},
		{models.ResourceTypeS3Bucket, func(i, n int) map[string]interface{} {
			return map[string]interface{}{
				"bucket": fmt.Sprintf("my-bucket-%d", i),
			}
		}},
		{models.ResourceTypeCloudWatch, func(i, n int) map[string]interface{} {
			return map[string]interface{}{
				"name": fmt.Sprintf("/aws/lambda/fn-%d", i),
				"arn":  fmt.Sprintf("arn:aws:logs:us-east-1:123456789012:log-group:/aws/lambda/fn-%d", i),
			}
		}},
		{models.ResourceTypeECSTaskDefinition, func(i, n int) map[string]interface{} {
			return map[string]interface{}{
				"family":                fmt.Sprintf("task-%d", i),
				"arn":                   fmt.Sprintf("arn:aws:ecs:us-east-1:123456789012:task-definition/task-%d:1", i),
				"container_definitions": fmt.Sprintf(`[{"name":"app","image":"123456789012.dkr.ecr.us-east-1.amazonaws.com/repo-%d:latest","logConfiguration":{"logDriver":"awslogs","options":{"awslogs-group":"/aws/lambda/fn-%d"}}}]`, i%max(n/10, 1), i%max(n/10, 1)),
			}
		}},
		{models.ResourceTypeECSService, func(i, n int) map[string]interface{} {
			return map[string]interface{}{
				"name":            fmt.Sprintf("svc-%d", i),
				"cluster":         "arn:aws:ecs:us-east-1:123456789012:cluster/cluster-0",
				"task_definition": fmt.Sprintf("arn:aws:ecs:us-east-1:123456789012:task-definition/task-%d:1", i%max(n/10, 1)),
			}
		}},
	}

	for i := 0; i < n; i++ {
		spec := typeSpecs[i%len(typeSpecs)]
		builder.AddResource(models.Resource{
			ID:         fmt.Sprintf("res-%04d", i),
			Name:       fmt.Sprintf("resource-%d", i),
			Type:       spec.rType,
			Provider:   "aws",
			State:      models.ResourceStateActive,
			Properties: spec.props(i, n),
			Tags:       map[string]string{"Name": fmt.Sprintf("resource-%d", i)},
			Source:     "test",
		})
	}

	diagram := builder.Build()
	return diagram, builder
}

func BenchmarkCreateConnections_50(b *testing.B) {
	benchCreateConnections(b, 50)
}

func BenchmarkCreateConnections_200(b *testing.B) {
	benchCreateConnections(b, 200)
}

func BenchmarkCreateConnections_1000(b *testing.B) {
	benchCreateConnections(b, 1000)
}

func benchCreateConnections(b *testing.B, n int) {
	// Build the diagram once, then benchmark just connection creation.
	// We rebuild per iteration because createConnections mutates the diagram.
	diagrams := make([]*models.Diagram, b.N)
	builders := make([]*models.DiagramBuilder, b.N)
	for i := 0; i < b.N; i++ {
		diagrams[i], builders[i] = buildTestDiagram(n)
	}

	p := NewTerraformParser()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.createConnections(diagrams[i], builders[i])
	}
}
