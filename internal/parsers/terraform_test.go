package parsers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMain generates fixture files before running tests.
func TestMain(m *testing.M) {
	for _, size := range []int{50, 500, 5000} {
		generateRawStateFixture(size)
		generateShowJSONFixture(size)
	}
	os.Exit(m.Run())
}

// generateRawStateFixture creates a synthetic raw .tfstate v4 file with n resources.
// Resources reference each other to exercise connection analyzers.
func generateRawStateFixture(n int) {
	resources := make([]map[string]interface{}, 0, n)

	// Distribute resource types to exercise different analyzers.
	types := []struct {
		tfType string
		attrs  func(i int) map[string]interface{}
	}{
		{"aws_vpc", func(i int) map[string]interface{} {
			return map[string]interface{}{
				"id":         fmt.Sprintf("vpc-%04d", i),
				"cidr_block": "10.0.0.0/16",
				"tags":       map[string]interface{}{"Name": fmt.Sprintf("vpc-%d", i)},
			}
		}},
		{"aws_subnet", func(i int) map[string]interface{} {
			return map[string]interface{}{
				"id":         fmt.Sprintf("subnet-%04d", i),
				"vpc_id":     fmt.Sprintf("vpc-%04d", i%max(n/10, 1)),
				"cidr_block": fmt.Sprintf("10.0.%d.0/24", i%256),
				"tags":       map[string]interface{}{"Name": fmt.Sprintf("subnet-%d", i)},
			}
		}},
		{"aws_security_group", func(i int) map[string]interface{} {
			return map[string]interface{}{
				"id":     fmt.Sprintf("sg-%04d", i),
				"name":   fmt.Sprintf("sg-%d", i),
				"vpc_id": fmt.Sprintf("vpc-%04d", i%max(n/10, 1)),
			}
		}},
		{"aws_instance", func(i int) map[string]interface{} {
			return map[string]interface{}{
				"id":                     fmt.Sprintf("i-%04d", i),
				"subnet_id":              fmt.Sprintf("subnet-%04d", i%max(n/5, 1)),
				"vpc_security_group_ids": []interface{}{fmt.Sprintf("sg-%04d", i%max(n/10, 1))},
				"iam_instance_profile":   fmt.Sprintf("profile-%d", i%max(n/20, 1)),
				"tags":                   map[string]interface{}{"Name": fmt.Sprintf("instance-%d", i)},
			}
		}},
		{"aws_lambda_function", func(i int) map[string]interface{} {
			return map[string]interface{}{
				"id":            fmt.Sprintf("lambda-%04d", i),
				"function_name": fmt.Sprintf("fn-%d", i),
				"role":          fmt.Sprintf("arn:aws:iam::123456789012:role/role-%d", i%max(n/20, 1)),
			}
		}},
		{"aws_iam_role", func(i int) map[string]interface{} {
			return map[string]interface{}{
				"id":   fmt.Sprintf("role-%04d", i),
				"name": fmt.Sprintf("role-%d", i),
				"arn":  fmt.Sprintf("arn:aws:iam::123456789012:role/role-%d", i),
			}
		}},
		{"aws_s3_bucket", func(i int) map[string]interface{} {
			return map[string]interface{}{
				"id":     fmt.Sprintf("bucket-%04d", i),
				"bucket": fmt.Sprintf("my-bucket-%d", i),
			}
		}},
		{"aws_cloudwatch_log_group", func(i int) map[string]interface{} {
			return map[string]interface{}{
				"id":   fmt.Sprintf("/aws/lambda/fn-%d", i),
				"name": fmt.Sprintf("/aws/lambda/fn-%d", i),
				"arn":  fmt.Sprintf("arn:aws:logs:us-east-1:123456789012:log-group:/aws/lambda/fn-%d", i),
			}
		}},
		{"aws_ecs_task_definition", func(i int) map[string]interface{} {
			return map[string]interface{}{
				"id":                    fmt.Sprintf("task-def-%04d", i),
				"family":                fmt.Sprintf("task-%d", i),
				"arn":                   fmt.Sprintf("arn:aws:ecs:us-east-1:123456789012:task-definition/task-%d:1", i),
				"container_definitions": fmt.Sprintf(`[{"name":"app","image":"123456789012.dkr.ecr.us-east-1.amazonaws.com/repo-%d:latest","logConfiguration":{"logDriver":"awslogs","options":{"awslogs-group":"/aws/lambda/fn-%d"}}}]`, i%max(n/10, 1), i%max(n/10, 1)),
			}
		}},
		{"aws_ecs_service", func(i int) map[string]interface{} {
			return map[string]interface{}{
				"id":              fmt.Sprintf("svc-%04d", i),
				"name":            fmt.Sprintf("svc-%d", i),
				"cluster":         fmt.Sprintf("arn:aws:ecs:us-east-1:123456789012:cluster/cluster-0"),
				"task_definition": fmt.Sprintf("arn:aws:ecs:us-east-1:123456789012:task-definition/task-%d:1", i%max(n/10, 1)),
			}
		}},
	}

	for i := 0; i < n; i++ {
		t := types[i%len(types)]
		resources = append(resources, map[string]interface{}{
			"mode":     "managed",
			"type":     t.tfType,
			"name":     fmt.Sprintf("res_%d", i),
			"provider": "provider[\"registry.terraform.io/hashicorp/aws\"]",
			"instances": []interface{}{
				map[string]interface{}{
					"schema_version": 0,
					"attributes":     t.attrs(i),
				},
			},
		})
	}

	state := map[string]interface{}{
		"version":           4,
		"terraform_version": "1.5.0",
		"serial":            1,
		"lineage":           "test",
		"outputs":           map[string]interface{}{},
		"resources":         resources,
	}

	data, _ := json.Marshal(state)
	path := filepath.Join("testdata", fmt.Sprintf("raw_state_%d.json", n))
	os.WriteFile(path, data, 0644)
}

