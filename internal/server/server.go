package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"sync"

	"terraviz/internal/models"
	"terraviz/web"

	"github.com/gin-gonic/gin"
)

const (
	// DefaultConnectionColor is the default color for user-created connections.
	DefaultConnectionColor = "#666666"

	// DefaultConnectionStyle is the default line style for user-created connections.
	DefaultConnectionStyle = "dashed"
)

// Server represents the web server for serving diagrams.
type Server struct {
	mu            sync.RWMutex
	port          int
	host          string
	router        *gin.Engine
	diagram       *models.ScanResult
	cachedDiagram []byte // JSON cache of diagram; nil when stale
}

// NewServer creates a new web server.
func NewServer(host string, port int) *Server {
	gin.SetMode(gin.ReleaseMode)

	s := &Server{
		port:   port,
		host:   host,
		router: gin.Default(),
	}

	s.setupRoutes()
	return s
}

// LoadDiagram loads a diagram file into the server.
func (s *Server) LoadDiagram(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read diagram file: %w", err)
	}

	var scanResult models.ScanResult
	if err := json.Unmarshal(data, &scanResult); err != nil {
		return fmt.Errorf("failed to parse diagram file: %w", err)
	}

	s.mu.Lock()
	s.diagram = &scanResult
	s.cachedDiagram = nil // invalidate cache
	s.mu.Unlock()

	return nil
}

// invalidateCache clears the cached diagram JSON. Must be called with s.mu held.
func (s *Server) invalidateCache() {
	s.cachedDiagram = nil
}

// Start starts the web server.
func (s *Server) Start() error {
	address := fmt.Sprintf("%s:%d", s.host, s.port)
	fmt.Printf("Starting web server at http://%s\n", address)
	return s.router.Run(address)
}

// setupRoutes sets up all the routes for the web server.
func (s *Server) setupRoutes() {
	// Parse HTML templates from the embedded filesystem.
	tmpl := template.Must(template.ParseFS(web.EmbeddedFiles, "templates/*"))
	s.router.SetHTMLTemplate(tmpl)

	// Serve static assets (JS, fonts, legacy icons) from the embedded filesystem.
	staticFS, err := fs.Sub(web.EmbeddedFiles, "static")
	if err != nil {
		panic(fmt.Sprintf("failed to create static sub-filesystem: %v", err))
	}
	s.router.StaticFS("/static", http.FS(staticFS))

	// Serve AWS icon packs from the embedded filesystem.
	iconsFS, err := fs.Sub(web.EmbeddedFiles, "icons")
	if err != nil {
		panic(fmt.Sprintf("failed to create icons sub-filesystem: %v", err))
	}
	s.router.StaticFS("/icons", http.FS(iconsFS))

	s.router.GET("/", s.handleEnhancedIndex)

	api := s.router.Group("/api")
	{
		api.GET("/diagram", s.handleGetDiagram)
		api.GET("/resources", s.handleGetResources)
		api.GET("/resources/search", s.handleSearchResources)
		api.GET("/resources/filter", s.handleFilterResources)
		api.POST("/resources/:id/hide", s.handleSetResourceVisibility(true))
		api.POST("/resources/:id/show", s.handleSetResourceVisibility(false))
		api.GET("/connections", s.handleGetConnections)
		api.POST("/connections", s.handleCreateConnection)
		api.DELETE("/connections/:id", s.handleDeleteConnection)
		api.GET("/stats", s.handleGetStats)
		api.POST("/export/:format", s.handleExport)
	}
}

// requireDiagram is a helper that checks if a diagram is loaded and returns it.
// Returns nil and sends an error response if no diagram is available.
func (s *Server) requireDiagram(c *gin.Context) *models.ScanResult {
	s.mu.RLock()
	d := s.diagram
	s.mu.RUnlock()

	if d == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No diagram loaded"})
		return nil
	}
	return d
}

// handleEnhancedIndex serves the main diagram page.
func (s *Server) handleEnhancedIndex(c *gin.Context) {
	s.mu.RLock()
	d := s.diagram
	s.mu.RUnlock()

	if d == nil {
		c.HTML(http.StatusOK, "error.html", gin.H{
			"error": "No diagram loaded. Please load a diagram file first.",
		})
		return
	}

	c.HTML(http.StatusOK, "index_enhanced.html", gin.H{
		"title":            "Cloud Architecture Visualizer",
		"diagram_name":     d.Diagram.Name,
		"resource_count":   len(d.Diagram.Resources),
		"connection_count": len(d.Diagram.Connections),
	})
}

// handleGetDiagram returns the complete diagram data, using a JSON cache.
func (s *Server) handleGetDiagram(c *gin.Context) {
	s.mu.RLock()
	cached := s.cachedDiagram
	d := s.diagram
	s.mu.RUnlock()

	if d == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No diagram loaded"})
		return
	}

	if cached != nil {
		c.Data(http.StatusOK, "application/json; charset=utf-8", cached)
		return
	}

	// Build and cache the JSON
	data, err := json.Marshal(d)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to serialize diagram"})
		return
	}

	s.mu.Lock()
	s.cachedDiagram = data
	s.mu.Unlock()

	c.Data(http.StatusOK, "application/json; charset=utf-8", data)
}

// handleGetResources returns all resources.
func (s *Server) handleGetResources(c *gin.Context) {
	d := s.requireDiagram(c)
	if d == nil {
		return
	}
	c.JSON(http.StatusOK, d.Diagram.Resources)
}

