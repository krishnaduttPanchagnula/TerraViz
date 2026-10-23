package exporters

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"

	"terraviz/internal/models"
)

// Layout constants for the draw.io graph.
const (
	drawioCellWidth   = 200.0
	drawioCellHeight  = 60.0
	drawioGapX        = 60.0
	drawioGapY        = 40.0
	drawioPadding     = 20.0
	drawioTitleHeight = 30.0
)

// neutralServiceColour is the fill of a resource whose service is unknown.
const neutralServiceColour = "#D1D5DB"

// awsServiceColours maps an AWS icon category to the brand colour of that
// service, the same palette the AWS architecture icon packs use.
var awsServiceColours = map[string]string{
	"Analytics":                    "#8C4FFF",
	"App-Integration":              "#E7157B",
	"Blockchain":                   "#8C4FFF",
	"Business-Applications":        "#99F5FF",
	"Containers":                   "#ED7100",
	"Compute":                      "#ED7100",
	"Customer-Enablement":          "#00A88F",
	"Database":                     "#2E27AD",
	"Developer-Tools":              "#00A88F",
	"End-User-Computing":           "#00A88F",
	"Front-End-Web-Mobile":         "#E7157B",
	"Games":                        "#B05BD1",
	"Internet-of-Things":           "#48A9A6",
	"Machine-Learning":             "#E7157B",
	"Management-Governance":        "#E7157B",
	"Media-Services":               "#8C4FFF",
	"Migration-Transfer":           "#E7157B",
	"Networking-Content-Delivery":  "#8C4FFF",
	"Quantum-Technologies":         "#4C27B3",
	"Robotics":                     "#00A88F",
	"Satellite":                    "#C9254D",
	"Security-Identity-Compliance": "#DD344C",
	"Storage":                      "#7AA116",
	"VR-AR":                        "#E7157B",
}

// drawioIDUnsafe matches the characters that are not allowed in a cell id.
var drawioIDUnsafe = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// DrawIO renders the diagram as a draw.io (mxGraph) XML file. Resources
// become vertices, connections become edges, and a resource that lives in a
// VPC or a subnet becomes a child of that container so draw.io draws the
// nesting.
func DrawIO(d *models.Diagram) ([]byte, error) {
	if d == nil {
		return nil, fmt.Errorf("building draw.io: no diagram")
	}

	parents := containment(d)

	// First pass: give every resource a cell id, so a child can point at a
	// parent that comes later in the list.
	used := map[string]bool{}
	cellIDs := make(map[string]string, len(d.Resources))
	for _, resource := range d.Resources {
		cellIDs[resource.ID] = uniqueID("r_"+sanitizeID(resource.ID), used)
	}

	// Second pass: the containment forest.
	children := map[string][]string{}
	var roots []string
	for _, resource := range d.Resources {
		parent := "1"
		if pid, ok := parents[resource.ID]; ok {
			if cellID, ok := cellIDs[pid]; ok {
				parent = cellID
			}
		}
		if parent == "1" {
			roots = append(roots, cellIDs[resource.ID])
			continue
		}
		children[parent] = append(children[parent], cellIDs[resource.ID])
	}

	// Third pass: the vertex cells.
	vertices := make([]mxCell, 0, len(d.Resources))
	for _, resource := range d.Resources {
		parent := "1"
		if pid, ok := parents[resource.ID]; ok {
			if cellID, ok := cellIDs[pid]; ok {
				parent = cellID
			}
		}

		colour := serviceColour(&resource)
		style := fmt.Sprintf("rounded=1;whiteSpace=wrap;html=1;fillColor=%s;strokeColor=%s;fontColor=#FFFFFF;", colour, colour)
		if len(children[cellIDs[resource.ID]]) > 0 {
			style = fmt.Sprintf("rounded=1;whiteSpace=wrap;html=1;fillColor=#F3F4F6;strokeColor=%s;dashed=1;verticalAlign=top;align=left;spacingLeft=8;spacingTop=4;fontStyle=1;fontColor=%s;", colour, colour)
		}

		vertices = append(vertices, mxCell{
			ID:     cellIDs[resource.ID],
			Value:  htmlEscape(resource.Name) + "<br>" + htmlEscape(string(resource.Type)),
			Style:  style,
			Vertex: "1",
			Parent: parent,
		})
	}

	// Lay the forest out: every root next to its siblings, every child
	// stacked inside its parent.
	positions := map[string]drawioGeometry{}
	offsetX := 0.0
	for _, root := range roots {
		width, height := drawioSize(root, children, positions)
		drawioPlace(root, offsetX, 0, width, height, children, positions)
		offsetX += width + drawioGapX
	}

	for i := range vertices {
		cell := vertices[i]
		geometry := positions[cell.ID]
		x, y := geometry.x, geometry.y
		if cell.Parent != "1" {
			if parent, ok := positions[cell.Parent]; ok {
				x -= parent.x
				y -= parent.y
			}
		}
		cell.Geometry = &mxGeometry{
			X:      x,
			Y:      y,
			Width:  geometry.width,
			Height: geometry.height,
			As:     "geometry",
		}
		vertices[i] = cell
	}

	// The connection edges.
	edges := make([]mxCell, 0, len(d.Connections))
	for _, connection := range d.Connections {
		source, ok := cellIDs[connection.SourceID]
		if !ok {
			continue
		}
		target, ok := cellIDs[connection.TargetID]
		if !ok {
			continue
		}
		edges = append(edges, mxCell{
			ID:     uniqueID("e_"+sanitizeID(connection.ID), used),
			Value:  htmlEscape(string(connection.Type)),
			Style:  "edgeStyle=orthogonalEdgeStyle;rounded=1;html=1;strokeColor=#6B7280;endArrow=block;endFill=1;",
			Edge:   "1",
			Parent: "1",
			Source: source,
			Target: target,
			Geometry: &mxGeometry{
				Relative: 1,
				As:       "geometry",
			},
		})
	}

	cells := []mxCell{{ID: "0"}, {ID: "1", Parent: "0"}}
	cells = append(cells, vertices...)
	cells = append(cells, edges...)

	file := mxFile{
		Host: "app.diagrams.net",
		Type: "device",
		Diagram: mxDiagram{
			ID:   sanitizeID(d.ID),
			Name: diagramName(d),
			Model: mxGraphModel{
				Grid:       1,
				GridSize:   10,
				Guides:     1,
				Tooltips:   1,
				Connect:    1,
				Arrows:     1,
				Fold:       1,
				Page:       1,
				PageScale:  1,
				PageWidth:  1169,
				PageHeight: 826,
				Root:       mxRoot{Cells: cells},
			},
		},
	}

	data, err := xml.MarshalIndent(file, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding draw.io XML: %w", err)
	}
	return append(data, '\n'), nil
}

