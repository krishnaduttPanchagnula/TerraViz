# Azure and GCP Extensibility Design

## Status

Proposed implementation plan.

## Summary

TerraViz currently supports AWS resources from Terraform state and a limited live AWS inventory. Azure and Google Cloud support should be added through independent interfaces for Terraform normalization, relationship inference, live inventory scanning, and remote state storage.

The design intentionally avoids one large cloud-provider interface. Parsing Terraform, scanning a live account, and reading an object store have different capabilities, credentials, and lifecycles. Keeping these contracts separate allows each provider to implement only the features it supports and keeps the existing AWS behavior stable during migration.

The recommended delivery order is:

1. Characterize current AWS output with tests.
2. Introduce provider-neutral interfaces without changing behavior.
3. Move existing AWS behavior behind those interfaces.
4. Add Azure and GCP Terraform adapters.
5. Add Azure live scanning through Azure Resource Graph.
6. Add GCP live scanning through Cloud Asset Inventory.
7. Generalize frontend classification and icons.
8. Add Azure Blob Storage and Google Cloud Storage imports.

## Goals

- Parse AWS, Azure, and GCP managed resources from raw Terraform state and `terraform show -json` output.
- Scan live Azure subscriptions and GCP projects.
- Normalize provider resources into one stable diagram model.
- Keep provider-specific SDK and schema details outside core graph logic.
- Infer relationships without creating cross-provider false matches.
- Render multi-cloud diagrams using provider, category, service, and resource hierarchy.
- Preserve current AWS commands, routes, resource IDs, and saved diagram compatibility.
- Make provider implementations independently testable with mocked SDK clients.
- Keep credentials out of logs, diagram metadata, API responses, and saved scan files.

## Non-Goals

- Supporting every Azure or GCP resource in the first release.
- Replacing provider SDKs with a universal cloud abstraction.
- Exposing live cloud scans through the HTTP server in the first release.
- Loading provider implementations through Go plugins or build tags.
- Changing existing AWS resource IDs during the initial migration.
- Automatically scanning every subscription, project, folder, or organization without explicit scope.

## Current Constraints

The following areas are currently AWS-specific:

- `internal/parsers/terraform.go` maps only `aws_*` Terraform resources.
- `internal/parsers/connections.go` contains AWS-specific relationship and external-reference rules.
- `internal/scanners/aws.go` combines authentication, collection, normalization, concurrency, and relationship inference.
- `internal/models/models.go` defines AWS resource constants and an AWS-only icon map.
- `cmd/main.go` constructs the AWS scanner directly.
- `internal/server/server.go` directly implements S3 imports.
- `web/static/js/diagram_enhanced.js` derives hierarchy from AWS resource type segments and uses AWS fallback icons.

The existing `Resource.Provider`, `DiagramBuilder`, connection model, filters, and diagram API are suitable foundations but require explicit provider-neutral classification and identity metadata.

## Architecture Principles

### Separate Capabilities

Use separate contracts for:

- Terraform resource normalization.
- Relationship analysis.
- Live inventory scanning.
- Remote state object storage.

A provider can support Terraform before it supports live scanning or object storage.

### Normalize at the Boundary

Provider SDK objects and Terraform schemas must be converted into `models.Resource` before entering graph construction. Core graph and UI code must not import cloud SDK types.

### Keep Native Identity

Every resource should retain its provider-native identifier. Provider-specific canonicalization happens before deduplication or relationship matching.

### Prefer Explicit Classification

The frontend should consume explicit category, service, kind, scope, and location fields. It should not infer semantics by splitting resource type strings.

### Preserve Compatibility

Existing JSON fields and AWS resource IDs remain valid. New fields are additive and use `omitempty` where appropriate.

### Fail Partially

A failed service collector should produce a warning or scoped error while allowing other services to complete. Authentication or invalid top-level scope remains a fatal error.

## Design Patterns

### Strategy

Each provider implements interchangeable Terraform normalization, relationship analysis, and scanner strategies.

### Registry

