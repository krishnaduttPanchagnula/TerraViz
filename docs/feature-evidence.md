# Feature Evidence

Raw evidence for every feature in the inventory in `docs/competitor-research.md`. Each entry names the competitor, the URL that was actually fetched, and one line on how that competitor implements the feature. Features marked **none found** were checked against every benchmark's own documentation and repository, and no benchmark states them; they are not claims that the competitors cannot do the thing, only that their documentation does not describe it.

Legend: **CC** = Cloudcraft · **TV** = TerraVision · **RV** = Rover · **PL** = Pluralith · **IM** = Inframap.

---

## 1. Input sources

**Raw `.tfstate` (v4)**
- IM — https://github.com/cycloidio/inframap — `inframap generate state.tfstate` reads a state file directly; the README states "For TFState generations we are limited to versions 3 and 4".
- TerraViz — has this today (`terraviz scan terraform file.tfstate`, raw v4 auto-detected).

**`terraform show -json` state output**
- TerraViz — has this today (auto-detects `format_version` and parses the terraform-json shape).
- No benchmark states it. RV, PL and TV all work from `terraform plan` output rather than state JSON.

**Terraform HCL / configuration input**
- IM — https://github.com/cycloidio/inframap — `inframap generate ./terraform/module-magento/` parses HCL directly; "for HCL we rely on interpolation to create the base graph".
- TV — https://github.com/patrickchugh/terravision — `terravision draw --source ./infra` runs `terraform plan` over the code, so the diagram reflects the HCL.
- RV — https://github.com/im2nguyen/rover — "parses the configuration in the root directory" alongside the plan.
- PL — https://www.pluralith.com/ — "Pluralith runs terraform plan under the hood to get data to visualise".
- No benchmark states it.

**`terraform plan` / plan JSON**
- TV — https://github.com/patrickchugh/terravision — "The diagram is derived from `terraform plan`, so it shows what the code actually deploys: conditionals, `count`, `for_each` and modules are resolved."
- RV — https://github.com/im2nguyen/rover — accepts `-planJSONPath=plan.json`; "uses a provided plan".
- PL — https://docs.pluralith.com/docs/commands/graph — `pluralith graph` "creates a Terraform execution plan, draws a diagram and pushes it to the Pluralith dashboard"; `--plan-file` and `--plan-file-json` accept an existing plan.
- No benchmark states it.
- IM — https://github.com/cycloidio/inframap — supports state and HCL only; no plan input documented.

**Live cloud account import**
- CC — https://docs.datadoghq.com/cloudcraft/getting-started/ — "Connect your AWS account to Cloudcraft", "Connect your Azure account with Cloudcraft", "Connect an Amazon EKS cluster", "Connect an Azure AKS cluster".
- TerraViz — has this today for a limited service set (`scan aws`: EC2, VPC, subnet, security group, S3, RDS, Lambda).

**Multi-region live scan**
- CC — https://docs.datadoghq.com/datadog_cloudcraft/ — account/region dropdowns per cloud, plus "Multi-account and multi-region infrastructure, including cross-account connections, shows up together on one diagram".
- TerraViz — has this today (`terraviz scan aws --regions us-east-1,eu-west-1`).

**Multi-account live scan**
- CC — https://docs.datadoghq.com/datadog_cloudcraft/ — multi-account diagrams via the Datadog AWS integration's account tags; https://docs.datadoghq.com/cloudcraft/getting-started/diagram-multiple-cloud-accounts/ is listed in the docs index.
- TerraViz — partial: one credential set per run (`--profile` selects a profile, not an account set).

**Remote state from S3, browsable in the UI**
- TerraViz — has this today (`GET /api/s3/buckets`, `GET /api/s3/objects`, `POST /api/s3/load`, with a selectable AWS profile).
- No benchmark states it.

**Native remote-backend support (S3/GCS/Swift)**
- IM — https://github.com/cycloidio/inframap — "We currently do not support graph generation from `tfstate` stored in such backends"; the documented workaround is `terraform state pull | inframap generate`.
- No benchmark states it. TerraViz — no: it can read S3 but does not run Terraform backends.

