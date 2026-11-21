package costs

import (
	"strings"
	"testing"

	"terraviz/internal/models"
)

func TestEstimate(t *testing.T) {
	d := &models.Diagram{Resources: []models.Resource{
		{ID: "a", Name: "web", Type: models.ResourceTypeEC2Instance},
		{ID: "b", Name: "data", Type: models.ResourceTypeDynamoDBTable},
		{ID: "c", Name: "mystery", Type: models.ResourceTypeUnknown},
	}}
	report := Estimate(d)
	if report.TotalMonthlyUSD != 18.5 {
		t.Errorf("total = %v, want 18.5", report.TotalMonthlyUSD)
	}
	if report.Unpriced != 1 {
		t.Errorf("unpriced = %d, want 1", report.Unpriced)
	}
	if report.Resources[2].MonthlyUSD != nil {
		t.Error("unknown type must not be priced at zero")
	}
	if !strings.Contains(report.Disclaimer, "Not a bill") {
		t.Error("missing disclaimer")
	}
}

func TestEstimateNil(t *testing.T) {
	if Estimate(nil).TotalMonthlyUSD != 0 {
		t.Error("nil diagram should cost nothing")
	}
}