A registry resolves the appropriate Terraform adapter from provider names such as `aws`, `azurerm`, `azuread`, `google`, and `google-beta`.

### Adapter

Provider SDK responses and Terraform resource schemas are adapted to the canonical TerraViz model.

### Composite

A live scanner is composed from service collectors. For example, an Azure scanner can combine Compute, Network, Storage, SQL, and App Service collectors.

### Factory

Provider-specific factories construct authenticated scanners and object stores from typed configuration.

### Pipeline

Ingestion follows a deterministic pipeline:

```text
decode -> identify provider -> normalize -> index -> infer references
       -> infer relationships -> validate -> sort -> finalize
```

### Dependency Injection

SDK clients, parsers, and object stores are supplied through interfaces so tests do not require cloud credentials or network access.

### Repository / Graph View

Relationship analyzers query an indexed, read-only graph view instead of repeatedly scanning resource slices or mutating the diagram directly.

### Capability Discovery

The server and frontend can expose supported providers, import mechanisms, and resource types without hard-coded UI assumptions.

### Contract Tests

Shared tests verify that every scanner and adapter produces valid resources, unique IDs, valid connection endpoints, consistent statistics, and credential-safe output.

## Proposed Package Layout

```text
internal/
  ingest/
    scanner.go
    collector.go
    result.go
  terraform/
    parser.go
    raw_state.go
    registry.go
    references.go
  providers/
    aws/
      config.go
      terraform.go
      relationships.go
      scanner.go
      collectors.go
      normalize.go
    azure/
      config.go
      terraform.go
      relationships.go
      scanner.go
      collectors.go
      normalize.go
    gcp/
      config.go
      terraform.go
      relationships.go
      scanner.go
      collectors.go
      normalize.go
  storage/
    store.go
    aws/s3.go
    azure/blob.go
    gcp/gcs.go
  models/
    models.go
    identity.go
    icons.go
    graph.go
```

This package movement should be incremental. Initial interfaces can live in the existing `parsers` and `scanners` packages until AWS behavior is protected by characterization tests.

## Core Types and Interfaces

### Provider

```go
type Provider string

const (
    ProviderAWS   Provider = "aws"
    ProviderAzure Provider = "azure"
    ProviderGCP   Provider = "gcp"
)
```

`Resource.Provider` can remain a string during the compatibility phase. Conversion helpers should validate and normalize it.

### Live Scanner

```go
type ScanRequest struct {
    Locations  []string
    Scopes     []string
    Filters    map[string]string
    IncludeAll bool
}

type Scanner interface {
    Provider() Provider
    Scan(context.Context, ScanRequest) (*models.ScanResult, error)
}
```

Existing compatibility wrapper:

```go
func (s *AWSScanner) ScanAccount(ctx context.Context) (*models.ScanResult, error) {
    return s.Scan(ctx, ScanRequest{Locations: s.regions})
}
```

### Service Collector

Use small collectors with narrow SDK dependencies:

```go
type Collector interface {
    Name() string
    Collect(context.Context, ScanRequest) CollectResult
}

type CollectResult struct {
    Resources   []models.Resource
    Connections []models.Connection
    Warnings    []string
    Errors      []error
}
```

Collectors return data rather than mutating `DiagramBuilder`. The scanner merges and sorts all results before graph finalization.

### Terraform Resource Input

```go
type ResourceInput struct {
    Address      string
    Type         string
    ProviderName string
    Module       string
    Attributes   map[string]any
    InStateFile  bool
}
```

### Terraform Adapter

```go
type TerraformAdapter interface {
    Provider() Provider
    SupportsTerraformType(terraformType string) bool
    NormalizeTerraformResource(
        context.Context,
        ResourceInput,
    ) (models.Resource, bool, error)
}
```

The boolean indicates whether the adapter supports the type. Unsupported resources produce warnings rather than conversion errors.

### Adapter Registry

```go
type TerraformAdapterRegistry interface {
    Register(TerraformAdapter) error
    AdapterForProvider(providerName string) (TerraformAdapter, bool)
}
```

Provider normalization must support:

- `registry.terraform.io/hashicorp/aws` -> `aws`
- `provider["registry.terraform.io/hashicorp/aws"]` -> `aws`
- `azurerm` -> `azure`
- `azuread` -> `azure-ad`
- `azapi` -> `azure`
- `google` -> `gcp`
- `google-beta` -> `gcp`

Azure AD should remain a distinct provider family where tenant-scoped identity semantics differ from Azure Resource Manager.

### Relationship Analyzer

```go
type RelationshipAnalyzer interface {
    Provider() Provider
    Analyze(context.Context, GraphView, ConnectionAdder) error
}

type GraphView interface {
    Resources() []models.Resource
    ResourceByID(string) (models.Resource, bool)
    ResourcesByType(models.ResourceType) []models.Resource
    ResourcesByNativeID(Provider, string) []models.Resource
    ResourcesByProperty(Provider, models.ResourceType, string, any) []models.Resource
}

type ConnectionAdder interface {
    AddConnection(models.Connection) error
}
```

All property and native-ID lookups must be constrained by provider and scope unless a rule explicitly permits cross-provider relationships.

### Reference Rules

```go
type ReferenceRule struct {
    SourceType     models.ResourceType
    AttributePath  []string
    TargetType     models.ResourceType
    TargetProperty string
    ConnectionType models.ConnectionType
    Description    string
    Predicate      func(models.Resource) bool
}

type ReferenceResolver interface {
    Provider() Provider
    Rules() []ReferenceRule
    CanonicalizeReference(models.ResourceType, string) string
    ParseScope(string) models.ResourceScope
}
```

These rules replace the AWS-only `externalReferenceSpecs`. Missing targets become typed inferred resources with provider-correct scope and icons.

### Object Store

```go
type ObjectStore interface {
    Provider() Provider
    ListContainers(context.Context) ([]Container, error)
    ListObjects(context.Context, string, string) ([]Object, error)
    Open(context.Context, string, string) (io.ReadCloser, ObjectMetadata, error)
}

type ObjectStoreFactory interface {
    New(context.Context, Provider, StoreOptions) (ObjectStore, error)
}
```

Provider implementations are S3, Azure Blob Storage, and Google Cloud Storage.

## Provider-Neutral Resource Model

The initial model change should be additive:

```go
type ResourceScope struct {
    TenantID       string `json:"tenant_id,omitempty"`
    OrganizationID string `json:"organization_id,omitempty"`
    AccountID      string `json:"account_id,omitempty"`
    SubscriptionID string `json:"subscription_id,omitempty"`
    ProjectID      string `json:"project_id,omitempty"`
    ProjectNumber  string `json:"project_number,omitempty"`
    ResourceGroup  string `json:"resource_group,omitempty"`
}

type ResourceLocation struct {
    Region string `json:"region,omitempty"`
    Zone   string `json:"zone,omitempty"`
    Global bool   `json:"global,omitempty"`
}

type ResourceProvenance struct {
    Origin         string  `json:"origin,omitempty"`
    Presence       string  `json:"presence,omitempty"`
    InStateFile    bool    `json:"in_state_file"`
    Inferred       bool    `json:"inferred,omitempty"`
    InferenceRule  string  `json:"inference_rule,omitempty"`
    ReferenceValue string  `json:"reference_value,omitempty"`
    Confidence     float64 `json:"confidence,omitempty"`
}
```

Add optional fields to `Resource`:

```go
NativeID       string             `json:"native_id,omitempty"`
NativeType     string             `json:"native_type,omitempty"`
Category       string             `json:"category,omitempty"`
Service        string             `json:"service,omitempty"`
Kind           string             `json:"kind,omitempty"`
DisplayType    string             `json:"display_type,omitempty"`
Scope          *ResourceScope     `json:"scope,omitempty"`
Location       *ResourceLocation  `json:"location,omitempty"`
Provenance     ResourceProvenance `json:"provenance,omitempty"`
```

Keep these existing fields populated for compatibility:

- AWS `Account`: account ID.
- Azure `Account`: subscription ID.
- GCP `Account`: project ID.
- `Region`: normalized regional location where available.
- `InStateFile`: mirror `Provenance.InStateFile` until a future major schema version.

### Resource Type Naming

Resource types remain open strings using:

```text
<provider>:<service>:<kind>
```

Examples:

```text
azure:compute:virtualmachine
azure:network:virtualnetwork
azure:network:subnet
azure:storage:account
gcp:compute:instance
gcp:compute:network
gcp:storage:bucket
gcp:sql:instance
```

Azure and GCP resources must not be represented using AWS resource types. Cross-cloud visual grouping uses `Category`, not type substitution.

### Resource Identity

Live resource identity should be generated from the provider and canonical native ID:

```text
hash(provider + "\x00" + canonicalNativeID)
```

Canonical forms:

- AWS: ARN when available; otherwise account, region, service, and native identifier.
- Azure: lower-cased full ARM resource ID.
- GCP: full resource name containing project and location.

Terraform resource addresses remain unchanged in the first implementation to preserve comparisons and saved connections. `NativeID` provides a future identity-aware matching path.

## DiagramBuilder Evolution

The current builder should gain additive APIs:

```go
type AddResourceResult struct {
    ID       string
    Added    bool
    Replaced bool
}

func (db *DiagramBuilder) AddResourceChecked(models.Resource) (AddResourceResult, error)
func (db *DiagramBuilder) AddConnectionObject(models.Connection) (bool, error)
func (db *DiagramBuilder) SetMetadata(string, any)
func (db *DiagramBuilder) Snapshot() models.Diagram
func (db *DiagramBuilder) Finalize(time.Time) models.Diagram
```

Required behavior:

- Duplicate resource IDs are idempotent when identity matches.
- Conflicting duplicate IDs return an error.
- Connections are deduplicated.
- Finalized resources and connections are sorted deterministically.
- Every connection endpoint must exist.
- Callers do not mutate internal diagram pointers directly.

Existing methods remain wrappers during migration.

## Terraform Parsing Design

### Generic Decoder Responsibilities

The Terraform package owns:

- Raw state v4 decoding.
- `terraform show -json` state decoding.
- Module and indexed address preservation.
- Provider name extraction.
- Data-source versus managed-resource distinction.
- Dispatch to provider adapters.
- Warning collection for unsupported types.
- Graph finalization.

It must not contain provider-specific type maps or connection analyzers.

### Unknown Resources

Unknown managed resources should optionally be preserved rather than silently dropped:

```text
azure:unknown:azurerm_example
gcp:unknown:google_example
```

Initially preserve current skip behavior by default and introduce `--include-unknown`. Once frontend generic-node rendering is mature, consider making inclusion the default in a major release.

### Mixed-Provider State

A Terraform state can contain multiple providers. Each resource is dispatched independently, and each relationship analyzer only processes its own provider unless explicitly configured otherwise.

### Plan JSON

Terraform plan JSON contains configuration expressions and richer reference data than state JSON. Add explicit envelope detection instead of treating every document with `format_version` as state output. Plan support can then use configuration expression references to identify external and planned resources more accurately.

## AWS Migration

AWS must be moved behind the new interfaces before Azure or GCP implementations begin.

1. Add golden tests for existing raw-state and show-JSON output.
2. Move `terraformTypeMap` into an AWS Terraform adapter.
3. Move AWS relationship analyzers and reference rules into `providers/aws`.
4. Make `AWSScanner` implement `Scanner` while preserving `ScanAccount`.
5. Split live scanning into independently testable service collectors.
6. Add pagination and deterministic sorting.
7. Preserve all existing resource IDs and JSON fields.

## Azure Implementation

### Authentication

Use Azure's default credential chain:

```go
credential, err := azidentity.NewDefaultAzureCredential(nil)
```

This supports environment credentials, workload identity, managed identity, and Azure CLI credentials. CLI flags select tenant and subscription scope but must not accept or serialize client secrets.

### Live Inventory

Use Azure Resource Graph for broad inventory:

- `github.com/Azure/azure-sdk-for-go/sdk/azidentity`
- `github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph`
- Optional subscription discovery through `armsubscriptions`.

Require explicit subscription IDs initially. Resource Graph reduces generated SDK dependencies and can query multiple subscriptions, but service-specific clients may later enrich resources that require details unavailable in Resource Graph.

### CLI

```text
terraviz scan azure \
  --subscriptions subscription-a,subscription-b \
  --locations eastus,westeurope \
  --tenant-id tenant-id \
  --cloud public
```

### Initial Terraform and Live Coverage

- Virtual machines and virtual machine scale sets.
- Virtual networks and subnets.
- Network interfaces and network security groups.
- Public IP addresses, load balancers, and application gateways.
- Storage accounts and containers where available.
- SQL servers and databases.
- App Service plans, web apps, and Function Apps.
- Key Vaults.
- Managed identities.
- Private endpoints.
- Resource groups as optional hierarchy containers.

### Initial Relationships

- VM -> network interface.
- Network interface -> subnet.
- Subnet -> virtual network.
- Network interface/subnet -> network security group.
- Load balancer -> frontend IP/subnet/backend pool.
- Function App/web app -> App Service plan.
- SQL database -> SQL server.
- Private endpoint -> subnet and target resource.
- Managed identity -> role assignment when role assignments are scanned.

### Azure Canonicalization

- ARM resource IDs are case-insensitive and must be lower-cased before indexing.
- Subscription, resource group, provider namespace, type, and name are parsed from the ARM ID.
- Tenant-level Azure AD resources must not be assigned an artificial subscription scope.

## GCP Implementation

### Authentication

Use Application Default Credentials:

- Workload identity when running in Google Cloud.
- `gcloud auth application-default login` for local use.
- `GOOGLE_APPLICATION_CREDENTIALS` for service-account files.

An optional `--credentials-file` can be supported, but neither its contents nor path should be stored in scan output.

### Live Inventory

Use Cloud Asset Inventory for broad inventory:

- `cloud.google.com/go/asset/apiv1`
- `google.golang.org/api/iterator`
- Optional Resource Manager APIs for project discovery.

Require explicit project IDs initially. Organization-wide scans require broader permissions, introduce high resource counts, and should be a later opt-in capability.

### CLI

```text
terraviz scan gcp \
  --projects project-a,project-b \
  --locations us-central1,europe-west1 \
  --quota-project billing-project
```

### Initial Terraform and Live Coverage

- Compute Engine instances.
- VPC networks, subnetworks, and firewall rules.
- Instance groups and network endpoint groups.
- Forwarding rules, target proxies, and backend services.
- Cloud Storage buckets.
- Cloud SQL instances.
- Cloud Functions and Cloud Run services.
- GKE clusters.
- Service accounts.
- Pub/Sub topics and subscriptions.
- VPC access connectors.

### Initial Relationships

- Compute instance -> subnetwork/network.
- Compute instance -> service account.
- Firewall rule -> network.
- Forwarding rule -> target proxy/backend service.
- Backend service -> instance group/network endpoint group.
- Cloud Run/Function -> service account and VPC connector.
- Cloud SQL instance -> network.
- GKE cluster -> network/subnetwork.
- Pub/Sub subscription -> topic.
- Storage notification -> Pub/Sub topic.

### GCP Canonicalization

GCP APIs return full URLs, relative self-links, project IDs, and project numbers. Normalize them into full resource names before indexing and relationship matching.

## CLI Integration

Provider commands remain explicit because their credentials and scope flags differ:

```text
terraviz scan terraform <state-file>
terraviz scan aws --regions ... --profile ...
terraviz scan azure --subscriptions ... --locations ...
terraviz scan gcp --projects ... --locations ...
```

Extract common execution behavior:

```go
func runScannerCommand(
    cmd *cobra.Command,
    scanner ingest.Scanner,
    request ingest.ScanRequest,
) error
```

Each provider command only reads flags, creates typed configuration, constructs a scanner, and invokes the shared runner. Commands use `cmd.Context()` for cancellation.

## Server and Object Storage

