"use strict";

const $ = (id) => document.getElementById(id);
const KIND_COLORS = {
  arn: "#2563eb", host: "#3b82f6", remote_state: "#60a5fa",
  id: "#93c5fd", name: "#cbd5e1",
};
const PROVIDER_COLORS = { aws: "#FF9900", gcp: "#4285F4", azure: "#0078D4", unknown: "#8b98a5" };

const state = {
  states: [], hidden: new Set(), deps: [], edges: [], graph: null,
  resources: [], drill: null, view: null, traceKeys: null,
  hiddenResources: new Set(), customLinks: [], linkStart: null,
  s3: { bucket: "", prefix: "", token: "", selected: new Set(), objects: [] },
  samples: new Map(), picked: [],
};

function savedTheme() {
  try { return localStorage.getItem("tq-theme"); } catch { return null; }
}
function storeTheme(theme) {
  try { localStorage.setItem("tq-theme", theme); } catch { /* storage disabled */ }
}
function updateHash() {
  const p = new URLSearchParams();
  if (!$("viewDiagram").hidden) p.set("view", "diagram");
  if (state.drill) p.set("state", state.drill);
  const hash = p.toString() ? `#${p}` : "";
  if (location.hash !== hash) history.replaceState(null, "", location.pathname + location.search + hash);
}

function setView(view) {
  const diagram = view === "diagram";
  $("viewDashboard").hidden = diagram;
  $("viewDiagram").hidden = !diagram;
  for (const b of document.querySelectorAll("nav button[data-view]")) {
    const active = (b.dataset.view === "diagram") === diagram;
    b.classList.toggle("active", active);
    if (active) b.setAttribute("aria-current", "page");
    else b.removeAttribute("aria-current");
  }
  updateHash();
  if (!diagram) $("graphWrap")?.scrollIntoView?.({ block: "nearest" });
}

function applyTheme(theme, persist = true) {
  document.documentElement.dataset.theme = theme;
  const btn = $("themeBtn");
  const label = theme === "dark" ? "Switch to light theme" : "Switch to dark theme";
  btn.setAttribute("aria-label", label);
  btn.title = label;
  $("themeIcon").setAttribute("href", theme === "dark" ? "#i-sun" : "#i-moon");
  if (persist) storeTheme(theme);
}

(function initTheme() {
  const theme = savedTheme() || (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
  applyTheme(theme, false);
})();

$("themeBtn").addEventListener("click", () => {
  applyTheme(document.documentElement.dataset.theme === "dark" ? "light" : "dark");
});

function el(tag, attrs = {}, text) {
  const n = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === "class") n.className = v;
    else if (k === "disabled") n.disabled = !!v;
    else n.setAttribute(k, v);
  }
  if (text !== undefined) n.textContent = text;
  return n;
}

async function api(path, opts) {
  const res = await fetch(path, opts);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
  return data;
}

function showError(msg) { $("errorText").textContent = msg; $("error").hidden = false; }
function clearError() { $("error").hidden = true; }
function drawerMsg(msg, ok = true) {
  const n = $("drawerMsg");
  n.textContent = msg;
  n.classList.toggle("warn", !ok);
}

function table(node, cols, rows, formats = {}) {
  node.replaceChildren();
  if (!rows.length) { node.appendChild(el("div", { class: "empty" }, "No results.")); return; }
  const thead = el("thead"), hr = el("tr");
  for (const c of cols) hr.appendChild(el("th", {}, c));
  thead.appendChild(hr);
  const tbody = el("tbody");
  for (const row of rows) {
    const tr = el("tr");
    for (let i = 0; i < cols.length; i++) {
      const td = el("td", {}, formats[i] ? formats[i](row[i], row) : String(row[i] ?? ""));
      if (typeof row[i] === "number") td.classList.add("num");
      tr.appendChild(td);
    }
    tbody.appendChild(tr);
  }
  node.appendChild(thead); node.appendChild(tbody);
}

const conf = (v) => Number(v).toFixed(2);
function svgIcon(id) {
  const svg = svgEl("svg", { class: "icon", "aria-hidden": "true" });
  svg.appendChild(svgEl("use", { href: `#${id}` }));
  return svg;
}

const icon = (provider, cls = "") => el("img", {
  src: `/icons/${provider || "unknown"}.svg`, alt: provider || "",
  width: "18", height: "18", class: cls,
});
// ---- data loading + rendering ----
async function loadAll() {
  clearError();
  const min = 0.5;
  $("mainContent").setAttribute("aria-busy", "true");
  $("srStatus").textContent = "Loading architecture data…";
  try {
    const [s, d, e, g, r] = await Promise.all([
      api("/api/states"),
      api(`/api/deps?min=${min}`),
      api(`/api/edges?min=${min}`),
      api(`/api/graph?min=${min}`),
      api("/api/resources"),
    ]);
    state.states = s.states;
    const names = new Set(s.states.map((x) => x.name));
    for (const n of [...state.hidden]) if (!names.has(n)) state.hidden.delete(n);
    if (state.drill && state.hidden.has(state.drill)) state.drill = null;
    state.deps = d.dependencies.filter((x) => visible(x.from) && visible(x.to));
    state.edges = e.edges.filter((x) => visible(x.from) && visible(x.to));
    state.graph = {
      nodes: g.nodes.filter((n) => visible(n.id)),
      edges: g.edges.filter((x) => visible(x.from) && visible(x.to)),
    };
    state.resources = r.resources.filter((x) => visible(x.state));
    render();
    $("srStatus").textContent = `Loaded ${s.states.length} statefiles and ${d.dependencies.length} dependencies.`;
  } catch (err) {
    showError(err.message);
    $("srStatus").textContent = `Could not load data: ${err.message}`;
  } finally {
    $("mainContent").setAttribute("aria-busy", "false");
  }
}

const visible = (name) => !state.hidden.has(name);

function providerOf(name) {
  return state.states.find((s) => s.name === name)?.provider || "unknown";
}

const accountKey = (a) => `${a.kind}:${a.id}`;
const shortAccount = (a) => a.kind === "unknown" ? "unspecified"
  : `${a.kind}:${a.id.length > 14 ? a.id.slice(0, 12) + "…" : a.id}`;

function accountsFor(name) {
  const found = state.states.find((s) => s.name === name)?.accounts;
  return found?.length ? found : [{ kind: "unknown", id: "unspecified" }];
}

function accountBadge(a) {
  const n = el("span", { class: "account-badge" });
  n.appendChild(icon(a.kind === "unknown" ? "unknown" : a.kind));
  n.appendChild(el("span", {}, shortAccount(a)));
  n.title = `${a.kind} · ${a.id}`;
  return n;
}

