/**
 * Self-check for the relation-graph logic in web/static/js/diagram_enhanced.js.
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

const FILE = path.join(__dirname, "..", "web", "static", "js", "diagram_enhanced.js");
const source = fs.readFileSync(FILE, "utf8");

const sandbox = {
  console,
  setTimeout: () => 0,
  clearTimeout: () => {},
  requestAnimationFrame: () => 0,
  URL: { createObjectURL: () => "", revokeObjectURL: () => {} },
  Blob: function () {},
  XMLSerializer: function () {},
  document: {
    createElement: () => ({ style: {}, classList: { add() {}, remove() {} }, appendChild() {} }),
    createElementNS: () => ({ setAttribute() {}, appendChild() {}, style: {} }),
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
    source.includes('style: "fill: var(--sm-node-type);"'),
    "the node type label uses an inline style",
  );
}

// ── the sidebar can be collapsed and reopened ───────────────────────────────
{
  assert.strictEqual(typeof Viewer.prototype.applySidebarState, "function");
  assert.strictEqual(typeof Viewer.prototype.toggleSidebar, "function");
}

console.log("all diagram JS checks passed");