### Live Scanning

Keep live cloud scans CLI-only initially. HTTP-triggered live scans require asynchronous jobs, authorization, quotas, cancellation, credential policy, and protection against expensive or unbounded scans.

If added later, use:

```go
type ScanService interface {
    Start(context.Context, Provider, ScanRequest) (string, error)
    Status(context.Context, string) (ScanJob, error)
    Result(context.Context, string) (*models.ScanResult, error)
    Cancel(context.Context, string) error
}
```

### Remote State Imports

Generalize S3 imports behind `ObjectStore` and add:

```text
GET  /api/storage/:provider/containers
GET  /api/storage/:provider/objects
POST /api/storage/:provider/load
```

Keep `/api/s3/*` as compatibility wrappers.

Provider mappings:

- AWS -> S3 bucket/object.
- Azure -> Blob Storage account/container/blob.
- GCP -> Cloud Storage bucket/object.

Use server-side credential chains. Do not accept raw long-lived secrets through browser requests.

## Frontend Design

### Provider-Neutral Hierarchy

Replace hard-coded AWS layers and `resource.type.split(":")` inference with explicit fields.

Default grouping:

```text
Provider -> Category -> Service -> Resource
```

Examples:

```text
AWS -> Networking -> VPC -> production-vpc
Azure -> Compute -> Virtual Machines -> api-vm
GCP -> Storage -> Cloud Storage -> assets-bucket
```

For single-provider diagrams, the provider box can be omitted while retaining category and service hierarchy.

### Provenance

`not-in-state-file` is provenance rather than a cloud service. Inferred resources should remain in their semantic provider/category/service hierarchy and use:

- Dashed card border.
- Presence badge.
- Inference details in the resource panel.
- Connection description and inference rule.

The current standalone `not-in-state-file` group remains during compatibility migration, then moves to visual decoration once explicit classification is available.

### Filters

Add facets for:

- Provider.
- Category.
- Service.
- Resource type.
- Scope.
- Region/location.
- Zone.
- Lifecycle state.
- Source/provenance.

### List, Details, and CSV

Use provider-neutral labels:

- `Scope` instead of only `Account`.
- `Location` and `Zone` instead of only `Region`.
- `Presence` or `Origin` in addition to `in_state_file`.

CSV should retain `in_state_file` for compatibility and add provider-neutral fields when the model supports them.

### Icons

Use provider-namespaced embedded assets:

```text
web/icons/
  neutral/
  aws/
  azure/
  gcp/
```

Resolution order:

1. Resource-specific `icon_url`.
2. Provider and native-type registry.
3. Provider service icon.
4. Provider generic icon.
5. Neutral category icon.
6. Neutral unknown-resource icon.

Use a single frontend icon resolver with an image-load fallback. Icons must remain same-origin so PNG and SVG export continue working offline. Verify Azure and Google Cloud icon redistribution licenses before embedding them.

## Capability API

Add an optional endpoint so the frontend does not hard-code providers and imports:

```text
GET /api/capabilities
```

Example:

```json
{
  "providers": ["aws", "azure", "gcp"],
  "terraform_providers": ["aws", "azurerm", "azuread", "google", "google-beta"],
  "live_scanners": ["aws", "azure", "gcp"],
  "object_stores": ["s3", "azure-blob", "gcs"]
}
```

## Dependencies

Likely Azure dependencies:

```text
github.com/Azure/azure-sdk-for-go/sdk/azidentity
github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph
github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions
```

Likely GCP dependencies:

```text
cloud.google.com/go/asset
google.golang.org/api
```

Only add SDK dependencies when implementing the corresponding live scanner. Terraform adapters do not need cloud SDKs.

Risks include increased binary size, compile time, dependency update frequency, and release artifact size. Validate `CGO_ENABLED=0` and all six existing OS/architecture release targets after each SDK addition.

## Backward Compatibility

