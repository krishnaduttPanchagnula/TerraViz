# Competitor Research: Terraform / Cloud Architecture Visualizers

**Date:** 2026-10-22
**Scope:** Tools that produce a visual/architectural representation of real or IaC-described cloud infrastructure, and are therefore substitutable for, or adjacent to, TerraViz.
**Method:** Web search + direct page/repository fetches. Every feature claim in this document and in `feature-evidence.md` comes from a page that was actually fetched. Where a vendor's own documentation did not state something, it is marked **unverified** rather than inferred.

---

## 1. Benchmark matrix

Five benchmarks were selected. The selection rule was: *a tool a TerraViz user could plausibly install instead of TerraViz, or a tool that owns one end of the same job* (import live cloud / render IaC state / keep diagrams current in CI).

| # | Competitor | Positioning & target user | Core product surface | Notable features for this category | Pricing / packaging | Content & SEO surface | Status |
|---|---|---|---|---|---|---|---|
| 1 | **Cloudcraft** (Datadog) — https://www.cloudcraft.co/ , docs https://docs.datadoghq.com/cloudcraft/ | Live, read-only architecture diagram of infrastructure you already run. Target: platform/SRE/FinOps engineers who already pay for Datadog. | Connect a live AWS/Azure/EKS/AKS account → the app generates and continuously refreshes a 3D (or 2D) isometric diagram. Click a node to open a panel with observability, cost and security data. | Live vs Snapshot diagram modes; Group By (VPC, Region, Network ACL, and cloud tags); Saved views; tag-based filtering with AND/OR/NOT search patterns; 5 overlays (Infrastructure, Observability, Security, Cloud Cost Management, APM); version history with naming, restore and "save version as blueprint"; full REST API at `api.cloudcraft.co` with Go/Python/Node SDKs; per-blueprint/account read/write ACLs; Confluence embedding | **Free inside Datadog** — "unlimited users, unlimited diagrams" with any Datadog subscription (https://www.cloudcraft.co/pricing). Standalone product still sold with a free manual-design tier and a paid per-user tier for live account connection; standalone per-user prices not published on the pricing page I fetched (**unverified**). Older third-party figures quoted $49–99/user/mo (secondary source, unverified) | docs.datadoghq.com/cloudcraft (standalone + "in Datadog"), blog.cloudcraft.co, Datadog release blog, API reference with OpenAPI spec and SDKs | **Alive.** Datadog-owned. Live + snapshot modes, wide cloud coverage, 2D/3D |
| 2 | **TerraVision** — https://github.com/patrickchugh/terravision , docs https://patrickchugh.github.io/terravision/ | Free OSS, client-side cloud architecture diagram generator, both directions: design→code and code→diagram. Target: engineers who want a diagram in their AI assistant and in their repo. | `terravision draw --source ./infra` runs `terraform plan` and renders an AWS/Azure/GCP architecture diagram with official icons. `terravision visualise` emits a self-contained interactive HTML page. Also a JSON-graph input format. | Built from `terraform plan` (so `count`/`for_each`/conditionals/modules resolve); multi-cloud with 427 AWS / 267 GCP / 263 Azure resource types; resources nested in region → AZ → VPC → subnet → security group; official icon sets; `--simplified` executive view; `--varfile` to draw prod/dev from code with no account access; `--ai-annotate` with Ollama / Bedrock / any OpenAI-compatible endpoint; MCP server + Claude/Codex/Copilot plugins + agent skill; GitHub Action and CI/CD integration; editable draw.io export; PNG/SVG/PDF; Terragrunt auto-detect | Free, AGPL-3.0, MIT-licensed CLI via `pipx`/`uv`/PyPI + Docker + Nix | Full MkDocs site (install, usage, CI/CD, MCP, gallery of example diagrams, `llms.txt`, comparison page "vs draw.io, Lucidchart, Eraser, Mermaid, live cloud scanners", per-cloud landing pages "AWS/Azure/GCP architecture diagram generator") | **Alive and very active** — last repo push 2026-10-22 (GitHub API), 1,660 stars, 1,065 commits |
| 3 | **Rover** — https://github.com/im2nguyen/rover | OSS "Interactive Terraform visualization. State and configuration explorer." Target: engineers who want a local web UI over a Terraform plan. | Runs `terraform plan`, parses plan + config, generates three artifacts — **resource overview**, **resource map**, **resource graph** — and serves an interactive visualization on `0.0.0.0:9000`. Docker-first. | Three distinct views over the same graph (overview / map / graph); plan-based, so planned changes are visible; `-planJSONPath` to run from a saved plan; `-genImage` to emit an SVG; `-tfBackendConfig` / `-tfVarsFile` / `-tfVar`; standalone mode emits a `rover.zip` with static assets openable in a browser; release binaries for linux/darwin/freebsd/windows amd64+arm64 | Free, MIT, Go, 3,348 stars, Docker + release zips | README only; no dedicated docs site (docs/ contains two screenshots) | **Low activity.** Latest release v0.3.3 published 2022-07-22; last repo push 2025-07-30; 156 commits total; 45 open issues. Building from source needs Go 1.21+ **and npm** |
| 4 | **Pluralith** — https://www.pluralith.com/ , docs https://docs.pluralith.com/ | Automated Terraform infrastructure visualisation and documentation for teams. Target: platform teams that want a fresh diagram on every PR without anyone drawing it. | `pluralith graph` runs `terraform plan` under the hood, draws a diagram, pushes it to the Pluralith dashboard, and posts it as a **pull-request comment**. Dedicated CI integrations for GitHub Actions, GitLab CI and Terraform Cloud/Enterprise. | Plan change highlighting; **drift detection**; cost overlay via Infracost (`--show-costs`, `--cost-mode delta|total`, `--cost-period hour|month`, `--cost-usage-file`); compliance via Regula (marked "Coming Soon"); diagram history in a hosted dashboard; `pluralith.yml` config with `hide`/`show`/`groups`/`ungroup`/`relations`/`unrelate` overrides addressed at module/resource granularity; `--local-only` PDF export; `--title`/`--version`/`--author` metadata; IAM in the paid tier; Markdown integration; access management | **Free Forever** for local development. Paid: flat packages by CI runs/month — **$250/mo for 500 CI runs**, +$0.60 per run beyond 500. Includes diagrams-as-PR-comments, Terraform change history, cost integration, IAM, unlimited users / projects / exports | docs.pluralith.com (Get started, CLI commands, CI integrations, config, diagram customization), GitHub `Pluralith/pluralith-cli`, HashiCorp resource "Pluralith: Developing a Proper Way to Visualize State" | **Alive, commercial.** CI-first, not local-first; requires an account + API key for anything beyond the local CLI |
| 5 | **Inframap** (Cycloid) — https://github.com/cycloidio/inframap , https://www.cycloid.io/open-source/inframap | OSS IaC visualiser: "Read your tfstate or HCL to generate a graph specific for each provider, showing only the resources that are most important/relevant." Target: engineers who want a clean architecture picture instead of `terraform graph` noise. | `inframap generate <state.tfstate|module-dir>` → Graphviz **DOT** on stdout, piped to `dot -Tpng`. A `prune` subcommand is listed for stripping unnecessary information from state/HCL so it can be shared safely, though the README still annotates it "(not supported yet)". | Provider-aware pruning (AWS has specific logic: external nodes ✔, grouping "WIP", IAM ✘ open issue #11; Google/Azure/OpenStack/FlexibleEngine fall back to unfiltered output); `--raw`, `--clean=false`, `--connections=false`, `--description-file`, `--alternative-node-names`; reads state v3 and v4 and HCL; **no** direct backend support (documented stdin/out workaround: `terraform state pull \| inframap generate`); Docker image with Graphviz baked in; Homebrew + AUR; MIT, Go, 2.1k stars | Free, MIT, Go; single static binary + Docker | GitHub README as the main doc, Cycloid marketing/blog pages, `awesome-tf` and `awesome-*` Terraform curated lists list it | **Alive.** Latest release v0.8.1 published 2026-04-23 (GitHub API). Static DOT output only — no browser UI |

