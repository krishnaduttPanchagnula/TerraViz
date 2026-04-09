package scanners

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"terraviz/internal/models"
)

const defaultRegion = "us-east-1"

// AWSScanner scans live AWS accounts.
type AWSScanner struct {
	cfg     aws.Config
	regions []string
	profile string
}

// NewAWSScanner creates a new AWS scanner.
// The provided context is used for loading the AWS configuration.
func NewAWSScanner(ctx context.Context, profile string, regions []string) (*AWSScanner, error) {
	var opts []func(*config.LoadOptions) error
	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	if len(regions) == 0 {
		regions = []string{defaultRegion}
	}

	return &AWSScanner{
		cfg:     cfg,
		regions: regions,
		profile: profile,
	}, nil
}

// ScanAccount scans an AWS account and returns a diagram.
func (s *AWSScanner) ScanAccount(ctx context.Context) (*models.ScanResult, error) {
	startTime := time.Now()

	builder := models.NewDiagramBuilder("AWS Live Infrastructure", "aws")
	builder.Build().Metadata["aws_regions"] = s.regions
	builder.Build().Metadata["aws_profile"] = s.profile

	var allErrors []string
	var allWarnings []string
	totalResources := 0

	for _, region := range s.regions {
		regionCfg := s.cfg.Copy()
		regionCfg.Region = region

		slog.Info("scanning region", "region", region)

		errors, warnings, count := s.scanEC2(ctx, regionCfg, region, builder)
		allErrors = append(allErrors, errors...)
		allWarnings = append(allWarnings, warnings...)
		totalResources += count

		// S3 is global; list buckets once from us-east-1.
		if region == defaultRegion {
			errors, warnings, count = s.scanS3(ctx, regionCfg, builder)
			allErrors = append(allErrors, errors...)
			allWarnings = append(allWarnings, warnings...)
			totalResources += count
		}

		errors, warnings, count = s.scanRDS(ctx, regionCfg, region, builder)
		allErrors = append(allErrors, errors...)
		allWarnings = append(allWarnings, warnings...)
		totalResources += count

		errors, warnings, count = s.scanLambda(ctx, regionCfg, region, builder)
		allErrors = append(allErrors, errors...)
		allWarnings = append(allWarnings, warnings...)
		totalResources += count
	}

	diagram := builder.Build()
	diagram.CreatedAt = time.Now()
	diagram.UpdatedAt = time.Now()

	s.createConnections(diagram, builder)

	scanDuration := int(time.Since(startTime).Milliseconds())

	return &models.ScanResult{
		Diagram:  *diagram,
		Errors:   allErrors,
		Warnings: allWarnings,
		Stats: models.ScanStats{
			ResourceCount:   totalResources,
			ConnectionCount: len(diagram.Connections),
			ErrorCount:      len(allErrors),
			WarningCount:    len(allWarnings),
			ScanDurationMs:  scanDuration,
		},
		ScanTime: time.Now(),
		ScanConfig: models.ScanConfig{
			Source:  "aws",
			Regions: s.regions,
		},
	}, nil
}

// scanEC2 scans EC2 resources (VPCs, subnets, security groups, instances) in a region.
func (s *AWSScanner) scanEC2(ctx context.Context, cfg aws.Config, region string, builder *models.DiagramBuilder) ([]string, []string, int) {
	var errors []string
	var warnings []string
	resourceCount := 0

	client := ec2.NewFromConfig(cfg)

	vpcsOutput, err := client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{})
	if err != nil {
		errors = append(errors, fmt.Sprintf("Failed to describe VPCs in %s: %v", region, err))
		return errors, warnings, resourceCount
	}
	for _, vpc := range vpcsOutput.Vpcs {
		builder.AddResource(*s.convertVPCToResource(vpc, region))
		resourceCount++
	}

	subnetsOutput, err := client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{})
	if err != nil {
		errors = append(errors, fmt.Sprintf("Failed to describe subnets in %s: %v", region, err))
	} else {
		for _, subnet := range subnetsOutput.Subnets {
			builder.AddResource(*s.convertSubnetToResource(subnet, region))
			resourceCount++
		}
	}

	sgOutput, err := client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{})
	if err != nil {
		errors = append(errors, fmt.Sprintf("Failed to describe security groups in %s: %v", region, err))
	} else {
		for _, sg := range sgOutput.SecurityGroups {
			builder.AddResource(*s.convertSecurityGroupToResource(sg, region))
			resourceCount++
		}
	}

	instancesOutput, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{})
	if err != nil {
		errors = append(errors, fmt.Sprintf("Failed to describe instances in %s: %v", region, err))
	} else {
		for _, reservation := range instancesOutput.Reservations {
			for _, instance := range reservation.Instances {
				if instance.State.Name == ec2types.InstanceStateNameTerminated {
					continue
				}
				builder.AddResource(*s.convertInstanceToResource(instance, region))
				resourceCount++
			}
		}
	}

	return errors, warnings, resourceCount
}

