package api

import (
	"net/http"
	"os/exec"
	"strings"
	"testing"
)

func TestEmbeddedAssetsServed(t *testing.T) {
	h := newServer(t).Handler()
	cases := []struct {
		path, contentType, contains string
	}{
		{"/", "text/html", "<title>terraquery</title>"},
		{"/style.css", "text/css", "--accent"},
		{"/app.js", "text/javascript", "renderGraph"},
		{"/icons/aws.svg", "image/svg+xml", "#FF9900"},
		{"/icons/gcp.svg", "image/svg+xml", "#4285F4"},
		{"/icons/azure.svg", "image/svg+xml", "#0078D4"},
		{"/icons/aws/lambda.png", "image/png", ""},
		{"/icons/aws/sqs.png", "image/png", ""},
	}
	for _, c := range cases {
		w := do(t, h, http.MethodGet, c.path, nil, "")
		if w.Code != 200 {
			t.Errorf("GET %s: %d", c.path, w.Code)
			continue
		}
		if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, c.contentType) {
			t.Errorf("GET %s: content-type %q, want %q", c.path, ct, c.contentType)
		}
		if !strings.Contains(w.Body.String(), c.contains) {
			t.Errorf("GET %s: body missing %q", c.path, c.contains)
		}
	}
}

// The overlay uses display:flex, which would otherwise beat the hidden
// attribute's UA display:none and leave the drawer permanently open.
func TestOverlayHiddenRulePresent(t *testing.T) {
	css, err := webFS.ReadFile("web/style.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{
		".app-shell{", ".sidebar{", ".col-right{", "@media(max-width:1100px)",
		".icon-sprite{", ".icon{", "tbody tr:hover", `html[data-theme="dark"]`,
		".flow-line{", ".tile-box{",
		":focus-visible", ".sr-only", "#error[hidden]{display:none}",
		"@media(max-width:640px)", `main[aria-busy="true"]::before`,
		"prefers-reduced-motion", "overscroll-behavior:contain",
		"color-scheme:dark", "touch-action:manipulation", "skip-link",
	} {
		if !strings.Contains(string(css), ref) {
			t.Errorf("style.css missing accessibility/responsive rule %s", ref)
		}
	}
	if !strings.Contains(string(css), ".overlay[hidden]{display:none}") {
		t.Fatal("missing .overlay[hidden] rule: close button would not work")
	}
	if !strings.Contains(string(css), ".archfull[hidden]{display:none}") {
		t.Fatal("missing .archfull[hidden] rule: full-screen close would not work")
	}
}

func TestProviderIconsHaveExplicitSize(t *testing.T) {
	for _, name := range []string{"aws", "gcp", "azure", "unknown"} {
		b, err := webFS.ReadFile("web/icons/" + name + ".svg")
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if !strings.Contains(s, `width="24"`) || !strings.Contains(s, `height="24"`) {
			t.Errorf("icon %s lacks explicit size: %s", name, s[:80])
		}
	}
	for _, name := range []string{"lambda", "s3", "sqs", "api-gateway", "alb"} {
		b, err := webFS.ReadFile("web/icons/aws/" + name + ".png")
		if err != nil || len(b) < 100 {
			t.Errorf("official AWS icon %s missing or empty: %v", name, err)
		}
	}
}