### Why these five

- **Cloudcraft** is the commercial category incumbent and the only benchmark that owns "live cloud → diagram" as its whole product. It sets the bar for overlays, saved views, versioning and API surface.
- **TerraVision** is the closest open-source peer in *feature completeness* (multi-cloud, plan-derived, AI, MCP, CI) and is the fastest-moving project in the category. It is the direct read-across for what TerraViz's open-source peers can do.
- **Rover** is the closest peer in *stack and interaction model* (Go binary, local web server, plan-derived) and is the most-starred OSS tool in the set (3,348 stars).
- **Pluralith** is the commercial peer that owns the "keep the diagram current in CI" job — the job TerraViz's `compare` command only half-covers.
- **Inframap** is the closest peer in *language and pruning philosophy* (Go, MIT, provider-aware graph simplification) and is the reference point for the "reduce the graph to what matters" school.

### Adjacent tools observed (not benchmarked in depth)

| Tool | URL | Positioning / notes | Pricing | Status |
|---|---|---|---|---|
| **HashiCorp Atlas** | https://www.hashicorp.com/en/blog/hashicorp-terraform-enterprise-general-availability | HashiCorp's hosted enterprise product (2015–2018) that included a visual view of infrastructure state. **Explicitly retired**: "The Atlas version of Terraform Enterprise will be decommissioned on Friday, March 30, 2018"; the hosted Packer offering and Artifact Registry features of Atlas were decommissioned 2018-05-31; "Atlas" was renamed to Terraform Enterprise. | n/a | **Retired / decommissioned (2018)** |
| **HashiCorp Infragraph** (HCP Terraform resource graph) | https://developer.hashicorp.com/hcp/docs/infragraph , https://www.hashicorp.com/en/blog/hcp-terraform-powered-by-infragraph-limited-availability-launch | The modern successor to Atlas-era visual infra: a centralised resource-graph service with connectors (AWS, Azure, GitHub, Terraform Enterprise), a graph explorer and a query builder | Limited availability for HCP Terraform Standard and Premium customers, US instances only | **Alive, limited availability.** Documented constraints: one instance per connection type, AWS connector limited to 100 accounts, "only a subset of planned AWS resources and resource fields are supported", "query API validation is limited" |
| **AWS Infrastructure Composer** (ex Application Composer) | https://docs.aws.amazon.com/infrastructure-composer/latest/dg/what-is-composer.html | Visual drag-and-drop IaC editor for CloudFormation/SAM; generates IaC templates in real time | No additional charge (pay only for provisioned resources) | **Being retired** — "no longer available to new customers as of October 29, 2026"; the standalone console end-of-support date is **December 7, 2026**; the experience moves to the AWS Toolkit for VS Code |
| **awslabs/diagram-as-code** | https://github.com/awslabs/diagram-as-code | OSS Go CLI (`awsdac`) that renders AWS architecture diagrams from YAML; compliant with AWS diagram guidelines; Go templates; MCP server; CloudFormation→diagram conversion (beta). 1.6k stars, Apache-2.0 | Free, Apache-2.0 | Alive |
| **Blast Radius** | https://github.com/28mm/blast-radius , https://28mm.github.io/blast-radius-docs/ | The canonical "blast radius" interactive Terraform dependency graph: Graphviz layout + d3.js, click a resource to highlight its dependencies and dependents, search, pan/zoom, prune-to-selection | Free, MIT, `pip install blastradius` | **Effectively dormant.** Last commit on the default branch 2020-01-06; last repo push 2024-08-14; 47 open issues. Spacelift (https://spacelift.io/blog/terraform-visualization) states it "has compatibility issues with Terraform 1.x+ and is no longer actively maintained as of 2025" |
| **Terraform Visual** | https://github.com/hieven/terraform-visual | OSS plan→interactive HTML report; `@terraform-visual/cli`; Docker image for CI | Free, MIT | Alive but small (675 stars, 19 commits) |
| **IcePanel** | https://icepanel.io/pricing | Model-based C4 architecture platform (adjacent leader: versioning, decision records, collaboration, AI summaries) | Free ($0, 1 editor / 100 objects) → Growth $40/editor/mo annual → Scale $80/editor/mo (20+ editors) → Enterprise custom | Alive |
| **Brainboard** | https://www.brainboard.co/pricing | Design-to-IaC platform: visual design generates Terraform; drift detection; GitOps; CI/CD | Free ($0, unlimited architectures + Terraform codegen) → Pro $99/user/mo → Enterprise custom (SSO, self-hosted runner, per-resource import fee) | Alive |
| **Lucidscale** | https://lucid.co/alternative/lucidscale-vs-cloudcraft | Automated live cloud import (AWS/Azure/GCP) inside Lucidchart | Priced separately from Lucidchart; per-seat (unverified on page fetched) | Alive |
| **Cloudairy** | https://cloudairy.com/ai/ai-terraform-diagram-maker | AI workspace: prompt→diagram, HCL/tfstate paste, multi-cloud icons, real-time collaboration | Free tier; paid tiers (unverified) | Alive |
| **AWS Resource Explorer** | https://docs.aws.amazon.com/resource-explorer/latest/userguide/welcome.html | Native AWS cross-region / multi-account resource **search** (not a diagrammer) | Included with AWS | Alive |
| **draw.io / Lucidchart / Hava.io / Holori / Overmind / eagle-eye / Datadef / InfraSketch / Spacelift** | — | Manual cloud-shape libraries, other IaC visualisers, and IaC platforms that also draw stacks | Varies | Mix of alive and small |

