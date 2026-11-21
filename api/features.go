package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"terraviz/db"
	"terraviz/internal/costs"
	"terraviz/internal/exporters"
	"terraviz/internal/models"
	"terraviz/internal/parsers"
	"terraviz/internal/snapshots"
	"terraviz/internal/summary"
	"terraviz/resolver"
	"terraviz/state"
)

// WebFS exposes the embedded dashboard assets so the server can mount icons.
func WebFS() fs.FS {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}
	return sub
}

// registerFeatureRoutes adds the read-only graph overlays and local snapshot
// history to the dashboard API.
func (s *Server) registerFeatureRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/summary", s.handleSummary)
	mux.HandleFunc("GET /api/costs", s.handleCosts)
	mux.HandleFunc("GET /api/export/drawio", s.handleDrawIO)
	mux.HandleFunc("GET /api/snapshots", s.handleListSnapshots)
	mux.HandleFunc("POST /api/snapshots", s.handleSaveSnapshot)
	mux.HandleFunc("GET /api/snapshots/{id}", s.handleRestoreSnapshot)
	mux.HandleFunc("DELETE /api/snapshots/{id}", s.handleDeleteSnapshot)
}

// graphDiagram adapts the SQLite graph into the shared Diagram model so the
// summary, cost and export packages work against the new dashboard too.
func (s *Server) graphDiagram() (*models.Diagram, error) {
	resources, err := s.DB.Resources("")
	if err != nil {
		return nil, err
	}
	deps, err := s.DB.Deps(0.5)
	if err != nil {
		return nil, err
	}

	d := &models.Diagram{
		Name:        "TerraViz graph",
		Source:      "terraform",
		Resources:   make([]models.Resource, 0, len(resources)),
		Connections: make([]models.Connection, 0, len(deps)),
	}
	for _, r := range resources {
		d.Resources = append(d.Resources, models.Resource{
			ID:          r.Address,
			Name:        r.Name,
			Type:        parsers.MapTerraformType(r.Type),
			Provider:    providerFromType(r.Type),
			State:       models.ResourceStateActive,
			Properties:  map[string]interface{}{"state": r.State, "scope_kind": r.ScopeKind, "scope_id": r.ScopeID},
			Source:      "terraform",
			InStateFile: true,
		})
	}
	for _, dep := range deps {
		d.Connections = append(d.Connections, models.Connection{
			ID:          fmt.Sprintf("dep-%s-%s", dep.From, dep.To),
			SourceID:    dep.From,
			TargetID:    dep.To,
			Type:        models.ConnectionTypeDependency,
			Description: strings.Join(dep.Kinds, ", "),
			Properties:  map[string]interface{}{"confidence": dep.Confidence, "evidence": dep.Evidence},
		})
	}
	return d, nil
}

func providerFromType(tfType string) string {
	switch {
	case strings.HasPrefix(tfType, "google_"):
		return "gcp"
	case strings.HasPrefix(tfType, "azurerm_"):
		return "azure"
	case strings.HasPrefix(tfType, "oci_"):
		return "oci"
	case strings.HasPrefix(tfType, "aws_"):
		return "aws"
	default:
		return "unknown"
	}
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	d, err := s.graphDiagram()
	if err != nil {
		fail(w, 500, err)
		return
	}
	text, source, err := summary.Generate(r.Context(), d, "")
	if err != nil {
		text, source = summary.Build(d), "local"
	}
	writeJSON(w, 200, map[string]any{"summary": text, "source": source})
}

func (s *Server) handleCosts(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	d, err := s.graphDiagram()
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, costs.Estimate(d))
}

func (s *Server) handleDrawIO(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	d, err := s.graphDiagram()
	if err != nil {
		fail(w, 500, err)
		return
	}
	data, err := exporters.DrawIO(d)
	if err != nil {
		fail(w, 500, err)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="architecture-diagram.drawio"`)
	_, _ = w.Write(data)
}

func (s *Server) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	list, err := snapshots.List()
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleSaveSnapshot(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	states, err := s.DB.RawStates()
	if err != nil {
		fail(w, 500, err)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	snap, err := snapshots.Save(body.Name, states, len(states))
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 201, snap)
}

func (s *Server) handleRestoreSnapshot(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	payload, err := snapshots.Load(r.PathValue("id"))
	if err != nil {
		fail(w, 400, err)
		return
	}
	var states []db.RawState
	if err := json.Unmarshal(payload, &states); err != nil {
		fail(w, 400, err)
		return
	}
	srcs := make([]resolver.Source, 0, len(states))
	for _, raw := range states {
		f, err := state.Parse([]byte(raw.Content), raw.Filename)
		if err != nil {
			fail(w, 400, err)
			return
		}
		srcs = append(srcs, resolver.Source{Name: raw.Name, Filename: raw.Filename, File: f})
	}
	if err := s.DB.Ingest(srcs); err != nil {
		fail(w, 500, err)
		return
	}
	if err := s.DB.ResolveAll(); err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"success": true, "states": len(srcs)})
}

func (s *Server) handleDeleteSnapshot(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodDelete) {
		return
	}
	if err := snapshots.Delete(r.PathValue("id")); err != nil {
		fail(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]any{"success": true})
}
