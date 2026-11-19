/**
 * Self-check for the relation-graph logic in legacyweb/static/js/diagram_enhanced.js.
 *
 * The browser script is not a module, so it is evaluated in a vm sandbox with a
 * minimal DOM stub and the pure helpers are driven through the class prototype.
 *
 * Run: node scripts/check-diagram-js.js
 */
"use strict";

const assert = require("assert");
const fs = require("fs");
const path = require("path");
const vm = require("vm");

const FILE = path.join(__dirname, "..", "legacyweb", "static", "js", "diagram_enhanced.js");
const TEMPLATE = path.join(__dirname, "..", "legacyweb", "templates", "index_enhanced.html");
const source = fs.readFileSync(FILE, "utf8");
const template = fs.readFileSync(TEMPLATE, "utf8");

// The theme tokens the canvas resolves against.
const TOKENS = {
  "--sm-edge-networking": "#38bdf8",
  "--sm-edge-access": "#ef4444",
  "--sm-edge-data": "#4ade80",
  "--sm-edge-trigger": "#fb923c",
  "--sm-edge-dependency": "#c084fc",
  "--sm-edge-reference": "#888888",
  "--sm-edge-default": "#888888",
  "--sm-layer-ec2": "#ff9900",
  "--sm-layer-other": "#64748b",
  "--sm-node-fill": "#1e1e1e",
  "--sm-border": "rgba(255, 255, 255, 0.08)",
  "--sm-border-hover": "rgba(255, 255, 255, 0.14)",
  "--sm-container-vpc": "#248814",
  "--sm-subnet-public": "#4ade80",
  "--sm-subnet-private": "#60a5fa",
  "--sm-subnet-default": "#38bdf8",
  "--sm-text": "#e2f5f1",
  "--sm-text-muted": "rgba(226, 245, 241, 0.62)",
  "--sm-text-xs": "10px",
  "--sm-text-sm": "11px",
  "--sm-text-md": "13px",
};

/**
 * A minimal element that records what the renderer puts on it, so the
 * drawing code can be asserted without a browser.
 */
function makeElement(tag) {
  const classes = new Set();
  const attrs = {};
  const style = {
    props: {},
    setProperty(name, value) {
      this.props[name] = String(value);
    },
    getPropertyValue(name) {
      return this.props[name] || "";
    },
  };
  const element = {
    tag,
    attrs,
    style,
    children: [],
    textContent: "",
    classList: {
      add: (c) => classes.add(c),
      remove: (c) => classes.delete(c),
      contains: (c) => classes.has(c),
      toggle: (c, force) => {
        const has = classes.has(c);
        const want = force === undefined ? !has : force;
        if (want) classes.add(c);
        else classes.delete(c);
        return want;
      },
    },
    setAttribute(name, value) {
      attrs[name] = String(value);
    },
    getAttribute(name) {
      return name in attrs ? attrs[name] : null;
    },
    hasAttribute(name) {
      return name in attrs;
    },
    removeAttribute(name) {
      delete attrs[name];
    },
    setAttributeNS() {},
    appendChild(child) {
      element.children.push(child);
      return child;
    },
    removeChild(child) {
      element.children = element.children.filter((c) => c !== child);
      return child;
    },
    addEventListener() {},
    get firstChild() {
      return element.children[0] || null;
    },
  };

  // className and classList are two views on the same list.
  Object.defineProperty(element, "className", {
    get: () => Array.from(classes).join(" "),
    set: (value) => {
      classes.clear();
      String(value)
        .split(/\s+/)
        .filter(Boolean)
        .forEach((c) => classes.add(c));
    },
  });

  const matches = (el, selector) => {
    if (selector.startsWith(".")) return el.classList.contains(selector.slice(1));
    if (selector.startsWith("[")) {
      const attr = selector.match(/^\[([\w-]+)\]$/);
      return attr ? el.hasAttribute(attr[1]) : false;
    }
    return el.tag === selector;
  };

  element.querySelectorAll = (selector) => {
    const found = [];
    const walk = (el) => {
      el.children.forEach((child) => {
        if (matches(child, selector)) found.push(child);
        walk(child);
      });
    };
    walk(element);
    return found;
  };
  element.querySelector = (selector) => element.querySelectorAll(selector)[0] || null;

  return element;
}

