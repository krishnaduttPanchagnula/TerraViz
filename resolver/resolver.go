// Package resolver infers cross-state dependencies.
//
// Approach: every managed resource in every state "exports" identifiers
// (ARN, hostnames, ids, names). Every string anywhere in another state
// (resource attributes, data sources, outputs) is a potential "reference".
// A reference that matches another state's identifier becomes an edge with
// a confidence that depends on how specific the identifier kind is and how
// many states own the same value.
package resolver

import (
	"net/url"
	"sort"
	"strings"

	"terraviz/state"
)

type Source struct {
	Name     string
	Filename string
	File     *state.File
}

// Edge is one piece of evidence that From depends on To.
type Edge struct {
	From       string  `json:"from"`
	FromAddr   string  `json:"from_addr"`
	FromAttr   string  `json:"from_attr"`
	To         string  `json:"to"`
	ToAddr     string  `json:"to_addr"`
	Kind       string  `json:"kind"` // arn | host | id | name | remote_state
	Matched    string  `json:"matched"`
	Confidence float64 `json:"confidence"`
}

// Dep is the state-level rollup of edges.
type Dep struct {
	From       string   `json:"from"`
	To         string   `json:"to"`
	Confidence float64  `json:"confidence"` // max over evidence
	Evidence   int      `json:"evidence"`
	Kinds      []string `json:"kinds"`
}

type ident struct {
	value string
	kind  string
	owner string
	addr  string
}

var kindRank = map[string]int{"name": 1, "id": 2, "host": 3, "arn": 4}

var exactConf = map[string]float64{"arn": 0.95, "host": 0.9, "id": 0.7, "name": 0.3}

// Names are matched exactly only; substring matching on names is too noisy.
var substrConf = map[string]float64{"arn": 0.9, "host": 0.85, "id": 0.55}

var hostKeys = map[string]bool{
	"dns_name": true, "domain_name": true, "endpoint": true, "address": true,
	"invoke_url": true, "url": true, "api_endpoint": true, "fqdn": true,
	"bucket_domain_name": true, "bucket_regional_domain_name": true,
	"primary_endpoint_address": true, "reader_endpoint": true, "cname": true,
	"default_hostname": true, "vault_uri": true, "primary_blob_endpoint": true,
	"hostname": true,
}

var nameKeys = map[string]bool{
	"name": true, "function_name": true, "bucket": true,
	"queue_name": true, "cluster_name": true, "table_name": true,
}

var genericNames = map[string]bool{
	"default": true, "main": true, "production": true, "staging": true,
	"private": true, "public": true,
}