// scanS3 scans S3 buckets.
func (s *AWSScanner) scanS3(ctx context.Context, cfg aws.Config, builder *models.DiagramBuilder) ([]string, []string, int) {
	var errors []string
	var warnings []string
	resourceCount := 0

	client := s3.NewFromConfig(cfg)

	bucketsOutput, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		errors = append(errors, fmt.Sprintf("Failed to list S3 buckets: %v", err))
		return errors, warnings, resourceCount
	}

	for _, bucket := range bucketsOutput.Buckets {
		region := defaultRegion
		locationOutput, err := client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{
			Bucket: bucket.Name,
		})
		if err == nil && locationOutput.LocationConstraint != "" {
			region = string(locationOutput.LocationConstraint)
		}

		builder.AddResource(*s.convertS3BucketToResource(bucket, region))
		resourceCount++
	}

	return errors, warnings, resourceCount
}

// scanRDS scans RDS instances in a region.
func (s *AWSScanner) scanRDS(ctx context.Context, cfg aws.Config, region string, builder *models.DiagramBuilder) ([]string, []string, int) {
	var errors []string
	var warnings []string
	resourceCount := 0

	client := rds.NewFromConfig(cfg)

	instancesOutput, err := client.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{})
	if err != nil {
		errors = append(errors, fmt.Sprintf("Failed to describe RDS instances in %s: %v", region, err))
		return errors, warnings, resourceCount
	}

	for _, instance := range instancesOutput.DBInstances {
		builder.AddResource(*s.convertRDSInstanceToResource(instance, region))
		resourceCount++
	}

	return errors, warnings, resourceCount
}

// scanLambda scans Lambda functions in a region.
func (s *AWSScanner) scanLambda(ctx context.Context, cfg aws.Config, region string, builder *models.DiagramBuilder) ([]string, []string, int) {
	var errors []string
	var warnings []string
	resourceCount := 0

	client := lambda.NewFromConfig(cfg)

	functionsOutput, err := client.ListFunctions(ctx, &lambda.ListFunctionsInput{})
	if err != nil {
		errors = append(errors, fmt.Sprintf("Failed to list Lambda functions in %s: %v", region, err))
		return errors, warnings, resourceCount
	}

	for _, function := range functionsOutput.Functions {
		builder.AddResource(*s.convertLambdaFunctionToResource(function, region))
		resourceCount++
	}

	return errors, warnings, resourceCount
}

// --- Resource conversion helpers ---

func (s *AWSScanner) convertVPCToResource(vpc ec2types.Vpc, region string) *models.Resource {
	name := aws.ToString(vpc.VpcId)
	if tags := s.extractEC2Tags(vpc.Tags); tags["Name"] != "" {
		name = tags["Name"]
	}

	return &models.Resource{
		ID:       fmt.Sprintf("vpc-%s-%s", region, aws.ToString(vpc.VpcId)),
		Name:     name,
		Type:     models.ResourceTypeVPC,
		Provider: "aws",
		Region:   region,
		State:    s.convertVPCState(vpc.State),
		Properties: map[string]interface{}{
			"vpc_id":          aws.ToString(vpc.VpcId),
			"cidr_block":      aws.ToString(vpc.CidrBlock),
			"state":           string(vpc.State),
			"is_default":      aws.ToBool(vpc.IsDefault),
			"dhcp_options_id": aws.ToString(vpc.DhcpOptionsId),
		},
		Tags:   s.extractEC2Tags(vpc.Tags),
		Source: "aws",
	}
}

