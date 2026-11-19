package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"terraviz/resolver"
	"terraviz/state"
)

func openTemp(t *testing.T) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func sources(t *testing.T) []resolver.Source {
	t.Helper()
	var out []resolver.Source
	for _, name := range []string{"network", "app"} {
		f, err := state.Load("../testdata/" + name + ".tfstate")
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, resolver.Source{
			Name: name, Filename: name + ".tfstate", File: f,
		})
	}
	return out
}

func TestIngestStoresStatesEdgesAndDeps(t *testing.T) {
	d := openTemp(t)
	if err := d.Ingest(sources(t)); err != nil {
		t.Fatal(err)
	}
	states, err := d.States()
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 || states[0].Name != "app" || states[1].Name != "network" {
		t.Fatalf("unexpected states: %+v", states)
	}
	deps, err := d.Deps(0.5)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 || deps[0].From != "app" || deps[0].To != "network" {
		t.Fatalf("unexpected deps: %+v", deps)
	}
	g, err := d.Graph(0.5)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 2 || len(g.Edges) != 1 {
		t.Fatalf("unexpected graph: %+v", g)
	}
	res, err := d.Query("SELECT COUNT(*) AS n FROM edges")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 || res.Rows[0][0].(int64) == 0 {
		t.Fatalf("expected edges to be stored, got %+v", res.Rows)
	}
}

func TestIngestIsIdempotent(t *testing.T) {
	d := openTemp(t)
	srcs := sources(t)
	for i := 0; i < 2; i++ {
		if err := d.Ingest(srcs); err != nil {
			t.Fatal(err)
		}
	}
	res, err := d.Query("SELECT COUNT(*) FROM states")
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows[0][0] != int64(2) {
		t.Fatalf("states duplicated across ingests: %v", res.Rows)
	}
	deps, err := d.Deps(0.5)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 {
		t.Fatalf("unexpected deps after re-ingest: %+v", deps)
	}
}

func TestSecretsNeverPersisted(t *testing.T) {
	d := openTemp(t)
	raw := `{"version":4,"serial":1,"lineage":"x","outputs":{"leak":{"value":"topsecret","sensitive":true}},"resources":[{"mode":"managed","type":"aws_x","name":"y","instances":[{"attributes":{"password":"hunter2","api_key":"k","arn":"arn:aws:s3:::ok","nested":[{"secret":"s","host":"h.example.com"}]}}]}]}`
	f, err := state.Parse([]byte(raw), "secret-state")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Ingest([]resolver.Source{{Name: "secret-state", File: f}}); err != nil {
		t.Fatal(err)
	}
	res, err := d.Query("SELECT raw FROM states WHERE name='secret-state'")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Rows[0][0].(string), "hunter2") ||
		strings.Contains(res.Rows[0][0].(string), "topsecret") {
		t.Fatal("secret leaked into states.raw")
	}
	res, err = d.Query("SELECT attributes FROM resources")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range res.Rows {
		a := row[0].(string)
		if strings.Contains(a, "hunter2") || strings.Contains(a, "api_key") || strings.Contains(a, "nested") && !strings.Contains(a, "host") {
			t.Fatalf("secret leaked into resources.attributes: %s", a)
		}
		if !strings.Contains(a, "arn:aws:s3:::ok") {
			t.Fatalf("ARN attribute was lost: %s", a)
		}
	}
}

func TestReadOnlyGuard(t *testing.T) {
	good := []string{
		"SELECT * FROM states",
		"  -- comment\n SELECT 1; ",
		"WITH x AS (SELECT 1) SELECT * FROM x",
		"EXPLAIN SELECT * FROM edges",
	}
	for _, q := range good {
		if err := IsReadOnly(q); err != nil {
			t.Errorf("IsReadOnly(%q) = %v, want nil", q, err)
		}
	}
	bad := []string{
		"", "DELETE FROM edges", "UPDATE states SET name='x'",
		"DROP TABLE states", "PRAGMA writable_schema=ON",
		"SELECT 1; DROP TABLE states", "/* c */ INSERT INTO states VALUES(1)",
	}
	for _, q := range bad {
		if err := IsReadOnly(q); err == nil {
			t.Errorf("IsReadOnly(%q) = nil, want error", q)
		}
	}
}

func TestReadOnlyConnectionBlocksWrites(t *testing.T) {
	d := openTemp(t)
	if err := d.Ingest(sources(t)); err != nil {
		t.Fatal(err)
	}
	// Bypass the Go-side guard to prove the DB itself refuses writes.
	if _, err := d.r.Exec("DELETE FROM edges"); err == nil {
		t.Fatal("read-only connection allowed DELETE")
	}
	res, err := d.Query("SELECT COUNT(*) FROM edges")
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows[0][0] == int64(0) {
		t.Fatal("edges were deleted")
	}
}