function render() {
  const { states, deps, edges } = state;
  $("statStates").textContent = states.length;
  $("statDeps").textContent = deps.length;
  $("statEdges").textContent = edges.length;
  $("statClouds").textContent = new Set(states.map((s) => s.provider).filter((p) => p !== "unknown")).size;

  $("depCount").textContent = deps.length;
  $("edgeCount").textContent = edges.length;
  $("graphCount").textContent = `${state.graph?.edges.length || 0} links`;
  $("graphCountTop").textContent = `${states.length} states`;
  const accounts = new Set();
  for (const s of states) for (const a of accountsFor(s.name)) accounts.add(accountKey(a));
  $("accountCountTop").textContent = `· ${accounts.size} accounts`;

  renderStateFilter();
  renderDonut(edges);
  renderGraph(state.graph);
  state.traceKeys = null;
  applyCanvasFilters();
  drawMinimap();
  renderServices();
  $("canvasBackBtn").hidden = !state.drill;
  $("archFullTitle").textContent = state.drill ? `${state.drill} — architecture` : "Dependency graph";
  updateHash();
  $("graphHint").textContent = state.drill
    ? "Dashed boxes are in other statefiles. Click for details, Shift-click two services to link them, double-click to hide one."
    : "Drag to pan, scroll to zoom. Click a state box to inspect its services.";

  table($("depsTable"), ["From", "To", "Conf", "Evidence", "Kinds"],
    deps.map((d) => [d.from, d.to, conf(d.confidence), d.evidence, (d.kinds || []).join(", ")]));
  table($("edgesTable"), ["From", "Address", "To", "Kind", "Conf", "Matched"],
    edges.map((e) => [e.from, e.from_addr, e.to, e.kind, conf(e.confidence), e.matched]));
}

function closeStateMenu() {
  $("stateMenu").hidden = true;
  $("stateFilterBtn").setAttribute("aria-expanded", "false");
}

function renderStateFilter() {
  const shown = state.states.filter((s) => visible(s.name)).length;
  $("stateFilterLabel").textContent = state.drill
    ? state.drill
    : shown === state.states.length ? "All states" : `${shown}/${state.states.length} states`;
  const box = $("stateChecks");
  box.replaceChildren();
  if (!state.states.length) {
    box.appendChild(el("p", { class: "hint" }, "No statefiles yet."));
    return;
  }

  const allBtn = el("button", { type: "button", class: "menu-all" }, "Show all statefiles");
  allBtn.addEventListener("click", () => { state.hidden.clear(); loadAll(); });
  box.appendChild(allBtn);

  // Group states by the cloud account/project/subscription they reference.
  const groups = new Map();
  for (const s of state.states) {
    for (const a of accountsFor(s.name)) {
      const k = accountKey(a);
      if (!groups.has(k)) groups.set(k, { scope: a, names: new Set() });
      groups.get(k).names.add(s.name);
    }
  }
  for (const [, group] of [...groups.entries()].sort((a, b) => a[0].localeCompare(b[0]))) {
    const names = [...group.names];
    const shownInGroup = names.filter(visible).length;
    const head = el("div", { class: "account-group" });
    const cb = el("input", { type: "checkbox", "aria-label": `Show all states in ${shortAccount(group.scope)}` });
    cb.checked = shownInGroup === names.length;
    cb.indeterminate = shownInGroup > 0 && shownInGroup < names.length;
    cb.addEventListener("change", () => {
      for (const n of names) {
        if (cb.checked) state.hidden.delete(n); else state.hidden.add(n);
      }
      loadAll();
    });
    head.appendChild(cb);
    head.appendChild(accountBadge(group.scope));
    head.appendChild(el("span", { class: "meta" }, `${shownInGroup}/${names.length}`));
    box.appendChild(head);

    for (const name of names.sort()) {
      const s = state.states.find((x) => x.name === name);
      const row = el("div", { class: visible(name) ? "state-check" : "state-check is-hidden" });
      const scb = el("input", { type: "checkbox", "aria-label": `Show ${name}` });
      scb.checked = visible(name);
      scb.addEventListener("change", () => {
        if (scb.checked) state.hidden.delete(name); else state.hidden.add(name);
        loadAll();
      });
      row.appendChild(scb);
      row.appendChild(icon(s.provider));
      row.appendChild(el("span", { class: "name" }, name));
      const drill = el("button", {
        type: "button", class: "drill", title: `Inspect ${name}`, "aria-label": `Inspect ${name}`,
      });
      drill.appendChild(svgIcon("i-arrow-left"));
      drill.addEventListener("click", () => { state.drill = name; closeStateMenu(); render(); });
      row.appendChild(drill);
      box.appendChild(row);
    }
  }
}

function renderDonut(edges) {
  const svg = $("donut"), legend = $("donutLegend");
  svg.replaceChildren(); legend.replaceChildren();
  const counts = {};
  for (const e of edges) counts[e.kind] = (counts[e.kind] || 0) + 1;
  const kinds = Object.keys(counts).sort((a, b) => counts[b] - counts[a]);
  const total = edges.length;
  const r = 58, cx = 80, cy = 80, circ = 2 * Math.PI * r;
  svg.appendChild(svgEl("circle", { class: "donut-track", cx, cy, r, fill: "none", "stroke-width": 16 }));
  let offset = 0;
  for (const k of kinds) {
    const len = (counts[k] / total) * circ;
    svg.appendChild(svgEl("circle", {
      cx, cy, r, fill: "none", stroke: KIND_COLORS[k] || "#8b98a5",
      "stroke-width": 16, "stroke-dasharray": `${len} ${circ - len}`,
      "stroke-dashoffset": -offset, transform: "rotate(-90 80 80)",
    }));
    offset += len;
    const row = el("div");
    row.appendChild(el("i", { style: `background:${KIND_COLORS[k] || "#8b98a5"}` }));
    row.appendChild(el("span", {}, k));
    row.appendChild(el("b", {}, String(counts[k])));
    legend.appendChild(row);
  }
  if (!total) {
    legend.appendChild(el("div", {}, "No evidence at this threshold"));
  }
  const t = svgEl("text", { x: cx, y: cy + 7 }, String(total));
  svg.appendChild(t);
}

function svgEl(tag, attrs = {}, text) {
  const n = document.createElementNS("http://www.w3.org/2000/svg", tag);
  for (const [k, v] of Object.entries(attrs)) n.setAttribute(k, v);
  if (text !== undefined) n.textContent = text;
  return n;
}

