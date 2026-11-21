// Package api exposes the ingestion, dependency graph and read-only SQL
// capabilities over HTTP. A minimal embedded page documents the endpoints;
// the real UI can be dropped into web/ later without changing Go code.
package api

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"terraviz/db"
	"terraviz/remote"
	"terraviz/resolver"
	"terraviz/s3source"
	"terraviz/state"
)

//go:embed web
var webFS embed.FS

type Server struct {
	DB         *db.DB
	MaxBytes   int64
	S3         s3source.Client // injected in tests; otherwise built from AWS env on first use
	SamplesDir string          // directory of bundled sample statefiles

	s3Once sync.Once
	s3c    s3source.Client
	s3Err  error

	remoteMu    sync.Mutex
	remoteCache map[string]remote.Client
	remoteErrs  map[string]error
	Remote      map[string]remote.Client // injected clients, keyed by provider (tests)
}

func New(d *db.DB) *Server {
	return &Server{DB: d, MaxBytes: 32 << 20, SamplesDir: "samples",
		remoteCache: map[string]remote.Client{}, remoteErrs: map[string]error{}}
}

func (s *Server) s3Client() (s3source.Client, error) {
	s.s3Once.Do(func() {
		if s.S3 != nil {
			s.s3c = s.S3
			return
		}
		s.s3c, s.s3Err = s3source.New(context.Background())
	})
	return s.s3c, s.s3Err
}

func s3Error(w http.ResponseWriter, err error) {
	if errors.Is(err, s3source.ErrNotConfigured) {
		writeJSON(w, http.StatusServiceUnavailable, errResp{Error: err.Error()})
		return
	}
	fail(w, 502, err)
}

func remoteError(w http.ResponseWriter, err error) {
	if errors.Is(err, remote.ErrNotConfigured) {
		writeJSON(w, http.StatusServiceUnavailable, errResp{Error: err.Error()})
		return
	}
	fail(w, 502, err)
}

func (s *Server) remoteClient(provider string) (remote.Client, error) {
	provider = remote.Normalize(provider)
	s.remoteMu.Lock()
	defer s.remoteMu.Unlock()
	if c, ok := s.Remote[provider]; ok && c != nil {
		return c, nil
	}
	if c, ok := s.remoteCache[provider]; ok {
		return c, s.remoteErrs[provider]
	}
	if provider == "aws" && s.S3 != nil {
		c := remote.FromS3(s.S3)
		s.remoteCache[provider] = c
		return c, nil
	}
	c, err := remote.New(context.Background(), provider)
	s.remoteCache[provider] = c
	s.remoteErrs[provider] = err
	return c, err
}

func remoteProvider(r *http.Request) string {
	return remote.Normalize(r.URL.Query().Get("provider"))
}

type errResp struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, errResp{Error: err.Error()})
}

func method(w http.ResponseWriter, r *http.Request, allowed string) bool {
	if r.Method == allowed {
		return true
	}
	w.Header().Set("Allow", allowed)
	fail(w, http.StatusMethodNotAllowed, fmt.Errorf("use %s", allowed))
	return false
}

func minParam(r *http.Request) (float64, error) {
	v := r.URL.Query().Get("min")
	if v == "" {
		return 0.5, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 || f > 1 {
		return 0, fmt.Errorf("min must be a number between 0 and 1")
	}
	return f, nil
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Printf("panic serving %s %s: %v", r.Method, r.URL.Path, v)
				fail(w, http.StatusInternalServerError, errors.New("internal error"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

// Mux returns the raw router so additional handlers (the resource-level
// diagram UI, icon packs) can be mounted before wrapping.
func (s *Server) Mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/states", s.handleStates)
	mux.HandleFunc("/api/resources", s.handleResources)
	mux.HandleFunc("/api/resolve", s.handleResolve)
	mux.HandleFunc("/api/deps", s.handleDeps)
	mux.HandleFunc("/api/edges", s.handleEdges)
	mux.HandleFunc("/api/graph", s.handleGraph)
	mux.HandleFunc("/api/query", s.handleQuery)
	mux.HandleFunc("/api/s3/status", s.handleS3Status)
	mux.HandleFunc("/api/s3/buckets", s.handleS3Buckets)
	mux.HandleFunc("/api/s3/objects", s.handleS3Objects)
	mux.HandleFunc("/api/s3/ingest", s.handleS3Ingest)
	mux.HandleFunc("/api/remote/status", s.handleRemoteStatus)
	mux.HandleFunc("/api/remote/buckets", s.handleRemoteBuckets)
	mux.HandleFunc("/api/remote/objects", s.handleRemoteObjects)
	mux.HandleFunc("/api/remote/ingest", s.handleRemoteIngest)
	mux.HandleFunc("/api/samples", s.handleSamples)
	mux.HandleFunc("/api/samples/ingest", s.handleSamplesIngest)
	s.registerFeatureRoutes(mux)

	mux.Handle("/", http.FileServer(http.FS(WebFS())))
	return mux
}

// Wrap adds timeouts, panic recovery and request logging around a router.
func (s *Server) Wrap(mux http.Handler) http.Handler {
	return logging(recoverer(http.TimeoutHandler(mux, 60*time.Second, "request timeout")))
}

// Handler returns the fully wired HTTP handler with timeouts, panic recovery
// and request logging.
func (s *Server) Handler() http.Handler { return s.Wrap(s.Mux()) }

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "db": s.DB.Path()})
}