func TestResolveAllRestoresEdges(t *testing.T) {
	d := openTemp(t)
	if err := d.Ingest(sources(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := d.w.Exec("DELETE FROM edges"); err != nil {
		t.Fatal(err)
	}
	if err := d.ResolveAll(); err != nil {
		t.Fatal(err)
	}
	g, err := d.Graph(0.5)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Edges) != 1 {
		t.Fatalf("ResolveAll did not rebuild edges: %+v", g.Edges)
	}
}

func TestScopesPersisted(t *testing.T) {
	d := openTemp(t)
	raw := `{"version":4,"serial":1,"lineage":"x","outputs":{},"resources":[
	  {"mode":"managed","type":"aws_sqs_queue","name":"q","instances":[{"attributes":{"arn":"arn:aws:sqs:us-east-1:111122223333:q"}}]},
	  {"mode":"managed","type":"aws_iam_role","name":"cross","instances":[{"attributes":{"arn":"arn:aws:iam::444455556666:role/cross"}}]},
	  {"mode":"managed","type":"google_storage_bucket","name":"b","instances":[{"attributes":{"id":"projects/acme-prod/b"}}]}
	]}`
	f, err := state.Parse([]byte(raw), "mixed")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Ingest([]resolver.Source{{Name: "mixed", File: f}}); err != nil {
		t.Fatal(err)
	}
	states, err := d.States()
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || len(states[0].Accounts) != 3 {
		t.Fatalf("state scopes: %+v", states)
	}
	resources, err := d.Resources("mixed")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range resources {
		got[r.Address] = r.ScopeKind + ":" + r.ScopeID
	}
	if got["aws_sqs_queue.q"] != "aws:111122223333" ||
		got["google_storage_bucket.b"] != "gcp:acme-prod" {
		t.Fatalf("resource scopes: %+v", got)
	}
}

func TestResourcesList(t *testing.T) {
	d := openTemp(t)
	if err := d.Ingest(sources(t)); err != nil {
		t.Fatal(err)
	}
	all, err := d.Resources("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) < 8 {
		t.Fatalf("expected resource instances, got %d", len(all))
	}
	app, err := d.Resources("app")
	if err != nil {
		t.Fatal(err)
	}
	if len(app) == 0 || len(app) >= len(all) {
		t.Fatalf("state filter failed: app=%d all=%d", len(app), len(all))
	}
	for _, r := range app {
		if r.State != "app" || r.Address == "" || r.Type == "" {
			t.Fatalf("malformed resource: %+v", r)
		}
	}
	found := false
	for _, r := range all {
		if r.State == "network" && r.Address == "aws_sqs_queue.orders" && r.Type == "aws_sqs_queue" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing known resource: %+v", all)
	}
}

func TestEdgesAndDepsThresholdFiltering(t *testing.T) {
	d := openTemp(t)
	if err := d.Ingest(sources(t)); err != nil {
		t.Fatal(err)
	}
	all, err := d.Edges(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	strong, err := d.Edges(0.9, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(strong) == 0 || len(strong) >= len(all) {
		t.Fatalf("threshold didn't filter edges: all=%d strong=%d", len(all), len(strong))
	}
	for _, e := range strong {
		if e.Confidence < 0.9 {
			t.Fatalf("edge below threshold: %.2f", e.Confidence)
		}
	}
	one, err := d.Edges(0, 1)
	if err != nil || len(one) != 1 {
		t.Fatalf("limit ignored: %d edges (%v)", len(one), err)
	}
	none, err := d.Deps(0.99)
	if err != nil || len(none) != 0 {
		t.Fatalf("expected no deps at 0.99: %+v (%v)", none, err)
	}
	loose, err := d.Deps(0)
	if err != nil || len(loose) != 1 {
		t.Fatalf("unexpected zero-threshold deps: %+v (%v)", loose, err)
	}
	defaultDeps, err := d.Deps(0.5)
	if err != nil || len(defaultDeps) != 1 || loose[0].Evidence <= defaultDeps[0].Evidence {
		t.Fatalf("zero threshold should include extra evidence: loose=%+v default=%+v", loose, defaultDeps)
	}
}

func TestStripSQLComments(t *testing.T) {
	cases := map[string]string{
		"-- hi\nSELECT 1":              "\nSELECT 1",
		"/* x */SELECT/*y*/1":          "SELECT1",
		"SELECT 1 -- trailing":         "SELECT 1 ",
		"SELECT /* a */ 1 FROM t;":     "SELECT  1 FROM t;",
		"SELECT '/* not a comment */'": "SELECT '/* not a comment */'",
	}
	for in, want := range cases {
		if got := stripSQLComments(in); got != want {
			t.Errorf("stripSQLComments(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOpenCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "x.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