function svgBase(svg) {
  svg.replaceChildren();
  const defs = svgEl("defs");
  const marker = svgEl("marker", { id: "arrow", viewBox: "0 0 10 10", refX: "9", refY: "5",
    markerWidth: "6", markerHeight: "6", orient: "auto-start-reverse" });
  marker.appendChild(svgEl("path", { class: "arrow-marker", d: "M 0 0 L 10 5 L 0 10 z" }));
  defs.appendChild(marker);
  svg.appendChild(defs);
}

function svgEmpty(svg, msg) {
  setBaseView(svg, 600, 220);
  svg.appendChild(svgEl("text", { class: "empty-label", x: "50%", y: "50%", "text-anchor": "middle" }, msg));
}

function drawEdges(svg, lines, pos, nw, nh) {
  const eg = svgEl("g");
  for (const e of lines) {
    const a = pos[e.from], b = pos[e.to];
    if (!a || !b) continue;
    eg.appendChild(svgEl("line", { class: "flow-line", "data-from": e.from, "data-to": e.to,
      "data-kind": e.kind || "", x1: a.x, y1: a.y + nh / 2, x2: b.x + nw,
      y2: b.y + nh / 2, "stroke-width": 1.5, "marker-end": "url(#arrow)" }));
  }
  svg.appendChild(eg);
}

function cloudOf(type) {
  if (type.startsWith("aws_")) return "aws";
  if (type.startsWith("google_")) return "gcp";
  if (type.startsWith("azurerm_")) return "azure";
  return "unknown";
}

// ---- free-flowing canvas: pan + zoom over the live SVG ----
function applyView() {
  const v = state.view;
  if (v) $("graph").setAttribute("viewBox", `${v.x} ${v.y} ${v.w} ${v.h}`);
}

function setBaseView(svg, w, h) {
  const vb = `0 0 ${w} ${h}`;
  svg.setAttribute("viewBox", vb);
  svg.dataset.baseView = vb;
  state.view = { x: 0, y: 0, w, h };
}

function resetView() {
  const base = $("graph").dataset.baseView?.split(" ").map(Number);
  if (!base) return;
  state.view = { x: 0, y: 0, w: base[2], h: base[3] };
  applyView();
}

function zoomAt(factor, px = 0.5, py = 0.5) {
  const svg = $("graph"), v = state.view;
  if (!v) return;
  const base = svg.dataset.baseView.split(" ").map(Number);
  const w = Math.max(base[2] * 0.05, Math.min(base[2] * 4, v.w * factor));
  const h = w * (base[3] / base[2]);
  const sx = v.x + px * v.w, sy = v.y + py * v.h;
  state.view = { x: sx - px * w, y: sy - py * h, w, h };
  applyView();
}

(function initCanvas() {
  const wrap = $("graphWrap");
  let drag = null;
  wrap.addEventListener("pointerdown", (e) => {
    if (e.button !== 0 || !state.view) return;
    state.dragged = false;
    // Capturing the pointer retargets the click away from nodes and controls.
    if (e.target.closest?.(".node, button")) return;
    drag = { x: e.clientX, y: e.clientY, view: { ...state.view } };
    wrap.setPointerCapture(e.pointerId);
    wrap.classList.add("dragging");
  });
  wrap.addEventListener("pointermove", (e) => {
    if (!drag) return;
    const rect = $("graph").getBoundingClientRect();
    const dx = e.clientX - drag.x, dy = e.clientY - drag.y;
    if (Math.abs(dx) + Math.abs(dy) > 4) state.dragged = true;
    state.view = {
      x: drag.view.x - dx * (drag.view.w / rect.width),
      y: drag.view.y - dy * (drag.view.h / rect.height),
      w: drag.view.w, h: drag.view.h,
    };
    applyView();
  });
  const endDrag = () => { drag = null; wrap.classList.remove("dragging"); };
  wrap.addEventListener("pointerup", endDrag);
  wrap.addEventListener("pointercancel", endDrag);
  wrap.addEventListener("wheel", (e) => {
    if (!state.view) return;
    e.preventDefault();
    const rect = $("graph").getBoundingClientRect();
    zoomAt(e.deltaY > 0 ? 1.12 : 1 / 1.12,
      (e.clientX - rect.left) / rect.width, (e.clientY - rect.top) / rect.height);
  }, { passive: false });
  $("zoomInBtn").addEventListener("click", () => zoomAt(1 / 1.25));
  $("zoomOutBtn").addEventListener("click", () => zoomAt(1.25));
  $("resetViewBtn").addEventListener("click", resetView);
})();

function renderGraph(g) {
  const svg = $("graph");
  svgBase(svg);
  if (state.drill) return renderResourceGraph(svg, state.drill);

  const nodes = g?.nodes || [];
  const edges = (g?.edges || []).filter((e) => nodes.some((n) => n.id === e.from) && nodes.some((n) => n.id === e.to));
  if (!nodes.length) return svgEmpty(svg, "No visible states.");

  const depth = Object.fromEntries(nodes.map((n) => [n.id, 0]));
  for (let pass = 0; pass < nodes.length; pass++) {
    let changed = false;
    for (const e of edges) if (depth[e.from] < depth[e.to] + 1) { depth[e.from] = depth[e.to] + 1; changed = true; }
    if (!changed) break;
  }
  const layers = {};
  for (const n of nodes) (layers[depth[n.id]] ||= []).push(n.id);

  const H = Math.max(260, 130 * Math.max(1, ...Object.values(layers).map((l) => l.length)));
  const W = Math.max(700, 300 * Object.keys(layers).length);
  setBaseView(svg, W, H);
  const nw = 190, nh = 74, gapX = 280;
  const pos = {};
  for (const [d, ids] of Object.entries(layers)) {
    ids.sort();
    ids.forEach((id, i) => {
      pos[id] = { x: 30 + Number(d) * gapX, y: (H - (ids.length * (nh + 20) - 20)) / 2 + i * (nh + 20) };
    });
  }
  drawEdges(svg, edges, pos, nw, nh);

  const ng = svgEl("g");
  for (const n of nodes) {
    const p = pos[n.id], provider = providerOf(n.id);
    const g2 = svgEl("g", { class: "node", "data-key": n.id, "data-provider": provider });
    g2.appendChild(svgEl("rect", { x: p.x, y: p.y, width: nw, height: nh, rx: 9,
      stroke: PROVIDER_COLORS[provider] || "#8b98a5" }));
    g2.appendChild(svgImage(`/icons/${provider}.svg`, { x: p.x + 11, y: p.y + 18,
      width: 24, height: 24 }));
    g2.appendChild(svgEl("text", { x: p.x + 43, y: p.y + 24 }, n.id));
    g2.appendChild(svgEl("text", { x: p.x + 43, y: p.y + 40, class: "small" }, `${n.resources} resources →`));
    g2.appendChild(svgEl("text", { x: p.x + 12, y: p.y + 60, class: "small" },
      accountsFor(n.id).map(shortAccount).join("  ·  ")));
    g2.setAttribute("tabindex", "0");
    g2.setAttribute("role", "button");
    g2.setAttribute("aria-label", `Inspect ${n.id}, ${n.resources} resources`);
    const drillIn = () => {
      if ($("traceDepth").value !== "0") { traceFrom(n.id); return; }
      state.drill = n.id;
      render();
    };
    g2.addEventListener("click", () => { if (!state.dragged) drillIn(); });
    g2.addEventListener("keydown", (e) => {
      if (e.key !== "Enter" && e.key !== " ") return;
      e.preventDefault();
      drillIn();
    });
    ng.appendChild(g2);
  }
  svg.appendChild(ng);
}