---

## 2. Feature inventory

Legend for the TerraViz column: **Y** = has the capability today · **P** = partial (exists but narrower / not in the UI / not the default path) · **N** = no · **?** = not documented on the pages fetched.

Columns: **CC** = Cloudcraft · **TV** = TerraVision · **RV** = Rover · **PL** = Pluralith · **IM** = Inframap · **TerraViz**.

### 2.1 Input sources

| Feature | CC | TV | RV | PL | IM | TerraViz |
|---|---|---|---|---|---|---|
| Raw `.tfstate` v4 | N | N | N | N | **Y** | **Y** |
| `terraform show -json` state output | N | N | N | N | N | **Y** |
| Terraform HCL / config (code-first) | N | **Y** | **Y** | **Y** | **Y** | N |
| `terraform plan` / plan JSON | N | **Y** | **Y** | **Y** | N | N |
| Live cloud account import | **Y** | N | N | N | N | **Y** (7 AWS types) |
| Multi-region live scan | **Y** | N | N | N | N | **Y** (`--regions`) |
| Multi-account live scan | **Y** | N | N | N | N | **P** (one account per run) |
| Remote state from S3, browsable in the UI | N | N | N | N | N | **Y** |
| Native remote-backend support (S3/GCS/swift) | N | N | N | N | N | N (Inframap documents a stdin/out workaround only) |
| Terragrunt project support | N | **Y** | N | N | N | N |
| CloudFormation template input | N | N | N | N | N | N |

