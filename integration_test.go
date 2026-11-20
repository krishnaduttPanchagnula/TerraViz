package main_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"terraviz/api"
	"terraviz/db"
)

func stateBytes(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name+".tfstate"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func postStates(t *testing.T, client *http.Client, base string, names ...string) (int, string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, name := range names {
		part, err := mw.CreateFormFile("states", name+".tfstate")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(stateBytes(t, name)); err != nil {
			t.Fatal(err)
		}
	}
	mw.Close()
	res, err := client.Post(base+"/api/states", mw.FormDataContentType(), &body)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func getJSON(t *testing.T, client *http.Client, url string, out any) {
	t.Helper()
	res, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("GET %s: %d %s", url, res.StatusCode, b)
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
}

// Full flow over a real TCP listener: upload -> deps/edges/graph -> SQL,
// then verify the data survives closing and reopening the database.
func TestServerEndToEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "e2e.db")
	d, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(api.New(d).Handler())
	client := srv.Client()
	base := srv.URL

	var health struct {
		Status string `json:"status"`
	}
	getJSON(t, client, base+"/api/health", &health)
	if health.Status != "ok" {
		t.Fatalf("health: %+v", health)
	}

	// Empty DB should serialize as [] (not null) so the UI can iterate it.
	var empty struct {
		Dependencies []any `json:"dependencies"`
	}
	getJSON(t, client, base+"/api/deps", &empty)
	if empty.Dependencies == nil {
		t.Fatal("empty deps serialized as null")
	}

	code, body := postStates(t, client, base, "network", "app")
	if code != 200 {
		t.Fatalf("upload: %d %s", code, body)
	}

	var states struct {
		States []struct {
			Name      string `json:"name"`
			Resources int    `json:"resources"`
		} `json:"states"`
	}
	getJSON(t, client, base+"/api/states", &states)
	if len(states.States) != 2 {
		t.Fatalf("states: %+v", states)
	}

	var deps struct {
		Dependencies []struct {
			From, To string
			Evidence int
		} `json:"dependencies"`
	}
	getJSON(t, client, base+"/api/deps?min=0.5", &deps)
	if len(deps.Dependencies) != 1 || deps.Dependencies[0].From != "app" ||
		deps.Dependencies[0].To != "network" || deps.Dependencies[0].Evidence != 6 {
		t.Fatalf("deps: %+v", deps)
	}

	var resources struct {
		Resources []struct {
			State, Address, Type string
		} `json:"resources"`
	}
	getJSON(t, client, base+"/api/resources", &resources)
	if len(resources.Resources) == 0 {
		t.Fatal("no resources returned after ingest")
	}
	// Resource-level edges are what the drill-down graph draws.
	var resourceEdges struct {
		Edges []struct {
			FromAddr string `json:"from_addr"`
			ToAddr   string `json:"to_addr"`
		} `json:"edges"`
	}
	getJSON(t, client, base+"/api/edges?min=0.8", &resourceEdges)
	hasAddrs := false
	for _, e := range resourceEdges.Edges {
		if e.FromAddr != "" && e.ToAddr != "" {
			hasAddrs = true
		}
	}
	if !hasAddrs {
		t.Fatal("edges lack resource addresses needed for drill-down")
	}

	var edges struct {
		Edges []struct {
			Confidence float64 `json:"confidence"`
		} `json:"edges"`
	}
	getJSON(t, client, base+"/api/edges?min=0.8", &edges)
	if len(edges.Edges) == 0 {
		t.Fatal("no evidence edges at 0.8")
	}
	for _, e := range edges.Edges {
		if e.Confidence < 0.8 {
			t.Fatalf("edge below min: %.2f", e.Confidence)
		}
	}

	var graph struct {
		Nodes []struct {
			ID        string `json:"id"`
			Resources int    `json:"resources"`
		} `json:"nodes"`
		Edges []struct {
			From, To string
			Kinds    []string `json:"kinds"`
		} `json:"edges"`
	}
	getJSON(t, client, base+"/api/graph?min=0.5", &graph)
	if len(graph.Nodes) != 2 || len(graph.Edges) != 1 || len(graph.Edges[0].Kinds) == 0 {
		t.Fatalf("graph: %+v", graph)
	}

	q, _ := json.Marshal(map[string]string{"sql": "SELECT from_state, to_state FROM dependencies"})
	res, err := client.Post(base+"/api/query", "application/json", bytes.NewReader(q))
	if err != nil {
		t.Fatal(err)
	}
	var query struct {
		Columns []string `json:"columns"`
		Rows    [][]any  `json:"rows"`
	}
	if err := json.NewDecoder(res.Body).Decode(&query); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if len(query.Rows) != 1 || query.Rows[0][0] != "app" {
		t.Fatalf("query: %+v", query)
	}

	bad, _ := json.Marshal(map[string]string{"sql": "DROP TABLE states"})
	res, err = client.Post(base+"/api/query", "application/json", bytes.NewReader(bad))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 400 || !strings.Contains(string(b), "read-only") {
		t.Fatalf("write query accepted: %d %s", res.StatusCode, b)
	}

	// The UI, its icons and the S3 status endpoint are served by the same server.
	res, err = client.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(b), "app.js") {
		t.Fatalf("index: %d", res.StatusCode)
	}
	res, err = client.Get(base + "/icons/azure.svg")
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(b), "#0078D4") {
		t.Fatalf("azure icon: %d", res.StatusCode)
	}
	var s3status struct {
		Configured bool   `json:"configured"`
		Error      string `json:"error"`
	}
	getJSON(t, client, base+"/api/s3/status", &s3status) // configured or not, must be valid JSON

	// Bundled samples are available as an offline alternative to S3.
	var samples struct {
		Samples []struct{ Name, Provider string } `json:"samples"`
	}
	getJSON(t, client, base+"/api/samples", &samples)
	if len(samples.Samples) != 7 {
		t.Fatalf("bundled samples: %+v", samples.Samples)
	}

	srv.Close()
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	persisted, err := reopened.Deps(0.5)
	if err != nil || len(persisted) != 1 {
		t.Fatalf("data did not persist: %+v (%v)", persisted, err)
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping binary end-to-end test in short mode")
	}
	bin := filepath.Join(t.TempDir(), "terraquery")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

