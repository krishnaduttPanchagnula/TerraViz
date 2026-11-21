// Package costs attaches a rough, offline monthly estimate to a diagram.
// Prices are illustrative on-demand us-east-1 figures baked into the binary:
// no cloud API call, account or network access is required. The report says so
// explicitly so nobody mistakes it for a bill.
package costs

import (
	"sort"

	"terraviz/internal/models"
)

// ResourceCost is one resource's estimate. MonthlyUSD is nil when the type is
// unpriced, so "unknown" is never shown as zero.
type ResourceCost struct {
	ResourceID string   `json:"resource_id"`
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	MonthlyUSD *float64 `json:"monthly_usd,omitempty"`
	Basis      string   `json:"basis,omitempty"`
}

// Report is the complete overlay.
type Report struct {
	Currency        string         `json:"currency"`
	Basis           string         `json:"basis"`
	Disclaimer      string         `json:"disclaimer"`
	Resources       []ResourceCost `json:"resources"`
	TotalMonthlyUSD float64        `json:"total_monthly_usd"`
	Unpriced        int            `json:"unpriced"`
}

const disclaimer = "Illustrative on-demand us-east-1 estimates for a typical small deployment. " +
	"Not a bill: actual cost depends on size, region, usage and discounts. " +
	"Verify with your cloud provider's pricing calculator."

// monthlyUSD is a deliberately coarse price table. The values are monthly
// (730 hours) and round; the overlay is a planning aid, not accounting.
var monthlyUSD = map[models.ResourceType]float64{
	models.ResourceTypeEC2Instance:      8.5,
	models.ResourceTypeS3Bucket:         2.3,
	models.ResourceTypeRDSInstance:      30,
	models.ResourceTypeLambdaFunction:   1,
	models.ResourceTypeLoadBalancer:     22,
	models.ResourceTypeLBListener:       0,
	models.ResourceTypeLBTargetGroup:    0,
	models.ResourceTypeVPC:              0,
	models.ResourceTypeSubnet:           0,
	models.ResourceTypeSecurityGroup:    0,
	models.ResourceTypeTransitGateway:   73,
	models.ResourceTypeRoute53Zone:      0.5,
	models.ResourceTypeRoute53Record:    0,
	models.ResourceTypeCloudFront:       5,
	models.ResourceTypeAPIGateway:       3.5,
	models.ResourceTypeSNSTopic:         0.5,
	models.ResourceTypeSQSQueue:         0.5,
	models.ResourceTypeDynamoDBTable:    10,
	models.ResourceTypeElastiCache:      15,
	models.ResourceTypeRedshift:         180,
	models.ResourceTypeKinesisStream:    15,
	models.ResourceTypeOpenSearch:       35,
	models.ResourceTypeAthenaWorkgroup:  5,
	models.ResourceTypeEFSFileSystem:    5,
	models.ResourceTypeEKSCluster:       73,
	models.ResourceTypeEKSNodeGroup:     30,
	models.ResourceTypeECSTask:          10,
	models.ResourceTypeAppRunnerService: 10,
	models.ResourceTypeEventBridgeRule:  1,
	models.ResourceTypeStepFunction:     5,
	models.ResourceTypeSecretsManager:   0.4,
	models.ResourceTypeKMSKey:           1,
	models.ResourceTypeCloudWatch:       2,
	models.ResourceTypeWAFWebACL:        5,
	models.ResourceTypeBackupPlan:       5,
	models.ResourceTypeCloudTrail:       2,
	models.ResourceTypeConfigRule:       2,
	models.ResourceTypeXRayGroup:        5,
}

// Estimate builds the cost overlay for a diagram.
func Estimate(d *models.Diagram) Report {
	report := Report{
		Currency:   "USD",
		Basis:      "approximate monthly (730h), on-demand us-east-1",
		Disclaimer: disclaimer,
		Resources:  []ResourceCost{},
	}
	if d == nil {
		return report
	}

	for _, r := range d.Resources {
		item := ResourceCost{ResourceID: r.ID, Name: r.Name, Type: string(r.Type)}
		if price, ok := monthlyUSD[r.Type]; ok {
			v := price
			item.MonthlyUSD = &v
			report.TotalMonthlyUSD += v
			item.Basis = "typical small deployment"
		} else {
			report.Unpriced++
		}
		report.Resources = append(report.Resources, item)
	}
	sort.Slice(report.Resources, func(i, j int) bool {
		return report.Resources[i].ResourceID < report.Resources[j].ResourceID
	})
	return report
}
