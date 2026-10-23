# Feature Priority

**Date:** 2026-10-23
**Input:** [`competitor-research.md`](competitor-research.md) (5 benchmarks: Cloudcraft, TerraVision, Rover, Pluralith, Inframap) and [`feature-evidence.md`](feature-evidence.md).
**Purpose:** Decide which features TerraViz should build next, in what order, and which it should deliberately *not* build.

---

## How the ranking works

Four factors, each scored 1-5:

| Factor | Meaning |
|---|---|
| **Reach** | How many of the 5 benchmarks have it. 5 = table stakes, 3 = expected, 1 = differentiator, 0 = unique to us. |
| **Fit** | Does it reinforce TerraViz's position — *local, zero-account, single binary, no runtime dependencies*? 5 = reinforces it, 1 = it would pull us toward a hosted, per-seat, telemetry-driven competitor. |
| **Uniqueness** | How few competitors have it. 5 = nobody else has it. |
| **Effort** | Build cost for us: **5 = nearly free, 1 = expensive.** Includes whether the hard half already exists. |

**Priority = (Reach + Fit + Uniqueness) × Effort ÷ 5.** A feature only scores if it is worth building (reach, fit, uniqueness) *and* if we can afford it (effort). Effort is a multiplier, not a divisor: an expensive feature is discounted, never rewarded.

Two rules override the arithmetic:

1. **Position rules out what breaks it.** Anything that requires a hosted dashboard, an account, a seat, or a vendor pipeline is out of scope even when it scores well — it is a different product.
2. **A feature that only makes sense on top of another one ranks below it.** (PR comments need a CI integration; a drift report needs a live scanner.)

The order *within* a tier is judgement on top of the arithmetic; the score column shows the raw numbers.

---

## The ranked list

### Now — the next release

| # | Feature | Reach | Fit | Uniq | Effort | Score | Why here |
|---|---|---|---|---|---|---|---|
| 1 | **"Referenced but missing" as a first-class finding** | 0 | 5 | 5 | 5 | 10.0 | We already compute it (`in_state_file: false`); nobody surfaces it as a reviewable list. Inframap has external nodes but only for ingress. Near-free, unique, and the most useful output for someone inheriting a codebase. The highest score in the set. |
| 2 | **Drift check: live scan vs. state** | 1 | 5 | 5 | 3 | 6.6 | Both halves exist (live scanner + diff engine) and nobody joins them: Pluralith's drift needs Infracost and an account, Cloudcraft's live view needs Datadog. The seam is unclaimed. Needs an AWS-id ↔ Terraform-address reconciliation layer — that is the real work. |
| 3 | **Saved views / shareable URL state** | 1 | 4 | 3 | 4 | 6.4 | Cloudcraft has saved views; a filter preset encoded in the URL is local, account-free and shareable by copy-paste. Cheap, and it makes the filter UI worth having. |
| 4 | **Live-scan coverage: 7 types → 30+** | 1 | 5 | 4 | 3 | 6.0 | Cloudcraft is the only benchmark with live import, and it is the incumbent's whole product. Our scanner covers EC2/VPC/subnet/SG/S3/RDS/Lambda; each new service is a scan method, and the shared `internal/relations` package gives it typed edges for free. Ranked below saved views on effort, above it on how much it changes what the product *is*. |
| 5 | **Simplified / executive view** | 2 | 4 | 4 | 3 | 6.0 | TerraVision (`--simplified`) and Inframap (provider-aware pruning) both reduce the graph to what matters. We have filters but no automatic pruning. Also the fix for the "diagram too tall to read" problem. |
| 6 | **PDF export** | 2 | 4 | 2 | 4 | 6.4 | TerraVision and Pluralith both emit PDF; it is a review artefact, not a picture. A print stylesheet gets us there without a PDF library. Cheap and uncontroversial, but it is a format, not a capability — hence last in this tier. |
| 7 | **Group by cloud tag (team / service / env)** | 1 | 4 | 3 | 3 | 4.8 | Cloudcraft groups by tags. The layout already draws nested containers, so a tag group is a grouping rule, not a new renderer. High value for platform teams, and it is a differentiator against the per-tag tools. |
| 8 | **Single-file static HTML export** | 2 | 4 | 3 | 3 | 5.4 | TerraVision and Rover both hand out a self-contained artefact. We are one step away: inline the JS, CSS and diagram JSON, and inline only the icons the diagram uses. Lets a colleague open the diagram with no binary and no server. |
| 9 | **CI integration: GitHub Action + PR comment** | 2 | 3 | 2 | 3 | 4.2 | TerraVision and Pluralith both post diagrams to PRs; it is the expected way this job is done. Ranks below the local features because a hosted CI pipeline pulls toward the competitor model, and it depends on #4 (a plan) to be interesting — a state diagram on a PR is still useful, but a *change* diagram is the point. |
| 10 | **Plan-based input (`terraform show -json` plan)** | 3 | 4 | 2 | 2 | 3.6 | The largest category gap: TerraVision, Rover and Pluralith all read plans. It complements rather than replaces our state-first position (what exists vs. what will change) and needs credentials, so plan-first tools cannot serve the audit and forensics cases we own. It scores low only because it is expensive: a plan schema, plan actions, and change display in the UI. |

### Next — the release after