- Preserve `scan terraform`, `scan aws`, `compare`, and `serve` behavior.
- Preserve `NewTerraformParser`, `NewAWSScanner`, `ScanAccount`, and `NewServer` wrappers.
- Preserve existing AWS Terraform and live-scan IDs.
- Preserve existing JSON fields and `DiagramVersion` while changes are additive.
- Keep `/api/s3/*` routes after generic storage routes are added.
- Keep old diagrams readable when new fields are absent.
- Do not change comparison identity until native-ID matching is separately designed and versioned.

## Security Requirements

- Never serialize access keys, client secrets, tokens, credential JSON, or credential file contents.
- Redact provider SDK errors before including them in user-visible scan output if they contain request headers or credential paths.
- Prefer default credential chains and workload identity.
- Require explicit Azure subscriptions and GCP projects by default.
- Apply API call timeouts, cancellation, pagination limits, and bounded concurrency.
- Keep live scanning disabled in the web server until a secure asynchronous job model exists.
- Do not allow arbitrary SDK endpoint overrides through unauthenticated HTTP APIs.

## Determinism and Performance

- Sort resources by provider, scope, type, and ID before finalizing.
- Sort connections by source ID, target ID, type, and description.
- Deduplicate resources by canonical provider-native identity.
- Deduplicate connections by stable connection identity.
- Use provider/scope/type/native-ID indexes for relationship analysis.
- Use bounded collector concurrency.
- Paginate every list/search API.
- Preserve partial scan results and warnings when one collector fails.

## Testing Strategy

### Characterization Tests

Before refactoring AWS:

- Golden-test representative raw Terraform state.
- Golden-test `terraform show -json` state.
- Record resource IDs, types, icons, warnings, statistics, and connection direction.
- Add tests for inferred resources not present in state.

### Shared Scanner Contract

Every scanner must satisfy tests for:

- Correct provider.
- Unique, non-empty resource IDs.
- Valid connection endpoints.
- Non-nil properties and tags.
- Statistics matching diagram lengths.
- Context cancellation.
- Deterministic output.
- No credentials in output.
- Partial collector failure behavior.

### Terraform Adapter Tests

For AWS, Azure, and GCP:

- Known type normalization.
- Unknown type warning or generic inclusion.
- Provider alias handling.
- Root, child-module, count, and `for_each` addresses.
- Native ID and scope extraction.
- Tags/labels.
- Region and zone extraction.
- External references and deduplication.
- Relationship type and direction.
- Malformed nested properties.
- Mixed-provider state isolation.

### Live Scanner Tests

Use narrow local interfaces around vendor SDK clients. Test:

- Pagination.
- Multiple regions/subscriptions/projects.
- Global and regional resources.
- Partial permissions.
- Timeouts and cancellation.
- Empty and partial SDK responses.
- Deduplication across collectors.
- Deterministic ordering.

### Graph Tests

- Duplicate resource identities.
- Duplicate connections.
- Missing endpoints.
- Cross-provider value collisions.
- Concurrent collection and finalization.
- `go test -race ./...`.

### Server Tests

- Existing `/api/s3/*` compatibility.
- Generic object-store routes.
- Upload and download size limits.
- Parser errors.
- Credential redaction.
- Provider filtering.
- Concurrent load/read operations.

### Compatibility Tests

Store representative v1 JSON and verify that it:

- Still unmarshals.
- Is served by the API.
- Renders with frontend normalization defaults.
- Supports filtering and comparison.
- Does not require newly added optional fields.

## Delivery Phases

### Phase 0: Characterize Current Behavior

- Add AWS Terraform golden fixtures.
- Add mocked AWS scanner tests.
- Add builder uniqueness and connection-integrity tests.
- Add JSON backward-compatibility fixtures.
- Establish `go test -race ./...` baseline.

### Phase 1: Introduce Interfaces

- Add provider constants and normalization helpers.
- Add `Scanner`, `Collector`, and typed scan configuration.
- Make AWS scanner satisfy `Scanner` while preserving `ScanAccount`.
- Add parser and relationship contracts.
- Add builder finalization and deterministic sorting.
- No Azure/GCP SDK dependencies yet.

### Phase 2: Move AWS Behind Interfaces