### 2.2 Diagram & graph model

| Feature | CC | TV | RV | PL | IM | TerraViz |
|---|---|---|---|---|---|---|
| AWS-style containment (VPC → subnet → resource) | **Y** | **Y** | ? | ? | **P** (AWS grouping is "WIP") | **Y** |
| Official cloud icon sets | **Y** | **Y** (AWS/GCP/Azure) | ? | ? | N (text nodes) | **Y** (~620 AWS SVGs embedded) |
| Multi-cloud provider coverage | **Y** (AWS/Azure/GCP/OCI/vSphere) | **Y** (AWS/GCP/Azure) | provider-agnostic | not documented | **P** (AWS only has specific logic) | N (AWS only) |
| Edges from Terraform's own dependency graph | N | **Y** | **Y** | **Y** | **Y** | **Y** |
| Service-semantic edge types (networking / access / data / trigger / dependency / reference) | ? | ? | ? | ? | N | **Y** (6 types + legend) |
| Edge metadata (protocol, ports, CIDR, merged reasons) | ? | ? | ? | ? | N | **Y** |
| Inference of referenced-but-absent resources | ? | ? | ? | ? | **P** ("external nodes" for ingress only) | **Y** (`in_state_file: false`) |
| Layout ordered by dependency position, not a fixed list | N | ? | ? | ? | N | **Y** |
| Simplified / executive high-level view | ? | **Y** (`--simplified`) | ? | ? | **P** (pruning by provider) | N |
| 3D isometric rendering | **Y** | N | N | N | N | N |
| Provider-aware graph pruning ("show what matters") | N | **P** (`--simplified`) | N | **Y** (auto hide/show) | **Y** | **P** (filters, not automatic pruning) |

### 2.3 Interaction

| Feature | CC | TV | RV | PL | IM | TerraViz |
|---|---|---|---|---|---|---|
| Zoom / pan / fit-to-view | **Y** | **Y** | **Y** | ? | N (static) | **Y** |
| Minimap | N | N | ? | N | N | **Y** |
| Search by resource name or ID | **Y** | **Y** | ? | ? | N | **Y** |
| Filters (type, provider, region, state, tag) | **Y** | ? | ? | ? | N | **Y** |
| Saved views / filter presets | **Y** | N | N | N | N | N |
| Group by cloud tags (team, service, env) | **Y** | N | N | N | N | N |
| Click-to-inspect resource detail panel | **Y** | **Y** | **Y** (resource overview) | ? | N | **Y** |
| Hover tooltips on nodes/edges | **Y** | ? | ? | ? | N | **Y** |
| Upstream + downstream dependency highlight | N | ? | ? | ? | N | **Y** |
| Selectable trace depth (1 / 2 / 3 / all hops) | N | N | N | N | N | **Y** |
| Hide / show resources | N | ? | N | **Y** (config file) | N | **Y** (runtime) |
| Add / delete connections at runtime | N | N | N | **Y** (config file) | N | **Y** (runtime, via REST) |
| Resource table / list view | N | N | **Y** (resource overview) | ? | N | **Y** |
| Presentation mode / flow animation | N | **Y** (animated data flow) | N | N | N | **Y** |
| Dark / light theme | ? | ? | ? | ? | N | **Y** |

### 2.4 Output & integration

| Feature | CC | TV | RV | PL | IM | TerraViz |
|---|---|---|---|---|---|---|
| PNG export | **Y** | **Y** | **Y** | ? | **Y** (via Graphviz) | **Y** |
| SVG export | **Y** | **Y** | **Y** (`-genImage`) | ? | **Y** (via Graphviz) | **Y** |
| CSV export | ? | N | N | N | N | **Y** |
| PDF export | ? | **Y** | N | **Y** (local-only mode) | N | N |
| Mermaid flowchart export | N | N | N | N | N | **Y** |
| Graphviz DOT export | N | N | N | N | **Y** (native) | **Y** |
| Editable draw.io export | N | **Y** | N | N | N | N |
| Visio / PowerPoint export | N | N | N | N | N | N |
| Single-file static HTML artefact | N | **Y** | **Y** (standalone zip) | N | N | N (needs the local server) |
| REST API for programmatic access | **Y** (+ Go/Python/Node SDKs) | N | N | N (CLI + dashboard) | N | **Y** (`/api/*`) |
| CI pipeline integration | ? | **Y** (GitHub Action, GitLab/Jenkins/Azure DevOps) | N | **Y** (Actions/GitLab/TFC) | N | N |
| Diagram posted as a PR comment | N | N | N | **Y** | N | N |
| Diagram-as-code authoring (YAML/JSON in) | N | **Y** (JSON graph) | N | N | N | N |
| Reverse: diagram → Terraform | N | **Y** (via MCP) | N | N | N | N |
| MCP server / agent-skill integration | N | **Y** | N | N | N | N |
| AI-generated annotations / summaries | N | **Y** (`--ai-annotate`) | N | N | N | N |
| Local-LLM option (no data leaves the machine) | N | **Y** (Ollama) | N | N | N | N |
| Machine-re-readable graph text export | N | N | N | N | **Y** (DOT) | **Y** (DOT + Mermaid + CSV) |

