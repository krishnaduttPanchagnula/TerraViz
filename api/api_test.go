package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"terraviz/db"
)

func newServer(t *testing.T) *Server {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return New(d)
}

func do(t *testing.T, h http.Handler, method, path string, body []byte, ct string) *httptest.ResponseRecorder {
	t.Helper()
	req := newTestRequest(method, path)
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func newTestRequest(method, path string) *http.Request {
	return httptest.NewRequest(method, path, nil)
}

func stateContent(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../testdata/" + name + ".tfstate")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func ingestBoth(t *testing.T, s *Server) *httptest.ResponseRecorder {
	t.Helper()
	h := s.Handler()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, name := range []string{"network", "app"} {
		part, err := mw.CreateFormFile("states", name+".tfstate")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(stateContent(t, name))); err != nil {
			t.Fatal(err)
		}
	}
	mw.Close()
	return do(t, h, http.MethodPost, "/api/states", buf.Bytes(), mw.FormDataContentType())
}

func TestHealthAndIndex(t *testing.T) {
	s := newServer(t)
	h := s.Handler()
	w := do(t, h, http.MethodGet, "/api/health", nil, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"ok"`) {
		t.Fatalf("health: %d %s", w.Code, w.Body.String())
	}
	w = do(t, h, http.MethodGet, "/", nil, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "terraquery") {
		t.Fatalf("index: %d %s", w.Code, w.Body.String())
	}
}

func TestUploadJSONAndQuery(t *testing.T) {
	s := newServer(t)
	h := s.Handler()
	payload := map[string]any{"states": []map[string]string{
		{"name": "network", "content": stateContent(t, "network")},
		{"name": "app", "content": stateContent(t, "app")},
	}}
	b, _ := json.Marshal(payload)
	w := do(t, h, http.MethodPost, "/api/states", b, "application/json")
	if w.Code != 200 {
		t.Fatalf("ingest: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Ingested []string `json:"ingested"`
		Edges    int      `json:"edges"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Ingested) != 2 || resp.Edges == 0 {
		t.Fatalf("bad ingest result: %+v", resp)
	}

	w = do(t, h, http.MethodGet, "/api/deps?min=0.5", nil, "")
	var deps struct {
		Dependencies []struct {
			From, To string
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &deps); err != nil {
		t.Fatal(err)
	}
	if len(deps.Dependencies) != 1 || deps.Dependencies[0].From != "app" || deps.Dependencies[0].To != "network" {
		t.Fatalf("unexpected deps: %+v", deps)
	}

	w = do(t, h, http.MethodGet, "/api/graph?min=0.5", nil, "")
	var g struct {
		Nodes, Edges []any
	}
	if err := json.Unmarshal(w.Body.Bytes(), &g); err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 2 || len(g.Edges) != 1 {
		t.Fatalf("unexpected graph: %+v", g)
	}

	q, _ := json.Marshal(map[string]string{"sql": "SELECT from_state, to_state FROM dependencies"})
	w = do(t, h, http.MethodPost, "/api/query", q, "application/json")
	var res struct {
		Columns []string `json:"columns"`
		Rows    [][]any  `json:"rows"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 || len(res.Columns) != 2 {
		t.Fatalf("unexpected query result: %+v", res)
	}
}

func TestUploadMultipartAndEdgeFilter(t *testing.T) {
	s := newServer(t)
	h := s.Handler()
	w := ingestBoth(t, s)
	if w.Code != 200 {
		t.Fatalf("multipart ingest: %d %s", w.Code, w.Body.String())
	}
	w = do(t, h, http.MethodGet, "/api/edges?min=0.8&limit=2", nil, "")
	var resp struct {
		Edges []struct {
			Confidence float64 `json:"confidence"`
		} `json:"edges"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Edges) > 2 {
		t.Fatalf("limit ignored: %d edges", len(resp.Edges))
	}
	for _, e := range resp.Edges {
		if e.Confidence < 0.8 {
			t.Fatalf("min ignored: %.2f", e.Confidence)
		}
	}
}

func TestResourcesEndpoint(t *testing.T) {
	s := newServer(t)
	h := s.Handler()
	if w := ingestBoth(t, s); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w := do(t, h, http.MethodGet, "/api/resources?state=network", nil, "")
	if w.Code != 200 {
		t.Fatalf("resources: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Resources []struct {
			State, Address, Type string
		} `json:"resources"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Resources) == 0 {
		t.Fatal("no resources returned")
	}
	for _, r := range resp.Resources {
		if r.State != "network" {
			t.Fatalf("state filter ignored: %+v", r)
		}
	}
	if w = do(t, h, http.MethodGet, "/api/resources", nil, ""); !strings.Contains(w.Body.String(), `"state":"app"`) {
		t.Fatalf("unfiltered resources missing app: %s", w.Body.String())
	}
}

func TestBadRequests(t *testing.T) {
	s := newServer(t)
	h := s.Handler()
	cases := []struct {
		method, path, ct string
		body             string
		want             int
	}{
		{"POST", "/api/states", "application/json", `{"states":[]}`, 400},
		{"POST", "/api/states", "application/json", `{"states":[{"name":"x","content":"not json"}]}`, 400},
		{"POST", "/api/states", "application/json", `{"states":[{"name":"x","content":"{\"version\":4}"},{"name":"x","content":"{\"version\":4}"}]}`, 400},
		{"GET", "/api/deps?min=abc", "", "", 400},
		{"GET", "/api/edges?limit=-1", "", "", 400},
		{"POST", "/api/query", "application/json", `{"sql":"DELETE FROM edges"}`, 400},
		{"POST", "/api/query", "application/json", `{"sql":"SELECT 1; DROP TABLE states"}`, 400},
		{"GET", "/api/deps", "", "", 200},
		{"DELETE", "/api/deps", "", "", 405},
	}
	for _, c := range cases {
		w := do(t, h, c.method, c.path, []byte(c.body), c.ct)
		if w.Code != c.want {
			t.Errorf("%s %s: got %d want %d (%s)", c.method, c.path, w.Code, c.want, w.Body.String())
		}
	}
}

func TestQueryGETAndResolve(t *testing.T) {
	s := newServer(t)
	h := s.Handler()
	w := ingestBoth(t, s)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = do(t, h, http.MethodGet, "/api/query?sql="+urlQuery("SELECT COUNT(*) FROM resources"), nil, "")
	if w.Code != 200 {
		t.Fatalf("query GET: %d %s", w.Code, w.Body.String())
	}
	w = do(t, h, http.MethodPost, "/api/resolve", nil, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"edges"`) {
		t.Fatalf("resolve: %d %s", w.Code, w.Body.String())
	}
	w = do(t, h, http.MethodGet, "/api/states", nil, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"app"`) {
		t.Fatalf("states: %d %s", w.Code, w.Body.String())
	}
}

func urlQuery(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, " ", "+"), "*", "%2A")
}