// generateShowJSONFixture creates a synthetic terraform show -json file with n resources.
func generateShowJSONFixture(n int) {
	resources := make([]map[string]interface{}, 0, n)

	types := []string{
		"aws_instance", "aws_vpc", "aws_subnet", "aws_security_group",
		"aws_lambda_function", "aws_iam_role", "aws_s3_bucket",
		"aws_cloudwatch_log_group", "aws_ecs_task_definition", "aws_ecs_service",
	}

	for i := 0; i < n; i++ {
		tfType := types[i%len(types)]
		resources = append(resources, map[string]interface{}{
			"address":        fmt.Sprintf("%s.res_%d", tfType, i),
			"mode":           "managed",
			"type":           tfType,
			"name":           fmt.Sprintf("res_%d", i),
			"provider_name":  "registry.terraform.io/hashicorp/aws",
			"schema_version": 0,
			"values": map[string]interface{}{
				"id":        fmt.Sprintf("id-%04d", i),
				"name":      fmt.Sprintf("resource-%d", i),
				"vpc_id":    fmt.Sprintf("vpc-%04d", i%max(n/10, 1)),
				"subnet_id": fmt.Sprintf("subnet-%04d", i%max(n/5, 1)),
				"tags":      map[string]interface{}{"Name": fmt.Sprintf("resource-%d", i)},
			},
			"sensitive_values": map[string]interface{}{},
		})
	}

	state := map[string]interface{}{
		"format_version":    "1.0",
		"terraform_version": "1.5.0",
		"values": map[string]interface{}{
			"root_module": map[string]interface{}{
				"resources": resources,
			},
		},
	}

	data, _ := json.Marshal(state)
	path := filepath.Join("testdata", fmt.Sprintf("show_json_%d.json", n))
	os.WriteFile(path, data, 0644)
}

func BenchmarkParseStateFile_RawState_50(b *testing.B) {
	benchParseRaw(b, 50)
}

func BenchmarkParseStateFile_RawState_500(b *testing.B) {
	benchParseRaw(b, 500)
}

func BenchmarkParseStateFile_RawState_5000(b *testing.B) {
	benchParseRaw(b, 5000)
}

func BenchmarkParseStateFile_ShowJSON_50(b *testing.B) {
	benchParseShowJSON(b, 50)
}

func BenchmarkParseStateFile_ShowJSON_500(b *testing.B) {
	benchParseShowJSON(b, 500)
}

func BenchmarkParseStateFile_ShowJSON_5000(b *testing.B) {
	benchParseShowJSON(b, 5000)
}

func benchParseRaw(b *testing.B, n int) {
	path := filepath.Join("testdata", fmt.Sprintf("raw_state_%d.json", n))
	if _, err := os.Stat(path); err != nil {
		b.Skipf("fixture not found: %s", path)
	}
	p := NewTerraformParser()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.ParseStateFile(path)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func benchParseShowJSON(b *testing.B, n int) {
	path := filepath.Join("testdata", fmt.Sprintf("show_json_%d.json", n))
	if _, err := os.Stat(path); err != nil {
		b.Skipf("fixture not found: %s", path)
	}
	p := NewTerraformParser()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.ParseStateFile(path)
		if err != nil {
			b.Fatal(err)
		}
	}
}