### 2.5 Collaboration & platform

| Feature | CC | TV | RV | PL | IM | TerraViz |
|---|---|---|---|---|---|---|
| Hosted dashboard with run/plan history | N (version history only) | N | N | **Y** | N | N |
| Team workspaces / orgs | N | N | N | **Y** (org + project) | N | N |
| Real-time multi-user editing | N | N | N | N | N | N |
| Comments on diagrams | N | N | N | N | N | N |
| RBAC / access control | **Y** (read/write ACLs per team/customer) | N | N | **Y** (IAM, paid tier) | N | N |
| SSO / SAML | N | N | N | N | N | N |
| Audit logs | N | N | N | N | N | N |
| Versioned diagrams with restore | **Y** | N | N | **Y** (`--version` + plan history) | N | N |
| Share links / embeds | **Y** (Confluence app) | N | N | N | N | N |
| Blueprints / reusable templates | **Y** | N | N | N | N | N |

### 2.6 Operational overlay

| Feature | CC | TV | RV | PL | IM | TerraViz |
|---|---|---|---|---|---|---|
| Per-resource cost overlay | **Y** (CCM) | N | N | **Y** (Infracost) | N | N |
| Cost mode / period control | ? | N | N | **Y** (delta vs total; hour vs month) | N | N |
| Budget / projected cost for a *proposed* design | **Y** (Cloudcraft Budget) | N | N | N | N | N |
| Drift detection | N | N | N | **Y** | N | **P** (manual `compare` of two snapshots) |
| Plan change highlighting in the diagram | N | N | N | **Y** | N | **P** (added/removed/modified resources, not plan actions) |
| Compliance / policy check overlay | N | N | N | **Y** (Regula, "Coming Soon") | N | N |
| Security misconfiguration overlay | **Y** (Cloud Security) | N | N | N | N | N |
| Observability overlay (agent coverage, logs) | **Y** | N | N | N | N | N |
| APM / distributed-trace overlay | **Y** (AWS only, preview) | N | N | N | N | N |
| Ownership / tag heatmap | **Y** (colour-coded tag groups) | N | N | N | N | N |
| Security-group / network-permission edge detail | ? | N | ? | N | N | **Y** (protocol, from/to port, CIDR) |

### 2.7 Packaging, licensing & coverage

| Feature | CC | TV | RV | PL | IM | TerraViz |
|---|---|---|---|---|---|---|
| Runs fully local, no account required | N (Datadog account) | **Y** | **Y** | **P** (CLI local, account for the rest) | **Y** | **Y** |
| Single self-contained binary | N | N (Python + Graphviz + Git) | **Y** | N | **Y** | **Y** |
| No external runtime dependency (no Node / Graphviz / Python / Terraform binary) | N | N | N | N | N | **Y** |
| Open-source license | N | **Y** (AGPL-3.0) | **Y** (MIT) | N (product) | **Y** (MIT) | **Y** (MIT) |
| Free, no per-seat pricing | N | **Y** | **Y** | **P** | **Y** | **Y** |
| Homebrew / package-manager install | N | **Y** (pipx/uv, Docker, Nix) | **Y** (release zips) | ? | **Y** (brew, AUR, Docker) | **Y** (brew, releases, GoReleaser) |
| Multi-OS builds | N (hosted) | N (hosted) | **Y** (linux/darwin/freebsd/windows) | N (hosted) | **Y** (linux/docker) | **Y** (linux/darwin/windows × amd64/arm64) |
| Bundled assets, no runtime CDN | N | ? | ? | N | N | **Y** (fonts + icons via `go:embed`) |
| Terraform resource-type coverage | hundreds (live, multi-cloud) | 427 AWS / 267 GCP / 263 Azure | provider-agnostic | not documented | provider subsets | ~21 AWS services mapped + generic fallback for unmapped types |
| Live-scan resource coverage | broad | n/a | n/a | n/a | n/a | 7 types: EC2, VPC, subnet, SG, S3, RDS, Lambda |

---

## 3. Underserved angles

Each of these is an area where **none of the five benchmarks does the job well**, verified against the sources cited in `feature-evidence.md`.

1. **Zero-dependency, zero-account, single-binary state visualiser with real architecture semantics.**
   The commercial tools all require an account or an agent (Datadog, Pluralith dashboard). The OSS peers all require a runtime stack: TerraVision needs Python + Graphviz + Git, Rover needs Node to build and npm-based UI, Inframap needs Graphviz to render. Only TerraViz ships a single Go binary with the HTML, JS, fonts and ~620 AWS icons embedded, and runs with no account, no network and no install-time dependencies. Nobody else occupies "run it on a locked-down laptop and it just works".

2. **One graph, many machine-readable renderers, offline.**
   Pluralith emits PDF; Inframap emits DOT only; Cloudcraft emits images and JSON; TerraVision emits PNG/SVG/PDF/draw.io and an HTML page. Nobody emits **Mermaid** from a Terraform dependency graph, and nobody pairs Mermaid with DOT and CSV from the same source graph. That matters because Mermaid renders in GitHub/GitLab markdown, docs sites and wikis, which is where architecture documentation actually lives.

