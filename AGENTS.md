# Cloud Architecture Visualizer -- Agent Guide

## What This Project Does

Go CLI tool that parses Terraform `.tfstate` files (v4 raw or `terraform show -json`) and live AWS accounts, then serves interactive SVG-based infrastructure diagrams via a Gin web server. The frontend renders zoomable, filterable diagrams in the browser.

**Repository:** `krishnaduttPanchagnula/TerraViz`

## Build / Run / Verify

```bash
go build -o terraviz ./cmd/main.go   # Build (from project root)
go vet ./...                                       # Static analysis
go test ./...                                      # Go unit tests (models, parsers)
node scripts/check-diagram-js.js                   # Frontend layout/trace self-check
```

With version info:
```bash
go build -ldflags "-s -w -X main.version=dev -X main.commit=$(git rev-parse HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o terraviz ./cmd/main.go
```

There are Go unit tests under `internal/models` and `internal/parsers`. The frontend has no test framework; `scripts/check-diagram-js.js` runs the layout and trace logic in a vm sandbox with a DOM stub.

## Project Layout

```
cmd/main.go                         Cobra CLI entry point (scan, compare, serve)
                                    Has build-time vars: version, commit, date (set via -ldflags)
internal/
  models/
    models.go                       Types: Resource, Connection, Diagram, ScanResult, ComparisonResult
                                    resourceIconMap (50+ types -> /icons/ paths)
    utils.go                        DiagramBuilder, CompareDiagrams, search/filter, FNV-1a ID generation
  parsers/
    terraform.go                    Terraform state parser (auto-detects raw v4 vs terraform-json)
    connections.go                  Connection inference from Terraform attributes + state dependency graph
    relations_test.go               Tests for dependency edges and the new relationship analysers
  relations/
    relations.go                    Shared edge collector + generic analysers (VPC/subnet/security group)
  scanners/
    aws.go                          Live AWS scanner (EC2, S3, RDS, Lambda via aws-sdk-go-v2)
  server/
    server.go                       Gin HTTP server, REST API, sync.RWMutex-protected diagram state
web/
  embed.go                          go:embed directive -- embeds templates, static, icons into binary
  templates/                        Go HTML templates (index_enhanced.html, error.html)
  static/js/                        Frontend JS (diagram_enhanced.js)
  static/fonts/                     Bundled WOFF2 fonts (Inter, JetBrains Mono) -- no CDN at runtime
  icons/                            Full AWS icon packs (~620 SVGs, served at /icons, used by both UIs)
scripts/
  check-diagram-js.js               Self-check for the frontend layout, containment and trace logic
docs/
  screenshots/                      UI screenshots for README (enhanced-ui, resource-details)
.goreleaser.yml                     GoReleaser v2 config (linux/darwin/windows x amd64/arm64)
.github/workflows/release.yml      GitHub Actions -- triggers on v* tags, runs GoReleaser
.gitignore                          Covers binaries, JSON artifacts, .tfstate, IDE files, node_modules
go.mod                              Module: terraviz, Go 1.25.0
```

All static assets (templates, JS, fonts, icons) are embedded into the binary via `go:embed`.
The built binary is fully self-contained -- no external files or CDN dependencies needed at runtime.

## Do NOT Modify

- `web/embed.go` -- go:embed directive, no logic to change

## Release Process

Releases are automated. Tag and push to trigger:
```bash
git tag v1.0.0
git push origin v1.0.0
```

The GitHub Actions workflow (`.github/workflows/release.yml`) will:
1. Check out the code with full history
2. Set up Go (version from go.mod)
3. Run `go vet` and a test build
4. Run GoReleaser, which builds for 6 platform/arch combos with `-ldflags` for version/commit/date
5. Create a GitHub release with archives + checksums

GoReleaser config (`.goreleaser.yml`):
- Binary name: `terraviz`
- Platforms: linux/darwin/windows x amd64/arm64
- CGO_ENABLED=0, stripped (`-s -w`)
- Archives: tar.gz (zip for Windows)
- Changelog: excludes docs/test/chore commits
- Homebrew formula: auto-updated in `HomebrewFormula/` on each release

### Homebrew Tap

The formula lives in `HomebrewFormula/terraviz.rb` (same repo).
Users install via: `brew install krishnaduttPanchagnula/TerraViz/terraviz`

GoReleaser auto-generates and commits the formula on every non-prerelease tag.
Requires a `HOMEBREW_TAP_TOKEN` secret (PAT with `repo` scope) in GitHub repo settings.

## CLI Commands

| Command | Description |
|---------|-------------|
| `scan terraform <file>` | Parse a .tfstate file, output diagram JSON |
| `scan aws` | Scan live AWS account (flags: `--regions`, `--profile`) |
| `compare <a.json> <b.json>` | Compare two scan results, output diff JSON |
| `serve <diagram.json>` | Start Gin server (flags: `--host`, `--port`) |

Global flags: `--output` / `-o` (default `diagram.json`), `--verbose` / `-v`, `--version`

## Key Patterns and Conventions

