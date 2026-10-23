package parsers

import (
	"testing"

	"terraviz/internal/models"
)

// findMissing returns the missing reference with the given ARN or id.
func findMissing(missing []models.ReferencedMissing, arn, id string) *models.ReferencedMissing {
	for i := range missing {
		if missing[i].ARN == arn && missing[i].ID == id {
			return &missing[i]
		}
	}
	return nil
}

func TestMissingReferenceARNNotInState(t *testing.T) {
	keyARN := "arn:aws:kms:us-east-1:123456789012:key/11111111-2222-3333-4444-555555555555"

	result := parseRawState(t,
		rawResource("aws_s3_bucket", "assets", nil, map[string]interface{}{
			"id":         "assets",
			"kms_key_id": keyARN,
		}),
	)

	if len(result.ReferencedMissing) != 1 {
		t.Fatalf("missing count = %d, want 1: %#v", len(result.ReferencedMissing), result.ReferencedMissing)
	}

	entry := result.ReferencedMissing[0]
	if entry.Type != models.ResourceTypeKMSKey {
		t.Errorf("type = %q, want %q", entry.Type, models.ResourceTypeKMSKey)
	}
	if entry.ARN != keyARN {
		t.Errorf("arn = %q, want %q", entry.ARN, keyARN)
	}
	if entry.ID != "" {
		t.Errorf("id = %q, want empty", entry.ID)
	}
	if len(entry.ReferencedBy) != 1 || entry.ReferencedBy[0] != "aws_s3_bucket.assets" {
		t.Errorf("referenced_by = %#v, want [aws_s3_bucket.assets]", entry.ReferencedBy)
	}
	if entry.Description == "" {
		t.Error("description is empty")
	}
}

func TestMissingReferenceARNInStateIsNotReported(t *testing.T) {
	keyARN := "arn:aws:kms:us-east-1:123456789012:key/11111111-2222-3333-4444-555555555555"

	result := parseRawState(t,
		rawResource("aws_kms_key", "main", nil, map[string]interface{}{
			"id":  "11111111-2222-3333-4444-555555555555",
			"arn": keyARN,
		}),
		rawResource("aws_s3_bucket", "assets", nil, map[string]interface{}{
			"id":         "assets",
			"kms_key_id": keyARN,
		}),
	)

	if len(result.ReferencedMissing) != 0 {
		t.Fatalf("missing count = %d, want 0: %#v", len(result.ReferencedMissing), result.ReferencedMissing)
	}
}

func TestMissingReferenceInNestedProperty(t *testing.T) {
	dlqARN := "arn:aws:sqs:us-east-1:123456789012:prod-dlq"
	targetGroupARN := "arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/prod-api/1a2b3c4d5e6f"

	result := parseRawState(t,
		rawResource("aws_sqs_queue", "events", nil, map[string]interface{}{
			"id":             "events",
			"redrive_policy": `{"deadLetterTargetArn":"` + dlqARN + `"}`,
		}),
		rawResource("aws_ecs_service", "svc", nil, map[string]interface{}{
			"id": "svc-1",
			"load_balancer": []interface{}{
				map[string]interface{}{"target_group_arn": targetGroupARN},
			},
		}),
	)

	if len(result.ReferencedMissing) != 2 {
		t.Fatalf("missing count = %d, want 2: %#v", len(result.ReferencedMissing), result.ReferencedMissing)
	}

	dlq := findMissing(result.ReferencedMissing, dlqARN, "")
	if dlq == nil {
		t.Fatalf("dead letter queue not reported: %#v", result.ReferencedMissing)
	}
	if dlq.Type != models.ResourceTypeSQSQueue {
		t.Errorf("dead letter queue type = %q, want %q", dlq.Type, models.ResourceTypeSQSQueue)
	}
	if len(dlq.ReferencedBy) != 1 || dlq.ReferencedBy[0] != "aws_sqs_queue.events" {
		t.Errorf("dead letter queue referenced_by = %#v", dlq.ReferencedBy)
	}

	targetGroup := findMissing(result.ReferencedMissing, targetGroupARN, "")
	if targetGroup == nil {
		t.Fatalf("target group not reported: %#v", result.ReferencedMissing)
	}
	if targetGroup.Type != models.ResourceTypeLBTargetGroup {
		t.Errorf("target group type = %q, want %q", targetGroup.Type, models.ResourceTypeLBTargetGroup)
	}
	if len(targetGroup.ReferencedBy) != 1 || targetGroup.ReferencedBy[0] != "aws_ecs_service.svc" {
		t.Errorf("target group referenced_by = %#v", targetGroup.ReferencedBy)
	}
}

func TestMissingReferenceOfUnmappedServiceKeepsARNType(t *testing.T) {
	tableARN := "arn:aws:dynamodb:us-east-1:123456789012:table/prod-sessions"

	result := parseRawState(t,
		rawResource("aws_lambda_function", "app", nil, map[string]interface{}{
			"id": "app",
			"environment_variables": []interface{}{
				map[string]interface{}{"name": "TABLE", "value": tableARN},
			},
		}),
	)

	if len(result.ReferencedMissing) != 1 {
		t.Fatalf("missing count = %d, want 1: %#v", len(result.ReferencedMissing), result.ReferencedMissing)
	}

	entry := result.ReferencedMissing[0]
	if entry.Type != models.ResourceType("table") {
		t.Errorf("type = %q, want %q", entry.Type, "table")
	}
	if entry.ARN != tableARN {
		t.Errorf("arn = %q, want %q", entry.ARN, tableARN)
	}
}