// Evaluates the real icon mapping from app.js so a broken lookup (e.g. an
// "undefined.png" URL) fails the build instead of silently showing no icons.
func TestServiceIconMapping(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	script := `
const fs = require('fs');
const src = fs.readFileSync('web/app.js', 'utf8');
const cut = (a, b) => { const i = src.indexOf(a), j = src.indexOf(b, i); if (i < 0 || j < 0) throw new Error('markers missing'); return src.slice(i, j); };
eval(cut('function cloudOf(type) {', '// ---- free-flowing canvas'));
eval(cut('const AWS_ICONS = {', '// Some SVG renderers'));
const cases = {
  'aws_lambda_function': '/icons/aws/lambda.png',
  'aws_s3_bucket': '/icons/aws/s3.png',
  'aws_sqs_queue': '/icons/aws/sqs.png',
  'aws_api_gateway_rest_api': '/icons/aws/api-gateway.png',
  'aws_ecs_task_definition': '/icons/aws/ecs.png',
  'aws_lb': '/icons/aws/alb.png',
  'aws_cost_report': '/icons/aws.svg',
  'google_cloud_run_service': '/icons/gcp.svg',
  'azurerm_storage_account': '/icons/azure.svg',
};
for (const [type, want] of Object.entries(cases)) {
  const got = serviceIcon(type);
  if (got !== want) { console.error(type + ' -> ' + got + ', want ' + want); process.exit(1); }
}
console.log('ok');
`
	cmd := exec.Command(node, "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("serviceIcon mapping: %v\n%s", err, out)
	}
}

func TestLayoutAndDiagramLabels(t *testing.T) {
	css, err := webFS.ReadFile("web/style.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{`.app-shell{display:flex;min-height:100vh`, `.graphwrap{position:relative`,
		`.graphwrap.nogrid{background-image:none}`} {
		if !strings.Contains(string(css), ref) {
			t.Errorf("style.css missing %s", ref)
		}
	}
	js, err := webFS.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{`step-badge`, `step-badge-text`} {
		if strings.Contains(string(js), gone) {
			t.Errorf("app.js still renders %s on the diagram", gone)
		}
	}
	for _, fn := range []struct{ start, end string }{
		{`function drawEdges(`, `function cloudOf(`},
		{`function drawNumberedEdges(`, `function renderResourceGraph(`},
	} {
		i := strings.Index(string(js), fn.start)
		j := strings.Index(string(js), fn.end)
		if i < 0 || j < i {
			t.Fatalf("missing function region %s", fn.start)
		}
		if strings.Contains(string(js)[i:j], "svgEl(\"text\"") {
			t.Errorf("%s still labels edges with confidence", fn.start)
		}
	}
	if !strings.Contains(string(js), `e.target.closest?.(".node, button")`) {
		t.Error("pan handler still captures clicks on nodes or canvas buttons")
	}
	if !strings.Contains(string(js), `$("canvasBackBtn").addEventListener("click", drillOut)`) {
		t.Error("canvas back button is not wired to drillOut")
	}
}

func TestIndexReferencesEmbeddedAssets(t *testing.T) {
	index, err := webFS.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(index)
	for _, ref := range []string{
		`href="style.css?v=18"`, `src="app.js?v=18"`, `src="icons/aws.svg"`,
		`id="services"`, `id="graphHint"`,
		`id="enlargeBtn"`, `id="archFull"`,
		`id="graphWrap"`, `id="zoomInBtn"`, `id="canvasBackBtn"`,
		`id="stateFilterBtn"`, `id="stateMenu"`, `id="stateChecks"`,
		`id="srStatus"`, `id="errorRetry"`, `role="tabpanel"`,
		`aria-selected`, `aria-busy`, `skip-link`, `theme-color`,
		`width="24" height="24"`, `class="app-shell"`, `class="sidebar"`,
		`class="col-left"`, `class="col-right"`, `id="graphCountTop"`,
		`class="icon-sprite"`, `id="i-network"`, `id="i-maximize"`, `id="i-plus"`,
		`id="themeBtn"`, `id="themeIcon"`, `id="i-sun"`, `id="i-moon"`,
		`id="accountCountTop"`,
		`id="graphSearch"`, `id="exportMenu"`, `id="minimapSvg"`, `id="traceDepth"`,
		`class="floating-toolbar"`, `class="canvas-filters"`, `id="viewMenu"`,
	} {
		if !strings.Contains(html, ref) {
			t.Errorf("index.html missing %s", ref)
		}
	}
	for _, gone := range []string{
		`id="min"`, `id="bars"`, `id="stateList"`, `id="stateCount"`,
		`id="stateSelect"`, `id="backBtn"`, `id="archBackBtn"`, `id="crumbs"`,
	} {
		if strings.Contains(html, gone) {
			t.Errorf("index.html still contains removed overview element %s", gone)
		}
	}
	js, err := webFS.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{
		`renderResourceGraph`, `/api/resources`, `state.drill`, `enlargeArch`,
		`serviceIcon`, `AWS_ICONS`, `zoomAt`, `setBaseView`, `graphWrap`,
		`service-group`, `grouped by statefile`, `trapFocus`, `selectTab`,
		`aria-busy`, `srStatus`, `aria-label`, `location.hash`, `history.replaceState`,
		`renderStateFilter`, `closeStateMenu`, `canvasBackBtn`,
		`accountsFor`, `accountBadge`, `shortAccount`, `scope_kind`,
		`applyTheme`, `savedTheme`, `prefers-color-scheme`,
		`updateHash`, `drawMinimap`, `traceFrom`, `exportCanvas`,
	} {
		if !strings.Contains(string(js), ref) {
			t.Errorf("app.js missing %s", ref)
		}
	}
	for _, asset := range []string{"web/style.css", "web/app.js"} {
		b, err := webFS.ReadFile(asset)
		if err != nil || len(b) == 0 {
			t.Errorf("embedded asset %s missing or empty: %v", asset, err)
		}
	}
}