3. **"Referenced but missing" as a first-class finding.**
   Inframap has "external nodes" but scoped to ingress. No benchmark surfaces *every* resource referenced by an identifier/ARN in state that is not itself in the state, as a reviewable list. For onboarding onto an inherited codebase, this is the single most useful output, and TerraViz already computes it (`in_state_file: false`).

4. **Depth-controlled, semantically typed dependency tracing.**
   Blast Radius (the original) offered "highlight this resource's dependencies and dependents" and is dormant — last default-branch commit 2020-01-06, and Spacelift documents Terraform 1.x compatibility problems. No maintained benchmark offers a **selectable trace depth (1/2/3/all hops)** over a **typed** edge set (networking / access / data / trigger / dependency / reference) with per-edge protocol and port metadata. TerraViz has the trace but the edge semantics are the differentiator that no peer has.

5. **Local, account-free infrastructure diffing.**
   Pluralith's change history, drift detection and cost overlay all live behind its hosted dashboard and a CI integration. TerraViz's `compare` command diffs two scan snapshots on the machine: added/removed/modified resources, connection diffs, per-property change reports — no account, no pipeline, no vendor.

6. **Live scan + diff as a drift check without a paid agent.**
   The two halves exist separately elsewhere: Pluralith has drift but needs Infracost + an account; Cloudcraft has live data but needs Datadog; Inframap has a `prune` command but no live scan. TerraViz already has a live AWS scanner *and* a diff engine — the seam between them (scan live → diff against the state-derived diagram → report drift) is unclaimed by anyone in this set.

7. **Documentation-grade exports for review artefacts, not just pictures.**
   CSV of the full resource inventory (TerraViz has it) plus a diff report are what compliance and audit actually consume. Cloudcraft's API can dump a resource inventory as JSON; Inframap lists a `prune` command for sanitising state for sharing (its README still marks it "not supported yet"). No benchmark combines *sanitised state* + *inventory CSV* + *diff report* into one offline documentation bundle.

8. **State-first, plan-agnostic visualisation of what actually exists.**
   Every OSS plan-based peer (TerraVision, Rover, Pluralith, Terraform Visual) needs `terraform plan`, which needs credentials, an initialised backend, and often a live target account. TerraViz reads the state file and renders what exists, with no plan and no credentials. That is a different job — post-incident forensics, audits, and reading an inherited state file — that no plan-first tool can do.

9. **No-overlay, no-lock-in "just show me the topology".**
   Cloudcraft's value depends on Datadog data; Pluralith's on Infracost/Regula; IcePanel's on a hosted model. The unoccupied slot is a dependency-free, telemetry-free, zero-account topology explorer that a team can run on a laptop and hand to an auditor. TerraViz's bundled-icons + embedded-assets + no-CDN design is already built for that, but nobody has written the content for it.

---

## 4. Where TerraViz stands

### Behind (competitors do this today)

| Gap | Who does it | Note |
|---|---|---|
| CI/PR integration | Pluralith, TerraVision | TerraViz has no pipeline integration; both peers post diagrams to PRs |
| Plan-based visualisation | TerraVision, Rover, Pluralith | TerraViz reads state only; the README states "Full Terraform plan action visualization is not currently supported" |
| Multi-cloud | TerraVision, Cloudcraft | TerraViz is AWS-only; Azure/GCP is planned in `docs/gcp_azure_implementation.md` |
| Cost overlay | Cloudcraft, Pluralith | TerraViz has no cost data at all |
| Drift detection | Pluralith | TerraViz has manual `compare`, not drift |
| Always-current live diagram | Cloudcraft | TerraViz requires a manual re-scan; no live refresh |
| Diagram history / versioning | Cloudcraft, Pluralith | TerraViz has no persisted diagram versions |
| AI / agent integration (MCP, skills, LLM annotations) | TerraVision | TerraViz has none |
| Team collaboration, RBAC, SSO, share links | Cloudcraft, Pluralith | TerraViz is single-user by design |
| Extra export formats (PDF, draw.io) | TerraVision, Pluralith | TerraViz has PNG/SVG/CSV/Mermaid/DOT |
| Live-scan breadth | Cloudcraft | TerraViz live scanner covers 7 resource types |

### Ahead (TerraViz does this today)