func waitHealth(t *testing.T, client *http.Client, base string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		res, err := client.Get(base + "/api/health")
		if err == nil {
			res.Body.Close()
			if res.StatusCode == 200 {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("server did not become healthy")
}

// Drives the actual compiled binary: CLI ingest, CLI SQL, then server mode.
func TestBinaryEndToEnd(t *testing.T) {
	bin := buildBinary(t)
	tmp := t.TempDir()
	netPath, _ := filepath.Abs(filepath.Join("testdata", "network.tfstate"))
	appPath, _ := filepath.Abs(filepath.Join("testdata", "app.tfstate"))
	cliDB := filepath.Join(tmp, "cli.db")

	outFile := filepath.Join(tmp, "out.json")
	cli := exec.Command(bin, "-db", cliDB, "-json", outFile,
		"network="+netPath, "app="+appPath)
	out, err := cli.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "app") || !strings.Contains(string(out), "network") {
		t.Fatalf("CLI output missing dependency:\n%s", out)
	}
	outBytes, err := os.ReadFile(outFile)
	if err != nil || !strings.Contains(string(outBytes), `"from": "app"`) {
		t.Fatalf("CLI JSON output: %v\n%s", err, outBytes)
	}
	if _, err := os.Stat(cliDB); err != nil {
		t.Fatalf("CLI did not create DB: %v", err)
	}

	query := exec.Command(bin, "-db", cliDB,
		"-query", "SELECT from_state, to_state FROM dependencies")
	out, err = query.CombinedOutput()
	if err != nil || !strings.Contains(string(out), `"app"`) {
		t.Fatalf("CLI query: %v\n%s", err, out)
	}

	blocked := exec.Command(bin, "-db", cliDB, "-query", "DELETE FROM edges")
	out, err = blocked.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "read-only") {
		t.Fatalf("CLI allowed write query: %v\n%s", err, out)
	}

	addr := freeAddr(t)
	serverDB := filepath.Join(tmp, "server.db")
	server := exec.Command(bin, "-serve", "-addr", addr, "-db", serverDB)
	var logs bytes.Buffer
	server.Stdout = &logs
	server.Stderr = &logs
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	defer func() {
		if !stopped {
			_ = server.Process.Kill()
			_ = server.Wait()
		}
	}()

	base := "http://" + addr
	client := &http.Client{Timeout: 2 * time.Second}
	waitHealth(t, client, base)

	code, body := postStates(t, client, base, "network", "app")
	if code != 200 {
		t.Fatalf("server upload: %d %s", code, body)
	}
	var deps struct {
		Dependencies []struct{ From, To string } `json:"dependencies"`
	}
	getJSON(t, client, base+"/api/deps", &deps)
	if len(deps.Dependencies) != 1 || deps.Dependencies[0].From != "app" {
		t.Fatalf("server deps: %+v", deps)
	}

	// Graceful shutdown on SIGTERM.
	if err := server.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("server exit: %v\n%s", err, logs.String())
		}
		stopped = true
	case <-time.After(10 * time.Second):
		t.Fatalf("server did not shut down: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "listening") {
		t.Fatalf("missing startup log: %s", logs.String())
	}
}

// Sanity check that the whole test suite runs against the same fixtures.
func TestFixturesPresent(t *testing.T) {
	for _, name := range []string{"network", "app"} {
		if len(stateBytes(t, name)) == 0 {
			t.Fatalf("empty fixture %s", name)
		}
	}
}