const sandbox = {
  console,
  setTimeout: () => 0,
  clearTimeout: () => {},
  requestAnimationFrame: () => 0,
  URL: { createObjectURL: () => "", revokeObjectURL: () => {} },
  Blob: function () {},
  XMLSerializer: function () {},
  getComputedStyle: () => ({
    getPropertyValue: (name) => TOKENS[name] || "",
  }),
  document: {
    createElement: (tag) => makeElement(tag),
    createElementNS: (ns, tag) => makeElement(tag),
    createTextNode: () => ({}),
    getElementById: () => null,
    querySelector: () => null,
    querySelectorAll: () => [],
    addEventListener: () => {},
    body: { appendChild() {} },
  },
};
sandbox.globalThis = sandbox;
vm.createContext(sandbox);
vm.runInContext(source, sandbox, { filename: FILE });

// `class` bindings live in the context's lexical scope, not on the sandbox
// object, so they are pulled out with a second evaluation.
const Viewer = vm.runInContext("EnhancedDiagramViewer", sandbox);

/** Build a viewer-like object with the given resources and connections. */
function makeViewer(resources, connections) {
  const viewer = Object.create(Viewer.prototype);
  viewer.connections = connections;
  viewer._resourceMap = new Map(resources.map((r) => [r.id, r]));
  viewer.buildGraphIndex();
  viewer.traceDepth = 2;
  return viewer;
}

const res = (id) => ({ id, name: id, type: "aws:s3:bucket", properties: {} });
const conn = (source, target) => ({ id: `${source}->${target}`, source_id: source, target_id: target });

// ── linear chain a -> b -> c -> d ────────────────────────────────────────────
{
  const resources = ["a", "b", "c", "d"].map(res);
  const viewer = makeViewer(resources, [conn("a", "b"), conn("b", "c"), conn("c", "d")]);

  const ids = (set) => Array.from(set).sort();

  const trace1 = viewer.getTrace(res("b"), 1);
  assert.deepStrictEqual(ids(trace1.upstreamIds), ["a"]);
  assert.deepStrictEqual(ids(trace1.downstreamIds), ["c"]);

  const trace2 = viewer.getTrace(res("b"), 2);
  assert.deepStrictEqual(ids(trace2.upstreamIds), ["a"]);
  assert.deepStrictEqual(ids(trace2.downstreamIds), ["c", "d"]);

  const traceAll = viewer.getTrace(res("b"), 0);
  assert.deepStrictEqual(ids(traceAll.downstreamIds), ["c", "d"]);

  // The viewer default is a 2 hop trace.
  assert.strictEqual(viewer.getTrace(res("b")).downstreamIds.size, 2);
}

// ── cycles terminate and do not loop forever ─────────────────────────────────
{
  const resources = ["a", "b", "c"].map(res);
  const viewer = makeViewer(resources, [conn("a", "b"), conn("b", "c"), conn("c", "a")]);
  const trace = viewer.getTrace(res("a"), 0);
  assert.deepStrictEqual(Array.from(trace.upstreamIds).sort(), ["b", "c"]);
  assert.deepStrictEqual(Array.from(trace.downstreamIds).sort(), ["b", "c"]);
}

// ── the trace never returns the selected resource itself ─────────────────────
{
  const resources = ["a", "b"].map(res);
  const viewer = makeViewer(resources, [conn("a", "b"), conn("b", "a")]);
  const trace = viewer.getTrace(res("a"), 0);
  assert.ok(!trace.upstreamIds.has("a"));
  assert.ok(!trace.downstreamIds.has("a"));
}

// ── layout: containment nests the instance inside subnet inside vpc ──────────
{
  const resources = [
    { id: "vpc", name: "vpc", type: "aws:ec2:vpc", properties: { cidr_block: "10.0.0.0/16" } },
    { id: "sub", name: "sub", type: "aws:ec2:subnet", properties: { vpc_id: "vpc" } },
    { id: "i", name: "i", type: "aws:ec2:instance", properties: { subnet_id: "sub" } },
  ];
  const connections = [conn("sub", "vpc"), conn("i", "sub")];
  const layout = Viewer.computeDiagramLayout(resources, connections, {}, 1400);

  const vpc = layout.containerBoxes.get("vpc");
  const sub = layout.containerBoxes.get("sub");
  assert.ok(vpc && sub, "vpc and subnet boxes exist");

  // A container box stores its size, its centre lives in the positions map.
  const inside = (point, box, centre) =>
    point.x > centre.x - box.width / 2 &&
    point.x < centre.x + box.width / 2 &&
    point.y > centre.y - box.height / 2 &&
    point.y < centre.y + box.height / 2;

  const pos = layout.positions.get("i");
  const subCentre = layout.positions.get("sub");
  const vpcCentre = layout.positions.get("vpc");
  assert.ok(inside(pos, sub, subCentre), "instance is inside the subnet box");
  assert.ok(inside(pos, vpc, vpcCentre), "instance is inside the VPC box");
}