function categoryOf(type) {
  const t = type.toLowerCase();
  if (/sqs|sns|pubsub|queue|event|kinesis|stream/.test(t)) return "Messaging";
  if (/s3|storage|bucket|blob/.test(t)) return "Storage";
  if (/db|database|sql|dynamo|neptune|redis|elasticache|documentdb|athena/.test(t)) return "Data";
  if (/lambda|function|cloud_run|web_app|app_service|appsync|instance|compute/.test(t)) return "Compute";
  if (/lb|load_balancer|api_gateway|gateway|route53|dns|cloudfront|cdn|record/.test(t)) return "Networking";
  if (/iam|kms|vault|secret|config|key/.test(t)) return "Security";
  if (/ecs|fargate|container|eks|kubernetes/.test(t)) return "Containers";
  return "Services";
}

// Official AWS service icons (AWS Labs architecture icon set). PNGs are
// embedded in the binary, so the diagram works offline.
const AWS_ICONS = {
  aws_lambda_function: "lambda", aws_lambda_event_source_mapping: "lambda",
  aws_s3_bucket: "s3", aws_s3_bucket_object: "s3", aws_s3_object: "s3",
  aws_sqs_queue: "sqs", aws_sns_topic: "sns",
  aws_dynamodb_table: "dynamodb", aws_db_instance: "rds", aws_rds_cluster: "rds",
  aws_api_gateway_rest_api: "api-gateway", aws_apigatewayv2_api: "api-gateway",
  aws_lb: "alb", aws_elb: "alb", aws_alb: "alb",
  aws_route53_record: "route53", aws_route53_zone: "route53",
  aws_vpc: "vpc", aws_subnet: "vpc",
  aws_ecs_task_definition: "ecs", aws_ecs_service: "ecs", aws_ecs_cluster: "ecs",
  aws_eks_cluster: "ecs", aws_ecr_repository: "ecs",
  aws_athena_workgroup: "athena", aws_opensearch_domain: "opensearch",
  aws_iam_role: "iam", aws_iam_policy: "iam", aws_iam_user: "iam",
  aws_kms_key: "kms", aws_cloudwatch_log_group: "cloudwatch",
  aws_instance: "ec2",
};

function serviceIcon(type) {
  if (type.startsWith("aws_")) {
    const name = AWS_ICONS[type] || AWS_ICONS[Object.keys(AWS_ICONS).find((p) => type.startsWith(p))];
    return name ? `/icons/aws/${name}.png` : "/icons/aws.svg";
  }
  return `/icons/${cloudOf(type)}.svg`;
}

// Some SVG renderers only honour the legacy xlink:href attribute on <image>.
function svgImage(href, attrs) {
  const n = svgEl("image", { ...attrs, href });
  n.setAttributeNS("http://www.w3.org/1999/xlink", "xlink:href", href);
  return n;
}

function drawNumberedEdges(svg, lines, pos) {
  const eg = svgEl("g");
  for (const l of lines) {
    const a = pos[l.from], b = pos[l.to];
    if (!a || !b) continue;
    eg.appendChild(svgEl("line", { class: "flow-line", "data-from": l.from, "data-to": l.to,
      "data-kind": l.kind || "", x1: a.cx, y1: a.cy, x2: b.cx,
      y2: b.cy, "stroke-width": 1.5, "marker-end": "url(#arrow)" }));
  }
  svg.appendChild(eg);
}

