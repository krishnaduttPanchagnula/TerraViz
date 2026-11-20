package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"terraviz/s3source"
)

type mockS3 struct {
	buckets []s3source.Bucket
	objects map[string]s3source.ListResult
	data    map[string][]byte
	err     error
}

func (m *mockS3) Buckets(context.Context) ([]s3source.Bucket, error) { return m.buckets, m.err }
func (m *mockS3) List(_ context.Context, bucket, prefix, token string) (s3source.ListResult, error) {
	if m.err != nil {
		return s3source.ListResult{}, m.err
	}
	return m.objects[bucket+"|"+prefix+"|"+token], nil
}
func (m *mockS3) Get(_ context.Context, bucket, key string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	b, ok := m.data[key]
	if !ok {
		return nil, errors.New("no such key: " + key)
	}
	return b, nil
}

func newMockS3() *mockS3 {
	content, err := os.ReadFile("../testdata/network.tfstate")
	if err != nil {
		panic(err)
	}
	return &mockS3{
		buckets: []s3source.Bucket{{Name: "zeta"}, {Name: "alpha"}},
		objects: map[string]s3source.ListResult{
			"alpha||": {Objects: []s3source.Object{
				{Key: "terraform/", IsDir: true},
				{Key: "readme.txt", Size: 12},
			}},
		},
		data: map[string][]byte{
			"terraform/network.tfstate": content,
		},
	}
}

func TestS3StatusNotConfigured(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("AWS_ENDPOINT_URL", "")
	t.Setenv("AWS_ENDPOINT_URL_S3", "")
	s := newServer(t)
	w := do(t, s.Handler(), http.MethodGet, "/api/s3/status", nil, "")
	if w.Code != 200 {
		t.Fatalf("status: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"configured":false`) {
		t.Fatalf("status: %s", w.Body.String())
	}
	w = do(t, s.Handler(), http.MethodGet, "/api/s3/buckets", nil, "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("buckets without creds: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "credentials") {
		t.Fatalf("unhelpful error: %s", w.Body.String())
	}
}

func TestS3EndpointsWithMock(t *testing.T) {
	s := newServer(t)
	s.S3 = newMockS3()
	h := s.Handler()

	w := do(t, h, http.MethodGet, "/api/s3/buckets", nil, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"alpha"`) {
		t.Fatalf("buckets: %d %s", w.Code, w.Body.String())
	}
	// Sorted by name.
	if strings.Index(w.Body.String(), "alpha") > strings.Index(w.Body.String(), "zeta") {
		t.Fatal("buckets not sorted")
	}

	w = do(t, h, http.MethodGet, "/api/s3/objects?bucket=alpha", nil, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"is_dir":true`) {
		t.Fatalf("objects: %d %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodGet, "/api/s3/objects", nil, "")
	if w.Code != 400 {
		t.Fatalf("objects without bucket: %d", w.Code)
	}

	body, _ := json.Marshal(map[string]any{
		"bucket": "alpha", "keys": []string{"terraform/network.tfstate", "terraform/"},
	})
	w = do(t, h, http.MethodPost, "/api/s3/ingest", body, "application/json")
	if w.Code != 200 {
		t.Fatalf("s3 ingest: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Ingested []string `json:"ingested"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Ingested) != 1 || resp.Ingested[0] != "network" {
		t.Fatalf("unexpected ingested names: %+v", resp)
	}

	// Directory keys are skipped, and an empty selection is rejected.
	body, _ = json.Marshal(map[string]any{"bucket": "alpha", "keys": []string{"terraform/"}})
	if w = do(t, h, http.MethodPost, "/api/s3/ingest", body, "application/json"); w.Code != 400 {
		t.Fatalf("empty s3 ingest: %d", w.Code)
	}
}

func TestSamplesEndpoints(t *testing.T) {
	s := newServer(t)
	s.SamplesDir = "../samples"
	h := s.Handler()

	w := do(t, h, http.MethodGet, "/api/samples", nil, "")
	if w.Code != 200 {
		t.Fatalf("samples: %d %s", w.Code, w.Body.String())
	}
	var list struct {
		Samples []struct {
			Name, Provider string
		} `json:"samples"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Samples) != 7 {
		t.Fatalf("expected 7 samples, got %+v", list.Samples)
	}
	providers := map[string]bool{}
	for _, x := range list.Samples {
		providers[x.Provider] = true
	}
	for _, want := range []string{"aws", "gcp", "azure"} {
		if !providers[want] {
			t.Fatalf("missing provider %s in samples: %+v", want, providers)
		}
	}

	// Select a subset that has a cross-cloud link (checkout-app -> gcp/azure).
	body, _ := json.Marshal(map[string]any{"samples": []string{"gcp-analytics", "checkout-app"}})
	w = do(t, h, http.MethodPost, "/api/samples/ingest", body, "application/json")
	if w.Code != 200 {
		t.Fatalf("samples ingest: %d %s", w.Code, w.Body.String())
	}
	deps := do(t, h, http.MethodGet, "/api/deps", nil, "")
	if !strings.Contains(deps.Body.String(), `"from":"checkout-app","to":"gcp-analytics"`) {
		t.Fatalf("missing cross-cloud dependency: %s", deps.Body.String())
	}

	// Ingest all, then check state providers are exposed for the UI icons.
	body, _ = json.Marshal(map[string]any{"samples": []string{}})
	w = do(t, h, http.MethodPost, "/api/samples/ingest", body, "application/json")
	if w.Code != 200 {
		t.Fatalf("ingest all samples: %s", w.Body.String())
	}
	states := do(t, h, http.MethodGet, "/api/states", nil, "")
	for _, p := range []string{`"provider":"aws"`, `"provider":"gcp"`, `"provider":"azure"`} {
		if !strings.Contains(states.Body.String(), p) {
			t.Fatalf("states missing %s: %s", p, states.Body.String())
		}
	}
}

func TestSampleStateName(t *testing.T) {
	cases := map[string]string{
		"terraform/network.tfstate":        "network",
		"a/b/terraform.tfstate":            "b",
		"x/my.state.json":                  "my.state",
		"bucket/path/to/terraform.tfstate": "to",
	}
	for in, want := range cases {
		if got := s3source.StateName(in); got != want {
			t.Errorf("StateName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSampleFilesMissingDir(t *testing.T) {
	s := newServer(t)
	s.SamplesDir = filepath.Join(t.TempDir(), "missing")
	if _, err := s.sampleFiles(); err == nil {
		t.Fatal("expected error for missing samples dir")
	}
}