// ── the service sections fill the canvas width instead of stacking ─────────
// The sections are placed left to right and they wrap into rows, so a
// diagram with many services is wide instead of a tall narrow column.
{
  const types = [
    "aws:ecs:cluster",
    "aws:ecr:repository",
    "aws:lambda:function",
    "aws:apigateway:restapi",
    "aws:elb:listener",
    "aws:s3:bucket",
    "aws:rds:instance",
    "aws:dynamodb:table",
    "aws:sns:topic",
    "aws:sqs:queue",
    "aws:logs:loggroup",
    "aws:iam:role",
  ];
  const layerDefs = {};
  types.forEach((type) => {
    const key = type.split(":")[1];
    if (!layerDefs[key]) layerDefs[key] = { name: key, color: "#fff" };
  });

  const resources = types.map((type, i) => ({
    id: "r" + i,
    name: "r" + i,
    type,
    properties: {},
  }));
  // A chain: every service feeds the next one, so the flow order is known.
  const connections = resources.slice(1).map((r, i) => ({
    id: "c" + i,
    source_id: resources[i].id,
    target_id: r.id,
  }));

  const canvasWidth = 1600;
  const layout = Viewer.computeDiagramLayout(
    resources,
    connections,
    layerDefs,
    canvasWidth,
  );

  const sections = Object.entries(layout.layerLayouts)
    .filter(([, l]) => l.bounds)
    .map(([key, l]) => ({ key, ...l.bounds }));
  assert.strictEqual(sections.length, types.length, "every service has a section");

  // The sections wrap: they sit side by side in the rows, they do not
  // stack one per row.
  const rows = new Set(sections.map((s) => s.y));
  assert.ok(rows.size > 1, "the sections wrap into more than one row");
  assert.ok(
    rows.size < sections.length,
    "the sections share the rows instead of stacking one per row",
  );

  // Every section stays inside the content width of the canvas.
  const maxContentWidth = canvasWidth - 40 * 2 - 24 * 2;
  const contentRight = 40 + 24 + maxContentWidth + 24;
  sections.forEach((s) => {
    assert.ok(s.x >= 40, `section ${s.key} keeps the left margin`);
    assert.ok(
      s.x + s.width <= contentRight,
      `section ${s.key} stays inside the content width`,
    );
  });

  // No two sections overlap.
  for (let i = 0; i < sections.length; i++) {
    for (let j = i + 1; j < sections.length; j++) {
      const a = sections[i];
      const b = sections[j];
      const overlap =
        a.x < b.x + b.width &&
        b.x < a.x + a.width &&
        a.y < b.y + b.height &&
        b.y < a.y + a.height;
      assert.ok(!overlap, `sections ${a.key} and ${b.key} do not overlap`);
    }
  }

  // The reading order is the flow order: left to right, top to bottom.
  const readingOrder = sections
    .slice()
    .sort((a, b) => a.y - b.y || a.x - b.x)
    .map((s) => s.key);
  assert.deepStrictEqual(
    readingOrder,
    types.map((t) => t.split(":")[1]),
    "the sections read in the flow order",
  );

  // The diagram is wider than it is tall: the canvas width is used.
  const width = Math.max(...sections.map((s) => s.x + s.width)) - 40;
  const height = Math.max(...sections.map((s) => s.y + s.height)) - 80;
  assert.ok(width > height, "the diagram is wider than it is tall");
}

// ── unknown terraform types are grouped by their service segment ─────────────
{
  const layerDefs = { ec2: { name: "EC2", color: "#fff" }, dynamodb: { name: "DynamoDB", color: "#fff" } };
  const resource = {
    id: "t",
    name: "t",
    type: "unknown",
    properties: { terraform_type: "aws_dynamodb_table" },
  };
  assert.strictEqual(Viewer.classifyResource(resource, layerDefs).layer, "dynamodb");
}