// Drill-down: an AWS-style architecture diagram. Resources in the selected
// state are grouped into dashed component panels; services in other statefiles
// are dashed panels on the right, numbered arrows show the dependencies.
function renderResourceGraph(svg, owner) {
  const byKey = new Map();
  state.resources.forEach((r) => byKey.set(`${r.state}|${r.address}`, r));

  const nodes = new Map();
  const addNode = (ownerName, address, fallbackType) => {
    const key = `${ownerName}|${address}`;
    if (nodes.has(key)) return key;
    const r = byKey.get(key);
    nodes.set(key, {
      key, owner: ownerName, address,
      type: r?.type || fallbackType || "resource",
      name: r?.name || address.split(".").pop() || address,
      local: ownerName === owner,
    });
    return key;
  };

  const lines = [];
  for (const e of state.edges) {
    if (e.from !== owner && e.to !== owner) continue;
    const from = addNode(e.from, e.from_addr || "output");
    const to = addNode(e.to, e.to_addr || "remote state");
    lines.push({ from, to, confidence: e.confidence, matched: e.matched, kind: e.kind });
  }
  state.resources.filter((r) => r.state === owner)
    .forEach((r) => addNode(owner, r.address, r.type));
  if (!nodes.size) return svgEmpty(svg, "No resources in this statefile.");

  const locals = new Map();
  const externals = new Map();
  for (const n of nodes.values()) {
    if (n.local) {
      const c = categoryOf(n.type);
      if (!locals.has(c)) locals.set(c, []);
      locals.get(c).push(n);
    } else {
      if (!externals.has(n.owner)) externals.set(n.owner, []);
      externals.get(n.owner).push(n);
    }
  }

  const tileW = 104, tileH = 86, gap = 10, cols = 3;
  const panelW = cols * tileW + (cols + 1) * gap;
  const rowsFor = (list) => Math.ceil(list.length / cols);
  const panelH = (list) => 34 + rowsFor(list) * tileH + (rowsFor(list) + 1) * gap;

  const localLayout = [];
  let ly = 52, rowH = 0;
  [...locals.entries()].sort().forEach(([label, list], i) => {
    if (i > 0 && i % 2 === 0) { ly += rowH + 22; rowH = 0; }
    const h = panelH(list);
    localLayout.push({ label, list, x: 40 + (i % 2) * (panelW + 22), y: ly, w: panelW, h });
    rowH = Math.max(rowH, h);
  });
  const localEndY = ly + rowH;
  const extX = 40 + 2 * (panelW + 22) + 16;
  const extLayout = [];
  let ey = 52;
  [...externals.entries()].sort().forEach(([label, list]) => {
    const h = panelH(list);
    extLayout.push({ label, list, x: extX, y: ey, w: panelW, h, external: true });
    ey += h + 22;
  });

  const W = Math.max(1160, extX + panelW + 40);
  const H = Math.max(400, localEndY + 40, ey + 40);
  setBaseView(svg, W, H);
  svg.appendChild(svgEl("rect", { x: 12, y: 12, width: W - 24, height: H - 24, rx: 12, class: "outer-box" }));
  svg.appendChild(svgEl("text", { x: 28, y: 38, class: "outer-title" }, `${owner} — architecture`));

  const panels = [...localLayout, ...extLayout];
  for (const p of panels) {
    svg.appendChild(svgEl("rect", { x: p.x, y: p.y, width: p.w, height: p.h, rx: 8, class: "group-box" }));
    svg.appendChild(svgEl("text", { x: p.x + 12, y: p.y + 20, class: "group-title" },
      p.external ? `${p.label} (other statefile)` : p.label));
  }

  const pos = {};
  for (const p of panels) {
    p.list.forEach((n, i) => {
      const col = i % cols, row = Math.floor(i / cols);
      const x = p.x + gap + col * tileW, y = p.y + 28 + gap + row * tileH;
      pos[n.key] = { x, y, cx: x + (tileW - 8) / 2, cy: y + 26 };
    });
  }
  drawNumberedEdges(svg, lines, pos);
  for (const link of state.customLinks) {
    const a = pos[link.from], b = pos[link.to];
    if (!a || !b) continue;
    svg.appendChild(svgEl("line", { class: "flow-line", "data-from": link.from, "data-to": link.to,
      "data-kind": "custom", x1: a.cx, y1: a.cy, x2: b.cx, y2: b.cy, stroke: "var(--accent)",
      "stroke-width": 2, "stroke-dasharray": "4 3", "marker-end": "url(#arrow)" }));
  }

  for (const n of nodes.values()) {
    const p = pos[n.key];
    if (!p) continue;
    const color = PROVIDER_COLORS[cloudOf(n.type)] || "#8b98a5";
    const g2 = svgEl("g", { class: "node", "data-key": n.key, "data-provider": cloudOf(n.type) });
    if (state.hiddenResources.has(n.key)) g2.style.display = "none";
    g2.appendChild(svgEl("rect", { class: "tile-box", x: p.x, y: p.y, width: tileW - 8,
      height: tileH - 10, rx: 7, stroke: color }));
    g2.appendChild(svgImage(serviceIcon(n.type), {
      x: p.x + (tileW - 8) / 2 - 20, y: p.y + 2, width: 40, height: 40,
    }));
    const cx = p.x + (tileW - 8) / 2, label = n.name.length > 14 ? n.name.slice(0, 13) + "…" : n.name;
    g2.appendChild(svgEl("text", { x: cx, y: p.y + 58, class: "tile-label" }, label));
    const sub = n.type.replace(/^(aws|google|azurerm)_/, "").slice(0, 18);
    g2.appendChild(svgEl("text", { x: cx, y: p.y + 71, class: "tile-sub" }, n.local ? sub : n.owner));
    const src = byKey.get(n.key);
    if (src?.scope_kind) {
      g2.appendChild(svgEl("text", { class: "tile-scope", x: p.x + tileW - 14, y: p.y + 14,
        "text-anchor": "end" }, shortAccount({ kind: src.scope_kind, id: src.scope_id })));
    }
    g2.appendChild(svgEl("title", {}, `${n.owner} · ${n.address} · ${n.type}`));
    const related = lines.filter((l) => l.from === n.key || l.to === n.key);
    const showDetail = () => {
      const detail = related.length
        ? related.map((l) => `${l.from === n.key ? "→" : "←"} ${l.matched}`).join("  ·  ")
        : "no resolved cross-state connections";
      $("graphHint").textContent = `${n.owner} / ${n.address} — ${detail}`;
    };
    g2.setAttribute("tabindex", "0");
    g2.setAttribute("role", "button");
    g2.setAttribute("aria-label", `${n.owner}: ${n.type} ${n.name}. Activate for connections.`);
    g2.addEventListener("click", (e) => {
      if (state.dragged) return;
      if (e.shiftKey) {
        if (!state.linkStart) { state.linkStart = n.key; showDetail(); return; }
        if (state.linkStart !== n.key) {
          state.customLinks.push({ from: state.linkStart, to: n.key });
          state.linkStart = null;
          render();
        }
        return;
      }
      showDetail();
      if ($("traceDepth").value !== "0") traceFrom(n.key);
    });
    g2.addEventListener("dblclick", () => {
      if (state.hiddenResources.has(n.key)) state.hiddenResources.delete(n.key);
      else state.hiddenResources.add(n.key);
      render();
    });
    g2.addEventListener("keydown", (e) => {
      if (e.key !== "Enter" && e.key !== " ") return;
      e.preventDefault();
      showDetail();
    });
    svg.appendChild(g2);
  }
}

function renderServices() {
  const box = $("services");
  box.replaceChildren();
  const grouped = new Map(); // statefile -> (resource type -> count)
  for (const r of state.resources) {
    if (!grouped.has(r.state)) grouped.set(r.state, new Map());
    const types = grouped.get(r.state);
    types.set(r.type, (types.get(r.type) || 0) + 1);
  }
  let typeCount = 0;
  for (const types of grouped.values()) typeCount += types.size;
  $("serviceCount").textContent = typeCount;
  $("servicesHint").textContent = state.drill
    ? `Services in ${state.drill}.`
    : `${grouped.size} statefiles — grouped by statefile. Click a state to inspect it.`;
  if (!grouped.size) {
    box.appendChild(el("p", { class: "hint" }, "No services yet — add a statefile."));
    return;
  }
  for (const owner of [...grouped.keys()].sort()) {
    const types = grouped.get(owner);
    const total = [...types.values()].reduce((a, b) => a + b, 0);
    const group = el("div", { class: "service-group" });
    const head = el("button", { type: "button", class: "service-group-head" });
    head.appendChild(icon(providerOf(owner)));
    head.appendChild(el("span", { class: "owner" }, owner));
    for (const a of accountsFor(owner)) head.appendChild(accountBadge(a));
    head.appendChild(el("span", { class: "meta" }, `${total} resources · ${types.size} types`));
    head.addEventListener("click", () => { state.drill = owner; render(); });
    group.appendChild(head);
    const chips = el("div", { class: "service-chips" });
    for (const [type, n] of [...types.entries()].sort()) {
      const chip = el("div", { class: "service" });
      chip.appendChild(el("img", { src: serviceIcon(type), alt: type, width: "15", height: "15" }));
      chip.appendChild(el("code", {}, type));
      chip.appendChild(el("b", {}, `×${n}`));
      chips.appendChild(chip);
    }
    group.appendChild(chips);
    box.appendChild(group);
  }
}