func (s *AWSScanner) convertSubnetToResource(subnet ec2types.Subnet, region string) *models.Resource {
	name := aws.ToString(subnet.SubnetId)
	if tags := s.extractEC2Tags(subnet.Tags); tags["Name"] != "" {
		name = tags["Name"]
	}

	return &models.Resource{
		ID:       fmt.Sprintf("subnet-%s-%s", region, aws.ToString(subnet.SubnetId)),
		Name:     name,
		Type:     models.ResourceTypeSubnet,
		Provider: "aws",
		Region:   region,
		State:    s.convertSubnetState(subnet.State),
		Properties: map[string]interface{}{
			"subnet_id":              aws.ToString(subnet.SubnetId),
			"vpc_id":                 aws.ToString(subnet.VpcId),
			"cidr_block":             aws.ToString(subnet.CidrBlock),
			"availability_zone":      aws.ToString(subnet.AvailabilityZone),
			"available_ip_addresses": aws.ToInt32(subnet.AvailableIpAddressCount),
			"state":                  string(subnet.State),
		},
		Tags:   s.extractEC2Tags(subnet.Tags),
		Source: "aws",
	}
}

func (s *AWSScanner) convertSecurityGroupToResource(sg ec2types.SecurityGroup, region string) *models.Resource {
	name := aws.ToString(sg.GroupName)
	if tags := s.extractEC2Tags(sg.Tags); tags["Name"] != "" {
		name = tags["Name"]
	}

	return &models.Resource{
		ID:       fmt.Sprintf("sg-%s-%s", region, aws.ToString(sg.GroupId)),
		Name:     name,
		Type:     models.ResourceTypeSecurityGroup,
		Provider: "aws",
		Region:   region,
		State:    models.ResourceStateActive,
		Properties: map[string]interface{}{
			"group_id":    aws.ToString(sg.GroupId),
			"group_name":  aws.ToString(sg.GroupName),
			"description": aws.ToString(sg.Description),
			"vpc_id":      aws.ToString(sg.VpcId),
		},
		Tags:   s.extractEC2Tags(sg.Tags),
		Source: "aws",
	}
}

func (s *AWSScanner) convertInstanceToResource(instance ec2types.Instance, region string) *models.Resource {
	name := aws.ToString(instance.InstanceId)
	if tags := s.extractEC2Tags(instance.Tags); tags["Name"] != "" {
		name = tags["Name"]
	}

	sgIDs := make([]string, len(instance.SecurityGroups))
	for i, sg := range instance.SecurityGroups {
		sgIDs[i] = aws.ToString(sg.GroupId)
	}

	return &models.Resource{
		ID:       fmt.Sprintf("instance-%s-%s", region, aws.ToString(instance.InstanceId)),
		Name:     name,
		Type:     models.ResourceTypeEC2Instance,
		Provider: "aws",
		Region:   region,
		State:    s.convertInstanceState(instance.State),
		Properties: map[string]interface{}{
			"instance_id":        aws.ToString(instance.InstanceId),
			"instance_type":      string(instance.InstanceType),
			"image_id":           aws.ToString(instance.ImageId),
			"vpc_id":             aws.ToString(instance.VpcId),
			"subnet_id":          aws.ToString(instance.SubnetId),
			"private_ip_address": aws.ToString(instance.PrivateIpAddress),
			"public_ip_address":  aws.ToString(instance.PublicIpAddress),
			"availability_zone":  aws.ToString(instance.Placement.AvailabilityZone),
			"security_groups":    sgIDs,
			"state":              string(instance.State.Name),
		},
		Tags:   s.extractEC2Tags(instance.Tags),
		Source: "aws",
	}
}

