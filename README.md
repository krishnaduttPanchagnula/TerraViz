# TerraViz — Cloud Architecture Visualizer

[![Release](https://img.shields.io/github/v/release/krishnaduttPanchagnula/TerraViz?style=flat-square)](https://github.com/krishnaduttPanchagnula/TerraViz/releases/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/krishnaduttPanchagnula/TerraViz?style=flat-square)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue?style=flat-square)](LICENSE)

Generate interactive architecture diagrams from Terraform state files or live AWS accounts. A single self-contained binary — no external dependencies, no CDN, no runtime assets.

![Enhanced UI](docs/screenshots/enhanced-ui.png)

## Installation

### Homebrew (macOS / Linux)

```bash
brew install krishnaduttPanchagnula/TerraViz/terraviz
```

The formula is automatically updated on every release.

### Download a Release

Download the latest binary for your platform from the [Releases page](https://github.com/krishnaduttPanchagnula/TerraViz/releases/latest).

```bash
# Linux (amd64)
curl -Lo terraviz.tar.gz \
  https://github.com/krishnaduttPanchagnula/TerraViz/releases/latest/download/terraviz_$(curl -s https://api.github.com/repos/krishnaduttPanchagnula/TerraViz/releases/latest | grep tag_name | cut -d '"' -f4 | sed 's/^v//')_linux_amd64.tar.gz
tar xzf terraviz.tar.gz
chmod +x terraviz
sudo mv terraviz /usr/local/bin/

# macOS (Apple Silicon)
curl -Lo terraviz.tar.gz \
  https://github.com/krishnaduttPanchagnula/TerraViz/releases/latest/download/terraviz_$(curl -s https://api.github.com/repos/krishnaduttPanchagnula/TerraViz/releases/latest | grep tag_name | cut -d '"' -f4 | sed 's/^v//')_darwin_arm64.tar.gz
tar xzf terraviz.tar.gz
chmod +x terraviz
sudo mv terraviz /usr/local/bin/
```

### Build from Source

```bash
git clone https://github.com/krishnaduttPanchagnula/TerraViz.git
cd TerraViz
go build -o terraviz ./cmd/main.go
```

To embed version info:

```bash
go build -ldflags "-s -w -X main.version=1.0.0 -X main.commit=$(git rev-parse HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o terraviz ./cmd/main.go
```

## Quick Start

```bash
# Scan a Terraform state file
terraviz scan terraform terraform.tfstate

# Serve the diagram
terraviz serve diagram.json

# Open in browser
open http://localhost:8080/enhanced
```

That's it. One binary, zero setup.

## Screenshots

### Enhanced UI — Interactive Diagram
Full-featured diagram view with zoom, pan, filtering, and search.

![Enhanced UI](docs/screenshots/enhanced-ui.png)

### Resource Details
Click any resource to see its properties, connections, and metadata.

![Resource Details](docs/screenshots/resource-details.png)

### Basic UI
Lightweight diagram view.

![Basic UI](docs/screenshots/basic-ui.png)

## Features

### Input Sources
- **Terraform State Files** — Parse `.tfstate` (v4 raw format) or `terraform show -json` output
- **Live AWS Account** — Scan EC2, S3, RDS, Lambda, VPCs, subnets, security groups, IAM, and more across multiple regions

### Interactive Web Diagrams
- SVG-based rendering with zoom, pan, and fit-to-view
- Filter by resource type, provider, region, or state
- Real-time search by resource name or ID
- Hide/show individual resources; add or delete connections at runtime
- Hover tooltips with resource details
- Two UI modes: basic (`/`) and enhanced (`/enhanced`)

### Infrastructure Comparison
- Compare two scan results to see added, removed, and modified resources
- Connection diffs (added/removed)
- Detailed per-property change reports with `--verbose`

### Self-Contained Binary
- `go build` produces a single binary (~47 MB) with all HTML templates, JavaScript, fonts, and ~620 AWS icons embedded via `go:embed`
- No external files, CDN dependencies, or runtime assets needed
- Download and run from anywhere

## CLI Reference

### Global Flags
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--output` | `-o` | `diagram.json` | Output file path |
| `--verbose` | `-v` | `false` | Verbose output |
| `--version` | | | Print version, commit, and build date |

### `scan terraform <state-file>`
Parses a Terraform state file. Supports both raw `.tfstate` (v4) and `terraform show -json` format. Auto-detects which format based on whether the JSON contains `format_version`.

### `scan aws`
Scans a live AWS account using the AWS SDK.

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--regions` | `-r` | `us-east-1` | Comma-separated AWS regions |
| `--profile` | `-p` | (default) | AWS credentials profile |

**Required IAM permissions:** `ec2:Describe*`, `s3:ListAllMyBuckets`, `s3:GetBucketLocation`, `rds:DescribeDBInstances`, `lambda:ListFunctions`, `elbv2:DescribeLoadBalancers`, `iam:ListRoles`, `iam:ListUsers`

### `compare <scan1.json> <scan2.json>`
Compares two scan result JSON files and outputs a comparison JSON. With `--verbose`, prints per-resource property diffs to stdout.

### `serve <diagram.json>`
Starts a local web server to display the interactive diagram.

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--port` | `-P` | `8080` | Port to bind |
| `--host` | `-H` | `localhost` | Host to bind |

**Routes:**
- `GET /` — Basic diagram view
- `GET /enhanced` — Enhanced diagram view (recommended)
- `GET /api/diagram` — Full diagram JSON
- `GET /api/resources` — All resources
- `GET /api/resources/search?q=<query>` — Search resources
- `GET /api/resources/filter?type=&provider=&region=&state=&tag=` — Filter resources
- `POST /api/resources/:id/hide` — Hide a resource
- `POST /api/resources/:id/show` — Show a resource
- `GET /api/connections` — All connections
- `POST /api/connections` — Create a connection
- `DELETE /api/connections/:id` — Delete a connection
- `GET /api/stats` — Diagram statistics
- `POST /api/export/:format` — Export (json supported; svg/png handled client-side)

## Supported AWS Resource Types

**Compute:** EC2 instances, Lambda functions, ECS clusters/services/task definitions
**Storage:** S3 buckets, ECR repositories
**Database:** RDS instances
**Networking:** VPCs, subnets, security groups, load balancers (ALB/NLB), listeners, target groups, Route53 zones/records
**Security:** IAM roles/users/policies, ACM certificates, WAF web ACLs, KMS keys, Secrets Manager
**Messaging:** SNS topics/subscriptions, SQS queues
**API:** API Gateway REST APIs, stages, resources, methods, integrations, domain names, usage plans, VPC links
**CDN:** CloudFront distributions
**Monitoring:** CloudWatch log groups
**Service Discovery:** Cloud Map namespaces/services

## Connection Types

Connections between resources are automatically inferred from Terraform state attributes (e.g., `vpc_id`, `subnet_id`, security group references, IAM role ARNs, target group attachments, SNS/SQS subscriptions).

| Type | Description |
|------|-------------|
| `networking` | Network-level (VPC, subnet, security group) |
| `access` | Security and permission relationships |
| `data` | Data flow connections |
| `trigger` | Event-driven connections |
| `dependency` | Resource dependencies |
| `reference` | Logical references |

## Project Structure

```
cmd/
  main.go                  CLI entry point (Cobra commands)
internal/
  models/
    models.go              Resource, Connection, Diagram, ScanResult types; icon map
    utils.go               DiagramBuilder, comparison logic, search/filter, FNV ID generation
  parsers/
    terraform.go           Terraform state parser (raw v4 + terraform-json)
    connections.go         Connection inference analyzers
  scanners/
    aws.go                 Live AWS account scanner (EC2, S3, RDS, Lambda)
  server/
    server.go              Gin HTTP server, REST API, mutex-protected state
web/
  embed.go                 go:embed directive (embeds all assets into binary)
  templates/               Go HTML templates (index.html, index_enhanced.html, error.html)
  static/
    js/                    Frontend JavaScript (diagram.js, diagram_enhanced.js)
    fonts/                 Bundled Inter + JetBrains Mono fonts (WOFF2)
  icons/                   Full AWS icon packs (~620 SVGs)
.goreleaser.yml            GoReleaser config for multi-platform releases
.github/workflows/         CI/CD (release on tag push)
```

## Releasing

Releases are automated via GitHub Actions and [GoReleaser](https://goreleaser.com/).

```bash
# Tag and push to trigger a release
git tag v1.0.0
git push origin v1.0.0
```

This builds binaries for Linux, macOS, and Windows (amd64 + arm64), creates a GitHub release with checksums and changelogs, and updates the Homebrew formula automatically.

## Development

```bash
# Install dependencies
go mod download

# Build
go build -o terraviz ./cmd/main.go

# Vet
go vet ./...

# Run with sample data
terraviz scan terraform payments-state.tfstate --output diagram.json
terraviz serve diagram.json --port 8081
```

## Key Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/gin-gonic/gin` | HTTP server |
| `github.com/hashicorp/terraform-json` | Terraform state parsing |
| `github.com/aws/aws-sdk-go-v2` | AWS API access (EC2, S3, RDS, Lambda) |
| `github.com/charmbracelet/lipgloss` | Terminal styling |

## License

MIT