**Terragrunt project support**
- TV — https://github.com/patrickchugh/terravision — "Terragrunt compatible — auto-detects single- and multi-module Terragrunt projects".
- No benchmark states it.

**CloudFormation template input**
- None found in the five benchmarks. (Adjacent: https://github.com/awslabs/diagram-as-code offers "CloudFormation Conversion [Beta] — Convert CloudFormation templates to diagrams", and AWS Infrastructure Composer imports CloudFormation/SAM templates.)

---

## 2. Diagram & graph model

**AWS-style containment (VPC → subnet → resource)**
- TV — https://patrickchugh.github.io/terravision/aws-architecture-diagram-generator/ — "Resources are drawn inside their region, availability zone, VPC, subnet and security group."
- CC — https://docs.datadoghq.com/datadog_cloudcraft/ — Group By includes VPC, Region and Network ACL layers; tag grouping adds further containers.
- IM — https://github.com/cycloidio/inframap — provider table lists AWS "Grouping" as **WIP** (issue #6); "Group elements that belong to the same group like VPCs or regions" is marked not yet done.
- RV — not described in the README.
- TerraViz — has this today (VPCs, subnets and their contents drawn as nested containers).

**Official cloud icon sets**
- TV — https://github.com/patrickchugh/terravision — "Using Official AWS, Azure and GCP icons"; a supported-provider table lists 427 AWS / 267 GCP / 263 Azure resource types.
- CC — https://docs.datadoghq.com/cloudcraft/ — AWS/Azure/GCP/OCI/vSphere coverage with the standard provider iconography.
- IM — https://github.com/cycloidio/inframap — output is Graphviz DOT with text node names; no icon assets.
- RV — not described in the README.
- TerraViz — has this today (~620 embedded AWS SVGs, served at `/icons/`).

**Multi-cloud provider coverage**
- CC — https://docs.datadoghq.com/datadog_cloudcraft/ — "diagrams represent AWS, Azure, GCP, OCI, and vSphere resources"; per-cloud prerequisites are documented for each.
- TV — https://github.com/patrickchugh/terravision — AWS/GCP/Azure all "Full support".
- IM — https://github.com/cycloidio/inframap — "We support all cloud providers, but we have some ... that we have specific logic"; only AWS has specific logic, others "simply all resources present without any simplification".
- PL — provider coverage not documented on the pages fetched.
- RV — provider-agnostic Terraform; not described.
- TerraViz — no: AWS only; Azure/GCP are planned in `docs/gcp_azure_implementation.md`.

**Edges from Terraform's own dependency graph**
- IM — https://github.com/cycloidio/inframap — "For a state file, we rely on the `dependencies` key (for the <0.13 we replace all `depends_on` for `dependencies`)".
- TV — https://github.com/patrickchugh/terravision — graph derived from `terraform plan` output.
- RV — https://github.com/im2nguyen/rover — generates a "resource graph" from the plan and configuration.
- PL — https://www.pluralith.com/ — "Pluralith generates connections between resources by checking dependencies in Terraform state" (stated in its diagram-customisation docs).
- TerraViz — has this today (Terraform's dependency graph is the base edge layer; semantic edges refine it).

**Service-semantic edge types (networking / access / data / trigger / dependency / reference)**
- None found in the five benchmarks.
- TerraViz — has this today: six connection types with a colour legend, and "a semantic edge wins over the generic Terraform dependency edge for the same pair".

**Edge metadata (protocol, ports, CIDR, merged reasons)**
- None found in the five benchmarks.
- TerraViz — has this today: `properties.protocol`/`port` for load-balancer targets, `protocol`/`from_port`/`to_port`/`cidr_blocks` for security-group rules, and merged reasons under `properties.reasons`.

**Inference of referenced-but-absent resources**
- IM — https://github.com/cycloidio/inframap — provider table lists "External Nodes: Show the ingress of the Nodes if any" as supported for AWS.
- None found in the other benchmarks.
- TerraViz — has this today: a resource referenced by an ID/ARN that is not in the state is created with `in_state_file: false` and drawn with the detected relationship.

**Layout ordered by dependency position, not a fixed list**
- None found in the five benchmarks.
- TerraViz — has this today: "services and resources are ordered by their position in the dependency graph instead of a fixed list".

**Simplified / executive high-level view**
- TV — https://github.com/patrickchugh/terravision — `--simplified` "Strips VPCs, subnets, and networking plumbing. Great for executive presentations."
- IM — https://github.com/cycloidio/inframap — "showing only the resources that are most important/relevant" is the tool's whole premise (`--connections=false` further reduces it).
- None found in CC, RV, PL.
- TerraViz — no: filters exist but there is no one-click simplified view.

**3D isometric rendering**
- CC — https://docs.datadoghq.com/datadog_cloudcraft/ — "Toggle the projection from 3D (default) to 2D to visualize your resources from a top-down view."
- None found in the others. TerraViz — no.

**Provider-aware graph pruning**
- IM — https://github.com/cycloidio/inframap — "Read your tfstate or HCL to generate a graph specific for each provider, showing only the resources that are most important/relevant"; per-provider node/edge type lists.
- PL — https://docs.pluralith.com/docs/more/diagram-customization — the layout engine "intelligently hides resources to make the diagram more readable", overridable per resource via `show`/`hide`.
- TV — https://github.com/patrickchugh/terravision — `--simplified` strips networking plumbing.
- TerraViz — partial: manual filters only, no automatic pruning.

---

## 3. Interaction

**Zoom / pan / fit-to-view**
- TV — https://patrickchugh.github.io/terravision/aws-architecture-diagram-generator/ — "a self-contained interactive HTML page with pan, zoom and search".
- RV — https://github.com/im2nguyen/rover — interactive visualisation served on `0.0.0.0:9000`.
- CC — https://docs.datadoghq.com/datadog_cloudcraft/ — "Use the zoom and hover features to pinpoint the most critical resources".
- IM — static DOT only; no browser UI. PL — not documented.
- TerraViz — has this today.

**Minimap**
- None found in the five benchmarks. TerraViz — has this today (minimap toggle in the View menu).

**Search by resource name or ID**
- CC — https://docs.datadoghq.com/datadog_cloudcraft/ — "Use the search bar to locate resources on the diagram by name, ID, or tag", greying out non-matching elements.
- CC — https://docs.datadoghq.com/cloudcraft/getting-started/use-filters-to-create-better-diagrams/ — search patterns match name/ID, `type=ec2`, IP address, tags, and "components inside a VPC, security group, or subnet", with AND/OR/NOT operators.
- TV — search in the interactive HTML output (same page as the zoom/pan quote above).
- RV, PL, IM — not documented.
- TerraViz — has this today (real-time search by resource name or ID).

**Filters (type, provider, region, state, tag)**
- CC — https://docs.datadoghq.com/datadog_cloudcraft/ — "+ Filter menu to filter your resources by commonly used tags such as service, team, region"; "More Filters option to filter by AWS and Azure tags, custom tags, and Terraform tags".
- Others — not documented.
- TerraViz — has this today (`GET /api/resources/filter?type=&provider=&region=&state=&tag=`).

**Saved views / filter presets**
- CC — https://docs.datadoghq.com/datadog_cloudcraft/ — "Saved views allow you to save specific filters on your diagram ... on your accounts, regions, environments, and resources."
- None found in the others. TerraViz — no.

**Group by cloud tags (team, service, env)**
- CC — https://docs.datadoghq.com/datadog_cloudcraft/ — "With Group By, Cloudcraft divides your diagram into distinct sections based on different group types"; tag grouping for AWS/Azure tags, GCP labels, OCI tags and vSphere tags, with colour-coded labels.
- None found in the others. TerraViz — no.

**Click-to-inspect resource detail panel**
- CC — https://docs.datadoghq.com/datadog_cloudcraft/ — "clicking on a resource opens a side panel with observability, cost, and security data, along with cross-links to other relevant Datadog products".
- TV — https://github.com/patrickchugh/terravision — "click any resource to see its settings (encryption, public access, ports, IAM)".
- RV — https://github.com/im2nguyen/rover — one of the three generated views is the "resource overview (`rso`)".
- PL, IM — not documented.
- TerraViz — has this today (properties, connections and metadata on click).

**Hover tooltips on nodes/edges**
- CC — https://docs.datadoghq.com/datadog_cloudcraft/ — "Hovering over a resource displays a panel with basic information".
- Adjacent precedent: https://28mm.github.io/blast-radius-docs/ — "Click again to dismiss the tooltip, and highlight dependents"; tooltips show "Resource Name, JSON Definition, Dependencies".
- TV, RV, PL, IM — not documented.
- TerraViz — has this today (hover tooltips plus edge tooltips showing type, description, protocol/ports and merged reasons).

**Upstream + downstream dependency highlight**
- None found in the five benchmarks.
- Adjacent precedent: https://28mm.github.io/blast-radius-docs/ — "Use the searchbox or mouse to highlight a resource of interest, and its dependencies. Click again to dismiss the tooltip, and highlight dependents." (project dormant, last default-branch commit 2020-01-06 per https://api.github.com/repos/28mm/blast-radius).
- TerraViz — has this today (selecting a resource highlights everything it reaches and everything that reaches it).

**Selectable trace depth (1 / 2 / 3 / all hops)**
- None found in the five benchmarks, and none found in the adjacent tools checked.
- TerraViz — has this today (trace depth selector for 1, 2, 3 or all hops).

**Hide / show resources**
- PL — https://docs.pluralith.com/docs/more/diagram-customization — `diagram.hide` and `diagram.show` in `pluralith.yml`, targetable at `resourceType`, `moduleName`, `moduleName.resourceType`, `moduleName.resourceType.resourceName` and `...resourceName.index`.
- Others — not documented.
- TerraViz — has this today, at runtime (`POST /api/resources/:id/hide` and `/show`).

**Add / delete connections at runtime**
- PL — https://docs.pluralith.com/docs/more/diagram-customization — `diagram.relations` (type `connected` or `nested`) and `diagram.unrelate` in the config file.
- Others — not documented.
- TerraViz — has this today, at runtime (`POST /api/connections`, `DELETE /api/connections/:id`).

**Resource table / list view**
- RV — https://github.com/im2nguyen/rover — the "resource overview (`rso`)" is one of three generated views.
- None found in the others.
- TerraViz — has this today (toggle between the architecture diagram and a full resource table).

**Presentation mode / flow animation**
- TV — https://github.com/patrickchugh/terravision — "Interactive HTML output — clickable nodes, pan/zoom, search, animated data flow".
- None found in the others.
- TerraViz — has this today (presentation mode and flow animation in the View menu).

**Dark / light theme**
- None found in the five benchmarks. TerraViz — has this today (browser-persisted dark/light themes).

---

## 4. Output & integration

**PNG export**
- TV — https://github.com/patrickchugh/terravision — `terravision draw --format png`; gallery pages show PNG output.
- RV — https://github.com/im2nguyen/rover — `-genImage true` "to generate and save the visualization as a SVG image" (SVG, not PNG).
- IM — https://github.com/cycloidio/inframap — `inframap generate ... | dot -Tpng > graph.png` (via Graphviz, not built in).
- PL, CC — not confirmed from the pages fetched (CC API docs describe "remote rendering ... as ready to use images").
- TerraViz — has this today.

**SVG export**
- TV — https://github.com/patrickchugh/terravision — `--format svg`; "PNG, SVG, PDF and other Graphviz formats".
- RV — https://github.com/im2nguyen/rover — `-genImage true` produces an SVG.
- IM — Graphviz `-Tsvg`.
- TerraViz — has this today.

**CSV export**
- None found in the five benchmarks.
- TerraViz — has this today (`exportToCSV`, all resource data).

**PDF export**
- TV — https://github.com/patrickchugh/terravision — output formats list includes PDF.
- PL — https://docs.pluralith.com/docs/commands/graph — `--local-only` "a PDF export of the Diagram gets generated locally and saved to disk"; `--file-name` and `--out-dir` control it.
- CC, RV, IM — not documented.
- TerraViz — no.

**Mermaid flowchart export**
- None found in the five benchmarks.
- TerraViz — has this today (`exportToMermaid` builds a `.mmd` flowchart with escaped labels and Mermaid-safe node ids).

**Graphviz DOT export**
- IM — https://github.com/cycloidio/inframap — DOT is the native output format: "We currently support DOT format" (Cycloid product page); `inframap generate` prints DOT to stdout.
- None found in the others.
- TerraViz — has this today (`exportToDOT`).

**Editable draw.io export**
- TV — https://github.com/patrickchugh/terravision — `--format drawio`; "Editable draw.io export — open in draw.io, Lucidchart, or any mxGraph editor".
- None found in the others. TerraViz — no.

**Visio / PowerPoint export**
- None found in the five benchmarks. (Adjacent: https://cloudairy.com/ai/ai-terraform-diagram-maker lists "Visio (.vsdx), PowerPoint (.pptx)" exports.)

**Single-file static HTML artefact**
- TV — https://github.com/patrickchugh/terravision — "a single self-contained file that works fully offline"; `terravision visualise --source ./path-to-your-terraform`.
- RV — https://github.com/im2nguyen/rover — standalone mode "generates a `rover.zip` file containing all the static assets", opened as `rover/index.html` in a browser.
- None found in the others.
- TerraViz — no: the browser UI is served by the local Gin server.

**REST API for programmatic access**
- CC — https://docs.datadoghq.com/cloudcraft/api/ — "The Cloudcraft API provides programmatic access and remote rendering of your architecture diagrams", Bearer auth, `api.cloudcraft.co`, 20 requests/10 s, read/write ACLs, and official Go / Python / Node SDKs.
- TerraViz — has this today (`GET /api/diagram`, `/api/resources`, `/api/resources/search`, `/api/resources/filter`, `/api/connections`, `/api/stats`, `/api/export/:format`, `/api/s3/*`, `/api/upload`).
- IM, TV, RV — no public HTTP API described.
- PL — CLI plus a hosted dashboard; no public API described.

**CI pipeline integration**
- PL — https://docs.pluralith.com/ — dedicated integration pages for "Github Actions", "Gitlab CI" and "Terraform Cloud/Enterprise"; the site markets "Runs in any CI".
- TV — https://github.com/patrickchugh/terravision — a GitHub Action (`patrickchugh/terravision-action@v2` with `source`, `outfile`, `format` inputs) plus a "CI/CD Integration" guide covering GitLab, Jenkins and Azure DevOps.
- None found in CC, RV, IM.
- TerraViz — no.

**Diagram posted as a PR comment**
- PL — https://www.pluralith.com/ — "Pluralith posts the Diagram as a pull request comment."
- None found in the others. TerraViz — no.

**Diagram-as-code authoring (input graph format)**
- TV — https://github.com/patrickchugh/terravision — a JSON graph format (`<resource_type>.<name>` keys → arrays of what they connect to or contain) consumed by `terravision draw --source architecture.tvg.json`.
- IM — https://github.com/cycloidio/inframap — DOT is the intermediate format but it is not an authoring input.
- None found in CC, RV, PL. TerraViz — no.

**Reverse: diagram → Terraform**
- TV — https://github.com/patrickchugh/terravision — "diagram to Terraform via MCP server"; "Write the Terraform for this architecture" is one of the three prompt patterns, and the assistant can redraw the code it writes to check it.
- None found in the others. TerraViz — no.

**MCP server / agent-skill integration**
- TV — https://github.com/patrickchugh/terravision — an MCP server (`terravision mcp --output-dir`), a Claude Desktop extension (`.mcpb`), Claude Code / ChatGPT / Codex / Gemini plugins, VS Code and Cursor MCP configs, an agent skill installable via `npx skills add patrickchugh/terravision`, and `llms.txt`.
- None found in the others. TerraViz — no.
- Adjacent: https://github.com/awslabs/diagram-as-code also ships an MCP server.

**AI-generated annotations / summaries**
- TV — https://github.com/patrickchugh/terravision — `--ai-annotate ollama|bedrock|restapi`; "Only metadata and the summary graph are sent to the LLM — never your `.tf` source"; backends are Ollama (local), AWS Bedrock via boto3, or any OpenAI-compatible endpoint configured with `TV_RESTAPI_URL`/`TV_RESTAPI_KEY`/`TV_RESTAPI_MODEL`.
- None found in the others. TerraViz — no.

**Machine-re-readable graph text export**
- IM — https://github.com/cycloidio/inframap — DOT output; also `--description-file` to "generate a description file of the Graph Nodes".
- TerraViz — has this today (DOT + Mermaid + CSV from the same relation graph).

---

## 5. Collaboration & platform

**Hosted dashboard with run/plan history**
- PL — https://www.pluralith.com/ — "The Pluralith Dashboard gives you a visual history of all your Terraform plans"; the paid tier lists "Terraform Change History".
- None found in the others. TerraViz — no.

**Team workspaces / orgs**
- PL — https://docs.pluralith.com/docs/more/diagram-customization — the config carries `org_id` and `project_id`; the paid tier lists "Unlimited Users" and "Unlimited Projects".
- None found in the others. TerraViz — no.

**Real-time multi-user editing / comments**
- None found in the five benchmarks. (Adjacent: IcePanel https://icepanel.io/pricing and Cloudairy https://cloudairy.com/ai/ai-terraform-diagram-maker both advertise real-time collaboration and comments.)

**RBAC / access control**
- CC — https://docs.datadoghq.com/cloudcraft/api/ — "Blueprints and AWS Accounts support the following access control properties: readAccess / writeAccess", each a `team/<id>` or `customer/<id>` string.
- CC — https://docs.datadoghq.com/datadog_cloudcraft/ — access requires the `cloudcraft_read` permission, included in the Datadog Read Only Role.
- PL — https://www.pluralith.com/ — the paid tier lists "IAM" and "Access Management".
- None found in TV, RV, IM. TerraViz — no.

**SSO / SAML / audit logs**
- None found in the five benchmarks. (Adjacent: IcePanel lists SSO and audit logs on its Scale/Enterprise plans; Brainboard lists SSO and audit logs on Enterprise.)

**Versioned diagrams with restore**
- CC — https://docs.datadoghq.com/cloudcraft/getting-started/version-history/ — versions are created automatically, can be named, restored, searched, filtered to named-only, and saved as a new blueprint; retention is 30 days for unnamed versions (Free/Pro) and 90 days (Enterprise).
- PL — https://docs.pluralith.com/docs/commands/graph — `--version` sets the architecture version shown in the PDF; the dashboard keeps plan history.
- None found in TV, RV, IM. TerraViz — no.

**Share links / embeds**
- CC — https://docs.datadoghq.com/cloudcraft/getting-started/ — "Embedding Cloudcraft diagrams with the Confluence app" is a documented feature.
- None found in the others. TerraViz — no.

**Blueprints / reusable templates**
- CC — https://docs.datadoghq.com/cloudcraft/getting-started/version-history/ — "Save as a new blueprint" from any version; blueprints carry ACLs and are addressable by the API.
- None found in the others. TerraViz — no.

---

## 6. Operational overlay

**Per-resource cost overlay**
- CC — https://docs.datadoghq.com/datadog_cloudcraft/ — the Cloud Cost Management overlay "Track and optimize resource spend"; https://www.cloudcraft.co/ — "Cloud cost overlay shows a heatmap of your environment's true costs and available savings on the diagram". Cost view is not available for GCP accounts.
- PL — https://docs.pluralith.com/docs/commands/graph — `--show-costs` "Determines whether the exported diagram includes cost information (requires Infracost)".
- None found in TV, RV, IM. TerraViz — no.

**Cost mode / period control**
- PL — https://docs.pluralith.com/docs/commands/graph — `--cost-mode` (`delta` or `total`, default `delta`), `--cost-period` (`hour` or `month`, default `month`), `--cost-usage-file`.
- None found in the others. TerraViz — no.

**Budget / projected cost for a proposed design**
- CC — https://www.datadoghq.com/blog/cloud-architecture-diagrams-cost-compliance-cloudcraft-datadog/ — "Cloudcraft Budget ... in the Budget tab, you can see the cost for each service in your architecture, with options for daily, weekly, and monthly projections."
- None found in the others. TerraViz — no.

**Drift detection**
- PL — https://www.pluralith.com/ — "Catch drift as soon as possible. Pluralith highlights which resources have drifted"; CLI flag `--show-drift` (https://docs.pluralith.com/docs/commands/graph).
- None found in CC, TV, RV, IM.
- TerraViz — partial: `terraviz compare a.json b.json` diffs two scan snapshots (added/removed/modified resources, connection diffs, per-property changes) but does not compare state against live infrastructure.

**Plan change highlighting in the diagram**
- PL — https://www.pluralith.com/ — "See all plan changes visualised in a proper diagram"; CLI flag `--show-changes`.
- None found in the others.
- TerraViz — partial: the `compare` command reports added/removed/modified resources and connection diffs between two snapshots, not plan actions on a pending plan.

**Compliance / policy check overlay**
- PL — https://www.pluralith.com/ — "Compliance Data — Is your infrastructure compliant? Pluralith runs Regula under the hood and shows you which resources need attention" (labelled "Coming Soon").
- None found in the others. TerraViz — no.

**Security misconfiguration overlay**
- CC — https://docs.datadoghq.com/datadog_cloudcraft/ — the Security overlay shows "IAM, firewall, and security group visibility" and requires Cloud Security to be enabled.
- None found in the others. TerraViz — no.

**Observability overlay**
- CC — https://docs.datadoghq.com/datadog_cloudcraft/ — the Observability overlay "Indicates which hosts have the Agent installed and what observability features are enabled"; https://www.cloudcraft.co/ — "See alerts and error logs directly on your infrastructure, and search logs to pinpoint exactly where issues are occurring."
- None found in the others. TerraViz — no.

**APM / distributed-trace overlay**
- CC — https://docs.datadoghq.com/datadog_cloudcraft/ — the APM overlay "Visualize distributed traces between cloud resources (AWS only, Preview)".
- None found in the others. TerraViz — no.

**Ownership / tag heatmap**
- CC — https://docs.datadoghq.com/datadog_cloudcraft/ — "When grouping by tags, color-coded labels are displayed on each group"; https://www.cloudcraft.co/ — "trace their ownership" via the cost heatmap.
- None found in the others. TerraViz — no.

**Security-group / network-permission edge detail**
- None found in the five benchmarks.
- TerraViz — has this today: security-group rules carry `protocol`, `from_port`, `to_port` and `cidr_blocks` in connection properties.

---

## 7. Packaging, licensing & coverage

**Runs fully local, no account required**
- TV — https://github.com/patrickchugh/terravision — "100% client-side — designed with security mind; no cloud access required, runs locally, your code never leaves your machine"; "Draw prod without prod access" using `--varfile`.
- RV — https://github.com/im2nguyen/rover — a local Docker container serving on `0.0.0.0:9000`; standalone mode produces a zip.
- IM — https://github.com/cycloidio/inframap — a CLI printing DOT to stdout; fully local.
- PL — partial: https://docs.pluralith.com/docs/commands/graph — `--local-only` "The diagram will not be pushed to the Pluralith Dashboard. Instead, a PDF export of the Diagram gets generated locally and saved to disk"; the free tier is "Local Development Free Forever" but the hosted parts need an account.
- CC — requires a Datadog account (https://www.cloudcraft.co/pricing).
- TerraViz — has this today: the CLI starts a local Gin server and processes state in-process; no hosted TerraViz service.

**Single self-contained binary**
- RV — https://github.com/im2nguyen/rover — Go release zips per OS/arch, downloaded from the Releases page.
- IM — https://github.com/cycloidio/inframap — a Go binary installable via brew, AUR, Docker or `go install`; "graphviz is now in the Docker image so no need of external tools".
- TV — no: requires Python 3.11+, Graphviz and Git; `pipx`/`uv`/Docker/Nix are the install paths.
- CC, PL — hosted.
- TerraViz — has this today (~47 MB single binary with templates, JS, fonts and ~620 icons embedded via `go:embed`).

**No external runtime dependency (no Node / Graphviz / Python / Terraform binary)**
- None found in the five benchmarks: TV needs Python + Graphviz + Git; RV's build needs npm and its runtime needs a Terraform binary; IM needs Graphviz to render; CC and PL are hosted.
- TerraViz — has this today: the runtime needs only the Go binary; `terraform` is not required to parse a state file.

**Open-source license**
- TV — https://api.github.com/repos/patrickchugh/terravision — AGPL-3.0.
- RV — https://api.github.com/repos/im2nguyen/rover — MIT.
- IM — https://github.com/cycloidio/inframap — MIT.
- PL — product is commercial; only the CLI repo is public (https://github.com/Pluralith/pluralith-cli).
- CC — commercial (Datadog).
- TerraViz — MIT.

**Free, no per-seat pricing**
- TV, RV, IM — free and open source (see above).
- PL — https://www.pluralith.com/ — "Local Development — Free Forever"; paid packages start at $250/mo for 500 CI runs.
- CC — https://www.cloudcraft.co/pricing — free inside Datadog but tied to a Datadog subscription; standalone is per-user.
- TerraViz — MIT, free, no account, no usage metering.

**Homebrew / package-manager install**
- IM — https://github.com/cycloidio/inframap — "brew install inframap", AUR package, Docker image.
- TV — https://github.com/patrickchugh/terravision — `pipx install terravision` / `uv tool install terravision` / `pip install`, plus Docker and Nix.
- RV — https://github.com/im2nguyen/rover — release zips per OS/arch (no package manager mentioned).
- CC, PL — hosted.
- TerraViz — has this today (`brew install krishnaduttPanchagnula/TerraViz/terraviz`; GoReleaser release archives for 6 platform/arch combos).

**Multi-OS builds**
- RV — https://api.github.com/repos/im2nguyen/rover/releases/latest — v0.3.3 ships darwin/linux/freebsd/windows binaries for amd64, arm64, arm and 386.
- IM — https://github.com/cycloidio/inframap — Linux/macOS binaries plus a Linux Docker image.
- TV — https://github.com/patrickchugh/terravision — pipx/uv packages plus Docker and Nix images.
- CC, PL — hosted.
- TerraViz — has this today (linux/darwin/windows × amd64/arm64 via GoReleaser).

**Bundled assets, no runtime CDN**
- None found in the five benchmarks.
- TerraViz — has this today: Inter + JetBrains Mono WOFF2 and the AWS icon set are embedded and served from `/icons/` and `/static/`, so the built binary needs no external files or CDN at runtime.

**Terraform resource-type coverage**
- TV — https://github.com/patrickchugh/terravision — provider table: AWS "427 types", Google Cloud "267 types", Azure "263 types".
- CC — https://docs.datadoghq.com/datadog_cloudcraft/ — "search for resources across hundreds of AWS services" is the Resource Explorer claim; Cloudcraft's own live coverage is AWS/Azure/GCP/OCI/vSphere resource types.
- IM — https://github.com/cycloidio/inframap — per-provider type lists; only AWS has specific node/edge logic.
- RV, PL — not quantified on the pages fetched.
- TerraViz — ~21 AWS services mapped to typed resources, with unmapped AWS types retained as `ResourceTypeUnknown` carrying `properties.terraform_type`; live scan covers 7 types.

**Live-scan resource coverage**
- CC — https://docs.datadoghq.com/datadog_cloudcraft/ — AWS, Azure, GCP, OCI and vSphere, with EKS and AKS cluster connections documented.
- TerraViz — 7 types: EC2 instances, VPCs, subnets, security groups, S3 buckets, RDS database instances, Lambda functions.
