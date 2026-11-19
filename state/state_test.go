package state

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLeavesSkipsSecrets(t *testing.T) {
	v := map[string]any{
		"password": "hunter2",
		"role_arn": "arn:aws:iam::111122223333:role/x",
		"nested":   []any{map[string]any{"api_key": "k", "host": "h.example.com"}},
	}
	got := map[string]string{}
	Leaves(v, "", func(p, s string) { got[p] = s })
	if _, ok := got["password"]; ok {
		t.Fatal("password leaked")
	}
	if _, ok := got["nested[0].api_key"]; ok {
		t.Fatal("api_key leaked")
	}
	if got["role_arn"] == "" || got["nested[0].host"] == "" {
		t.Fatalf("expected arn and host leaves, got %v", got)
	}
}

func TestScopes(t *testing.T) {
	attrs := map[string]any{
		"arn":         "arn:aws:sqs:us-east-1:111122223333:orders",
		"account_id":  "444455556666",
		"id":          "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg/providers/Microsoft.Web/sites/app",
		"self_link":   "https://www.googleapis.com/compute/v1/projects/acme-prod/zones/us-central1-a/instances/i",
		"description": "arn:aws:s3:::shared-bucket has no account",
		"random":      "999999999999",
	}
	got := Scopes(attrs)
	want := map[Scope]bool{
		{"aws", "111122223333"}:                           true,
		{"aws", "444455556666"}:                           true,
		{"azure", "12345678-1234-1234-1234-123456789012"}: true,
		{"gcp", "acme-prod"}:                              true,
	}
	if len(got) != len(want) {
		t.Fatalf("Scopes = %+v, want %+v", got, want)
	}
	for _, sc := range got {
		if !want[sc] {
			t.Errorf("unexpected scope %+v", sc)
		}
	}
}

func TestSanitized(t *testing.T) {
	raw := []byte(`{"version":4,"serial":1,"lineage":"x","outputs":{
		"leak":{"value":"topsecret","sensitive":true},
		"ok":{"value":"h.example.com","sensitive":false}},
		"resources":[{"mode":"managed","type":"aws_x","name":"y","instances":[{"attributes":{
			"password":"hunter2","role_arn":"arn:aws:iam::1:role/x",
			"nested":[{"api_key":"k","host":"h.example.com"}]}}]}]}`)
	f, err := Parse(raw, "test")
	if err != nil {
		t.Fatal(err)
	}
	c := Sanitized(f)
	if _, ok := c.Outputs["leak"]; ok {
		t.Fatal("sensitive output not removed")
	}
	b, _ := json.Marshal(c)
	s := string(b)
	for _, secret := range []string{"hunter2", "topsecret", "api_key", "password"} {
		if strings.Contains(s, secret) {
			t.Fatalf("secret %q survived sanitization: %s", secret, s)
		}
	}
	if !strings.Contains(s, "arn:aws:iam::1:role/x") || !strings.Contains(s, "h.example.com") {
		t.Fatalf("non-secret values were lost: %s", s)
	}
}
