package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"terraviz/internal/models"
	"terraviz/internal/parsers"
	"terraviz/web"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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

		// S3 backend integration
		s3Group := api.Group("/s3")
		{
			s3Group.GET("/buckets", s.handleListBuckets)
			s3Group.GET("/objects", s.handleListObjects)
			s3Group.POST("/load", s.handleLoadFromS3)
		}

		// Local file upload
		api.POST("/upload", s.handleUploadStateFile)
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

	data := gin.H{
		"title":            "Cloud Architecture Visualizer",
		"diagram_name":     "No Diagram Loaded",
		"resource_count":   0,
		"connection_count": 0,
	}

	if d != nil {
		data["diagram_name"] = d.Diagram.Name
		data["resource_count"] = len(d.Diagram.Resources)
		data["connection_count"] = len(d.Diagram.Connections)
	}

	c.HTML(http.StatusOK, "index_enhanced.html", data)
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

// handleUploadStateFile accepts a multipart file upload of a Terraform state file,
// parses it, and sets it as the active diagram.
func (s *Server) handleUploadStateFile(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided"})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to read uploaded file: %v", err)})
		return
	}

	parser := parsers.NewTerraformParser()
	result, err := parser.ParseStateData(data, header.Filename)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Failed to parse state file: %v", err)})
		return
	}

	result.Diagram.Name = fmt.Sprintf("Local: %s", header.Filename)
	result.ScanConfig.InputPath = header.Filename

	s.mu.Lock()
	s.diagram = result
	s.cachedDiagram = nil
	s.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"message":          fmt.Sprintf("Loaded state from %s", header.Filename),
		"resource_count":   len(result.Diagram.Resources),
		"connection_count": len(result.Diagram.Connections),
	})
}

// s3APITimeout is the timeout for S3 API calls made by the server.
const s3APITimeout = 30 * time.Second

// newS3Client creates an S3 client using the default credential provider chain.
// An optional profile query parameter is supported.
func newS3Client(ctx context.Context, profile string, region string) (*s3.Client, error) {
	var opts []func(*config.LoadOptions) error
	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	return s3.NewFromConfig(cfg), nil
}

// handleListBuckets returns all S3 buckets for the configured AWS account.
// Query params: profile (optional), region (optional).
func (s *Server) handleListBuckets(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), s3APITimeout)
	defer cancel()

	client, err := newS3Client(ctx, c.Query("profile"), c.Query("region"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to initialize AWS client: %v", err)})
		return
	}

	output, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to list buckets: %v", err)})
		return
	}

	type bucketInfo struct {
		Name      string `json:"name"`
		CreatedAt string `json:"created_at,omitempty"`
	}

	buckets := make([]bucketInfo, 0, len(output.Buckets))
	for _, b := range output.Buckets {
		info := bucketInfo{Name: *b.Name}
		if b.CreationDate != nil {
			info.CreatedAt = b.CreationDate.Format(time.RFC3339)
		}
		buckets = append(buckets, info)
	}

	c.JSON(http.StatusOK, gin.H{"buckets": buckets})
}

// handleListObjects lists objects in an S3 bucket with optional prefix for tree navigation.
// Query params: bucket (required), prefix (optional), profile (optional), region (optional).
func (s *Server) handleListObjects(c *gin.Context) {
	bucket := c.Query("bucket")
	if bucket == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Query parameter 'bucket' is required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), s3APITimeout)
	defer cancel()

	client, err := newS3Client(ctx, c.Query("profile"), c.Query("region"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to initialize AWS client: %v", err)})
		return
	}

	prefix := c.Query("prefix")
	input := &s3.ListObjectsV2Input{
		Bucket:    &bucket,
		Prefix:    &prefix,
		Delimiter: strPtr("/"),
	}

	output, err := client.ListObjectsV2(ctx, input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to list objects: %v", err)})
		return
	}

	type objectInfo struct {
		Key          string `json:"key"`
		Size         int64  `json:"size"`
		LastModified string `json:"last_modified,omitempty"`
		IsFolder     bool   `json:"is_folder"`
	}

	items := make([]objectInfo, 0, len(output.CommonPrefixes)+len(output.Contents))

	// Folders (common prefixes)
	for _, cp := range output.CommonPrefixes {
		items = append(items, objectInfo{
			Key:      *cp.Prefix,
			IsFolder: true,
		})
	}

	// Files
	for _, obj := range output.Contents {
		key := *obj.Key
		// Skip the prefix itself if it appears as an object
		if key == prefix {
			continue
		}
		info := objectInfo{
			Key:  key,
			Size: *obj.Size,
		}
		if obj.LastModified != nil {
			info.LastModified = obj.LastModified.Format(time.RFC3339)
		}
		items = append(items, info)
	}

	c.JSON(http.StatusOK, gin.H{"objects": items, "prefix": prefix, "bucket": bucket})
}

// handleLoadFromS3 downloads a state file from S3 and loads it as the active diagram.
// JSON body: {"bucket": "...", "key": "...", "profile": "...", "region": "..."}.
func (s *Server) handleLoadFromS3(c *gin.Context) {
	var req struct {
		Bucket  string `json:"bucket" binding:"required"`
		Key     string `json:"key" binding:"required"`
		Profile string `json:"profile"`
		Region  string `json:"region"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), s3APITimeout)
	defer cancel()

	client, err := newS3Client(ctx, req.Profile, req.Region)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to initialize AWS client: %v", err)})
		return
	}

	output, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &req.Bucket,
		Key:    &req.Key,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to download state file: %v", err)})
		return
	}
	defer output.Body.Close()

	data, err := io.ReadAll(output.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to read state file: %v", err)})
		return
	}

	// Parse the state data
	parser := parsers.NewTerraformParser()
	sourceName := fmt.Sprintf("s3://%s/%s", req.Bucket, req.Key)
	result, err := parser.ParseStateData(data, sourceName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Failed to parse state file: %v", err)})
		return
	}

	// Extract a short name from the key for the diagram
	parts := strings.Split(req.Key, "/")
	name := parts[len(parts)-1]
	if name == "" && len(parts) > 1 {
		name = parts[len(parts)-2]
	}
	result.Diagram.Name = fmt.Sprintf("S3: %s", name)
	result.ScanConfig.InputPath = sourceName

	// Set as active diagram
	s.mu.Lock()
	s.diagram = result
	s.cachedDiagram = nil
	s.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"message":          fmt.Sprintf("Loaded state from %s", sourceName),
		"resource_count":   len(result.Diagram.Resources),
		"connection_count": len(result.Diagram.Connections),
	})
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string {
	return &s
}