// ── exports: mermaid flowchart ──────────────────────────────────────────────
{
  const resources = [
    { id: "aws_lb_listener.main", name: 'Listener "prod"', type: "aws:elb:listener" },
    { id: "aws_lb_target_group.app", name: "target group", type: "aws:elb:targetgroup" },
    { id: "aws_instance.app", name: "app {web}", type: "aws:ec2:instance" },
  ];
  const connections = [
    {
      id: "c1",
      source_id: "aws_lb_listener.main",
      target_id: "aws_lb_target_group.app",
      type: "data",
      description: "Listener forwards to target group",
    },
    {
      id: "c2",
      source_id: "aws_lb_target_group.app",
      target_id: "aws_instance.app",
      type: "access",
    },
    // Endpoint outside the visible set: the edge is dropped.
    { id: "c3", source_id: "aws_instance.app", target_id: "gone", type: "networking" },
  ];

  const mermaid = Viewer.buildMermaid(resources, connections);
  const lines = mermaid.split("\n").filter((l) => l.trim());

  assert.strictEqual(lines[0], "graph TD");
  // one node per resource
  assert.strictEqual(lines.filter((l) => /^\s+\w+\["/.test(l)).length, 3);
  // one edge per connection between two visible resources
  assert.strictEqual(lines.filter((l) => l.includes("-->")).length, 2);

  // ids are sanitised: no dots, no spaces
  assert.ok(!mermaid.includes("aws_lb_listener.main"), "raw id is not used as node id");
  assert.ok(mermaid.includes("n_aws_lb_listener_main"), "id is sanitised");
  assert.ok(/^ {2}n_aws_lb_listener_main\[/.test(mermaid.split("\n")[1]), "node line uses the sanitised id");

  // labels are escaped
  assert.ok(mermaid.includes("#quot;"), "double quotes are escaped");
  assert.ok(mermaid.includes("#123;") && mermaid.includes("#125;"), "braces are escaped");
  assert.ok(!mermaid.includes('"prod"'), "raw quotes do not survive");

  // the connection type is the link label
  assert.ok(mermaid.includes('-->|"data"|'), "edge label is the connection type");
  assert.ok(mermaid.includes('-->|"access"|'), "edge label is the connection type");
}

// ── simplified / executive view ─────────────────────────────────────────────
{
  // Three EC2 instances collapse into one group, two subnets into another, and
  // the vpc stays on its own. An external (referenced-but-missing) placeholder
  // stays individual.
  const resources = [
    { id: "i1", type: "aws:ec2:instance", name: "web-1", icon_url: "/icons/x.svg", in_state_file: true },
    { id: "i2", type: "aws:ec2:instance", name: "web-2", icon_url: "/icons/x.svg", in_state_file: true },
    { id: "i3", type: "aws:ec2:instance", name: "web-3", icon_url: "/icons/x.svg", in_state_file: true },
    { id: "s1", type: "aws:ec2:subnet", name: "subnet-a", in_state_file: true },
    { id: "s2", type: "aws:ec2:subnet", name: "subnet-b", in_state_file: true },
    { id: "v1", type: "aws:ec2:vpc", name: "vpc-main", in_state_file: true },
    { id: "ext1", type: "aws:acm:certificate", name: "missing-cert", in_state_file: false },
  ];
  const connections = [
    { id: "e1", source_id: "i1", target_id: "s1", type: "networking" },
    { id: "e2", source_id: "i2", target_id: "s1", type: "networking" }, // duplicate pair collapsed
    { id: "e3", source_id: "i3", target_id: "s2", type: "networking" },
    { id: "e4", source_id: "s1", target_id: "v1", type: "networking" },
    { id: "e5", source_id: "s2", target_id: "v1", type: "networking" },
    { id: "self", source_id: "i1", target_id: "i2", type: "dependency" }, // same group -> self-edge dropped
  ];
  const out = Viewer.buildSimplifiedGraph(resources, connections);

  // 1. one group per (layer, type) PLUS the external placeholder
  assert.strictEqual(out.resources.length, 4, "3 groups + 1 external placeholder");
  const groups = out.resources.filter((r) => !r._simplified_members || r.in_state_file !== false);
  const instanceGroup = out.resources.find((r) => (r.name || "").startsWith("3 "));
  assert.ok(instanceGroup, "instance group exists");
  assert.strictEqual(instanceGroup._simplified_members.length, 3);
  assert.ok(instanceGroup.name.includes("instance"), "group name carries the type label");

  // external placeholder stays individual
  const ext = out.resources.find((r) => r.id === "ext1");
  assert.ok(ext, "external placeholder survives individual");
  assert.strictEqual(ext.in_state_file, false);

  // 2. self-edge dropped: no self-loop
  assert.ok(
    out.connections.every((c) => c.source_id !== c.target_id),
    "no self-edges survive",
  );

  // 3. the duplicate instance->subnet pair is collapsed to one edge
  const i2s = out.connections.filter(
    (c) => c.source_id.includes("instance") && c.target_id.includes("subnet") ||
           c.source_id.includes("ec2::aws:ec2:instance"),
  );
  assert.strictEqual(i2s.length, 1, "duplicate pairs collapsed");

  // 4. every endpoint points at a real resource in the simplified set
  const ids = new Set(out.resources.map((r) => r.id));
  for (const c of out.connections) {
    assert.ok(ids.has(c.source_id), "source resolves");
    assert.ok(ids.has(c.target_id), "target resolves");
  }

  // 5. the total visible resource count went down (it actually simplified)
  assert.ok(out.resources.length < resources.length, "graph got simpler");
}

// ── exports: graphviz dot ───────────────────────────────────────────────────
{
  const resources = [
    { id: "aws_lb_listener.main", name: 'Listener "prod"', type: "aws:elb:listener" },
    { id: "aws_instance.app", name: "app", type: "aws:ec2:instance" },
  ];
  const connections = [
    {
      id: "c1",
      source_id: "aws_lb_listener.main",
      target_id: "aws_instance.app",
      type: "networking",
    },
  ];

  const dot = Viewer.buildDot(resources, connections);
  const lines = dot.split("\n").filter((l) => l.trim());

  assert.strictEqual(lines[0], 'digraph "architecture" {');
  assert.strictEqual(lines[lines.length - 1], "}");
  assert.strictEqual(lines.filter((l) => l.includes("shape=box")).length, 2);
  assert.strictEqual(lines.filter((l) => l.includes("->")).length, 1);

  // label = name + type, quoted and escaped
  assert.ok(
    dot.includes('"aws_lb_listener.main" [label="Listener \\"prod\\"\\naws:elb:listener", shape=box];'),
    "node label is escaped: " + dot,
  );
  assert.ok(
    dot.includes('"aws_lb_listener.main" -> "aws_instance.app" [label="networking"];'),
    "edge carries the connection type",
  );
}

// ── edge tooltip rows ───────────────────────────────────────────────────────
{
  const rows = Viewer.buildEdgeTooltipRows({
    id: "c1",
    type: "data",
    description: "Listener forwards to target group",
    bidirectional: true,
    properties: {
      protocol: "HTTPS",
      port: 443,
      from_port: 443,
      to_port: 443,
      cidr_blocks: ["0.0.0.0/0"],
      reasons: ["first description", "second description"],
    },
  });
  const byLabel = new Map(rows.map((r) => [r.label, r.value]));

  assert.strictEqual(byLabel.get("Description"), "Listener forwards to target group");
  assert.strictEqual(byLabel.get("Protocol"), "HTTPS");
  assert.strictEqual(byLabel.get("Port"), "443");
  assert.strictEqual(byLabel.get("From Port"), "443");
  assert.strictEqual(byLabel.get("To Port"), "443");
  assert.strictEqual(byLabel.get("CIDR"), "0.0.0.0/0");
  assert.strictEqual(byLabel.get("Reason 1"), "first description");
  assert.strictEqual(byLabel.get("Reason 2"), "second description");
  assert.strictEqual(byLabel.get("Direction"), "bidirectional");
}

// ── var() must not be used in SVG presentation attributes ───────────────────
// Browsers do not substitute custom properties in presentation attributes, so
// such a fill silently falls back to black.
{
  assert.ok(
    !/fill: "var\(|stroke: "var\(/.test(source),
    "no var() in SVG presentation attributes",
  );
  assert.ok(
    source.includes("fill: this.theme.nodeType"),
    "the node type label uses the resolved theme colour",
  );
}

// ── the sidebar can be collapsed and reopened ───────────────────────────────
{
  assert.strictEqual(typeof Viewer.prototype.applySidebarState, "function");
  assert.strictEqual(typeof Viewer.prototype.toggleSidebar, "function");
}

// ── the canvas palette is themed, not hardcoded ────────────────────────────
{
  const edgeStyles = vm.runInContext("buildEdgeStyles()", sandbox);
  const layers = vm.runInContext("buildLayers()", sandbox);

  // Every edge and every layer points at a CSS custom property.
  Object.entries(edgeStyles).forEach(([type, style]) => {
    assert.ok(
      style.color.startsWith("--sm-edge-"),
      `edge ${type} uses a token, got ${style.color}`,
    );
    assert.ok(
      style.dotColor.startsWith("--sm-edge-"),
      `edge ${type} dot uses a token, got ${style.dotColor}`,
    );
  });
  Object.entries(layers).forEach(([key, layer]) => {
    assert.ok(
      layer.color.startsWith("--sm-layer-"),
      `layer ${key} uses a token, got ${layer.color}`,
    );
  });

  // The unreachable generic layers are gone: only "other" catches the rest.
  ["analytics", "app-integration", "database", "management-governance",
   "storage", "compute", "networking-content-delivery",
   "security-identity-compliance"].forEach((key) => {
    assert.ok(!layers[key], `the unreachable layer ${key} is removed`);
  });
  assert.ok(layers.other, "the fallback layer exists");

  // The palette resolves to concrete values once per render.
  const viewer = Object.create(Viewer.prototype);
  viewer.edgeStyles = edgeStyles;
  viewer.layers = layers;
  viewer.resolveTheme();

  assert.strictEqual(viewer.theme.edges.networking.color, "#38bdf8");
  assert.strictEqual(viewer.theme.edges.default.dotColor, "#888888");
  assert.strictEqual(viewer.theme.layers.ec2, "#ff9900");
  assert.strictEqual(viewer.theme.nodeFill, "#1e1e1e");
  assert.strictEqual(viewer.theme.containerVpc, "#248814");
  assert.strictEqual(viewer.theme.subnetPublic, "#4ade80");
  assert.strictEqual(viewer.theme.subnetPrivate, "#60a5fa");
  assert.strictEqual(viewer.theme.subnetDefault, "#38bdf8");

  // No hardcoded colour is left in the canvas code.
  assert.ok(
    !/(color|fill|stroke|dotColor)\s*:\s*"#[0-9a-fA-F]/.test(source),
    "no hardcoded hex colour in the JS",
  );
  assert.ok(
    !/#[0-9a-fA-F]{3,8}\b(?![;"])/.test(
      source.replace(/"#12[345];"/g, ""),
    ),
    "no hardcoded hex colour in the JS",
  );
}

// ── the toolbar exposes exactly the intended controls ───────────────────────
{
  const toolbar = template.slice(
    template.indexOf('<div class="floating-toolbar">'),
    template.indexOf("<!-- Main Canvas -->"),
  );

  const idsOf = (className) =>
    Array.from(toolbar.matchAll(/<button\b([^>]*)>/g))
      .map((m) => m[1])
      .filter((attrs) =>
        new RegExp(`class="[^"]*\\b${className}\\b[^"]*"`).test(attrs),
      )
      .map((attrs) => (attrs.match(/id="([^"]+)"/) || [])[1])
      .filter(Boolean);

  // The seven visible controls.
  assert.deepStrictEqual(idsOf("toolbar-btn").sort(), [
    "fit-view",
    "presentation-mode",
    "toggle-export-menu",
    "toggle-settings",
    "toggle-view-options",
    "zoom-in",
    "zoom-out",
  ]);

  // The toggles and the exports keep their ids, they just live in a popover.
  assert.deepStrictEqual(idsOf("toolbar-option").sort(), [
    "export-csv",
    "export-dot",
    "export-mermaid",
    "export-pdf",
    "export-png",
    "export-svg",
    "simplified-view",
    "toggle-flow",
    "toggle-grid",
    "toggle-labels",
    "toggle-minimap",
  ]);

  // The legend moved into the view popover, the diff button is gone.
  assert.ok(template.includes('id="view-popover"'), "the view popover exists");
  assert.ok(template.includes('id="export-popover"'), "the export popover exists");
  assert.ok(!template.includes("toggle-legend"), "the legend button is removed");
  assert.ok(!template.includes("legend-popover"), "the legend popover is removed");
  assert.ok(!template.includes("deployment-diff"), "the deployment diff button is removed");
  assert.strictEqual(
    (template.match(/class="legend-item"/g) || []).length,
    6,
    "the legend keeps its six entries",
  );
}

// ── the sidebar is flat: no collapsible sections ────────────────────────────
{
  assert.ok(!template.includes("toggleSidebarSection"), "no collapsible sidebar sections");
  assert.ok(!template.includes("sidebar-section-header"), "no sidebar section headers");
  assert.ok(!template.includes("sidebar-section-chevron"), "no sidebar chevrons");

  const captions = Array.from(template.matchAll(/class="sidebar-caption">([^<]+)</g))
    .map((m) => m[1].trim());
  assert.deepStrictEqual(captions, ["Search", "Files", "Filters", "Resources"]);
}

// ── the import entry point follows the diagram state ─────────────────────────
{
  assert.ok(
    template.includes(".filter-sidebar:not(.has-diagram) .sidebar-import-row"),
    "the sidebar import row is hidden without a diagram",
  );

  const makeEl = () => {
    const classes = new Set();
    return {
      classList: {
        add: (c) => classes.add(c),
        remove: (c) => classes.delete(c),
        contains: (c) => classes.has(c),
        toggle: (c, force) => {
          const has = classes.has(c);
          const want = force === undefined ? !has : force;
          if (want) classes.add(c);
          else classes.delete(c);
          return want;
        },
      },
    };
  };

  const sidebar = makeEl();
  const empty = makeEl();
  sandbox.document.getElementById = (id) => {
    if (id === "filter-sidebar") return sidebar;
    if (id === "empty-state") return empty;
    return null;
  };

  const viewer = Object.create(Viewer.prototype);
  viewer.resources = [];
  viewer.updateEmptyState();
  assert.ok(!sidebar.classList.contains("has-diagram"), "no import row without a diagram");
  assert.ok(!empty.classList.contains("hidden"), "the empty state is shown");

  viewer.resources = [{ id: "a" }];
  viewer.updateEmptyState();
  assert.ok(sidebar.classList.contains("has-diagram"), "the import row is shown");
  assert.ok(empty.classList.contains("hidden"), "the empty state is hidden");
}

// ── the header only shows "Visible" when a filter hides resources ───────────
{
  const makeEl = () => {
    const classes = new Set();
    return {
      textContent: "",
      classList: {
        add: (c) => classes.add(c),
        remove: (c) => classes.delete(c),
        contains: (c) => classes.has(c),
        toggle: (c, force) => {
          const has = classes.has(c);
          const want = force === undefined ? !has : force;
          if (want) classes.add(c);
          else classes.delete(c);
          return want;
        },
      },
    };
  };

  const counter = makeEl();
  const item = makeEl();
  sandbox.document.getElementById = (id) => {
    if (id === "visible-count") return counter;
    if (id === "visible-stat") return item;
    return null;
  };

  const viewer = Object.create(Viewer.prototype);
  viewer.resources = [{ id: "a" }, { id: "b" }];
  viewer.filteredResources = viewer.resources;
  viewer.updateStats();
  assert.ok(item.classList.contains("hidden"), "the visible counter is hidden");

  viewer.filteredResources = [viewer.resources[0]];
  viewer.updateStats();
  assert.ok(!item.classList.contains("hidden"), "the visible counter is shown");
  assert.strictEqual(counter.textContent, 1);
}

// ── the header is a tool readout, not a metric panel ────────────────────────
{
  assert.ok(template.includes('class="header-status"'), "the header has a status strip");
  assert.ok(!template.includes("header-stats"), "the hero metric block is gone");
  assert.ok(!template.includes("stat-value"), "the hero metric numbers are gone");
  assert.ok(!template.includes("stat-label"), "the hero metric labels are gone");
  assert.ok(
    template.includes('id="resource-count"') &&
      template.includes('id="connection-count"'),
    "the status strip still carries the counts",
  );
}

// ── the browser surfaces carry the design ───────────────────────────────────
{
  assert.ok(template.includes("::selection"), "the text selection is themed");
  assert.ok(template.includes("caret-color"), "the caret is themed");
  assert.ok(
    template.includes("font-variant-numeric: tabular-nums"),
    "the counts and the table use tabular numerals",
  );
  assert.ok(
    /::-webkit-scrollbar\s*\{[^}]*width:\s*10px/.test(template),
    "the scrollbar is themed and usable",
  );
  assert.ok(
    !/:focus-visible\s*\{[^}]*border-radius/.test(template),
    "the focus ring does not reshape the element",
  );
  assert.ok(
    !template.includes("border-left: 2px solid #"),
    "no coloured left border on the list items",
  );
}

// ── the canvas type follows the token scale ────────────────────────────────
{
  assert.ok(
    !/font-size:\s*"[0-9]/.test(source),
    "no literal font size in the canvas code",
  );
  assert.ok(
    source.includes("this.theme.fontType") && source.includes("this.theme.fontName"),
    "the canvas type sizes come from the tokens",
  );
  assert.ok(
    /\.resource-label\s*\{[^}]*font-size:\s*var\(--sm-text-md\)/.test(template),
    "the canvas name label uses the token scale",
  );
  assert.ok(
    template.includes("@keyframes tv-node-rise"),
    "the canvas has an authored entrance",
  );
  assert.ok(
    /@media \(prefers-reduced-motion: reduce\)[\s\S]*tv-node-rise|@media \(prefers-reduced-motion: reduce\)[\s\S]*animation: none/.test(
      template,
    ),
    "the entrance respects the reduced motion preference",
  );
}

// ── the node card carries its service colour ───────────────────────────────
{
  const viewer = Object.create(Viewer.prototype);
  viewer.theme = {
    nodeFill: "#1e1e1e",
    nodeBorder: "rgba(255, 255, 255, 0.14)",
    nodeLabel: "#f5f5f7",
    nodeType: "rgba(245, 245, 247, 0.6)",
    fontName: "13px",
    fontSmall: "11px",
    fontType: "10px",
    layers: { s3: "#3f891a" },
  };
  viewer.filteredResources = [
    { id: "b", name: "prod-assets-bucket", type: "aws:s3:bucket", layer: "s3" },
  ];
  viewer._containerBoxes = new Map();
  viewer.showLabels = true;

  const nodes = makeElement("g");
  viewer.renderNodes(nodes);

  const node = nodes.children[0];
  assert.ok(node, "the node is drawn");
  assert.strictEqual(node.attrs["data-id"], "b");

  // The service colour is structural: it is the band on top of the card.
  const tab = node.querySelector(".node-card-tab");
  assert.ok(tab, "the card has a service band");
  assert.strictEqual(tab.attrs.fill, "#3f891a");

  // The card body is a quiet surface, the border is not the service colour.
  const card = node.querySelector(".node-card-bg");
  assert.ok(card, "the card has a body");
  assert.strictEqual(card.attrs.stroke, "rgba(255, 255, 255, 0.14)");
  assert.strictEqual(card.attrs.fill, "#1e1e1e");

  // The decorative ring is gone: the card is drawn with a single rectangle.
  assert.strictEqual(
    node.querySelectorAll("rect").length,
    1,
    "no decorative ring around the card",
  );

  // The type label is the machine side of the card: mono, token sized.
  const typeLabel = node.querySelector(".node-type-label");
  assert.ok(typeLabel, "the card shows the resource type");
  assert.strictEqual(typeLabel.attrs["font-size"], "10px");
  assert.ok(
    typeLabel.attrs["font-family"].includes("JetBrains Mono"),
    "the type is set in the mono face",
  );

  // The name is the human side: it inherits the sans face from the stylesheet.
  assert.ok(node.querySelector(".resource-label"), "the card shows the name");

  // The entrance marks the group that moves, not the node itself.
  const inner = node.querySelector(".node-inner");
  assert.strictEqual(inner.attrs["data-entrance"], "node");
  assert.strictEqual(inner.style.getPropertyValue("--i"), "0");
  assert.strictEqual(
    node.style.getPropertyValue("--node-service"),
    "#3f891a",
    "the service colour is available to the hover state",
  );
}

// ── the exported diagram shows the settled graph ───────────────────────────
{
  const marked = makeElement("g");
  marked.setAttribute("data-entrance", "node");
  marked.setAttribute("opacity", "0.42");
  const root = makeElement("svg");
  root.appendChild(marked);

  Viewer.stripEntranceState(root);

  assert.strictEqual(marked.getAttribute("opacity"), null);
  assert.strictEqual(marked.getAttribute("data-entrance"), null);
}

// ── the service band is a rounded top edge, not a rectangle ────────────────
{
  const path = Viewer.serviceTabPath(60, 52, 8, 8);
  assert.ok(path.startsWith("M -52,-52"), "the band starts at the top left");
  assert.ok(path.includes("A 8,8 0 0 1 60,-44"), "the top corners are rounded");
  assert.ok(path.includes("H -60"), "the band has a flat bottom");
  assert.ok(path.endsWith("Z"), "the band is closed");
}

// ── the canvas labels stay inside their width budget ───────────────────────
{
  assert.strictEqual(Viewer.truncateLabel("short", 18), "short");
  assert.strictEqual(
    Viewer.truncateLabel("aws:ec2:securitygroup", 18).length,
    18,
    "a truncated label never exceeds the budget",
  );
  assert.strictEqual(
    Viewer.canvasTypeLabel("aws:ec2:instance"),
    "ec2:instance",
    "the provider prefix is dropped on the card",
  );
  assert.strictEqual(
    Viewer.canvasTypeLabel("unknown"),
    "unknown",
    "a type without a provider keeps its text",
  );
}


{
  [template, source].forEach((text) => {
    ["\u{1F3D7}\uFE0F", "\u25BC", "\u2630", "\u{1F4C1}", "\u{1F4C2}", "\u{1F4E6}", "\u00D7"]
      .forEach((glyph) => {
        assert.ok(!text.includes(glyph), `no ${glyph} in the UI`);
      });
  });
}

console.log("all diagram JS checks passed");