func (s *Server) handleStates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		states, err := s.DB.States()
		if err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]any{"states": states})
	case http.MethodPost:
		s.ingest(w, r)
	default:
		method(w, r, http.MethodGet)
	}
}

type ingestInput struct {
	Name     string `json:"name"`
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

func stateName(name, filename string) string {
	if name != "" {
		return name
	}
	// Browsers may send a full Windows path as the multipart filename.
	base := filepath.Base(strings.ReplaceAll(filename, "\\", "/"))
	return strings.TrimSuffix(strings.TrimSuffix(base, ".tfstate"), ".json")
}

func tooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr) || strings.Contains(err.Error(), "request body too large")
}

func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.MaxBytes)
	var inputs []ingestInput

	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var req struct {
			States []ingestInput `json:"states"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			if tooLarge(err) {
				fail(w, http.StatusRequestEntityTooLarge, errors.New("request too large"))
			} else {
				fail(w, 400, fmt.Errorf("invalid JSON: %w", err))
			}
			return
		}
		inputs = req.States
	} else {
		if err := r.ParseMultipartForm(s.MaxBytes); err != nil {
			if tooLarge(err) {
				fail(w, http.StatusRequestEntityTooLarge, errors.New("request too large"))
			} else {
				fail(w, 400, fmt.Errorf("expected JSON or multipart upload: %w", err))
			}
			return
		}
		files := r.MultipartForm.File["states"]
		if len(files) == 0 {
			fail(w, 400, errors.New(`no files provided under field "states"`))
			return
		}
		var names []string
		if raw := r.FormValue("names"); raw != "" {
			names = strings.Split(raw, ",")
		}
		for i, fh := range files {
			name := ""
			if i < len(names) {
				name = strings.TrimSpace(names[i])
			}
			f, err := fh.Open()
			if err != nil {
				fail(w, 400, err)
				return
			}
			b, err := io.ReadAll(io.LimitReader(f, s.MaxBytes))
			f.Close()
			if err != nil {
				fail(w, 400, err)
				return
			}
			inputs = append(inputs, ingestInput{Name: name, Filename: fh.Filename, Content: string(b)})
		}
	}

	s.persist(w, inputs)
}

// persist parses, validates and ingests states from any source (upload, S3 or
// bundled samples) through the same path.
func (s *Server) persist(w http.ResponseWriter, inputs []ingestInput) {
	if len(inputs) == 0 {
		fail(w, 400, errors.New("no states provided"))
		return
	}
	srcs := make([]resolver.Source, 0, len(inputs))
	names := make([]string, 0, len(inputs))
	seen := map[string]bool{}
	for _, in := range inputs {
		name := stateName(in.Name, in.Filename)
		if name == "" {
			fail(w, 400, errors.New("a state needs a name or a filename"))
			return
		}
		if seen[name] {
			fail(w, 400, fmt.Errorf("duplicate state name %q in one request", name))
			return
		}
		seen[name] = true
		f, err := state.Parse([]byte(in.Content), name)
		if err != nil {
			fail(w, 400, err)
			return
		}
		srcs = append(srcs, resolver.Source{Name: name, Filename: in.Filename, File: f})
		names = append(names, name)
	}
	if err := s.DB.Ingest(srcs); err != nil {
		fail(w, 500, err)
		return
	}
	edges, _ := s.DB.Edges(0, 0)
	writeJSON(w, 200, map[string]any{"ingested": names, "edges": len(edges)})
}

func (s *Server) handleResources(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	resources, err := s.DB.Resources(r.URL.Query().Get("state"))
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"resources": resources})
}

func (s *Server) handleResolve(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	if err := s.DB.ResolveAll(); err != nil {
		fail(w, 500, err)
		return
	}
	edges, err := s.DB.Edges(0, 0)
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"edges": len(edges)})
}

func (s *Server) handleDeps(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	min, err := minParam(r)
	if err != nil {
		fail(w, 400, err)
		return
	}
	deps, err := s.DB.Deps(min)
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"min": min, "dependencies": deps})
}

func (s *Server) handleEdges(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	min, err := minParam(r)
	if err != nil {
		fail(w, 400, err)
		return
	}
	limit := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, err = strconv.Atoi(v)
		if err != nil || limit < 0 {
			fail(w, 400, errors.New("limit must be a non-negative integer"))
			return
		}
	}
	edges, err := s.DB.Edges(min, limit)
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"edges": edges})
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	min, err := minParam(r)
	if err != nil {
		fail(w, 400, err)
		return
	}
	g, err := s.DB.Graph(min)
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, g)
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	q := ""
	switch r.Method {
	case http.MethodGet:
		q = r.URL.Query().Get("sql")
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, s.MaxBytes)
		var req struct {
			SQL string `json:"sql"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			if tooLarge(err) {
				fail(w, http.StatusRequestEntityTooLarge, errors.New("request too large"))
			} else {
				fail(w, 400, fmt.Errorf("invalid JSON: %w", err))
			}
			return
		}
		q = req.SQL
	default:
		method(w, r, http.MethodPost)
		return
	}
	if err := db.IsReadOnly(q); err != nil {
		fail(w, 400, err)
		return
	}
	res, err := s.DB.Query(q)
	if err != nil {
		fail(w, 400, err)
		return
	}
	writeJSON(w, 200, res)
}

