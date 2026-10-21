package scanners

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"terraviz/internal/models"
	"terraviz/internal/relations"
)

// scannedResources builds the resources of a small live scan.
func scannedResources(t *testing.T) []models.Resource {
	t.Helper()

	scanner := &AWSScanner{}

	vpc := scanner.convertVPCToResource(ec2types.Vpc{VpcId: aws.String("vpc-1")}, "us-east-1", nil)
	subnet := scanner.convertSubnetToResource(ec2types.Subnet{
		SubnetId: aws.String("subnet-1"),
		VpcId:    aws.String("vpc-1"),
	}, "us-east-1", nil)
	securityGroup := scanner.convertSecurityGroupToResource(ec2types.SecurityGroup{
		GroupId:   aws.String("sg-1"),
		GroupName: aws.String("web"),
		VpcId:     aws.String("vpc-1"),
	}, "us-east-1", nil)
	instance := scanner.convertInstanceToResource(ec2types.Instance{
		InstanceId: aws.String("i-1"),
		VpcId:      aws.String("vpc-1"),
		SubnetId:   aws.String("subnet-1"),
		SecurityGroups: []ec2types.GroupIdentifier{
			{GroupId: aws.String("sg-1")},
		},
		Placement: &ec2types.Placement{AvailabilityZone: aws.String("us-east-1a")},
		State:     &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
	}, "us-east-1", nil)

	return []models.Resource{*vpc, *subnet, *securityGroup, *instance}
}

func TestScannedResourcesExposeTheirAWSID(t *testing.T) {
	for _, resource := range scannedResources(t) {
		if id, ok := resource.Properties["id"].(string); !ok || id == "" {
			t.Errorf("resource %s has no id property: %#v", resource.ID, resource.Properties)
		}
	}
}

func TestScannedResourcesProduceGenericEdges(t *testing.T) {
	edges := relations.AnalyzeGeneric(scannedResources(t))

	pairs := make(map[string]relations.Edge, len(edges))
	for _, edge := range edges {
		pairs[edge.Source+"->"+edge.Target] = edge
	}

	cases := []struct {
		source      string
		target      string
		edgeType    models.ConnectionType
		description string
	}{
		{"subnet-us-east-1-subnet-1", "vpc-us-east-1-vpc-1", models.ConnectionTypeNetworking, "Subnet belongs to VPC"},
		{"sg-us-east-1-sg-1", "vpc-us-east-1-vpc-1", models.ConnectionTypeNetworking, "Security group belongs to VPC"},
		{"instance-us-east-1-i-1", "vpc-us-east-1-vpc-1", models.ConnectionTypeNetworking, "EC2 instance in VPC"},
		{"instance-us-east-1-i-1", "subnet-us-east-1-subnet-1", models.ConnectionTypeNetworking, "EC2 instance in subnet"},
		{"instance-us-east-1-i-1", "sg-us-east-1-sg-1", models.ConnectionTypeAccess, "EC2 instance uses security group"},
	}

	for _, tc := range cases {
		edge, ok := pairs[tc.source+"->"+tc.target]
		if !ok {
			t.Fatalf("missing edge %s -> %s: %#v", tc.source, tc.target, edges)
		}
		if edge.Type != tc.edgeType {
			t.Errorf("edge %s -> %s type = %q, want %q", tc.source, tc.target, edge.Type, tc.edgeType)
		}
		if edge.Description != tc.description {
			t.Errorf("edge %s -> %s description = %q, want %q", tc.source, tc.target, edge.Description, tc.description)
		}
	}
}

func TestScannedSecurityGroupRulesProduceEdges(t *testing.T) {
	scanner := &AWSScanner{}

	source := scanner.convertSecurityGroupToResource(ec2types.SecurityGroup{
		GroupId:   aws.String("sg-1"),
		GroupName: aws.String("web"),
		VpcId:     aws.String("vpc-1"),
		IpPermissions: []ec2types.IpPermission{
			{
				IpProtocol: aws.String("tcp"),
				FromPort:   aws.Int32(443),
				ToPort:     aws.Int32(443),
				IpRanges: []ec2types.IpRange{
					{CidrIp: aws.String("10.0.0.0/16")},
				},
				UserIdGroupPairs: []ec2types.UserIdGroupPair{
					{GroupId: aws.String("sg-2")},
				},
			},
		},
	}, "us-east-1", nil)

	target := scanner.convertSecurityGroupToResource(ec2types.SecurityGroup{
		GroupId:   aws.String("sg-2"),
		GroupName: aws.String("db"),
		VpcId:     aws.String("vpc-1"),
	}, "us-east-1", nil)

	edges := relations.AnalyzeGeneric([]models.Resource{*source, *target})

	var found *relations.Edge
	for i := range edges {
		if edges[i].Source == source.ID && edges[i].Target == target.ID {
			found = &edges[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("missing security group rule edge: %#v", edges)
	}
	if found.Properties["protocol"] != "tcp" {
		t.Errorf("protocol = %v, want tcp", found.Properties["protocol"])
	}
	if found.Properties["from_port"] != int64(443) {
		t.Errorf("from_port = %v, want 443", found.Properties["from_port"])
	}
	if found.Properties["to_port"] != int64(443) {
		t.Errorf("to_port = %v, want 443", found.Properties["to_port"])
	}
}