### Go Style
- Module name: `terraviz`
- All errors wrapped with `fmt.Errorf("context: %w", err)` (never `%v`)
- `os.ReadFile` / `os.WriteFile` (never `io/ioutil`)
- `hash/fnv` for ID generation (not `crypto/md5`)
- `log/slog` for structured logging in scanners (not `fmt.Printf`)
- Import aliases for AWS types: `ec2types`, `s3types`, `rdstypes`, `lambdatypes`

### Architecture
- `DiagramBuilder` is the central construction API -- all parsers and scanners use it
- `DiagramBuilder.AddResource` stores resources in a slice and keeps a pointer map (`resourceMap`) pointing into that slice. The pointer is set to `&db.diagram.Resources[len(...)-1]` after append to avoid stale pointers.
- `DiagramBuilder.AddResource` auto-populates `Resource.IconURL` via `GetResourceIcon()` when empty
- `Server.diagram` is protected by `sync.RWMutex` -- read handlers use `RLock`, mutation handlers (hide/show, create/delete connection) use `Lock`
- Connection analyzers in `connections.go` use helper functions: `findByProperty`, `findAllByProperty`, `findAllByType`, `connectToSecurityGroups`
- `terraformTypeMap` and `resourceIconMap` are package-level vars (not rebuilt per call)
- Unmapped Terraform types are kept as `ResourceTypeUnknown` with `Properties["terraform_type"]` set (AWS providers only); the frontend groups them by that value
- Edges point from the referencing resource to the referenced one. Containment edges (instance -> subnet -> VPC) are rendered as nesting, not as arrows
- A semantic edge wins over the generic Terraform dependency edge for the same pair (`relations.Collector.AddDependency`)
- `internal/relations` is shared by the Terraform parser and the live AWS scanner: `AnalyzeGeneric` derives the VPC/subnet/security group edges from normalized property keys (`id`, `vpc_id`, `subnet_id`, `vpc_security_group_ids`, `ingress`), so both sources produce the same edges
- `relations.Collector` keeps one edge per unordered pair: extra reasons go to `Properties["reasons"]`, opposite directions set `Bidirectional`

### Icon System
- `resourceIconMap` in `models.go` maps 50+ `ResourceType` constants to `/icons/` paths (matching the embedded `web/icons/` AWS icon packs)
- `DefaultResourceIcon` = `/icons/General-Icons/Marketplace_Dark.svg`
- The JS frontend (`diagram_enhanced.js`) prefers `resource.icon_url` from the API, falling back to its own local icon map
- Icons are served from the embedded filesystem at `/icons/`

### Types to Know
- `models.ScanResult` -- Top-level output of any scan (contains `Diagram`, `Stats`, `Errors`, `Warnings`)
- `models.ComparisonResult` -- Output of `CompareDiagrams()` (contains `Added`, `Removed`, `Modified`, `ConnectionsAdded`, `ConnectionsRemoved`, `Summary`)
- `models.ComparisonSummary` -- Has `AddedCount`, `RemovedCount`, `ModifiedCount`, `UnchangedCount`
- `models.ResourceType` -- String type, e.g. `"aws:ec2:instance"`, `"aws:s3:bucket"`
- `models.ConnectionType` -- One of: `networking`, `access`, `data`, `trigger`, `dependency`, `reference`

### Server Routes
The enhanced UI is served at `/` (it is the only UI; `server.go:handleEnhancedIndex`).
API routes are under `/api/` -- see `server.go:setupRoutes()` for full list.

## Dependencies

| Package | Purpose |
|---------|---------|
| `spf13/cobra` | CLI framework |
| `gin-gonic/gin` | HTTP server |
| `hashicorp/terraform-json` | Terraform state types |
| `aws/aws-sdk-go-v2` | AWS API (EC2, S3, RDS, Lambda) |
| `charmbracelet/lipgloss` | Terminal styling |

## Common Tasks

### Adding a new Terraform resource type
1. Add `ResourceType` constant in `models/models.go`
2. Add mapping in `terraformTypeMap` in `parsers/terraform.go`
3. Add connection analyzer in `parsers/connections.go` (use existing helpers)
4. Add icon mapping in `resourceIconMap` in `models/models.go`
5. Build and vet: `go build -o terraviz ./cmd/main.go && go vet ./...`

### Adding a new AWS service to the live scanner
1. Add AWS SDK import in `scanners/aws.go` with consistent alias (e.g., `svcTypes`)
2. Add scan method (e.g., `scanDynamoDB`) following existing patterns
3. Call it from `ScanAccount` with proper error collection
4. Build and vet

### Adding a new API endpoint
1. Add handler method on `*Server` in `server/server.go`
2. Register route in `setupRoutes()`
3. Use `requireDiagram()` helper; use `s.mu.Lock()`/`s.mu.RLock()` appropriately
4. Build and vet

### Creating a new release
1. Ensure all changes are committed and pushed
2. Tag: `git tag v1.x.x`
3. Push: `git push origin v1.x.x`
4. GitHub Actions will build and publish automatically