function drillOut() { state.drill = null; closeStateMenu(); render(); }
$("canvasBackBtn").addEventListener("click", drillOut);
$("stateFilterBtn").addEventListener("click", () => {
  const menu = $("stateMenu");
  const open = menu.hidden;
  menu.hidden = !open;
  $("stateFilterBtn").setAttribute("aria-expanded", String(open));
  if (open) menu.querySelector("input")?.focus();
});
document.addEventListener("click", (e) => {
  if (!$("stateMenu").hidden && !e.target.closest(".state-filter")) closeStateMenu();
});

// ---- canvas tools shared with the resource-level explorer ----
function downloadFile(name, content, mime) {
  const url = URL.createObjectURL(new Blob([content], { type: mime }));
  const a = el("a", { href: url, download: name });
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

function drawMinimap() {
  const svg = $("minimapSvg");
  svg.replaceChildren();
  const base = $("graph").dataset.baseView?.split(" ").map(Number);
  if (!base) return;
  const w = 180, h = 120, sx = w / base[2], sy = h / base[3];
  svg.setAttribute("viewBox", `0 0 ${w} ${h}`);
  for (const l of $("graph").querySelectorAll("line")) {
    svg.appendChild(svgEl("line", {
      x1: Number(l.getAttribute("x1")) * sx, y1: Number(l.getAttribute("y1")) * sy,
      x2: Number(l.getAttribute("x2")) * sx, y2: Number(l.getAttribute("y2")) * sy,
      stroke: "var(--line-strong)", "stroke-width": 1, opacity: l.style.opacity || 1,
    }));
  }
  for (const g of $("graph").querySelectorAll("g.node")) {
    const r = g.querySelector("rect");
    if (!r) continue;
    svg.appendChild(svgEl("rect", {
      x: Number(r.getAttribute("x")) * sx, y: Number(r.getAttribute("y")) * sy,
      width: Number(r.getAttribute("width")) * sx, height: Number(r.getAttribute("height")) * sy,
      fill: g.classList.contains("dim") ? "var(--line)" : "var(--accent)",
    }));
  }
}

function applyCanvasFilters() {
  const q = $("graphSearch").value.trim().toLowerCase();
  const provider = $("graphProvider").value;
  const kind = $("graphKind").value;
  for (const n of $("graph").querySelectorAll("g.node")) {
    const text = (n.textContent + " " + (n.getAttribute("data-key") || "")).toLowerCase();
    const ok = (!q || text.includes(q)) && (!provider || n.dataset.provider === provider);
    n.classList.toggle("dim", !ok && state.traceKeys === null);
  }
  for (const l of $("graph").querySelectorAll("line[data-kind]")) {
    l.style.opacity = !kind || l.dataset.kind === kind ? "" : "0.08";
  }
  drawMinimap();
}

function traceFrom(key) {
  const depth = $("traceDepth").value;
  if (depth === "0" || !key) { state.traceKeys = null; applyCanvasFilters(); return; }
  const max = depth === "all" ? Infinity : Number(depth);
  const adj = new Map();
  for (const l of $("graph").querySelectorAll("line[data-from][data-to]")) {
    const a = l.dataset.from, b = l.dataset.to;
    if (!adj.has(a)) adj.set(a, new Set());
    if (!adj.has(b)) adj.set(b, new Set());
    adj.get(a).add(b);
    adj.get(b).add(a);
  }
  const seen = new Set([key]);
  let frontier = [key];
  for (let d = 0; d < max && frontier.length; d++) {
    const next = [];
    for (const k of frontier) for (const nb of adj.get(k) || []) {
      if (!seen.has(nb)) { seen.add(nb); next.push(nb); }
    }
    frontier = next;
  }
  state.traceKeys = seen;
  for (const n of $("graph").querySelectorAll("g.node")) {
    const hit = seen.has(n.dataset.key);
    n.classList.toggle("dim", !hit);
    n.classList.toggle("trace", hit);
  }
  drawMinimap();
}

function exportCanvas(format) {
  const svg = $("graph");
  const stamp = new Date().toISOString().slice(0, 10);
  if (format === "svg") {
    const clone = svg.cloneNode(true);
    clone.setAttribute("xmlns", "http://www.w3.org/2000/svg");
    downloadFile(`terraviz-${stamp}.svg`, new XMLSerializer().serializeToString(clone), "image/svg+xml");
  } else if (format === "png") {
    const clone = svg.cloneNode(true);
    clone.setAttribute("xmlns", "http://www.w3.org/2000/svg");
    const source = new XMLSerializer().serializeToString(clone);
    const img = new Image();
    const url = URL.createObjectURL(new Blob([source], { type: "image/svg+xml" }));
    img.onload = () => {
      const c = el("canvas");
      c.width = img.width || 1600; c.height = img.height || 900;
      c.getContext("2d").drawImage(img, 0, 0);
      c.toBlob((b) => {
        const u = URL.createObjectURL(b);
        const a = el("a", { href: u, download: `terraviz-${stamp}.png` });
        document.body.appendChild(a); a.click(); a.remove();
        URL.revokeObjectURL(u); URL.revokeObjectURL(url);
      });
    };
    img.src = url;
  } else if (format === "csv") {
    const rows = [["from", "to", "kind", "matched", "confidence"]].concat(
      state.edges.map((e) => [e.from, e.to, e.kind, e.matched, e.confidence]));
    downloadFile(`terraviz-${stamp}.csv`, rows.map((r) => r.map((x) => `"${String(x ?? "").replace(/"/g, '""')}"`).join(",")).join("\n"), "text/csv");
  } else if (format === "mermaid") {
    const lines = ["flowchart LR"];
    for (const e of state.graph?.edges || []) lines.push(`  ${e.from} --> ${e.to}`);
    downloadFile(`terraviz-${stamp}.mmd`, lines.join("\n") + "\n", "text/plain");
  } else if (format === "dot") {
    const lines = ["digraph terraquery {", "  rankdir=LR;"];
    for (const e of state.graph?.edges || []) lines.push(`  "${e.from}" -> "${e.to}";`);
    downloadFile(`terraviz-${stamp}.dot`, lines.join("\n") + "}\n", "text/plain");
  }
}

for (const [btn, menu] of [["viewMenuBtn", "viewMenu"], ["exportMenuBtn", "exportMenu"]]) {
  $(btn).addEventListener("click", () => {
    const m = $(menu);
    m.hidden = !m.hidden;
    $(btn).setAttribute("aria-expanded", String(!m.hidden));
  });
}
$("optGrid").addEventListener("change", (e) => $("graphWrap").classList.toggle("grid", e.target.checked));
$("optLabels").addEventListener("change", (e) => $("graph").classList.toggle("nolabels", !e.target.checked));
$("optFlow").addEventListener("change", (e) => $("graph").classList.toggle("flow", e.target.checked));
$("optMinimap").addEventListener("change", (e) => { $("minimap").hidden = !e.target.checked; drawMinimap(); });
$("optLegend").addEventListener("change", (e) => { $("canvasLegend").hidden = !e.target.checked; });
$("optSimple").addEventListener("change", (e) => $("graph").classList.toggle("simple", e.target.checked));
for (const input of ["graphSearch", "graphProvider", "graphKind"]) {
  $(input).addEventListener("input", () => { state.traceKeys = null; applyCanvasFilters(); });
}
$("traceDepth").addEventListener("change", () => {
  if ($("traceDepth").value === "0") { state.traceKeys = null; applyCanvasFilters(); }
});
for (const b of document.querySelectorAll("[data-export]")) {
  b.addEventListener("click", () => exportCanvas(b.dataset.export));
}
$("showAllNodes").addEventListener("click", () => { state.hiddenResources.clear(); render(); });
$("clearCustom").addEventListener("click", () => { state.customLinks = []; state.linkStart = null; render(); });
$("presentBtn").addEventListener("click", () => {
  document.body.classList.toggle("present");
  if (document.body.classList.contains("present")) document.documentElement.requestFullscreen?.().catch(() => {});
  else document.exitFullscreen?.().catch(() => {});
});

// ---- dialogs: focus management + trap ----
let focusBeforeDialog = null;
function trapFocus(e) {
  if (e.key !== "Tab") return;
  const items = [...e.currentTarget.querySelectorAll(
    'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
  )].filter((n) => n.offsetParent !== null);
  if (!items.length) return;
  const first = items[0], last = items[items.length - 1];
  if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
  else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
}
function restoreFocus() { focusBeforeDialog?.focus?.(); }

// ---- full screen: move the LIVE canvas so drill-down/out still work ----
let wrapHome = null;
function closeArch() {
  $("archFull").hidden = true;
  if (wrapHome) wrapHome.appendChild($("graphWrap"));
  restoreFocus();
}
function enlargeArch() {
  focusBeforeDialog = document.activeElement;
  const wrap = $("graphWrap");
  if (!wrapHome) wrapHome = wrap.parentNode;
  $("archFullBody").appendChild(wrap);
  $("archFull").hidden = false;
  $("archFullTitle").textContent = state.drill ? `${state.drill} — architecture` : "Dependency graph";
  $("archFullClose").focus();
}
$("enlargeBtn").addEventListener("click", enlargeArch);
$("archFullClose").addEventListener("click", closeArch);
$("archFull").addEventListener("keydown", trapFocus);

// ---- drawer ----
function openDrawer() {
  focusBeforeDialog = document.activeElement;
  $("overlay").hidden = false;
  drawerMsg("");
  $("closeBtn").focus();
}
function closeDrawer() {
  $("overlay").hidden = true;
  restoreFocus();
}
$("addBtn").addEventListener("click", openDrawer);
$("closeBtn").addEventListener("click", closeDrawer);
$("overlay").addEventListener("click", (e) => { if (e.target === $("overlay")) closeDrawer(); });
$("overlay").addEventListener("keydown", trapFocus);
document.addEventListener("keydown", (e) => {
  if (e.key !== "Escape") return;
  if (!$("stateMenu").hidden) closeStateMenu();
  else if (!$("archFull").hidden) closeArch();
  else if (!$("overlay").hidden) closeDrawer();
});

const tabs = [...document.querySelectorAll(".tab")];
function selectTab(tab, moveFocus = false) {
  for (const t of tabs) {
    const active = t === tab;
    t.classList.toggle("active", active);
    t.setAttribute("aria-selected", String(active));
    t.tabIndex = active ? 0 : -1;
  }
  for (const p of document.querySelectorAll(".tabpane")) {
    p.classList.toggle("active", p.id === `tab-${tab.dataset.tab}`);
  }
  if (moveFocus) tab.focus();
  drawerMsg("");
}
for (const [i, tab] of tabs.entries()) {
  tab.addEventListener("click", () => selectTab(tab));
  tab.addEventListener("keydown", (e) => {
    const step = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
    if (!step) return;
    e.preventDefault();
    selectTab(tabs[(i + step + tabs.length) % tabs.length], true);
  });
}

async function afterIngest(data) {
  drawerMsg(`Added ${data.ingested.join(", ")} — ${data.edges} edges.`);
  await loadAll();
}

// S3 browser
let s3Configured = false;
async function loadS3Status() {
  try {
    const s = await api("/api/s3/status");
    s3Configured = s.configured;
    const pill = $("s3Pill");
    $("s3PillText").textContent = s.configured ? `S3: ${s.region || "configured"}` : "S3: not configured";
    pill.className = `pill ${s.configured ? "ok" : "off"}`;
    $("s3Hint").textContent = s.configured
      ? `Region ${s.region || "default"}${s.endpoint ? ` · endpoint ${s.endpoint}` : ""}`
      : `Not configured: ${s.error || "set AWS_REGION and credentials"}. Use Bundled samples or Upload instead.`;
    if (!s.configured) return;
    const data = await api("/api/s3/buckets");
    const sel = $("bucket");
    sel.replaceChildren(el("option", { value: "" }, "Select a bucket…"));
    for (const b of data.buckets) sel.appendChild(el("option", { value: b.name }, b.name));
    if (data.buckets.length) { sel.value = data.buckets[0].name; state.s3.bucket = sel.value; await loadObjects(); }
  } catch (err) { $("s3Hint").textContent = err.message; }
}

async function loadObjects(next = false) {
  if (!state.s3.bucket) return;
  const params = new URLSearchParams({ bucket: state.s3.bucket, prefix: state.s3.prefix });
  if (next && state.s3.token) params.set("token", state.s3.token);
  try {
    const data = await api(`/api/s3/objects?${params}`);
    state.s3.objects = next ? [...state.s3.objects, ...data.objects] : data.objects;
    state.s3.token = data.next_token || "";
    $("s3More").hidden = !state.s3.token;
    renderS3List();
  } catch (err) { $("s3Hint").textContent = err.message; }
}

function renderS3List() {
  const box = $("s3List");
  box.replaceChildren();
  const base = state.s3.prefix;
  for (const o of state.s3.objects) {
    const short = o.key.startsWith(base) ? o.key.slice(base.length) : o.key;
    if (o.is_dir) {
      const row = el("div", { class: "file dir" });
      row.appendChild(el("span", {}, "▸"));
      row.appendChild(el("span", { class: "key" }, short));
      row.addEventListener("click", () => {
        state.s3.prefix = o.key; state.s3.token = ""; state.s3.selected.clear();
        $("prefix").value = o.key; loadObjects();
      });
      box.appendChild(row);
      continue;
    }
    const row = el("label", { class: "file" });
    const cb = el("input", { type: "checkbox" });
    cb.checked = state.s3.selected.has(o.key);
    cb.addEventListener("change", () => {
      if (cb.checked) state.s3.selected.add(o.key); else state.s3.selected.delete(o.key);
      $("s3Add").disabled = state.s3.selected.size === 0;
    });
    row.appendChild(cb);
    row.appendChild(el("span", { class: "key" }, short));
    row.appendChild(el("span", { class: "size" }, `${(o.size / 1024).toFixed(1)} KB`));
    box.appendChild(row);
  }
  if (!state.s3.objects.length) box.appendChild(el("div", { class: "empty" }, "No objects here."));
  $("s3Add").disabled = state.s3.selected.size === 0;
}

$("bucket").addEventListener("change", (e) => {
  state.s3.bucket = e.target.value; state.s3.prefix = ""; state.s3.token = "";
  state.s3.selected.clear(); $("prefix").value = ""; loadObjects();
});
$("prefix").addEventListener("change", (e) => {
  state.s3.prefix = e.target.value; state.s3.token = ""; state.s3.selected.clear(); loadObjects();
});
$("s3More").addEventListener("click", () => loadObjects(true));
$("s3Add").addEventListener("click", async () => {
  try {
    const data = await api("/api/s3/ingest", { method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ bucket: state.s3.bucket, keys: [...state.s3.selected] }) });
    await afterIngest(data);
  } catch (err) { drawerMsg(err.message, false); }
});