// Regional service hosts are shared by everyone, so a URL on one of these is
// only meaningful as a full URL (e.g. an SQS queue URL).
var genericLabels = map[string]bool{
	"sqs": true, "sns": true, "s3": true, "dynamodb": true, "sts": true,
	"kms": true, "lambda": true, "events": true,
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func hostOf(v string) string {
	v = strings.TrimSpace(v)
	host, full := v, false
	if strings.Contains(v, "://") {
		u, err := url.Parse(v)
		if err != nil || u.Hostname() == "" {
			return ""
		}
		host, full = u.Hostname(), true
	} else if i := strings.LastIndex(v, ":"); i > 0 && isDigits(v[i+1:]) {
		host = v[:i] // strip :port
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	dot := strings.Index(host, ".")
	if dot < 0 {
		return ""
	}
	if genericLabels[host[:dot]] {
		if full {
			return strings.ToLower(v)
		}
		return ""
	}
	return host
}

func usable(id ident) bool {
	v := id.value
	if v == "" || isDigits(v) { // also drops bare 12-digit account ids
		return false
	}
	switch id.kind {
	case "arn":
		return strings.HasPrefix(v, "arn:")
	case "host":
		return strings.Contains(v, ".")
	case "id":
		return len(v) >= 8
	case "name":
		return len(v) >= 6 && !genericNames[strings.ToLower(v)]
	}
	return false
}

func buildIndex(srcs []Source) []ident {
	var out []ident
	pos := map[string]int{}
	add := func(id ident) {
		if !usable(id) {
			return
		}
		k := id.owner + "|" + id.addr + "|" + id.value
		if i, ok := pos[k]; ok {
			if kindRank[id.kind] > kindRank[out[i].kind] {
				out[i].kind = id.kind
			}
			return
		}
		pos[k] = len(out)
		out = append(out, id)
	}
	for _, s := range srcs {
		for _, r := range s.File.Resources {
			if r.Mode != "managed" {
				continue
			}
			for _, inst := range r.Instances {
				addr := r.Address(inst.IndexKey)
				for k, v := range inst.Attributes {
					str, ok := v.(string)
					if !ok || str == "" {
						continue
					}
					switch {
					case k == "arn":
						add(ident{str, "arn", s.Name, addr})
					case hostKeys[k] || (k == "name" && r.Type == "aws_route53_record"):
						if h := hostOf(str); h != "" {
							add(ident{h, "host", s.Name, addr})
						}
					case k == "id":
						add(ident{str, "id", s.Name, addr})
					case nameKeys[k]:
						add(ident{str, "name", s.Name, addr})
					}
				}
				// API Gateway's default hostname is derived from id + region
				// and never appears as an attribute on the API itself.
				if r.Type == "aws_api_gateway_rest_api" || r.Type == "aws_apigatewayv2_api" {
					id, _ := inst.Attributes["id"].(string)
					arn, _ := inst.Attributes["arn"].(string)
					parts := strings.Split(arn, ":")
					if id != "" && len(parts) > 3 && parts[3] != "" {
						add(ident{id + ".execute-api." + parts[3] + ".amazonaws.com", "host", s.Name, addr})
					}
				}
			}
		}
	}
	return out
}

func isWord(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		c == '-' || c == '_' || c == '.'
}

// containsToken reports whether sub occurs in s as a whole token, so that
// "api.example.com" does not match "myapi.example.com" or "x.api.example.com",
// and a queue ARN does not match "<arn>-dlq".
func containsToken(s, sub string) bool {
	for off := 0; off < len(s); {
		i := strings.Index(s[off:], sub)
		if i < 0 {
			return false
		}
		i += off
		j := i + len(sub)
		if (i == 0 || !isWord(s[i-1])) && (j == len(s) || !isWord(s[j])) {
			return true
		}
		off = i + 1
	}
	return false
}

// Resolve returns every cross-state edge it can find, at any confidence.
// Callers filter by Confidence. Cost is O(strings x identifiers); swap the
// substring loop for Aho-Corasick if inputs get large.
func Resolve(srcs []Source) []Edge {
	idx := buildIndex(srcs)

	exact := map[string][]ident{}
	owners := map[string]map[string]bool{}
	for _, id := range idx {
		exact[id.value] = append(exact[id.value], id)
		if owners[id.value] == nil {
			owners[id.value] = map[string]bool{}
		}
		owners[id.value][id.owner] = true
	}

	best := map[string]Edge{}
	emit := func(e Edge) {
		k := strings.Join([]string{e.From, e.FromAddr, e.To, e.ToAddr, e.Kind}, "|")
		if old, ok := best[k]; !ok || e.Confidence > old.Confidence {
			best[k] = e
		}
	}

	// foreignOwners counts states other than `self` that own value.
	foreignOwners := func(value, self string) int {
		n := 0
		for o := range owners[value] {
			if o != self {
				n++
			}
		}
		return n
	}

	for _, a := range srcs {
		scan := func(addr, attr, val string) {
			if val == "" {
				return
			}
			low := strings.ToLower(val)

			keys := []string{val}
			if low != val {
				keys = append(keys, low)
			}
			for _, k := range keys {
				for _, id := range exact[k] {
					n := foreignOwners(id.value, a.Name)
					if id.owner == a.Name || n == 0 {
						continue
					}
					emit(Edge{a.Name, addr, attr, id.owner, id.addr, id.kind, id.value,
						exactConf[id.kind] / float64(n)})
				}
			}

			for _, id := range idx {
				c, ok := substrConf[id.kind]
				if !ok || id.owner == a.Name || len(val) <= len(id.value) {
					continue
				}
				hay := val
				if id.kind == "host" {
					hay = low
				}
				n := foreignOwners(id.value, a.Name)
				if n == 0 || !containsToken(hay, id.value) {
					continue
				}
				emit(Edge{a.Name, addr, attr, id.owner, id.addr, id.kind, id.value,
					c / float64(n)})
			}
		}

		for _, r := range a.File.Resources {
			for _, inst := range r.Instances {
				addr := r.Address(inst.IndexKey)
				state.Leaves(inst.Attributes, "", func(p, v string) { scan(addr, p, v) })

				if r.Mode == "data" && r.Type == "terraform_remote_state" {
					cfg, _ := inst.Attributes["config"].(map[string]any)
					key, _ := cfg["key"].(string)
					for _, b := range srcs {
						if key != "" && b.Name != a.Name &&
							strings.Contains(strings.ToLower(key), strings.ToLower(b.Name)) {
							emit(Edge{a.Name, addr, "config.key", b.Name, "", "remote_state", key, 0.9})
						}
					}
				}
			}
		}
		for name, o := range a.File.Outputs {
			if o.Sensitive {
				continue
			}
			state.Leaves(o.Value, name, func(p, v string) { scan("output."+name, p, v) })
		}
	}

	edges := make([]Edge, 0, len(best))
	for _, e := range best {
		edges = append(edges, e)
	}
	sort.Slice(edges, func(i, j int) bool {
		a, b := edges[i], edges[j]
		if a.From != b.From {
			return a.From < b.From
		}
		if a.To != b.To {
			return a.To < b.To
		}
		if a.Confidence != b.Confidence {
			return a.Confidence > b.Confidence
		}
		return a.FromAddr+a.FromAttr < b.FromAddr+b.FromAttr
	})
	return edges
}

// Summarize rolls edges at or above min confidence up to state-level deps.
func Summarize(edges []Edge, min float64) []Dep {
	type acc struct {
		dep   Dep
		kinds map[string]bool
	}
	m := map[string]*acc{}
	for _, e := range edges {
		if e.Confidence < min {
			continue
		}
		k := e.From + "|" + e.To
		x := m[k]
		if x == nil {
			x = &acc{dep: Dep{From: e.From, To: e.To}, kinds: map[string]bool{}}
			m[k] = x
		}
		x.dep.Evidence++
		if e.Confidence > x.dep.Confidence {
			x.dep.Confidence = e.Confidence
		}
		x.kinds[e.Kind] = true
	}
	out := make([]Dep, 0, len(m))
	for _, x := range m {
		for k := range x.kinds {
			x.dep.Kinds = append(x.dep.Kinds, k)
		}
		sort.Strings(x.dep.Kinds)
		out = append(out, x.dep)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}