| # | Feature | Reach | Fit | Uniq | Effort | Score | Why here |
|---|---|---|---|---|---|---|---|
| 11 | **MCP server / agent skill over the graph** | 1 | 4 | 4 | 3 | 5.4 | TerraVision ships both. The graph is already machine-readable (DOT, Mermaid, CSV, `/api`), so this is a thin wrapper — and it is the natural way an agent reads architecture. |
| 12 | **Typed AWS resource coverage (beyond 21 services)** | 2 | 5 | 3 | 2 | 4.0 | Every benchmark except Inframap renders hundreds of types. We keep unmapped types generically, so the graph is complete, but typed coverage is what produces typed edges, correct icons and real filtering. It is ongoing work rather than one feature, so it is continuous. |
| 13 | **AI architecture summary, local-first** | 1 | 4 | 3 | 2 | 3.2 | TerraVision has `--ai-annotate` with an Ollama option. A local endpoint keeps the "nothing leaves the machine" promise, and a *summary of the graph* (not of the code) is genuinely useful on a 200-resource estate. Scores low only because it is a new dependency surface (an LLM endpoint) and needs a careful privacy story. |
| 14 | **draw.io export** | 1 | 3 | 2 | 2 | 2.4 | TerraVision emits editable draw.io. Valuable because the output stays *editable* for teams that draw by hand afterwards. Low strategic value, moderate effort. |
| 15 | **Terraform resource-type coverage beyond AWS** | 2 | 2 | 1 | 1 | 1.0 | Cloudcraft and TerraVision are multi-cloud. `docs/gcp_azure_implementation.md` exists, but doubling the provider work is the largest cost in the set and buys the least differentiation for a tool whose edges are AWS-semantic. Defer until the AWS graph is excellent. |

### Later — only when the position allows it

| # | Feature | Reach | Fit | Uniq | Effort | Score | Why here |
|---|---|---|---|---|---|---|---|
| 16 | **Local snapshot history with restore** | 2 | 3 | 3 | 3 | 4.8 | Cloudcraft and Pluralith version their diagrams. A *local* history (`terraviz snapshot` + a browsable list) is the version that fits us; a hosted one is not. |
| 17 | **Cost overlay** | 2 | 2 | 1 | 2 | 2.0 | Cloudcraft and Pluralith both have it (Pluralith via Infracost). It needs a pricing source and a plugin boundary; it is the classic "someone else's category". |
| 18 | **Public share links** | 1 | 1 | 1 | 1 | 0.6 | Needs hosting. Breaks the no-account position. |

### Not building — deliberately out of scope

Each of these scores well on *reach* and badly on *fit*. Copying them would make TerraViz a worse version of a paid product.

| Feature | Who has it | Why not |
|---|---|---|
| Hosted dashboard with run history | Pluralith | Requires an account and a vendor. The offline bundle (#8) covers the same need. |
| Team workspaces, orgs, RBAC, SSO, audit logs | Cloudcraft, Pluralith | Enterprise identity features for a tool that runs on a laptop. |
| Real-time multi-user editing, comments | IcePanel, Cloudairy | A collaborative editor is a different product with a different cost base. |
| 3D isometric rendering | Cloudcraft | A rendering style, not a capability. |
| Diagram-as-code authoring (YAML in), diagram → Terraform | TerraVision, Brainboard | A second product direction. The reverse direction also requires a Terraform runtime, which we deliberately do not depend on. |
| CloudFormation template input | (adjacent) | Nobody in the benchmark set does it well either. |

---

## What we already lead with — protect these

These are in the "no benchmark has this" column, and they are the reason to keep the current direction rather than copy a competitor.

| Lead | Evidence |
|---|---|
| **Single binary, zero external runtime** | Every OSS peer needs a stack: TerraVision needs Python + Graphviz + Git, Rover needs npm, Inframap needs Graphviz. We ship HTML, JS, fonts and ~620 AWS icons inside the binary. |
| **Mermaid + DOT + CSV from one graph** | Nobody emits Mermaid from a Terraform dependency graph. Mermaid renders where architecture docs actually live. |
| **Typed, protocol-carrying edges** | Six edge types with protocol, ports, CIDR and merged reasons. No benchmark documents an edge with this much metadata. |
| **Selectable trace depth over typed edges** | The original precedent (Blast Radius) is dormant since 2020. We have 1/2/3/all-hop tracing in both directions. |
| **Referenced-but-absent inference** | Every resource referenced by an id/ARN in state that is not in the state. Inframap does ingress only. |
| **Account-free local diffing** | Pluralith's diff and drift live behind a hosted dashboard. Ours is `terraviz compare a.json b.json` on the machine. |

---

## Recommended next three

1. **#1 referenced-but-missing list** — nearly free, unique, immediately useful. Ship it as a panel and a CSV column.
2. **#4 live-scan coverage** — the incumbent's product is live import; we have the skeleton, just not the breadth. Every new service reuses `internal/relations`, so the edges come free.
3. **#2 drift check** — the unclaimed seam, and the strongest argument for the product: scan the live account, diff it against the state, report what reality disagrees with. Needs the id-reconciliation layer, which is the real work.

Do **#10 plan input** after those: it is the largest category gap but also the largest build, and it should land on a graph that is already complete and correct. **#3 saved views** and **#5 simplified view** are the cheapest way to make a big estate readable, and both are prerequisites for the plan view being usable at all.

---

## Sources

Every claim traces to a fetched page. See [`feature-evidence.md`](feature-evidence.md) for the per-feature evidence (competitor, URL, how it implements it) and [`competitor-research.md`](competitor-research.md) for the benchmark matrix, the full inventory and the sources list.

Not verified: Cloudcraft's standalone per-user price and Lucidscale's seat price (not published on the pages fetched) — marked unverified in the research rather than taken from third-party comparison blogs.