// handleSearchResources searches for resources by name or ID.
func (s *Server) handleSearchResources(c *gin.Context) {
	d := s.requireDiagram(c)
	if d == nil {
		return
	}

	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Query parameter 'q' is required"})
		return
	}

	results := d.Diagram.SearchResources(query)
	c.JSON(http.StatusOK, results)
}

// handleFilterResources filters resources based on criteria.
func (s *Server) handleFilterResources(c *gin.Context) {
	d := s.requireDiagram(c)
	if d == nil {
		return
	}

	filters := make(map[string]string)
	for _, key := range []string{"type", "provider", "region", "state", "tag"} {
		if v := c.Query(key); v != "" {
			filters[key] = v
		}
	}

	results := d.Diagram.FilterResources(filters)
	c.JSON(http.StatusOK, results)
}

// handleSetResourceVisibility returns a handler that sets a resource's Hidden field.
func (s *Server) handleSetResourceVisibility(hidden bool) gin.HandlerFunc {
	action := "shown"
	if hidden {
		action = "hidden"
	}

	return func(c *gin.Context) {
		resourceID := c.Param("id")

		// Hold write lock for the entire read-check-mutate sequence.
		s.mu.Lock()
		defer s.mu.Unlock()

		if s.diagram == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "No diagram loaded"})
			return
		}
		d := s.diagram

		for i := range d.Diagram.Resources {
			if d.Diagram.Resources[i].ID == resourceID {
				d.Diagram.Resources[i].Hidden = hidden
				s.invalidateCache()
				c.JSON(http.StatusOK, gin.H{"success": true, "message": "Resource " + action})
				return
			}
		}

		c.JSON(http.StatusNotFound, gin.H{"error": "Resource not found"})
	}
}

// handleGetConnections returns all connections.
func (s *Server) handleGetConnections(c *gin.Context) {
	d := s.requireDiagram(c)
	if d == nil {
		return
	}
	c.JSON(http.StatusOK, d.Diagram.Connections)
}

// handleCreateConnection creates a new connection between resources.
func (s *Server) handleCreateConnection(c *gin.Context) {
	var req struct {
		SourceID    string `json:"source_id" binding:"required"`
		TargetID    string `json:"target_id" binding:"required"`
		Type        string `json:"type"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Hold the write lock for the entire read-validate-mutate sequence
	// to prevent races between validation and append.
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.diagram == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No diagram loaded"})
		return
	}
	d := s.diagram

	// Validate that both resources exist.
	var sourceExists, targetExists bool
	for _, resource := range d.Diagram.Resources {
		if resource.ID == req.SourceID {
			sourceExists = true
		}
		if resource.ID == req.TargetID {
			targetExists = true
		}
		if sourceExists && targetExists {
			break
		}
	}

	if !sourceExists || !targetExists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Source or target resource does not exist"})
		return
	}

	connType := models.ConnectionTypeReference
	if req.Type != "" {
		connType = models.ConnectionType(req.Type)
	}

	conn := models.Connection{
		ID:          fmt.Sprintf("user-%s-%s", req.SourceID, req.TargetID),
		SourceID:    req.SourceID,
		TargetID:    req.TargetID,
		Type:        connType,
		Description: req.Description,
		Properties:  make(map[string]interface{}),
		Color:       DefaultConnectionColor,
		Style:       DefaultConnectionStyle,
	}

	d.Diagram.Connections = append(d.Diagram.Connections, conn)
	s.invalidateCache()

	c.JSON(http.StatusCreated, conn)
}

// handleDeleteConnection deletes a connection.
func (s *Server) handleDeleteConnection(c *gin.Context) {
	connectionID := c.Param("id")

	// Hold write lock for the entire read-check-mutate sequence.
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.diagram == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No diagram loaded"})
		return
	}
	d := s.diagram

	for i, conn := range d.Diagram.Connections {
		if conn.ID == connectionID {
			d.Diagram.Connections = append(
				d.Diagram.Connections[:i],
				d.Diagram.Connections[i+1:]...,
			)
			s.invalidateCache()
			c.JSON(http.StatusOK, gin.H{"success": true, "message": "Connection deleted"})
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "Connection not found"})
}

// handleGetStats returns diagram statistics.
func (s *Server) handleGetStats(c *gin.Context) {
	d := s.requireDiagram(c)
	if d == nil {
		return
	}

	stats := d.Diagram.GetResourceStats()

	result := make(map[string]interface{}, len(stats)+4)
	for k, v := range stats {
		result[k] = v
	}

	result["scan_time"] = d.ScanTime.Format("2006-01-02 15:04:05")
	result["scan_duration_ms"] = d.Stats.ScanDurationMs
	result["error_count"] = len(d.Errors)
	result["warning_count"] = len(d.Warnings)

	c.JSON(http.StatusOK, result)
}

// handleExport handles diagram export requests.
func (s *Server) handleExport(c *gin.Context) {
	d := s.requireDiagram(c)
	if d == nil {
		return
	}

	switch format := c.Param("format"); format {
	case "json":
		c.Header("Content-Disposition", "attachment; filename=diagram.json")
		c.JSON(http.StatusOK, d)
	case "svg":
		c.JSON(http.StatusNotImplemented, gin.H{"error": "SVG export is handled client-side"})
	case "png":
		c.JSON(http.StatusNotImplemented, gin.H{"error": "PNG export is handled client-side"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported export format. Supported: json, svg, png"})
	}
}
