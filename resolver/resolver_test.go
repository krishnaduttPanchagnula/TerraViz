package resolver

import (
	"testing"

	"terraviz/state"
)

func load(t *testing.T) []Source {
	t.Helper()
	var out []Source
	for _, n := range []string{"network", "app"} {
		f, err := state.Load("../testdata/" + n + ".tfstate")
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, Source{Name: n, File: f})
	}
	return out
}

func find(edges []Edge, from, to, matched string) (Edge, bool) {
	for _, e := range edges {
		if e.From == from && e.To == to && e.Matched == matched {
			return e, true
		}
	}
	return Edge{}, false
}

func TestAppDependsOnNetwork(t *testing.T) {
	edges := Resolve(load(t))
	for _, m := range []string{
		"arn:aws:sqs:us-east-1:111122223333:orders",               // exact ARN
		"internal-1234567890.us-east-1.elb.amazonaws.com",         // host inside a JSON string, with :port
		"abc123def4.execute-api.us-east-1.amazonaws.com",          // derived API Gateway host
		"api.internal.example.com",                                // Route53 name in nested env block
		"https://sqs.us-east-1.amazonaws.com/111122223333/orders", // SQS URL (generic host, full-URL match)
	} {
		e, ok := find(edges, "app", "network", m)
		if !ok {
			t.Errorf("missing edge for %s", m)
		} else if e.Confidence < 0.8 {
			t.Errorf("%s: confidence %.2f too low", m, e.Confidence)
		}
	}
	if _, ok := find(edges, "app", "network", "prod/network/terraform.tfstate"); !ok {
		t.Error("missing terraform_remote_state edge")
	}
}

func TestNoReverseEdges(t *testing.T) {
	for _, e := range Resolve(load(t)) {
		if e.From == "network" {
			t.Errorf("unexpected edge network -> %s (%s)", e.To, e.Matched)
		}
	}
}

func TestNameMatchIsLowConfidence(t *testing.T) {
	e, ok := find(Resolve(load(t)), "app", "network", "orders")
	if !ok {
		t.Fatal("expected a name-level edge")
	}
	if e.Kind != "name" || e.Confidence >= 0.5 {
		t.Errorf("name edge should be low confidence, got %s %.2f", e.Kind, e.Confidence)
	}
}

func TestSummarizeFiltersWeakEdges(t *testing.T) {
	deps := Summarize(Resolve(load(t)), 0.5)
	if len(deps) != 1 || deps[0].From != "app" || deps[0].To != "network" {
		t.Fatalf("unexpected deps: %+v", deps)
	}
	for _, k := range deps[0].Kinds {
		if k == "name" {
			t.Error("name edges should be filtered at min=0.5")
		}
	}
}

func TestCloudProviderHostAttributes(t *testing.T) {
	raw := `{"version":4,"serial":1,"lineage":"x","outputs":{},"resources":[
	  {"mode":"managed","type":"azurerm_linux_web_app","name":"portal","instances":[{"attributes":{"default_hostname":"acme-portal.azurewebsites.net"}}]},
	  {"mode":"managed","type":"azurerm_key_vault","name":"kv","instances":[{"attributes":{"vault_uri":"https://acme-kv.vault.azure.net/"}}]},
	  {"mode":"managed","type":"google_cloud_run_service","name":"api","instances":[{"attributes":{"url":"https://analytics-abc.a.run.app"}}]}
	]}`
	owner, err := state.Parse([]byte(raw), "owner")
	if err != nil {
		t.Fatal(err)
	}
	consumerRaw := `{"version":4,"serial":1,"lineage":"y","outputs":{},"resources":[
	  {"mode":"managed","type":"aws_lambda_function","name":"f","instances":[{"attributes":{"urls":["acme-portal.azurewebsites.net","https://acme-kv.vault.azure.net/","https://analytics-abc.a.run.app/x"]}}]}
	]}`
	consumer, err := state.Parse([]byte(consumerRaw), "consumer")
	if err != nil {
		t.Fatal(err)
	}
	edges := Resolve([]Source{{Name: "owner", File: owner}, {Name: "consumer", File: consumer}})
	for _, host := range []string{
		"acme-portal.azurewebsites.net",
		"acme-kv.vault.azure.net",
		"analytics-abc.a.run.app",
	} {
		found := false
		for _, e := range edges {
			if e.Matched == host {
				found = true
			}
		}
		if !found {
			t.Errorf("missing host edge for %s; got %+v", host, edges)
		}
	}
}

func TestTokenBoundaries(t *testing.T) {
	cases := []struct {
		s, sub string
		want   bool
	}{
		{"https://api.example.com/v1", "api.example.com", true},
		{"https://myapi.example.com", "api.example.com", false},
		{"https://x.api.example.com", "api.example.com", false},
		{"arn:aws:sqs:us-east-1:1:q-dlq", "arn:aws:sqs:us-east-1:1:q", false},
		{"arn:aws:sqs:us-east-1:1:q/*", "arn:aws:sqs:us-east-1:1:q", true},
	}
	for _, c := range cases {
		if got := containsToken(c.s, c.sub); got != c.want {
			t.Errorf("containsToken(%q,%q)=%v want %v", c.s, c.sub, got, c.want)
		}
	}
}