func (s *AWSScanner) convertS3BucketToResource(bucket s3types.Bucket, region string) *models.Resource {
	return &models.Resource{
		ID:       fmt.Sprintf("s3-%s", aws.ToString(bucket.Name)),
		Name:     aws.ToString(bucket.Name),
		Type:     models.ResourceTypeS3Bucket,
		Provider: "aws",
		Region:   region,
		State:    models.ResourceStateActive,
		Properties: map[string]interface{}{
			"bucket_name":   aws.ToString(bucket.Name),
			"creation_date": bucket.CreationDate,
		},
		Tags:   make(map[string]string),
		Source: "aws",
	}
}

func (s *AWSScanner) convertRDSInstanceToResource(instance rdstypes.DBInstance, region string) *models.Resource {
	props := map[string]interface{}{
		"db_instance_identifier": aws.ToString(instance.DBInstanceIdentifier),
		"db_instance_class":      aws.ToString(instance.DBInstanceClass),
		"engine":                 aws.ToString(instance.Engine),
		"engine_version":         aws.ToString(instance.EngineVersion),
		"allocated_storage":      aws.ToInt32(instance.AllocatedStorage),
		"db_name":                aws.ToString(instance.DBName),
		"master_username":        aws.ToString(instance.MasterUsername),
		"availability_zone":      aws.ToString(instance.AvailabilityZone),
	}

	// Guard against nil Endpoint (e.g. instance still creating).
	if instance.Endpoint != nil {
		props["endpoint"] = aws.ToString(instance.Endpoint.Address)
		props["port"] = aws.ToInt32(instance.Endpoint.Port)
	}

	// Guard against nil DBSubnetGroup.
	if instance.DBSubnetGroup != nil {
		props["db_subnet_group_name"] = aws.ToString(instance.DBSubnetGroup.DBSubnetGroupName)
	}

	return &models.Resource{
		ID:         fmt.Sprintf("rds-%s-%s", region, aws.ToString(instance.DBInstanceIdentifier)),
		Name:       aws.ToString(instance.DBInstanceIdentifier),
		Type:       models.ResourceTypeRDSInstance,
		Provider:   "aws",
		Region:     region,
		State:      s.convertRDSState(aws.ToString(instance.DBInstanceStatus)),
		Properties: props,
		Tags:       make(map[string]string),
		Source:     "aws",
	}
}

func (s *AWSScanner) convertLambdaFunctionToResource(function lambdatypes.FunctionConfiguration, region string) *models.Resource {
	return &models.Resource{
		ID:       fmt.Sprintf("lambda-%s-%s", region, aws.ToString(function.FunctionName)),
		Name:     aws.ToString(function.FunctionName),
		Type:     models.ResourceTypeLambdaFunction,
		Provider: "aws",
		Region:   region,
		State:    s.convertLambdaState(string(function.State)),
		Properties: map[string]interface{}{
			"function_name": aws.ToString(function.FunctionName),
			"function_arn":  aws.ToString(function.FunctionArn),
			"runtime":       string(function.Runtime),
			"handler":       aws.ToString(function.Handler),
			"code_size":     function.CodeSize,
			"description":   aws.ToString(function.Description),
			"timeout":       aws.ToInt32(function.Timeout),
			"memory_size":   aws.ToInt32(function.MemorySize),
			"last_modified": aws.ToString(function.LastModified),
		},
		Tags:   make(map[string]string),
		Source: "aws",
	}
}

// --- State conversion helpers ---

func (s *AWSScanner) convertVPCState(state ec2types.VpcState) models.ResourceState {
	switch state {
	case ec2types.VpcStateAvailable:
		return models.ResourceStateActive
	case ec2types.VpcStatePending:
		return models.ResourceStatePending
	default:
		return models.ResourceStateUnknown
	}
}

func (s *AWSScanner) convertSubnetState(state ec2types.SubnetState) models.ResourceState {
	switch state {
	case ec2types.SubnetStateAvailable:
		return models.ResourceStateActive
	case ec2types.SubnetStatePending:
		return models.ResourceStatePending
	default:
		return models.ResourceStateUnknown
	}
}

func (s *AWSScanner) convertInstanceState(state *ec2types.InstanceState) models.ResourceState {
	switch state.Name {
	case ec2types.InstanceStateNameRunning:
		return models.ResourceStateActive
	case ec2types.InstanceStateNameStopped, ec2types.InstanceStateNameStopping:
		return models.ResourceStateInactive
	case ec2types.InstanceStateNamePending:
		return models.ResourceStatePending
	case ec2types.InstanceStateNameTerminated:
		return models.ResourceStateTerminated
	default:
		return models.ResourceStateUnknown
	}
}