// containment derives the parent of a resource from the networking
// connections: a subnet lives in its VPC, and a resource lives in its subnet.
// A resource that sits in a subnet keeps the subnet as its parent, even when
// it also points at the VPC.
func containment(d *models.Diagram) map[string]string {
	types := make(map[string]models.ResourceType, len(d.Resources))
	for _, resource := range d.Resources {
		types[resource.ID] = resource.Type
	}

	parents := map[string]string{}
	set := func(child, parent string) {
		if _, exists := parents[child]; !exists {
			parents[child] = parent
		}
	}

	// Subnet -> VPC.
	for _, connection := range d.Connections {
		if connection.Type != models.ConnectionTypeNetworking {
			continue
		}
		if types[connection.SourceID] == models.ResourceTypeSubnet &&
			types[connection.TargetID] == models.ResourceTypeVPC {
			set(connection.SourceID, connection.TargetID)
		}
	}

	// Resource -> subnet.
	for _, connection := range d.Connections {
		if connection.Type != models.ConnectionTypeNetworking {
			continue
		}
		if types[connection.TargetID] == models.ResourceTypeSubnet &&
			types[connection.SourceID] != models.ResourceTypeVPC {
			set(connection.SourceID, connection.TargetID)
		}
	}

	// Resource -> VPC.
	for _, connection := range d.Connections {
		if connection.Type != models.ConnectionTypeNetworking {
			continue
		}
		if types[connection.TargetID] == models.ResourceTypeVPC &&
			types[connection.SourceID] != models.ResourceTypeSubnet {
			set(connection.SourceID, connection.TargetID)
		}
	}

	return parents
}

// serviceColour returns the brand colour of the service a resource belongs to.
func serviceColour(resource *models.Resource) string {
	path := strings.TrimPrefix(resource.GetResourceIcon(), "/icons/")
	if index := strings.Index(path, "/"); index > 0 {
		if colour, ok := awsServiceColours[path[:index]]; ok {
			return colour
		}
	}
	return neutralServiceColour
}

// sanitizeID turns a resource id into something a cell id can hold.
func sanitizeID(id string) string {
	cleaned := drawioIDUnsafe.ReplaceAllString(id, "_")
	if cleaned == "" {
		return "resource"
	}
	return cleaned
}

// uniqueID makes an id unique by appending a counter.
func uniqueID(base string, used map[string]bool) string {
	id := base
	for suffix := 2; used[id]; suffix++ {
		id = fmt.Sprintf("%s_%d", base, suffix)
	}
	used[id] = true
	return id
}