// Bundled samples
async function loadSamples() {
  const data = await api("/api/samples");
  state.samples = new Map(data.samples.map((s) => [s.name, s]));
  const box = $("sampleList");
  box.replaceChildren();
  for (const s of data.samples) {
    const row = el("label", { class: "file" });
    const cb = el("input", { type: "checkbox" });
    cb.checked = true;
    cb.addEventListener("change", () => {
      if (cb.checked) state.samples.set(s.name, s); else state.samples.delete(s.name);
      $("sampleAdd").disabled = state.samples.size === 0;
    });
    row.appendChild(cb);
    row.appendChild(icon(s.provider));
    row.appendChild(el("span", { class: "key" }, s.name));
    row.appendChild(el("span", { class: "size" }, `${(s.size / 1024).toFixed(1)} KB`));
    box.appendChild(row);
  }
  $("sampleAdd").disabled = state.samples.size === 0;
}
$("sampleAdd").addEventListener("click", async () => {
  try {
    const data = await api("/api/samples/ingest", { method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ samples: [...state.samples.keys()] }) });
    await afterIngest(data);
  } catch (err) { drawerMsg(err.message, false); }
});

// File upload
function setFiles(files) {
  state.picked = Array.from(files);
  $("fileNames").textContent = state.picked.length ? state.picked.map((f) => f.name).join(", ") : "No files selected.";
  $("uploadBtn").disabled = state.picked.length === 0;
}
$("drop").addEventListener("click", () => $("files").click());
$("drop").addEventListener("keydown", (e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); $("files").click(); } });
$("files").addEventListener("change", (e) => setFiles(e.target.files));
for (const ev of ["dragenter", "dragover"]) $("drop").addEventListener(ev, (e) => { e.preventDefault(); $("drop").classList.add("drag"); });
for (const ev of ["dragleave", "drop"]) $("drop").addEventListener(ev, (e) => { e.preventDefault(); $("drop").classList.remove("drag"); });
$("drop").addEventListener("drop", (e) => setFiles(e.dataTransfer.files));
$("uploadBtn").addEventListener("click", async () => {
  $("uploadBtn").disabled = true;
  drawerMsg("Uploading…", false);
  const form = new FormData();
  for (const f of state.picked) form.append("states", f, f.name);
  const names = $("names").value.trim();
  if (names) form.append("names", names);
  try { await afterIngest(await api("/api/states", { method: "POST", body: form })); }
  catch (err) { drawerMsg(err.message, false); }
  finally { $("uploadBtn").disabled = state.picked.length === 0; }
});