- Extract AWS Terraform type mapping.
- Extract AWS relationship and external-reference rules.
- Split the AWS live scanner into collectors.
- Verify semantic equivalence with golden output.
- Preserve CLI and API behavior.

### Phase 3: Azure and GCP Terraform

- Add `azurerm`, `azuread`, `azapi`, `google`, and `google-beta` adapters.
- Add initial normalized resource types and relationships.
- Add provider-neutral model fields.
- Add mixed-provider state fixtures.
- Preserve unsupported-resource warnings.

### Phase 4: Provider-Neutral Frontend

- Add frontend normalization for old and new resource payloads.
- Replace AWS hard-coded hierarchy with provider/category/service grouping.
- Add neutral filters, details, list columns, and CSV fields.
- Add provider-aware icon registry and neutral fallbacks.
- Preserve rendering of existing diagrams.

### Phase 5: Azure Live Scanner

- Add Azure Resource Graph client abstraction.
- Add subscription-scoped collection and ARM ID normalization.
- Add relationships and inferred external resources.
- Add `scan azure` CLI command.
- Add pagination, cancellation, partial-permission, and contract tests.

### Phase 6: GCP Live Scanner

- Add Cloud Asset Inventory client abstraction.
- Add project-scoped collection and full-resource-name normalization.
- Add relationships and inferred external resources.
- Add `scan gcp` CLI command.
- Add pagination, cancellation, partial-permission, and contract tests.

### Phase 7: Multi-Cloud Object Stores

- Extract current S3 behavior behind `ObjectStore`.
- Add Azure Blob Storage.
- Add Google Cloud Storage.
- Add generic storage routes and capability discovery.
- Preserve legacy S3 routes.

### Phase 8: Identity-Aware Comparison

- Populate `NativeID` and structured scope consistently.
- Match comparisons by provider/native identity when safe.
- Define migration behavior for changed IDs.
- Increment the diagram schema version only if existing semantics change.

## Risks and Mitigations

### Identity Instability

Changing IDs can make comparisons report every resource as removed and added. Preserve current IDs and add `NativeID` first.

### Cross-Provider False Relationships

Two clouds can use identical names or IDs. Require provider and scope in graph indexes and relationship rules.

### Provider Terminology Mismatch

`Account` and `Region` do not fully represent subscription/project/tenant/zone semantics. Add structured scope and location while retaining compatibility fields.

### SDK and Binary Growth

Azure and GCP SDKs increase binary and build size. Add only broad-inventory clients initially and measure release artifacts after each dependency.

### Incomplete Inventory

Azure Resource Graph and Cloud Asset Inventory may omit detailed child resources. Start with broad inventory and add service-specific enrichment only where required.

### Credentials Leakage

Keep credentials in provider credential chains and typed runtime configuration. Add tests that scan results and logs do not contain secrets.

### Rate Limits and Pagination

Use bounded concurrency, full pagination, provider SDK retry policies, and context-aware timeouts.

### Icon Licensing

Confirm redistribution rights before embedding Azure and Google Cloud icon packs. Provide neutral icons when licensing is unclear.

### Nondeterministic Output

Concurrent collectors can append in completion order. Sort resources, connections, errors, and warnings before finalization.

## Acceptance Criteria

The extensibility foundation is complete when:

- Existing AWS Terraform and live scans produce equivalent resources and relationships.
- AWS, Azure, and GCP Terraform resources can coexist in one state file.
- Provider adapters can be registered without modifying the generic Terraform decoder.
- Relationship analyzers cannot connect different providers accidentally.
- Azure and GCP scanners satisfy the common scanner contract.
- Every connection endpoint exists and every resource ID is unique.
- Multi-cloud hierarchy renders provider, category, service, and resources correctly.
- Missing provider icons fall back to neutral icons rather than AWS icons.
- Existing v1 diagram JSON remains loadable and renderable.
- Existing AWS CLI commands and S3 routes remain compatible.
- No scan result, log, or API response contains credentials.
- `go test ./...`, `go test -race ./...`, `go vet ./...`, and release-target builds pass.