func (s *AWSScanner) convertRDSState(state string) models.ResourceState {
	switch strings.ToLower(state) {
	case "available":
		return models.ResourceStateActive
	case "stopped", "stopping":
		return models.ResourceStateInactive
	case "creating", "starting":
		return models.ResourceStatePending
	case "failed":
		return models.ResourceStateFailed
	default:
		return models.ResourceStateUnknown
	}
}

func (s *AWSScanner) convertLambdaState(state string) models.ResourceState {
	switch strings.ToLower(state) {
	case "active":
		return models.ResourceStateActive
	case "pending":
		return models.ResourceStatePending
	case "inactive":
		return models.ResourceStateInactive
	case "failed":
		return models.ResourceStateFailed
	default:
		return models.ResourceStateUnknown
	}
}

func (s *AWSScanner) extractEC2Tags(ec2Tags []ec2types.Tag) map[string]string {
	tags := make(map[string]string, len(ec2Tags))
	for _, tag := range ec2Tags {
		tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	return tags
}

// --- Connection analysis ---

// createConnections analyzes AWS resources and creates connections between them.
func (s *AWSScanner) createConnections(diagram *models.Diagram, builder *models.DiagramBuilder) {
	rm := make(map[string]*models.Resource, len(diagram.Resources))
	for i := range diagram.Resources {
		r := &diagram.Resources[i]
		rm[r.ID] = r
	}

	for i := range diagram.Resources {
		r := &diagram.Resources[i]
		s.analyzeAWSResourceConnections(r, rm, builder)
	}
}

func (s *AWSScanner) analyzeAWSResourceConnections(resource *models.Resource, rm map[string]*models.Resource, builder *models.DiagramBuilder) {
	switch resource.Type {
	case models.ResourceTypeEC2Instance:
		s.analyzeEC2InstanceConnections(resource, rm, builder)
	case models.ResourceTypeSubnet:
		s.analyzeSubnetConnections(resource, rm, builder)
	}
}

func (s *AWSScanner) analyzeEC2InstanceConnections(resource *models.Resource, rm map[string]*models.Resource, builder *models.DiagramBuilder) {
	if vpcID, ok := resource.Properties["vpc_id"].(string); ok && vpcID != "" {
		vpcResourceID := fmt.Sprintf("vpc-%s-%s", resource.Region, vpcID)
		if _, exists := rm[vpcResourceID]; exists {
			builder.AddConnection(resource.ID, vpcResourceID, models.ConnectionTypeNetworking, "EC2 instance in VPC")
		}
	}

	if subnetID, ok := resource.Properties["subnet_id"].(string); ok && subnetID != "" {
		subnetResourceID := fmt.Sprintf("subnet-%s-%s", resource.Region, subnetID)
		if _, exists := rm[subnetResourceID]; exists {
			builder.AddConnection(resource.ID, subnetResourceID, models.ConnectionTypeNetworking, "EC2 instance in subnet")
		}
	}

	if sgIDs, ok := resource.Properties["security_groups"].([]string); ok {
		for _, sgID := range sgIDs {
			sgResourceID := fmt.Sprintf("sg-%s-%s", resource.Region, sgID)
			if _, exists := rm[sgResourceID]; exists {
				builder.AddConnection(resource.ID, sgResourceID, models.ConnectionTypeAccess, "EC2 instance uses security group")
			}
		}
	}
}

func (s *AWSScanner) analyzeSubnetConnections(resource *models.Resource, rm map[string]*models.Resource, builder *models.DiagramBuilder) {
	if vpcID, ok := resource.Properties["vpc_id"].(string); ok && vpcID != "" {
		vpcResourceID := fmt.Sprintf("vpc-%s-%s", resource.Region, vpcID)
		if _, exists := rm[vpcResourceID]; exists {
			builder.AddConnection(resource.ID, vpcResourceID, models.ConnectionTypeNetworking, "Subnet in VPC")
		}
	}
}
