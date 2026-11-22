# Features to Develop — TerraViz

**Generated:** 2026-11-21
**Method:** Consolidation of the `competitor-seo-researcher` and `competitor-profiling` skill outputs, refreshed against live sources (GitHub API + vendor docs fetched 2026-11-21). Web search was unavailable (HTTP 402), so direct page/repository fetches were used. The full evidence trail stays in [`competitor-research.md`](competitor-research.md), [`feature-evidence.md`](feature-evidence.md) and [`feature-priority.md`](feature-priority.md); this file is the single actionable list.

**Freshness check:** No material competitor change since the 2026-10-22 study. TerraVision is now 1,698 stars, last pushed 2026-11-20 (still the fastest mover); Rover 3,358 stars, dormant since 2025-07-30; Inframap latest release still v0.8.1 (2026-04-23); Cloudcraft and Pluralith feature sets unchanged.

---

## Implementation status (2026-11-21)

| Feature | Status | Where |
|---|---|---|
| MCP server / agent skill | **Done** | `internal/mcp`, `terraviz mcp [file]`, 7 tools over stdio JSON-RPC (also `TERRAVIZ_DIAGRAM`) |
| Broader typed AWS coverage | **Done** | 22 more typed services in `models.go` + `terraformTypeMap`, with embedded icons |
| Local-first AI summary | **Done** | `internal/summary`, `terraviz summary`, `GET /api/summary`; offline by default, optional loopback LLM via `TERRAVIZ_LLM_ENDPOINT` (remote hosts rejected) |
| draw.io export | **Done** (already existed) | `internal/exporters/drawio.go`, `terraviz export -f drawio`, `GET /api/export/drawio`, dashboard export button |
| Multi-cloud (GCP / Azure) | **Done (state parsing)** | `internal/parsers/multicloud.go`: 14 GCP + 15 Azure Terraform types; unmapped types from supported providers are retained generically. Live scanners remain AWS-only. |
| Local snapshot history | **Done** | `internal/snapshots`, `terraviz snapshot` / `terraviz snapshots`, `/api/snapshots` CRUD. Snapshots store the ingested statefiles and can be restored. Stored under `TERRAVIZ_HOME` or the user config dir. |
| Cost overlay | **Done (illustrative)** | `internal/costs`, `GET /api/costs`; coarse offline price table, explicitly labelled "not a bill" |
| Public share links | **Done, reinterpreted** | A URL cannot work without a reachable host, so sharing uses a portable JSON bundle (`GET/POST /api/bundle`) of the sanitized statefiles, imported from "Add statefiles → Import share file". "Copy link" is kept for same-network access and warns on loopback hosts. |

**Bugs fixed along the way:** gzip responses advertised the uncompressed `Content-Length` and were rejected by browsers (`ECONNRESET`); the relation type index ranged over a map, making edge selection nondeterministic.

**Architecture note:** the old `legacyweb/` UI and its Gin server were removed. The only UI is the SQLite-backed dashboard embedded in `api/web/`. The single-file HTML export format was dropped with the legacy UI it depended on; CSV, Mermaid, DOT, draw.io and Markdown exports remain.

**Validation:** `go test ./...`, `go vet ./...`, and 10 Playwright tests (`npm run test:e2e`) covering the dashboard, the new API endpoints, the Insights drawer, MCP stdio protocol and draw.io download.

---

## 1. Benchmark matrix (SEO-researcher output)