| Capability | Evidence in this study |
|---|---|
| Single self-contained binary; no Node, Graphviz, Python or Terraform binary needed; no CDN | Every OSS peer in the set has an external runtime dependency; Cloudcraft and Pluralith need accounts |
| Mermaid export from a Terraform dependency graph | No benchmark offers Mermaid |
| Multi-hop trace with selectable depth (1/2/3/all) over both directions | No benchmark offers depth control; the closest (Blast Radius) is dormant since 2020 |
| Typed semantic edges (6 types) with legend, protocol/port/CIDR metadata and merged reasons | No benchmark documents typed semantic edges with this metadata |
| Referenced-but-absent resource inference as a first-class node set | Inframap has "external nodes" for ingress only |
| Local account-free diffing of two scan snapshots (resources, connections, per-property) | Pluralith's equivalent is dashboard + CI only |
| S3 remote-state browsing and loading directly from the browser UI | No benchmark does this in-UI; Inframap documents that it cannot read backends |
| Runtime connection editing (add/delete) through a REST API | Only Pluralith's config-file `relations`/`unrelate` is comparable, and it is static config |
| Embedded AWS icon set (~620 SVGs) and fonts, served locally | TerraVision also bundles official icons; Inframap and Rover do not |
| AWS security-group rule detail on edges (protocol, from/to port, CIDR blocks) | No benchmark documents this |

---

## 5. Sources

Every URL below was fetched during this research. Notes describe what was taken from each.

### Benchmarks

- https://www.cloudcraft.co/ — homepage: "Live AWS, Azure, GCP infrastructure diagrams", multi-account/multi-region, overlay descriptions (cost heatmap, security findings, observability).
- https://www.cloudcraft.co/pricing — "Cloudcraft is free in Datadog · Unlimited users · Unlimited diagrams" with any Datadog subscription; no standalone prices published on the page.
- https://docs.datadoghq.com/cloudcraft/ — standalone vs in-Datadog split; doc index.
- https://docs.datadoghq.com/cloudcraft/getting-started/ — feature index (AWS/Azure/EKS/AKS connect, group-by, filters, live vs snapshot, version history, Confluence embed, API keys).
- https://docs.datadoghq.com/cloudcraft/getting-started/live-vs-snapshot-diagrams/ — Live diagrams auto-update; Snapshot diagrams are editable; switching back to Live creates a new version.
- https://docs.datadoghq.com/cloudcraft/getting-started/use-filters-to-create-better-diagrams/ — search-pattern syntax (name/ID, `type=`, IP, tags, inside VPC/SG/subnet) and AND/OR/NOT operators; Auto Layout vs Filtered Layout; excluded-services column.
- https://docs.datadoghq.com/cloudcraft/getting-started/version-history/ — auto-created versions, naming, restore, "Save as a new blueprint", 30-day (Free/Pro) vs 90-day (Enterprise) unnamed-version retention.
- https://docs.datadoghq.com/cloudcraft/api/ — `api.cloudcraft.co`, Bearer auth, 20 req/10 s limit, read/write ACLs per team/customer, Go/Python/Node SDKs.
- https://docs.datadoghq.com/datadog_cloudcraft/ — cloud coverage (AWS/Azure/GCP/OCI/vSphere), Group By incl. Network ACL, tag grouping, Saved views, 2D/3D projection toggle, search & highlight, five overlays and their gating products, `cloudcraft_read` RBAC permission, 10,000-resource filter guidance.
- https://github.com/patrickchugh/terravision — full feature list: plan-derived diagrams, multi-cloud type counts, `--simplified`, `--ai-annotate ollama|bedrock|restapi`, MCP server, agent skill, Claude/Codex/Copilot plugins, GitHub Action + CI/CD, draw.io export, interactive HTML, Terragrunt support, varfile-per-environment, per-resource click detail; the "How it compares" table.
- https://api.github.com/repos/patrickchugh/terravision — maintenance status: 1,660 stars, AGPL-3.0, last push 2026-10-22, 1,065 commits.
- https://github.com/im2nguyen/rover — three generated artifacts (resource overview / resource map / resource graph), Docker quickstart, `-planJSONPath`, standalone `rover.zip`, `-genImage`, `-tfBackendConfig`/`-tfVarsFile`/`-tfVar`, Go 1.21 + npm build requirement.
- https://api.github.com/repos/im2nguyen/rover — 3,348 stars, MIT, Go, 156 commits, last push 2025-07-30, 45 open issues.
- https://api.github.com/repos/im2nguyen/rover/releases/latest — latest release v0.3.3 published 2022-07-22; per-OS binaries.
- https://www.pluralith.com/ — three-step flow (plan → CI → PR comment), change highlighting, drift detection, Infracost cost, Regula compliance "Coming Soon", pricing slider ($250/mo for 500 CI runs, +$0.60/run beyond, unlimited users/projects/exports, "Local Development Free Forever"), CI integrations.
- https://docs.pluralith.com/ — docs IA: get started, CLI commands, CI integrations, config, diagram customization.
- https://docs.pluralith.com/docs/commands/graph — exact flag set (`--show-changes`, `--show-drift`, `--show-costs`, `--cost-mode`, `--cost-period`, `--cost-usage-file`, `--local-only`, `--plan-file`, `--plan-file-json`, `--title`, `--version`, `--author`, `--config-file`).
- https://docs.pluralith.com/docs/more/diagram-customization — `pluralith.yml` `diagram:` block with `hide`/`show`/`groups`/`ungroup`/`relations`/`unrelate` and module/resource targeting.
- https://github.com/cycloidio/inframap — provider support table (AWS grouping WIP, external nodes ✔, IAM ✘ #11; Google/Azure/OpenStack/FlexibleEngine no special logic), `generate`/`prune` subcommands, state v3/v4, HCL support, no backend support + stdin/out workaround, DOT output, Docker with Graphviz, brew/AUR.
- https://github.com/cycloidio/inframap/releases — release history.
- https://api.github.com/repos/cycloidio/inframap/releases/latest — v0.8.1 published 2026-04-23 (active maintenance).