func TestStateName(t *testing.T) {
	cases := []struct{ name, file, want string }{
		{"app", "ignored.tfstate", "app"},
		{"", "prod/network.tfstate", "network"},
		{"", "/tmp/x/terraform.tfstate", "terraform"},
		{"", "state.json", "state"},
		{"", "C:\\x\\thing.tfstate", "thing"},
	}
	for _, c := range cases {
		if got := stateName(c.name, c.file); got != c.want {
			t.Errorf("stateName(%q,%q) = %q, want %q", c.name, c.file, got, c.want)
		}
	}
}

func TestMinParamBounds(t *testing.T) {
	for _, c := range []struct {
		query string
		want  float64
		ok    bool
	}{
		{"", 0.5, true},
		{"?min=0.9", 0.9, true},
		{"?min=0", 0, true},
		{"?min=1", 1, true},
		{"?min=-0.1", 0, false},
		{"?min=1.5", 0, false},
		{"?min=abc", 0, false},
	} {
		req := newTestRequest(http.MethodGet, "/api/deps"+c.query)
		got, err := minParam(req)
		if (err == nil) != c.ok {
			t.Errorf("minParam%s: err=%v, want ok=%v", c.query, err, c.ok)
		} else if err == nil && got != c.want {
			t.Errorf("minParam%s = %v, want %v", c.query, got, c.want)
		}
	}
}

func TestRequestSizeLimit(t *testing.T) {
	s := newServer(t)
	s.MaxBytes = 128
	w := do(t, s.Handler(), http.MethodPost, "/api/states",
		[]byte(`{"states":[{"name":"x","content":"`+strings.Repeat("a", 512)+`"}]}`),
		"application/json")
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized upload: %d %s", w.Code, w.Body.String())
	}
}

func TestMethodAndThresholdBehavior(t *testing.T) {
	s := newServer(t)
	h := s.Handler()
	if w := do(t, h, http.MethodPost, "/api/health", nil, ""); w.Code != 405 {
		t.Errorf("POST health: %d", w.Code)
	}
	if w := ingestBoth(t, s); w.Code != 200 {
		t.Fatalf("ingest: %s", w.Body.String())
	}
	// 0.99 is above the strongest edge (0.95): no deps, but raw edges still exist.
	w := do(t, h, http.MethodGet, "/api/deps?min=0.99", nil, "")
	if !strings.Contains(w.Body.String(), `"dependencies":[]`) {
		t.Fatalf("high threshold deps: %s", w.Body.String())
	}
	// min=0 includes low-confidence name evidence that the default hides.
	w = do(t, h, http.MethodGet, "/api/deps?min=0", nil, "")
	if !strings.Contains(w.Body.String(), `"evidence":7`) {
		t.Fatalf("zero threshold deps: %s", w.Body.String())
	}
}