| Tool | Positioning | Core surface | Packaging | Status |
|---|---|---|---|---|
| **Cloudcraft** (Datadog) | Live, read-only cloud diagram for platform/SRE/FinOps | Live + snapshot, group-by, 5 overlays, saved views, version history, full REST API + SDKs | Free inside Datadog; standalone per-user | Alive, incumbent |
| **TerraVision** | OSS, client-side, both directions (text/code → diagram, diagram → Terraform) | Plan-derived, multi-cloud, 427/267/263 types, MCP server + AI-assistant plugins, `--ai-annotate` (Ollama/Bedrock), CI action, interactive HTML, draw.io | Free, AGPL-3.0, Python + Graphviz + Git | Alive, very active |
| **Rover** | OSS local web UI over a Terraform plan | Overview / map / graph views, plan JSON, standalone zip | Free, MIT, Go binary (npm to build) | Dormant |
| **Pluralith** | CI-first automated diagrams + docs for teams | Plan → PR comment, drift, Infracost cost, Regula (soon), hosted history | $250/mo per 500 CI runs; local CLI free | Alive, commercial |
| **Inframap** | OSS "show only what matters"IaC visualiser | State v3/v4 + HCL → Graphviz DOT, provider-aware pruning | Free, MIT, Go binary + Graphviz | Alive, slow |

**Winning patterns in this niche:** plan-derived accuracy, CI redraw + PR comment, tag-based grouping and saved views, LLM/MCP agent surfaces, live-account overlays (cost/security/observability), and a self-contained interactive artefact.

**Underserved angles (where none of the five does the job well):** zero-dependency single-binary state visualiser; Mermaid + DOT + CSV from one graph; "referenced but missing" as a first-class finding; typed depth-controlled dependency tracing; local account-free diffing; state-vs-live drift without a paid agent; documentation-grade offline bundles; state-first (not plan-first) forensics.

---

## 2. Quick competitor profiles (profiling output — quick scan)

- **Cloudcraft** — Strongest on live import, overlays and API; weakness for TerraViz's audience is the Datadog account requirement. Implication: our no-account topology explorer is a different job, not a worse Cloudcraft.
- **TerraVision** — Closest OSS peer on feature breadth and the only one with a real MCP/agent + AI story. Weakness: needs Python + Graphviz + Git and uses `terraform plan` (credentials/backend). Implication: our edge semantics and zero-runtime binary are defensible; MCP is the gap worth closing.
- **Rover** — Closest in stack (Go, local web UI) but dormant since 2022 releases. Implication: not a live threat; its three-view model is a minor UX idea to borrow.
- **Pluralith** — Owns "diagram current in CI" and drift/cost, but behind a hosted dashboard and CI runs. Implication: local diff/drift is an unclaimed seam.
- **Inframap** — Clean provider-aware pruning, DOT-only, static output, no browser UI. Implication: "simplified view" is a proven idea we lack.

---

## 3. The feature list — what to develop

Scores: **Reach** (how many benchmarks have it), **Fit** (reinforces local/zero-account/single-binary), **Uniq** (how few have it), **Effort** (5 = nearly free). `Priority = (Reach + Fit + Uniq) × Effort ÷ 5`. Full scoring rationale is in `feature-priority.md`.

### Tier 1 — Now (next release)

| # | Feature | Score | One-line why |
|---|---|---|---|
| 1 | **"Referenced but missing" as a first-class finding** | 10.0 | Already computed (`in_state_file: false`); unique and most useful for inherited codebases. Surface as a panel + CSV column. |
| 2 | **Drift check: live scan vs. state** | 6.6 | Scanner + diff engine both exist; only the AWS-id ↔ Terraform-address reconciliation is new. Unclaimed seam. |
| 3 | **Saved views / shareable URL state** | 6.4 | Filter preset in the URL: local, account-free, copy-paste shareable. |
| 4 | **Live-scan coverage 7 → 30+ services** | 6.0 | The incumbent's whole product is live import; each new service reuses `internal/relations` for typed edges. |
| 5 | **Simplified / executive view** | 6.0 | TerraVision `--simplified` and Inframap pruning prove it; also fixes "diagram too tall to read". |
| 6 | **PDF export** | 6.4 | A print stylesheet, not a PDF library; review artefact. Last in tier because it is a format, not a capability. |
| 7 | **Group by cloud tag (team / service / env)** | 4.8 | Cloudcraft does it; layout already draws nested containers, so it is a grouping rule. |
| 8 | **Single-file static HTML export** | 2 steps away | Inline JS/CSS/JSON + used icons; colleague opens with no binary or server. |
| 9 | **CI integration: GitHub Action + PR comment** | 4.2 | Expected way the job is done (TV + Pluralith); depends on #10 to be compelling. |
| 10 | **Plan-based input (`terraform show -json` plan)** | 3.6 | Largest category gap, largest build; complements state-first, does not replace it. |

