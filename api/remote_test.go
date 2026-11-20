package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"terraviz/remote"
)

type mockRemote struct {
	buckets []remote.Bucket
	objects map[string]remote.ListResult
	data    map[string][]byte
}

func (m *mockRemote) Buckets(context.Context) ([]remote.Bucket, error) { return m.buckets, nil }
func (m *mockRemote) List(_ context.Context, _, prefix, _ string) (remote.ListResult, error) {
	return m.objects[prefix], nil
}
func (m *mockRemote) Get(_ context.Context, _, key string) ([]byte, error) { return m.data[key], nil }

func newMockRemote() *mockRemote {
	b, err := os.ReadFile("../testdata/network.tfstate")
	if err != nil {
		panic(err)
	}
	return &mockRemote{
		buckets: []remote.Bucket{{Name: "zeta"}, {Name: "alpha"}},
		objects: map[string]remote.ListResult{
			"": {Objects: []remote.Object{{Key: "terraform/", IsDir: true}, {Key: "a.tfstate", Size: 12}}},
		},
		data: map[string][]byte{"terraform/network.tfstate": b},
	}
}

func TestRemoteProvidersEndToEnd(t *testing.T) {
	s := newServer(t)
	for _, provider := range []string{"aws", "gcp", "azure", "oci"} {
		s.Remote = map[string]remote.Client{provider: newMockRemote()}
		h := s.Handler()

		status := do(t, h, http.MethodGet, "/api/remote/status?provider="+provider, nil, "")
		if status.Code != 200 || !strings.Contains(status.Body.String(), `"configured":true`) {
			t.Fatalf("%s status: %d %s", provider, status.Code, status.Body.String())
		}

		buckets := do(t, h, http.MethodGet, "/api/remote/buckets?provider="+provider, nil, "")
		if buckets.Code != 200 || !strings.Contains(buckets.Body.String(), `"name":"alpha"`) {
			t.Fatalf("%s buckets: %d %s", provider, buckets.Code, buckets.Body.String())
		}

		objects := do(t, h, http.MethodGet, "/api/remote/objects?provider="+provider+"&bucket=alpha", nil, "")
		if objects.Code != 200 || !strings.Contains(objects.Body.String(), `"is_dir":true`) {
			t.Fatalf("%s objects: %d %s", provider, objects.Code, objects.Body.String())
		}

		body, _ := json.Marshal(map[string]any{
			"provider": provider, "bucket": "alpha",
			"keys": []string{"terraform/network.tfstate"},
		})
		ingest := do(t, h, http.MethodPost, "/api/remote/ingest", body, "application/json")
		if ingest.Code != 200 {
			t.Fatalf("%s ingest: %d %s", provider, ingest.Code, ingest.Body.String())
		}
	}
	states := do(t, s.Handler(), http.MethodGet, "/api/states", nil, "")
	if !strings.Contains(states.Body.String(), `"name":"network"`) {
		t.Fatalf("ingested state missing: %s", states.Body.String())
	}
}

func TestRemoteNormalize(t *testing.T) {
	cases := map[string]string{
		"": "aws", "s3": "aws", "GCS": "gcp", "google": "gcp",
		"azurerm": "azure", "AZURE": "azure", "oracle": "oci", "obj": "obj",
	}
	for in, want := range cases {
		if got := remote.Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
