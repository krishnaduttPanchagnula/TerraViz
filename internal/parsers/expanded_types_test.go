package parsers

import (
	"strings"
	"testing"

	"terraviz/internal/models"
)

// Every newly typed Terraform resource must map to a typed model with a real
// embedded icon, not the generic fallback.
func TestExpandedAWSTypeCoverage(t *testing.T) {
	cases := map[string]models.ResourceType{
		"aws_dynamodb_table":        models.ResourceTypeDynamoDBTable,
		"aws_elasticache_cluster":   models.ResourceTypeElastiCache,
		"aws_redshift_cluster":      models.ResourceTypeRedshift,
		"aws_kinesis_stream":        models.ResourceTypeKinesisStream,
		"aws_opensearch_domain":     models.ResourceTypeOpenSearch,
		"aws_athena_workgroup":      models.ResourceTypeAthenaWorkgroup,
		"aws_efs_file_system":       models.ResourceTypeEFSFileSystem,
		"aws_eks_cluster":           models.ResourceTypeEKSCluster,
		"aws_ecs_task":              models.ResourceTypeECSTask,
		"aws_autoscaling_group":     models.ResourceTypeAutoScalingGroup,
		"aws_apprunner_service":     models.ResourceTypeAppRunnerService,
		"aws_ec2_transit_gateway":   models.ResourceTypeTransitGateway,
		"aws_cloudwatch_event_rule": models.ResourceTypeEventBridgeRule,
		"aws_sfn_state_machine":     models.ResourceTypeStepFunction,
		"aws_ses_email_identity":    models.ResourceTypeSESEmailIdentity,
		"aws_cognito_user_pool":     models.ResourceTypeCognitoUserPool,
		"aws_backup_plan":           models.ResourceTypeBackupPlan,
		"aws_cloudtrail":            models.ResourceTypeCloudTrail,
		"aws_config_config_rule":    models.ResourceTypeConfigRule,
		"aws_xray_group":            models.ResourceTypeXRayGroup,
	}

	p := NewTerraformParser()
	for tfType, want := range cases {
		t.Run(tfType, func(t *testing.T) {
			got := p.mapTerraformType(tfType)
			if got != want {
				t.Errorf("mapTerraformType(%q) = %q, want %q", tfType, got, want)
			}
			r := &models.Resource{Type: got}
			icon := r.GetResourceIcon()
			if icon == models.DefaultResourceIcon {
				t.Errorf("no icon mapped for %s", got)
			}
			if !strings.HasPrefix(icon, "/icons/") {
				t.Errorf("invalid icon path %q", icon)
			}
		})
	}
}