### Tier 2 — Next

| # | Feature | Score | One-line why |
|---|---|---|---|
| 11 | **MCP server / agent skill over the graph** | 5.4 | TerraVision ships both; graph is already machine-readable, so this is a thin wrapper. |
| 12 | **Typed AWS coverage beyond ~21 services** | 4.0 | Unmapped types render generically; typed coverage yields typed edges, correct icons, real filtering. Ongoing. |
| 13 | **AI architecture summary, local-first** | 3.2 | TerraVision's `--ai-annotate` has an Ollama option; summary of the *graph*, not the code, with a privacy story. |
| 14 | **draw.io export** | 2.4 | TerraVision emits editable draw.io; output stays editable for hand-polishing. |
| 15 | **Multi-cloud (GCP / Azure)** | 1.0 | `docs/gcp_azure_implementation.md` exists, but it is the largest cost for the least differentiation. Defer until the AWS graph is excellent. |

### Tier 3 — Later, only when the position allows

| # | Feature | Score | One-line why |
|---|---|---|---|
| 16 | **Local snapshot history with restore** | 4.8 | Cloudcraft/Pluralith version diagrams; a *local* history fits us, a hosted one does not. |
| 17 | **Cost overlay** | 2.0 | Cloudcraft + Pluralith (Infracost); needs a pricing source and a plugin boundary. |
| 18 | **Public share links** | 0.6 | Requires hosting. Breaks the no-account position. |

### Not building — deliberately out of scope

Hosted dashboard/history (Pluralith) · team workspaces, RBAC, SSO, audit logs (Cloudcraft/Pluralith) · real-time multi-user editing and comments (IcePanel/Cloudairy) · 3D isometric rendering (Cloudcraft) · diagram-as-code authoring and diagram→Terraform (TerraVision/Brainboard) · CloudFormation input. Each scores well on reach but badly on fit — copying them makes TerraViz a worse paid product.

---

## 4. Recommended first three

1. **#1 Referenced-but-missing list** — nearly free, unique, immediately useful. Ship as a panel + CSV column.
2. **#4 Live-scan coverage** — the incumbent's product; we have the skeleton, and shared `internal/relations` gives edges free.
3. **#2 Drift check** — scan live, diff against state, report what reality disagrees with. The reconciliation layer is the real work.

Then **#10 plan input** (largest gap, largest build) on a graph that is already complete; **#3 saved views** and **#5 simplified view** are the cheapest readability wins and prerequisites for the plan view.

---

## 5. Protect these leads (do not trade them away)

Single binary with zero external runtime · Mermaid + DOT + CSV from one graph · six typed, protocol-carrying edge types · selectable 1/2/3/all-hop tracing over typed edges · referenced-but-absent inference · account-free local diffing.

## Sources

All feature claims trace to pages fetched for this study and the 2026-10-22 study; see [`competitor-research.md`](competitor-research.md) §5 and [`feature-evidence.md`](feature-evidence.md). Live-refresh fetches on 2026-11-21: GitHub API for `patrickchugh/terravision`, `im2nguyen/rover`, `cycloidio/inframap/releases/latest`, plus pluralith.com, docs.datadoghq.com/cloudcraft/getting-started and the TerraVision README. Unverified (not published on pages fetched): Cloudcraft standalone per-user price, Lucidscale seat price.