func TestNoMissingReferenceForOwnIdentifiers(t *testing.T) {
	result := parseRawState(t,
		rawResource("aws_iam_role", "app", nil, map[string]interface{}{
			"id":  "prod-lambda-role",
			"arn": "arn:aws:iam::123456789012:role/prod-lambda-role",
		}),
		rawResource("aws_iam_policy", "app", nil, map[string]interface{}{
			"id":  "prod-policy",
			"arn": "arn:aws:iam::123456789012:policy/prod-policy",
		}),
	)

	if len(result.ReferencedMissing) != 0 {
		t.Fatalf("missing count = %d, want 0: %#v", len(result.ReferencedMissing), result.ReferencedMissing)
	}
}

func TestMissingReferenceKeepsExternalPlaceholder(t *testing.T) {
	result := parseRawState(t,
		rawResource("aws_instance", "app", nil, map[string]interface{}{
			"id":                     "i-0api111111",
			"vpc_security_group_ids": []interface{}{"sg-0missing123"},
		}),
	)

	placeholder := false
	for _, resource := range result.Diagram.Resources {
		if resource.Source == externalReferenceSource {
			placeholder = true
		}
	}
	if !placeholder {
		t.Fatalf("expected the external reference placeholder: %#v", result.Diagram.Resources)
	}

	if len(result.ReferencedMissing) != 1 {
		t.Fatalf("missing count = %d, want 1: %#v", len(result.ReferencedMissing), result.ReferencedMissing)
	}

	entry := result.ReferencedMissing[0]
	if entry.Type != models.ResourceTypeSecurityGroup {
		t.Errorf("type = %q, want %q", entry.Type, models.ResourceTypeSecurityGroup)
	}
	if entry.ID != "sg-0missing123" {
		t.Errorf("id = %q, want %q", entry.ID, "sg-0missing123")
	}
	if len(entry.ReferencedBy) != 1 || entry.ReferencedBy[0] != "aws_instance.app" {
		t.Errorf("referenced_by = %#v, want [aws_instance.app]", entry.ReferencedBy)
	}
}

func TestMissingReferenceToResourceOfUnmappedTypeIsNotReported(t *testing.T) {
	tableARN := "arn:aws:dynamodb:us-east-1:123456789012:table/prod-sessions"

	result := parseRawState(t,
		rawResource("aws_dynamodb_table", "sessions", nil, map[string]interface{}{
			"id":  "prod-sessions",
			"arn": tableARN,
		}),
		rawResource("aws_lambda_function", "app", nil, map[string]interface{}{
			"id":                    "app",
			"environment_variables": []interface{}{map[string]interface{}{"name": "TABLE", "value": tableARN}},
		}),
	)

	if len(result.ReferencedMissing) != 0 {
		t.Fatalf("missing count = %d, want 0: %#v", len(result.ReferencedMissing), result.ReferencedMissing)
	}
}

func TestWildcardPolicyReferenceToBucketInStateIsNotReported(t *testing.T) {
	result := parseRawState(t,
		rawResource("aws_s3_bucket", "assets", nil, map[string]interface{}{
			"id":  "prod-assets-bucket",
			"arn": "arn:aws:s3:::prod-assets-bucket",
		}),
		rawResource("aws_iam_policy", "reader", nil, map[string]interface{}{
			"id":  "reader",
			"arn": "arn:aws:iam::123456789012:policy/reader",
			"policy": `{"Statement":[{"Effect":"Allow","Action":"s3:GetObject",` +
				`"Resource":"arn:aws:s3:::prod-assets-bucket/*"}]}`,
		}),
	)

	if len(result.ReferencedMissing) != 0 {
		t.Fatalf("missing count = %d, want 0: %#v", len(result.ReferencedMissing), result.ReferencedMissing)
	}
}

func TestWildcardPolicyReferenceToMissingBucketIsReported(t *testing.T) {
	result := parseRawState(t,
		rawResource("aws_iam_policy", "reader", nil, map[string]interface{}{
			"id":  "reader",
			"arn": "arn:aws:iam::123456789012:policy/reader",
			"policy": `{"Statement":[{"Effect":"Allow","Action":"s3:GetObject",` +
				`"Resource":"arn:aws:s3:::other-bucket/*"}]}`,
		}),
	)

	if len(result.ReferencedMissing) != 1 {
		t.Fatalf("missing count = %d, want 1: %#v", len(result.ReferencedMissing), result.ReferencedMissing)
	}

	entry := result.ReferencedMissing[0]
	if entry.Type != models.ResourceTypeS3Bucket {
		t.Errorf("type = %q, want %q", entry.Type, models.ResourceTypeS3Bucket)
	}
	if entry.ARN != "arn:aws:s3:::other-bucket" {
		t.Errorf("arn = %q, want %q", entry.ARN, "arn:aws:s3:::other-bucket")
	}
	if len(entry.ReferencedBy) != 1 || entry.ReferencedBy[0] != "aws_iam_policy.reader" {
		t.Errorf("referenced_by = %#v, want [aws_iam_policy.reader]", entry.ReferencedBy)
	}
}