// drawioGeometry is the position and the size of a cell.
type drawioGeometry struct {
	x      float64
	y      float64
	width  float64
	height float64
}

// drawioSize returns the size of the subtree rooted at a cell.
func drawioSize(id string, children map[string][]string, positions map[string]drawioGeometry) (float64, float64) {
	if geometry, ok := positions[id]; ok && geometry.width > 0 {
		return geometry.width, geometry.height
	}

	kids := children[id]
	if len(kids) == 0 {
		positions[id] = drawioGeometry{width: drawioCellWidth, height: drawioCellHeight}
		return drawioCellWidth, drawioCellHeight
	}

	total := 0.0
	width := drawioCellWidth
	for i, kid := range kids {
		kidWidth, kidHeight := drawioSize(kid, children, positions)
		if kidWidth > width {
			width = kidWidth
		}
		total += kidHeight
		if i < len(kids)-1 {
			total += drawioGapY
		}
	}

	positions[id] = drawioGeometry{width: width + 2*drawioPadding, height: total + 2*drawioPadding + drawioTitleHeight}
	return width + 2*drawioPadding, total + 2*drawioPadding + drawioTitleHeight
}

// drawioPlace positions the subtree rooted at a cell, with the top left
// corner of the cell at (x, y).
func drawioPlace(id string, x, y, width, height float64, children map[string][]string, positions map[string]drawioGeometry) {
	positions[id] = drawioGeometry{x: x, y: y, width: width, height: height}

	kids := children[id]
	if len(kids) == 0 {
		return
	}

	childY := y + drawioPadding + drawioTitleHeight
	for _, kid := range kids {
		kidWidth, kidHeight := drawioSize(kid, children, positions)
		drawioPlace(kid, x+drawioPadding+(width-2*drawioPadding-kidWidth)/2, childY, kidWidth, kidHeight, children, positions)
		childY += kidHeight + drawioGapY
	}
}

// mxFile is the root element of a draw.io document.
type mxFile struct {
	XMLName xml.Name  `xml:"mxfile"`
	Host    string    `xml:"host,attr"`
	Type    string    `xml:"type,attr"`
	Diagram mxDiagram `xml:"diagram"`
}

// mxDiagram holds the graph model of a draw.io document.
type mxDiagram struct {
	ID    string       `xml:"id,attr"`
	Name  string       `xml:"name,attr"`
	Model mxGraphModel `xml:"mxGraphModel"`
}

// mxGraphModel holds the cells of a draw.io document.
type mxGraphModel struct {
	DX         int    `xml:"dx,attr"`
	DY         int    `xml:"dy,attr"`
	Grid       int    `xml:"grid,attr"`
	GridSize   int    `xml:"gridSize,attr"`
	Guides     int    `xml:"guides,attr"`
	Tooltips   int    `xml:"tooltips,attr"`
	Connect    int    `xml:"connect,attr"`
	Arrows     int    `xml:"arrows,attr"`
	Fold       int    `xml:"fold,attr"`
	Page       int    `xml:"page,attr"`
	PageScale  int    `xml:"pageScale,attr"`
	PageWidth  int    `xml:"pageWidth,attr"`
	PageHeight int    `xml:"pageHeight,attr"`
	Math       int    `xml:"math,attr"`
	Shadow     int    `xml:"shadow,attr"`
	Root       mxRoot `xml:"root"`
}

// mxRoot holds the cells of a draw.io graph.
type mxRoot struct {
	Cells []mxCell `xml:"mxCell"`
}

// mxCell is a vertex or an edge.
type mxCell struct {
	ID       string      `xml:"id,attr"`
	Value    string      `xml:"value,attr,omitempty"`
	Style    string      `xml:"style,attr,omitempty"`
	Vertex   string      `xml:"vertex,attr,omitempty"`
	Edge     string      `xml:"edge,attr,omitempty"`
	Parent   string      `xml:"parent,attr,omitempty"`
	Source   string      `xml:"source,attr,omitempty"`
	Target   string      `xml:"target,attr,omitempty"`
	Geometry *mxGeometry `xml:"mxGeometry,omitempty"`
}

// mxGeometry is the position and the size of a cell.
type mxGeometry struct {
	X        float64 `xml:"x,attr,omitempty"`
	Y        float64 `xml:"y,attr,omitempty"`
	Width    float64 `xml:"width,attr,omitempty"`
	Height   float64 `xml:"height,attr,omitempty"`
	Relative int     `xml:"relative,attr,omitempty"`
	As       string  `xml:"as,attr,omitempty"`
}
