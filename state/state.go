// Package state parses Terraform state files (format v4) and exposes just
// enough structure for dependency resolution.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

type File struct {
	Version   int               `json:"version"`
	Serial    int               `json:"serial"`
	Lineage   string            `json:"lineage"`
	Outputs   map[string]Output `json:"outputs"`
	Resources []Resource        `json:"resources"`
}

type Output struct {
	Value     any  `json:"value"`
	Sensitive bool `json:"sensitive"`
}

type Resource struct {
	Module    string     `json:"module"`
	Mode      string     `json:"mode"` // "managed" or "data"
	Type      string     `json:"type"`
	Name      string     `json:"name"`
	Provider  string     `json:"provider"`
	Instances []Instance `json:"instances"`
}

type Instance struct {
	IndexKey   any            `json:"index_key"`
	Attributes map[string]any `json:"attributes"`
}

// Address renders the Terraform address of one instance of the resource.
func (r Resource) Address(idx any) string {
	a := r.Type + "." + r.Name
	if r.Mode == "data" {
		a = "data." + a
	}
	if r.Module != "" {
		a = r.Module + "." + a
	}
	switch i := idx.(type) {
	case string:
		a += fmt.Sprintf("[%q]", i)
	case float64:
		a += fmt.Sprintf("[%d]", int(i))
	}
	return a
}

func Load(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b, path)
}

func Parse(b []byte, path string) (*File, error) {
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if f.Version < 4 {
		return nil, fmt.Errorf("%s: unsupported state version %d (need >= 4)", path, f.Version)
	}
	return &f, nil
}

// Keys that probably hold secrets are skipped so they never reach the
// resolver (or, later, the database). ARN-valued keys are always kept.
var secretKey = regexp.MustCompile(`(?i)(password|secret|token|private_key|access_key|api_key|credential)`)

func isSecretKey(k string) bool {
	return secretKey.MatchString(k) && !strings.HasSuffix(strings.ToLower(k), "arn")
}

// Leaves calls fn for every string leaf under v, in deterministic order,
// skipping secret-looking keys.
func Leaves(v any, path string, fn func(path, val string)) {
	switch t := v.(type) {
	case string:
		fn(path, t)
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			if !isSecretKey(k) {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			p := k
			if path != "" {
				p = path + "." + k
			}
			Leaves(t[k], p, fn)
		}
	case []any:
		for i, c := range t {
			Leaves(c, fmt.Sprintf("%s[%d]", path, i), fn)
		}
	}
}

var (
	arnAccount    = regexp.MustCompile(`^arn:aws[a-z-]*:([^:]*):[^:]*:(\d{12}):`)
	gcpProject    = regexp.MustCompile(`(?:^|/)projects/([a-z][a-z0-9-]{4,28}[a-z0-9])/`) // e.g. projects/acme-prod/
	azureSub      = regexp.MustCompile(`(?i)/subscriptions/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)
	bareAccountID = regexp.MustCompile(`^(\d{12})$`)
)

// Scope identifies the cloud account/project/subscription a resource lives in.
type Scope struct {
	Kind string `json:"kind"` // aws | gcp | azure
	ID   string `json:"id"`
}

// Scopes reads the identifiers in attrs and returns the distinct cloud
// accounts referenced there. Secret-looking values are skipped by Leaves.
func Scopes(attrs map[string]any) []Scope {
	seen := map[Scope]bool{}
	Leaves(attrs, "", func(path, v string) {
		if m := arnAccount.FindStringSubmatch(v); m != nil {
			seen[Scope{"aws", m[2]}] = true
		}
		if m := azureSub.FindStringSubmatch(v); m != nil {
			seen[Scope{"azure", strings.ToLower(m[1])}] = true
		}
		if m := gcpProject.FindStringSubmatch(v); m != nil {
			seen[Scope{"gcp", m[1]}] = true
		}
		if strings.Contains(strings.ToLower(path), "account") {
			if m := bareAccountID.FindStringSubmatch(v); m != nil {
				seen[Scope{"aws", m[1]}] = true
			}
		}
	})
	out := make([]Scope, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// scrub returns a deep copy of v with secret-looking keys removed.
func scrub(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if !isSecretKey(k) {
				out[k] = scrub(val)
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, c := range t {
			out[i] = scrub(c)
		}
		return out
	default:
		return v
	}
}

// Sanitized copies f with secret-looking attributes and sensitive outputs
// removed. Everything persisted (and re-resolved) goes through this first.
func Sanitized(f *File) *File {
	out := &File{Version: f.Version, Serial: f.Serial, Lineage: f.Lineage,
		Outputs: map[string]Output{}}
	for k, o := range f.Outputs {
		if !o.Sensitive {
			out.Outputs[k] = Output{Value: scrub(o.Value)}
		}
	}
	for _, r := range f.Resources {
		rc := r
		rc.Instances = make([]Instance, len(r.Instances))
		for i, inst := range r.Instances {
			clean := scrub(inst.Attributes)
			attrs, _ := clean.(map[string]any)
			rc.Instances[i] = Instance{IndexKey: inst.IndexKey, Attributes: attrs}
		}
		out.Resources = append(out.Resources, rc)
	}
	return out
}