// SQL
for (const link of document.querySelectorAll(".nav-link")) {
  link.addEventListener("click", () => {
    for (const l of document.querySelectorAll(".nav-link")) {
      const active = l === link;
      l.classList.toggle("active", active);
      if (active) l.setAttribute("aria-current", "page"); else l.removeAttribute("aria-current");
    }
  });
}

$("errorRetry").addEventListener("click", loadAll);

$("sqlBtn").addEventListener("click", async () => {
  clearError();
  try {
    const data = await api("/api/query", { method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ sql: $("sql").value }) });
    table($("sqlTable"), data.columns, data.rows,
      Object.fromEntries(data.columns.map((_, i) => [i, (v) => (v === null ? "NULL" : String(v))])));
  } catch (err) { showError(err.message); }
});

const initialParams = new URLSearchParams(location.hash.slice(1));
if (initialParams.get("state")) state.drill = initialParams.get("state");
if (initialParams.get("view") === "diagram") setView("diagram");

for (const b of document.querySelectorAll("nav button[data-view]")) {
  b.addEventListener("click", () => setView(b.dataset.view));
}
for (const a of document.querySelectorAll(".section-link")) {
  a.addEventListener("click", (e) => {
    e.preventDefault();
    setView("dashboard");
    const target = document.querySelector(a.getAttribute("href"));
    if (target) setTimeout(() => target.scrollIntoView({ behavior: "smooth", block: "start" }), 0);
  });
}

loadS3Status();
loadSamples().catch((err) => drawerMsg(err.message, false));
loadAll();
