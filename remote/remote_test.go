package remote

import "testing"

func TestNormalizeAndStatus(t *testing.T) {
	for in, want := range map[string]string{
		"": "aws", "S3": "aws", "gcs": "gcp", "azurerm": "azure", "oracle": "oci",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
	for _, p := range Providers() {
		s := ConfigStatus(p)
		if s["provider"] != p {
			t.Errorf("ConfigStatus(%s) provider = %v", p, s["provider"])
		}
		if _, ok := s["configured"]; !ok {
			t.Errorf("ConfigStatus(%s) missing configured flag", p)
		}
	}
}

func TestStateName(t *testing.T) {
	cases := map[string]string{
		"terraform/prod.tfstate":        "prod",
		"a/b/terraform.tfstate":         "b",
		"path/terraform.tfstate":        "path",
		"checkout/stack/module.tfstate": "module",
	}
	for in, want := range cases {
		if got := StateName(in); got != want {
			t.Errorf("StateName(%q) = %q, want %q", in, got, want)
		}
	}
}