### Adjacent tools and status checks

- https://www.hashicorp.com/en/blog/hashicorp-terraform-enterprise-general-availability — **Atlas decommissioning**: "The Atlas version of Terraform Enterprise will be decommissioned on Friday, March 30, 2018"; Packer/Artifact Registry features of Atlas decommissioned 2018-05-31; "Atlas" renamed to Terraform Enterprise.
- https://developer.hashicorp.com/hcp/docs/infragraph — Infragraph overview, connectors, constraints (one instance per connection type, 100 AWS accounts, subset of AWS resources, limited query API validation), real-time updates only on triggered/scheduled sync.
- https://www.hashicorp.com/en/blog/hcp-terraform-powered-by-infragraph-limited-availability-launch — limited availability for HCP Terraform Standard/Premium, US instances, connector ecosystem (AWS, Azure, GitHub, Terraform Enterprise), graph explorer + query builder.
- https://newsroom.ibm.com/2025-09-25-hashicorp-previews-the-future-of-agentic-infrastructure-automation-with-project-infragraph — Project Infragraph announced 2025-09-25 as a real-time infrastructure graph.
- https://docs.aws.amazon.com/infrastructure-composer/latest/dg/what-is-composer.html — end-of-support notice: standalone console unavailable from 2026-12-07; replaced by a visual IaC editor in AWS console pages and the VS Code Toolkit.
- https://aws.amazon.com/infrastructure-composer/ — "no longer available to new customers as of October 29, 2026"; no additional charge.
- https://github.com/awslabs/diagram-as-code — 1.6k stars, Apache-2.0, Go CLI `awsdac`, YAML input, AWS-guideline compliance, Go templates, MCP server, CloudFormation conversion (beta).
- https://api.github.com/repos/28mm/blast-radius — 2,194 stars, MIT, last commit on default branch 2020-01-06, last push 2024-08-14, 47 open issues, not archived.
- https://28mm.github.io/blast-radius-docs/ — feature set: highlight a resource and its dependencies, click to dismiss tooltip and highlight dependents, search, pan/zoom, prune-to-selection, local server on 127.0.0.1:5000, example configs for AWS/Azure/DO/GCP/k8s/OpenStack.
- https://spacelift.io/blog/terraform-visualization — "Blast Radius has compatibility issues with Terraform 1.x+ and is no longer actively maintained as of 2025"; also the feature write-ups for Inframap and Terraform Visual used to cross-check.
- https://github.com/hieven/terraform-visual — plan→interactive HTML report, CLI, Docker image, 675 stars.
- https://icepanel.io/pricing — full plan matrix and feature matrix (versioning, decision records, exports incl. LLMs.txt, AI object summaries, MCP server, API import/export, webhooks, SSO, audit logs, real-time collaboration, share links).
- https://www.brainboard.co/pricing — Free/Pro $99/Enterprise plan matrices, drift detection, git, CI/CD, RBAC, per-resource import fee.
- https://docs.datadoghq.com/cloudcraft/api/ — (also used) Cloudcraft API/SDK surface.
- https://lucid.co/alternative/lucidscale-vs-cloudcraft — Lucidscale multi-cloud import vs Cloudcraft filter/line differences.
- https://cloudairy.com/ai/ai-terraform-diagram-maker — AI prompt→diagram, HCL/tfstate parsing, multi-cloud icons, real-time collaboration, export format list.
- https://docs.aws.amazon.com/resource-explorer/latest/userguide/welcome.html — cross-region aggregator index, multi-account search via AWS Organizations, resource tagging.
- https://www.cycloid.io/open-source/inframap — Cycloid's own product page for Inframap (multi-provider claim, PNG/DOT output, CI use).
- https://phoenixnap.com/kb/terraform-visualization — third-party round-up used only to confirm the set of tools and their input types.
- https://www.terraform.io/docs/commands/graph.html via https://developer.hashicorp.com/terraform/cli/commands/graph — `terraform graph` emits DOT (the baseline TerraViz and Inframap both improve on).
- https://github.com/hashicorp/terraform/issues/39057 — maintainer discussion stating `terraform graph` renders the dependency graph, not infrastructure topology, and that `terraform show -json` is the intended interface for external visualisation (supports TerraViz's state-first position).

### Note on search-result noise

Several ranking pages for this category (InfraSketch, architecturediagram.ai, holori.com, infrasketch.net) are vendor blogs with competitive comparison tables. They were used only to discover candidate tools; every feature attributed to a benchmark in this document was re-verified on that vendor's own documentation or repository, with the exception of Cloudcraft's standalone per-user price and Lucidscale's seat price, which are marked **unverified**.
