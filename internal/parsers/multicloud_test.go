package parsers

import (
	"encoding/json"
	"testing"

	"terraviz/internal/models"
)

func cloudResource(resType, name, provider string) map[string]interface{} {
	return map[string]interface{}{
		"mode":     "managed",
		"type":     resType,
		"name":     name,
		"provider": provider,
		"instances": []interface{}{
			map[string]interface{}{"attributes": map[string]interface{}{"id": name}},
		},
	}
}

func TestMultiCloudStateParsing(t *testing.T) {
	data, err := json.Marshal(map[string]interface{}{
		"version": 4,
		"resources": []interface{}{
			cloudResource("google_compute_instance", "web", `provider["registry.terraform.io/hashicorp/google"]`),
			cloudResource("google_storage_bucket", "assets", `provider["registry.terraform.io/hashicorp/google"]`),
			cloudResource("google_cloud_run_service", "api", `provider["registry.terraform.io/hashicorp/google"]`),
			cloudResource("azurerm_virtual_machine", "vm", `provider["registry.terraform.io/hashicorp/azurerm"]`),
			cloudResource("azurerm_mystery_service", "extra", `provider["registry.terraform.io/hashicorp/azurerm"]`),
			cloudResource("null_resource", "noop", `provider["registry.terraform.io/hashicorp/null"]`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewTerraformParser().ParseStateData(data, "multi.tfstate")
	if err != nil {
		t.Fatal(err)
	}

	byID := map[string]models.Resource{}
	for _, r := range result.Diagram.Resources {
		byID[r.ID] = r
	}
	if len(byID) != 5 {
		t.Fatalf("expected 5 kept resources, got %d: %#v", len(byID), result.Diagram.Resources)
	}

	cases := map[string]struct {
		kind     models.ResourceType
		provider string
	}{
		"google_compute_instance.web":   {models.ResourceTypeGCPInstance, "gcp"},
		"google_storage_bucket.assets":  {models.ResourceTypeGCPStorageBucket, "gcp"},
		"google_cloud_run_service.api":  {models.ResourceTypeUnknown, "gcp"},
		"azurerm_virtual_machine.vm":    {models.ResourceTypeAzureVM, "azure"},
		"azurerm_mystery_service.extra": {models.ResourceTypeUnknown, "azure"},
	}
	for id, want := range cases {
		r, ok := byID[id]
		if !ok {
			t.Errorf("missing %s", id)
			continue
		}
		if r.Type != want.kind || r.Provider != want.provider {
			t.Errorf("%s = (%s, %s), want (%s, %s)", id, r.Type, r.Provider, want.kind, want.provider)
		}
	}
	if _, ok := byID["null_resource.noop"]; ok {
		t.Error("unsupported provider should still be dropped")
	}
}