// ---- S3 browser ----

func (s *Server) handleS3Status(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	_, err := s.s3Client()
	status := map[string]any{
		"configured": err == nil,
		"region":     s3source.Region(),
		"endpoint":   s3source.Endpoint(),
	}
	if err != nil {
		status["error"] = err.Error()
	}
	writeJSON(w, 200, status)
}

func (s *Server) handleS3Buckets(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	c, err := s.s3Client()
	if err != nil {
		s3Error(w, err)
		return
	}
	buckets, err := c.Buckets(r.Context())
	if err != nil {
		s3Error(w, err)
		return
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i].Name < buckets[j].Name })
	writeJSON(w, 200, map[string]any{"buckets": buckets})
}

func (s *Server) handleS3Objects(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	bucket := r.URL.Query().Get("bucket")
	if bucket == "" {
		fail(w, 400, errors.New("bucket is required"))
		return
	}
	c, err := s.s3Client()
	if err != nil {
		s3Error(w, err)
		return
	}
	res, err := c.List(r.Context(), bucket, r.URL.Query().Get("prefix"), r.URL.Query().Get("token"))
	if err != nil {
		s3Error(w, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) handleS3Ingest(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.MaxBytes)
	var req struct {
		Bucket string   `json:"bucket"`
		Keys   []string `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, 400, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	if req.Bucket == "" || len(req.Keys) == 0 {
		fail(w, 400, errors.New("bucket and keys are required"))
		return
	}
	c, err := s.s3Client()
	if err != nil {
		s3Error(w, err)
		return
	}
	inputs := make([]ingestInput, 0, len(req.Keys))
	for _, key := range req.Keys {
		if strings.HasSuffix(key, "/") {
			continue
		}
		b, err := c.Get(r.Context(), req.Bucket, key)
		if err != nil {
			s3Error(w, fmt.Errorf("read s3://%s/%s: %w", req.Bucket, key, err))
			return
		}
		inputs = append(inputs, ingestInput{
			Name:     s3source.StateName(key),
			Filename: key,
			Content:  string(b),
		})
	}
	s.persist(w, inputs)
}

// ---- provider-agnostic remote state (AWS / GCP / Azure / OCI) ----

func (s *Server) handleRemoteStatus(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	provider := remoteProvider(r)
	_, err := s.remoteClient(provider)
	status := remote.ConfigStatus(provider)
	status["configured"] = err == nil
	if err != nil {
		status["error"] = err.Error()
	}
	writeJSON(w, 200, status)
}

func (s *Server) handleRemoteBuckets(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	c, err := s.remoteClient(remoteProvider(r))
	if err != nil {
		remoteError(w, err)
		return
	}
	buckets, err := c.Buckets(r.Context())
	if err != nil {
		remoteError(w, err)
		return
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i].Name < buckets[j].Name })
	writeJSON(w, 200, map[string]any{"buckets": buckets, "provider": remoteProvider(r)})
}

func (s *Server) handleRemoteObjects(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	bucket := r.URL.Query().Get("bucket")
	if bucket == "" {
		fail(w, 400, errors.New("bucket is required"))
		return
	}
	c, err := s.remoteClient(remoteProvider(r))
	if err != nil {
		remoteError(w, err)
		return
	}
	res, err := c.List(r.Context(), bucket, r.URL.Query().Get("prefix"), r.URL.Query().Get("token"))
	if err != nil {
		remoteError(w, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) handleRemoteIngest(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.MaxBytes)
	var req struct {
		Provider string   `json:"provider"`
		Bucket   string   `json:"bucket"`
		Keys     []string `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, 400, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	provider := remote.Normalize(req.Provider)
	if req.Bucket == "" || len(req.Keys) == 0 {
		fail(w, 400, errors.New("bucket and keys are required"))
		return
	}
	c, err := s.remoteClient(provider)
	if err != nil {
		remoteError(w, err)
		return
	}
	inputs := make([]ingestInput, 0, len(req.Keys))
	for _, key := range req.Keys {
		if strings.HasSuffix(key, "/") {
			continue
		}
		b, err := c.Get(r.Context(), req.Bucket, key)
		if err != nil {
			remoteError(w, fmt.Errorf("read %s://%s/%s: %w", provider, req.Bucket, key, err))
			return
		}
		inputs = append(inputs, ingestInput{Name: remote.StateName(key), Filename: key, Content: string(b)})
	}
	s.persist(w, inputs)
}

// ---- bundled samples (works without AWS credentials) ----

type sampleInfo struct {
	Name     string `json:"name"`
	File     string `json:"file"`
	Provider string `json:"provider"`
	Size     int64  `json:"size"`
}

func sampleProvider(b []byte) string {
	s := string(b)
	switch {
	case strings.Contains(s, `"aws_`) || strings.Contains(s, "arn:aws:"):
		return "aws"
	case strings.Contains(s, `"google_`) || strings.Contains(s, "googleapis.com"):
		return "gcp"
	case strings.Contains(s, `"azurerm_`):
		return "azure"
	case strings.Contains(s, `"oci_`):
		return "oci"
	default:
		return "unknown"
	}
}

func (s *Server) sampleFiles() ([]sampleInfo, error) {
	entries, err := os.ReadDir(s.SamplesDir)
	if err != nil {
		return nil, err
	}
	out := []sampleInfo{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tfstate") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.SamplesDir, e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, sampleInfo{s3source.StateName(e.Name()), e.Name(), sampleProvider(b), int64(len(b))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *Server) handleSamples(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	samples, err := s.sampleFiles()
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"samples": samples})
}

func (s *Server) handleSamplesIngest(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.MaxBytes)
	var req struct {
		Samples []string `json:"samples"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, 400, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	all, err := s.sampleFiles()
	if err != nil {
		fail(w, 500, err)
		return
	}
	wanted := map[string]bool{}
	for _, n := range req.Samples {
		wanted[n] = true
	}
	inputs := []ingestInput{}
	for _, f := range all {
		if len(req.Samples) > 0 && !wanted[f.Name] {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.SamplesDir, f.File))
		if err != nil {
			fail(w, 500, err)
			return
		}
		inputs = append(inputs, ingestInput{Name: f.Name, Filename: f.File, Content: string(b)})
	}
	s.persist(w, inputs)
}
