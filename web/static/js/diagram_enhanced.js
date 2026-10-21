// Enhanced Cloud Architecture Visualizer - Modern StackMap-inspired UI
"use strict";

// ─── Constants ─────────────────────────────────────────────────────────────────

const ZOOM_MIN = 0.1;
const ZOOM_MAX = 5;
const ZOOM_FACTOR_IN = 1.1;
const ZOOM_FACTOR_OUT = 0.9;

const NODE_RADIUS = 40;
const GLOW_RING_RADIUS = 50;
const ICON_OFFSET = 24; // half of icon dimension (48/2)
const ICON_SIZE = 48;

// Rectangular card dimensions for AWS-style node rendering
const NODE_CARD_W = 110;
const NODE_CARD_H = 90;
const NODE_CARD_RX = 10;
const NODE_ICON_Y = -18; // icon center Y offset within card (upper portion)

const LABEL_Y_OFFSET = 22;
const TYPE_LABEL_Y_OFFSET = 36;
const LABEL_MAX_CHARS = 14;
const TYPE_LABEL_MAX_CHARS = 20;
const TYPE_LABEL_FONT_SIZE = "9";
const TYPE_LABEL_FONT = "'JetBrains Mono', monospace";

// Layer label badge dimensions
const LAYER_LABEL_H = 24;
const LAYER_LABEL_RX = 6;
const LAYER_LABEL_FONT_SIZE = 11;
const LAYER_LABEL_PAD_X = 10;
const LAYER_LABEL_PAD_Y = 8;

const LAYOUT_PADDING_X = 40;
const LAYOUT_GAP = 80;
const LAYER_CONTENT_PADDING_X = 24;
const LAYER_HEADER_HEIGHT = 44;
const LAYER_CONTENT_PADDING_BOTTOM = 24;
const SUBGROUP_PADDING_X = 24;
const SUBGROUP_HEADER_HEIGHT = 36;
const SUBGROUP_PADDING_BOTTOM = 24;
const SUBGROUP_GAP = 30;
const NODE_GAP_X = 40;
const NODE_GAP_Y = 40;
const NODES_PER_ROW = 4;
const LAYOUT_MIN_CANVAS_WIDTH = 1400;
const LAYOUT_START_Y = 80;

const GRID_SPACING = 180;
const GRID_MIN_COLS = 4;
const GRID_START_X = 100;
const GRID_START_Y = 150;
const GRID_ROW_HEIGHT = 160;

// Nested AWS-style boxes (VPC > subnet > cards)
const VPC_BOX_PAD_X = 22;
const VPC_BOX_PAD_TOP = 40;
const VPC_BOX_PAD_BOTTOM = 18;
const VPC_BOX_MIN_WIDTH = 260;
const SUBNET_BOX_PAD_X = 16;
const SUBNET_BOX_PAD_TOP = 34;
const SUBNET_BOX_PAD_BOTTOM = 14;
const SUBNET_BOX_MIN_WIDTH = 170;
const SUBNET_BOX_MIN_HEIGHT = 58;
const BOX_CARD_GAP_X = 24;
const BOX_CARD_GAP_Y = 18;
const BOX_GAP = 26;
const BOX_MEMBER_GAP = 18;

const MINIMAP_WIDTH = 200;
const MINIMAP_HEIGHT = 120;
const MINIMAP_PADDING = 50;
const MINIMAP_DOT_SIZE = 2;
const MINIMAP_DOT_HIDDEN_SIZE = 1;
const MINIMAP_SCALE_FACTOR = 0.8;

const CONN_LABEL_FONT_SIZE = "9";
const CONN_LABEL_MAX_CHARS = 24;
const CONN_LABEL_CHAR_WIDTH = 5.5;
const CONN_LABEL_PAD = 12;
const CONN_LABEL_HEIGHT = 16;

const FIT_VIEW_MARGIN = 100;
const FIT_VIEW_MAX_ZOOM = 1.5;

const FLOW_DOT_RADIUS = 3;
const FLOW_DOT2_RADIUS = 2.5;
const FLOW_DOT_SHORT_DUR = "1.5s";
const FLOW_DOT_LONG_DUR = "2.5s";
const FLOW_LONG_DIST = 300;

const CONN_CLOSE_DIST = 200;
const CONN_MED_DIST = 500;
const CONN_CURVE_FACTOR = 0.2;
const CONN_ELBOW_RADIUS = 12;

const EXPORT_SCALE = 2;
const RESIZE_DEBOUNCE_MS = 150;

const SEARCH_DEBOUNCE_MS = 300;

const FIT_VIEW_DELAY_MS = 200;
const RESET_FIT_DELAY_MS = 100;

// ─── Helpers ───────────────────────────────────────────────────────────────────

/** Escape HTML entities to prevent XSS when inserting user-provided strings. */
function escapeHTML(str) {
  const div = document.createElement("div");
  div.appendChild(document.createTextNode(str));
  return div.innerHTML;
}

/** Create an SVG element in the SVG namespace with optional attributes. */
function svgEl(tag, attrs) {
  const el = document.createElementNS("http://www.w3.org/2000/svg", tag);
  if (attrs) {
    for (const [k, v] of Object.entries(attrs)) {
      el.setAttribute(k, String(v));
    }
  }
  return el;
}

function truncateText(text, maxLength) {
  return text.length > maxLength ? text.substring(0, maxLength) + "..." : text;
}

/** Simple debounce helper. */
function debounce(fn, ms) {
  let timer;
  return function (...args) {
    clearTimeout(timer);
    timer = setTimeout(() => fn.apply(this, args), ms);
  };
}

/** Safe min/max for large arrays (avoids stack overflow from spread operator). */
function safeMin(arr) {
  return arr.reduce((a, b) => (a < b ? a : b), Infinity);
}
function safeMax(arr) {
  return arr.reduce((a, b) => (a > b ? a : b), -Infinity);
}

/** Trigger the download of a text file generated in the browser. */
function downloadTextFile(filename, text, mimeType) {
  const url = URL.createObjectURL(new Blob([text], { type: mimeType }));
  const link = document.createElement("a");
  link.download = filename;
  link.href = url;
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

// ─── Layout model ─────────────────────────────────────────────────────────────
// The functions below are pure: they take resources/connections and return the
// geometry of the diagram. Nothing here touches the DOM, so the layout maths
// can be exercised without a browser.

const TYPE_VPC = "aws:ec2:vpc";
const TYPE_SUBNET = "aws:ec2:subnet";
const TYPE_SECURITY_GROUP = "aws:ec2:securitygroup";

/**
 * Resource types that describe a network container. An edge pointing at one of
 * them is a containment edge: it describes nesting, not traffic flow.
 */
const CONTAINER_TYPES = new Set([TYPE_VPC, TYPE_SUBNET, TYPE_SECURITY_GROUP]);

const TERRAFORM_PROVIDER_PREFIXES = new Set([
  "aws",
  "google",
  "azurerm",
  "kubernetes",
  "helm",
  "null",
  "tls",
  "random",
  "time",
  "local",
  "vault",
  "datadog",
]);

// Service segment -> layer key, for segments that do not match a layer name.
const LAYER_ALIASES = {
  cloudfront: "ec2",
  kms: "iam",
  step: "sfn",
  steps: "sfn",
  stepfunctions: "sfn",
  apigw: "apigateway",
  lb: "elb",
  alb: "elb",
  nlb: "elb",
  cw: "cloudwatch",
  dynamo: "dynamodb",
};

/** True when the layer definition really owns that key (no prototype hits). */
function hasLayer(layerDefs, key) {
  return !!key && Object.prototype.hasOwnProperty.call(layerDefs, key);
}

/** Strip the provider prefix of a Terraform type ("aws_dynamodb_table"). */
function stripProviderPrefix(tfType) {
  const idx = tfType.indexOf("_");
  if (
    idx > 0 &&
    TERRAFORM_PROVIDER_PREFIXES.has(tfType.slice(0, idx).toLowerCase())
  ) {
    return tfType.slice(idx + 1);
  }
  return tfType;
}

/** Service segment of a Terraform type ("aws_dynamodb_table" -> "dynamodb"). */
function terraformServiceSegment(resource) {
  const tf = resource.properties && resource.properties.terraform_type;
  if (typeof tf !== "string" || !tf.trim()) return null;
  const segment = stripProviderPrefix(tf.trim()).split("_")[0].toLowerCase();
  return segment || null;
}

/** Resource kind of a Terraform type ("aws_dynamodb_table" -> "table"). */
function terraformResourceKind(resource) {
  const tf = resource.properties && resource.properties.terraform_type;
  if (typeof tf !== "string" || !tf.trim()) return null;
  const parts = stripProviderPrefix(tf.trim()).split("_").map((p) => p.toLowerCase());
  return parts.length > 1 ? parts[parts.length - 1] : parts[0];
}

/**
 * Map a resource onto its service layer and sub-group.
 *
 * Resources with an unknown type are grouped by the service segment of their
 * Terraform type ("aws_dynamodb_table" -> dynamodb) instead of falling back
 * into the EC2 bucket.
 */
function classifyResource(resource, layerDefs) {
  const type = resource.type || "";
  const parts = type.split(":");

  if (type === "unknown" || parts.length < 2) {
    const segment = terraformServiceSegment(resource);
    let layerKey = segment;
    if (layerKey && !hasLayer(layerDefs, layerKey)) {
      layerKey = LAYER_ALIASES[layerKey];
    }
    if (!hasLayer(layerDefs, layerKey)) layerKey = "other";
    return {
      layer: layerKey,
      subgroup: terraformResourceKind(resource) || "default",
    };
  }

  let layerKey = resource.group || parts[1];
  const subgroupKey = parts.length >= 3 ? parts[2] : "default";
  if (!hasLayer(layerDefs, layerKey)) {
    layerKey = LAYER_ALIASES[layerKey] || "ec2";
  }
  if (!hasLayer(layerDefs, layerKey)) layerKey = "other";
  return { layer: layerKey, subgroup: subgroupKey };
}

/**
 * Longest-path layering over the flow edges.
 *
 * The rank of a resource is the length of the longest flow path that ends on
 * it, so rank 0 is the most upstream resource (top of the canvas).
 *
 * The graph may contain cycles (IAM role <-> instance profile), so the
 * traversal is iterative and keeps an in-progress marker: an edge back to a
 * node that is still on the stack is ignored instead of looping forever.
 *
 * @param {Array} resources
 * @param {Map} flowIn predecessors of every resource (target -> sources)
 */
function computeFlowRanks(resources, flowIn) {
  const rank = new Map();
  const state = new Map(); // 0 = unvisited, 1 = in progress, 2 = done
  resources.forEach((r) => {
    rank.set(r.id, 0);
    state.set(r.id, 0);
  });

  for (const root of resources) {
    if (state.get(root.id) !== 0) continue;
    state.set(root.id, 1);
    const stack = [{ id: root.id, i: 0, pending: null }];

    while (stack.length) {
      const frame = stack[stack.length - 1];

      if (frame.pending !== null) {
        const child = frame.pending;
        frame.pending = null;
        rank.set(
          frame.id,
          Math.max(rank.get(frame.id) || 0, (rank.get(child) || 0) + 1),
        );
      }

      const outs = flowIn.get(frame.id) || [];
      if (frame.i < outs.length) {
        const next = outs[frame.i++];
        const st = state.get(next) || 0;
        if (st === 1) continue; // back edge: ignore it, the cycle is broken here
        if (st === 2) {
          rank.set(
            frame.id,
            Math.max(rank.get(frame.id) || 0, (rank.get(next) || 0) + 1),
          );
          continue;
        }
        state.set(next, 1);
        frame.pending = next;
        stack.push({ id: next, i: 0, pending: null });
        continue;
      }

      state.set(frame.id, 2);
      stack.pop();
    }
  }

  return rank;
}

/**
 * Order resources with the barycentre heuristic: resources are grouped by
 * their flow rank and, inside a rank, sorted by the average position of the
 * neighbours that were placed already. A few sweeps remove most crossings.
 */
function orderResourcesByBarycenter(resources, ranks, flowOut, flowIn) {
  const initial = new Map();
  resources.forEach((r, i) => initial.set(r.id, i));

  let order = resources
    .slice()
    .sort(
      (a, b) =>
        (ranks.get(a.id) || 0) - (ranks.get(b.id) || 0) ||
        initial.get(a.id) - initial.get(b.id),
    );

  for (let sweep = 0; sweep < 3; sweep++) {
    const pos = new Map();
    order.forEach((r, i) => pos.set(r.id, i));

    const groups = new Map();
    order.forEach((r) => {
      const rank = ranks.get(r.id) || 0;
      if (!groups.has(rank)) groups.set(rank, []);
      groups.get(rank).push(r);
    });

    const next = [];
    Array.from(groups.keys())
      .sort((a, b) => a - b)
      .forEach((rank) => {
        const scored = groups.get(rank).map((r) => {
          const nb = (flowOut.get(r.id) || [])
            .concat(flowIn.get(r.id) || [])
            .filter((id) => (ranks.get(id) || 0) < rank && pos.has(id));
          const bary = nb.length
            ? nb.reduce((acc, id) => acc + pos.get(id), 0) / nb.length
            : pos.get(r.id);
          return { r, bary, tie: pos.get(r.id) };
        });
        scored.sort((a, b) => a.bary - b.bary || a.tie - b.tie);
        scored.forEach((s) => next.push(s.r));
      });
    order = next;
  }

  return order;
}

/** Re-sort a list of resources by the barycentre of their neighbours. */
function refineWithBarycenter(list, posOf, neighboursOf, sweeps) {
  let current = list.slice();
  for (let sweep = 0; sweep < sweeps; sweep++) {
    const scored = current.map((r) => {
      const nb = neighboursOf(r)
        .map((id) => posOf.get(id))
        .filter((v) => v !== undefined);
      const bary = nb.length
        ? nb.reduce((a, b) => a + b, 0) / nb.length
        : posOf.get(r.id) || 0;
      return { r, bary, tie: posOf.get(r.id) || 0 };
    });
    scored.sort((a, b) => a.bary - b.bary || a.tie - b.tie);
    current = scored.map((s) => s.r);
  }
  return current;
}

/** Label shown on a VPC box. */
function vpcLabel(resource) {
  return resource.name || resource.id;
}

/** Secondary line of a VPC box (CIDR range). */
function vpcSubLabel(resource) {
  const props = resource.properties || {};
  const cidr = props.cidr_block || props.cidr;
  return cidr ? String(cidr) : "";
}

/** Label shown on a subnet box. */
function subnetLabel(resource) {
  return resource.name || resource.id;
}

/** "Public" / "Private" hint for a subnet, when it can be derived. */
function subnetTier(resource) {
  const props = resource.properties || {};
  if (props.tier) return String(props.tier);
  if (props.public === true || props.public === "true") return "Public";
  if (
    props.map_public_ip_on_launch === true ||
    props.map_public_ip_on_launch === "true"
  ) {
    return "Public";
  }
  const name = String(resource.name || "").toLowerCase();
  if (name.includes("public")) return "Public";
  if (name.includes("private")) return "Private";
  return "";
}

/** Colour of a subnet box, based on its tier when it can be derived. */
function subnetColor(resource) {
  const tier = subnetTier(resource);
  if (tier === "Public") return "#4ADE80";
  if (tier === "Private") return "#60A5FA";
  return "#38BDF8";
}

/** Secondary line of a subnet box (availability zone, tier, CIDR). */
function subnetSubLabel(resource) {
  const props = resource.properties || {};
  const bits = [];
  const az =
    props.availability_zone ||
    props.availability_zone_id ||
    props.availabilityZone;
  if (az) bits.push(String(az));
  const tier = subnetTier(resource);
  if (tier) bits.push(tier);
  if (props.cidr_block) bits.push(String(props.cidr_block));
  return bits.join(" · ");
}

/**
 * Compute the complete diagram layout.
 *
 * @param {Array} resources
 * @param {Array} connections
 * @param {Object} layerDefs layer key -> {name, color}
 * @param {number} canvasWidth
 * @returns {Object} positions, container boxes, layer layouts and the
 *                   containment hierarchy the renderer works with.
 */
function computeDiagramLayout(resources, connections, layerDefs, canvasWidth) {
  const resById = new Map();
  resources.forEach((r) => resById.set(r.id, r));

  const assignment = new Map();
  resources.forEach((r) =>
    assignment.set(r.id, classifyResource(r, layerDefs)),
  );

  // ── edges ────────────────────────────────────────────────────────────────
  const flowOut = new Map();
  const flowIn = new Map();
  const containEdges = [];
  const addTo = (map, key, value) => {
    let list = map.get(key);
    if (!list) {
      list = [];
      map.set(key, list);
    }
    list.push(value);
  };

  (connections || []).forEach((c) => {
    const source = c.source_id || c.source;
    const target = c.target_id || c.target;
    if (!source || !target) return;
    if (!resById.has(source) || !resById.has(target)) return;
    if (CONTAINER_TYPES.has(resById.get(target).type)) {
      // containment: nesting, not flow
      containEdges.push({ source, target });
      return;
    }
    addTo(flowOut, source, target);
    addTo(flowIn, target, source);
  });

  const ranks = computeFlowRanks(resources, flowIn);
  const order = orderResourcesByBarycenter(resources, ranks, flowOut, flowIn);
  const orderIndex = new Map();
  order.forEach((r, i) => orderIndex.set(r.id, i));
  const neighboursOf = (r) =>
    (flowOut.get(r.id) || []).concat(flowIn.get(r.id) || []);

  // ── containment hierarchy ────────────────────────────────────────────────
  const subnetCandidates = new Map();
  const vpcParent = new Map();
  containEdges.forEach((edge) => {
    const targetType = resById.get(edge.target).type;
    if (targetType === TYPE_SUBNET) addTo(subnetCandidates, edge.source, edge.target);
    else if (targetType === TYPE_VPC) vpcParent.set(edge.source, edge.target);
  });

  // placement: resource id -> {container, kind}
  const placement = new Map();
  resources.forEach((r) => {
    if (r.type === TYPE_VPC || r.type === TYPE_SUBNET) return;
    const candidates = subnetCandidates.get(r.id);
    if (candidates && candidates.length === 1) {
      const subnet = resById.get(candidates[0]);
      const declared = (r.properties || {}).subnet_id;
      const subnetAwsId = (subnet.properties || {}).id;
      if (!declared || declared === subnetAwsId || declared === subnet.id) {
        placement.set(r.id, { container: subnet.id, kind: "subnet" });
        return;
      }
    }
    const vpc = vpcParent.get(r.id);
    if (vpc) placement.set(r.id, { container: vpc, kind: "vpc" });
  });

  const subnetParent = new Map();
  resources.forEach((r) => {
    if (r.type !== TYPE_SUBNET) return;
    const vpc = vpcParent.get(r.id);
    if (vpc) subnetParent.set(r.id, vpc);
  });

  const subnetMembers = new Map();
  resources.forEach((r) => {
    const p = placement.get(r.id);
    if (!p || p.kind !== "subnet") return;
    if (!subnetMembers.has(p.container)) subnetMembers.set(p.container, []);
    subnetMembers.get(p.container).push(r);
  });
  subnetMembers.forEach((list) =>
    list.sort((a, b) => orderIndex.get(a.id) - orderIndex.get(b.id)),
  );

  // ── network hierarchy ───────────────────────────────────────────────────
  const networkIds = new Set();
  const containerChildren = new Map();
  const networkEntries = [];

  resources.forEach((vpc) => {
    if (vpc.type !== TYPE_VPC) return;
    networkEntries.push({
      kind: "vpc",
      resource: vpc,
      subnets: resources.filter(
        (s) => s.type === TYPE_SUBNET && subnetParent.get(s.id) === vpc.id,
      ),
      members: resources.filter((m) => {
        const p = placement.get(m.id);
        return (
          p && p.kind === "vpc" && p.container === vpc.id && m.type !== TYPE_SECURITY_GROUP
        );
      }),
      securityGroups: resources.filter((m) => {
        const p = placement.get(m.id);
        return p && p.kind === "vpc" && p.container === vpc.id && m.type === TYPE_SECURITY_GROUP;
      }),
    });
  });

  resources.forEach((subnet) => {
    if (subnet.type !== TYPE_SUBNET || subnetParent.has(subnet.id)) return;
    networkEntries.push({
      kind: "subnet",
      resource: subnet,
      members: (subnetMembers.get(subnet.id) || []).slice(),
    });
  });

  const minRank = (list) =>
    list.reduce((acc, r) => Math.min(acc, ranks.get(r.id) || 0), Infinity);

  networkEntries.forEach((entry) => {
    if (entry.kind !== "vpc") return;
    entry.subnets.sort((a, b) => {
      const ra = minRank(subnetMembers.get(a.id) || []);
      const rb = minRank(subnetMembers.get(b.id) || []);
      if (ra !== rb) {
        if (ra === Infinity) return 1;
        if (rb === Infinity) return -1;
        return ra - rb;
      }
      return String(a.name || "").localeCompare(String(b.name || ""));
    });
  });

  networkEntries.forEach((entry) => {
    networkIds.add(entry.resource.id);
    const children = [];
    if (entry.kind === "vpc") {
      entry.subnets.forEach((subnet) => {
        networkIds.add(subnet.id);
        children.push(subnet.id);
        const members = subnetMembers.get(subnet.id) || [];
        members.forEach((m) => {
          networkIds.add(m.id);
          children.push(m.id);
        });
        containerChildren.set(
          subnet.id,
          members.map((m) => m.id),
        );
      });
      entry.members.forEach((m) => {
        networkIds.add(m.id);
        children.push(m.id);
      });
      entry.securityGroups.forEach((m) => {
        networkIds.add(m.id);
        children.push(m.id);
      });
    } else {
      entry.members.forEach((m) => {
        networkIds.add(m.id);
        children.push(m.id);
      });
    }
    containerChildren.set(entry.resource.id, children);
  });

  // Cards that live inside a network box (used for the section ranking).
  const entryResources = (entry) => {
    if (entry.kind !== "vpc") return entry.members;
    const list = entry.members.concat(entry.securityGroups);
    entry.subnets.forEach((s) => {
      (subnetMembers.get(s.id) || []).forEach((m) => list.push(m));
    });
    return list;
  };

  const avgRank = (list) => {
    if (!list.length) return 0;
    return (
      list.reduce((acc, r) => acc + (ranks.get(r.id) || 0), 0) / list.length
    );
  };

  networkEntries.sort(
    (a, b) =>
      avgRank(entryResources(a)) - avgRank(entryResources(b)) ||
      String(a.resource.name || "").localeCompare(String(b.resource.name || "")),
  );

  // ── service layer sections ──────────────────────────────────────────────
  const layerSections = [];
  Object.keys(layerDefs).forEach((key) => {
    const members = resources.filter((r) => {
      const a = assignment.get(r.id);
      return (
        a.layer === key &&
        !networkIds.has(r.id) &&
        r.type !== TYPE_VPC &&
        r.type !== TYPE_SUBNET
      );
    });
    if (!members.length) return;

    const subgroups = new Map();
    members.forEach((r) => {
      const sg = assignment.get(r.id).subgroup;
      if (!subgroups.has(sg)) subgroups.set(sg, []);
      subgroups.get(sg).push(r);
    });

    const sgKeys = Array.from(subgroups.keys()).sort((a, b) => {
      const ra = minRank(subgroups.get(a));
      const rb = minRank(subgroups.get(b));
      if (ra !== rb) return ra === Infinity ? 1 : rb === Infinity ? -1 : ra - rb;
      return String(a).localeCompare(String(b));
    });

    sgKeys.forEach((sg) => {
      subgroups.set(
        sg,
        refineWithBarycenter(subgroups.get(sg), orderIndex, neighboursOf, 2),
      );
    });

    layerSections.push({
      kind: "layer",
      key,
      name: layerDefs[key].name,
      color: layerDefs[key].color,
      subgroups,
      sgKeys,
      resources: members,
    });
  });

  // ── sections, ordered by rank (most upstream first) ─────────────────────
  const sections = [];
  if (networkEntries.length) {
    const leaves = [];
    networkEntries.forEach((entry) => {
      entryResources(entry).forEach((r) => leaves.push(r));
    });
    sections.push({ kind: "network", entries: networkEntries, resources: leaves });
  }
  layerSections.forEach((s) => sections.push(s));
  sections.forEach((s, i) => {
    s.index = i;
    s.rank = avgRank(s.resources);
  });
  sections.sort((a, b) => a.rank - b.rank || a.index - b.index);

  // ── geometry ────────────────────────────────────────────────────────────
  const positions = new Map();
  const containerBoxes = new Map();
  const layerLayouts = {};
  const maxContentWidth =
    Math.max(canvasWidth, LAYOUT_MIN_CANVAS_WIDTH) -
    LAYOUT_PADDING_X * 2 -
    LAYER_CONTENT_PADDING_X * 2;

  const sizeSubnetBox = (members) => {
    const count = members.length;
    if (!count) {
      return {
        cols: 1,
        rows: 1,
        width: SUBNET_BOX_MIN_WIDTH,
        height: SUBNET_BOX_MIN_HEIGHT,
      };
    }
    const cols = Math.min(count, NODES_PER_ROW);
    const rows = Math.ceil(count / NODES_PER_ROW);
    const width =
      SUBNET_BOX_PAD_X * 2 +
      cols * NODE_CARD_W +
      Math.max(0, cols - 1) * BOX_CARD_GAP_X;
    const height =
      SUBNET_BOX_PAD_TOP +
      rows * NODE_CARD_H +
      Math.max(0, rows - 1) * BOX_CARD_GAP_Y +
      SUBNET_BOX_PAD_BOTTOM;
    return {
      cols,
      rows,
      width: Math.max(width, SUBNET_BOX_MIN_WIDTH),
      height: Math.max(height, SUBNET_BOX_MIN_HEIGHT),
    };
  };

  const sizeNetworkEntry = (entry) => {
    if (entry.kind === "subnet") return sizeSubnetBox(entry.members);

    const subnetSizes = entry.subnets.map((s) =>
      sizeSubnetBox(subnetMembers.get(s.id) || []),
    );
    let subnetRowWidth = 0;
    let subnetRowHeight = 0;
    subnetSizes.forEach((s, i) => {
      subnetRowWidth += s.width + (i ? BOX_GAP : 0);
      subnetRowHeight = Math.max(subnetRowHeight, s.height);
    });

    const direct = entry.members.concat(entry.securityGroups);
    const cols = Math.max(1, Math.min(direct.length, NODES_PER_ROW));
    const rows = Math.max(1, Math.ceil(direct.length / NODES_PER_ROW));
    const membersWidth =
      cols * NODE_CARD_W + Math.max(0, cols - 1) * BOX_CARD_GAP_X;
    const membersHeight =
      rows * NODE_CARD_H + Math.max(0, rows - 1) * BOX_CARD_GAP_Y;

    const width = Math.max(subnetRowWidth, membersWidth) + VPC_BOX_PAD_X * 2;
    const height =
      VPC_BOX_PAD_TOP +
      subnetRowHeight +
      (direct.length ? BOX_MEMBER_GAP + membersHeight : 0) +
      VPC_BOX_PAD_BOTTOM;

    return {
      width: Math.max(width, VPC_BOX_MIN_WIDTH),
      height,
      subnetSizes,
      cols,
      rows,
      subnetRowHeight,
    };
  };

  const placeNetworkEntry = (entry, size, originX, originY) => {
    if (entry.kind === "subnet") {
      containerBoxes.set(entry.resource.id, {
        kind: "subnet",
        width: size.width,
        height: size.height,
        label: subnetLabel(entry.resource),
        sublabel: subnetSubLabel(entry.resource),
      });
      positions.set(entry.resource.id, {
        x: originX + size.width / 2,
        y: originY + size.height / 2,
      });
      entry.members.forEach((r, idx) => {
        const col = idx % size.cols;
        const row = Math.floor(idx / size.cols);
        positions.set(r.id, {
          x:
            originX +
            SUBNET_BOX_PAD_X +
            NODE_CARD_W / 2 +
            col * (NODE_CARD_W + BOX_CARD_GAP_X),
          y:
            originY +
            SUBNET_BOX_PAD_TOP +
            NODE_CARD_H / 2 +
            row * (NODE_CARD_H + BOX_CARD_GAP_Y),
        });
      });
      return;
    }

    containerBoxes.set(entry.resource.id, {
      kind: "vpc",
      width: size.width,
      height: size.height,
      label: vpcLabel(entry.resource),
      sublabel: vpcSubLabel(entry.resource),
    });
    positions.set(entry.resource.id, {
      x: originX + size.width / 2,
      y: originY + size.height / 2,
    });

    const subnetsTop = originY + VPC_BOX_PAD_TOP;
    let x = originX + VPC_BOX_PAD_X;
    entry.subnets.forEach((subnet, i) => {
      const ssize = size.subnetSizes[i];
      containerBoxes.set(subnet.id, {
        kind: "subnet",
        width: ssize.width,
        height: ssize.height,
        label: subnetLabel(subnet),
        sublabel: subnetSubLabel(subnet),
      });
      positions.set(subnet.id, {
        x: x + ssize.width / 2,
        y: subnetsTop + ssize.height / 2,
      });
      const members = subnetMembers.get(subnet.id) || [];
      members.forEach((r, idx) => {
        const col = idx % ssize.cols;
        const row = Math.floor(idx / ssize.cols);
        positions.set(r.id, {
          x:
            x +
            SUBNET_BOX_PAD_X +
            NODE_CARD_W / 2 +
            col * (NODE_CARD_W + BOX_CARD_GAP_X),
          y:
            subnetsTop +
            SUBNET_BOX_PAD_TOP +
            NODE_CARD_H / 2 +
            row * (NODE_CARD_H + BOX_CARD_GAP_Y),
        });
      });
      x += ssize.width + BOX_GAP;
    });

    const direct = entry.members.concat(entry.securityGroups);
    if (direct.length) {
      const top = subnetsTop + size.subnetRowHeight + BOX_MEMBER_GAP;
      direct.forEach((r, idx) => {
        const col = idx % size.cols;
        const row = Math.floor(idx / size.cols);
        positions.set(r.id, {
          x:
            originX +
            VPC_BOX_PAD_X +
            NODE_CARD_W / 2 +
            col * (NODE_CARD_W + BOX_CARD_GAP_X),
          y:
            top +
            NODE_CARD_H / 2 +
            row * (NODE_CARD_H + BOX_CARD_GAP_Y),
        });
      });
    }
  };

  let currentY = LAYOUT_START_Y;
  let networkLayout = null;

  sections.forEach((section) => {
    if (section.kind === "network") {
      const sizes = section.entries.map(sizeNetworkEntry);
      let rowX = LAYOUT_PADDING_X;
      let rowY = currentY;
      let rowMaxH = 0;
      let maxRight = rowX;
      let maxBottom = rowY;

      sizes.forEach((size, i) => {
        if (
          rowX + size.width > LAYOUT_PADDING_X + maxContentWidth &&
          rowX > LAYOUT_PADDING_X
        ) {
          rowY += rowMaxH + BOX_GAP;
          rowX = LAYOUT_PADDING_X;
          rowMaxH = 0;
        }
        placeNetworkEntry(section.entries[i], size, rowX, rowY);
        maxRight = Math.max(maxRight, rowX + size.width);
        maxBottom = Math.max(maxBottom, rowY + size.height);
        rowX += size.width + BOX_GAP;
        rowMaxH = Math.max(rowMaxH, size.height);
      });

      section.bounds = {
        x: LAYOUT_PADDING_X,
        y: currentY,
        width: maxRight - LAYOUT_PADDING_X + LAYER_CONTENT_PADDING_X,
        height: maxBottom - currentY + LAYER_CONTENT_PADDING_BOTTOM,
      };
      currentY = section.bounds.y + section.bounds.height + LAYOUT_GAP;
      networkLayout = { bounds: section.bounds, entries: section.entries };
      return;
    }

    // service layer box
    const sgLayouts = {};
    section.sgKeys.forEach((sgKey) => {
      const list = section.subgroups.get(sgKey) || [];
      const cols = Math.max(1, Math.min(list.length, NODES_PER_ROW));
      const rows = Math.max(1, Math.ceil(list.length / NODES_PER_ROW));
      sgLayouts[sgKey] = {
        resources: list,
        cols,
        rows,
        width:
          SUBGROUP_PADDING_X * 2 +
          cols * NODE_CARD_W +
          Math.max(0, cols - 1) * NODE_GAP_X,
        height:
          SUBGROUP_HEADER_HEIGHT +
          rows * NODE_CARD_H +
          Math.max(0, rows - 1) * NODE_GAP_Y +
          SUBGROUP_PADDING_BOTTOM,
      };
    });

    const layerX = LAYOUT_PADDING_X;
    const layerY = currentY;
    const contentX = layerX + LAYER_CONTENT_PADDING_X;
    const contentY = layerY + LAYER_HEADER_HEIGHT;
    let rowX = contentX;
    let rowY = contentY;
    let rowMaxH = 0;
    let maxRight = contentX;
    let maxBottom = contentY;

    section.sgKeys.forEach((sgKey) => {
      const sg = sgLayouts[sgKey];

      if (
        rowX + sg.width > contentX + maxContentWidth &&
        rowX > contentX
      ) {
        rowY += rowMaxH + SUBGROUP_GAP;
        rowX = contentX;
        rowMaxH = 0;
      }

      sg.x = rowX;
      sg.y = rowY;

      sg.resources.forEach((resource, idx) => {
        const col = idx % NODES_PER_ROW;
        const row = Math.floor(idx / NODES_PER_ROW);
        positions.set(resource.id, {
          x:
            rowX +
            SUBGROUP_PADDING_X +
            NODE_CARD_W / 2 +
            col * (NODE_CARD_W + NODE_GAP_X),
          y:
            rowY +
            SUBGROUP_HEADER_HEIGHT +
            NODE_CARD_H / 2 +
            row * (NODE_CARD_H + NODE_GAP_Y),
        });
      });

      maxRight = Math.max(maxRight, rowX + sg.width);
      maxBottom = Math.max(maxBottom, rowY + sg.height);
      rowX += sg.width + SUBGROUP_GAP;
      rowMaxH = Math.max(rowMaxH, sg.height);
    });

    const bounds = {
      x: layerX,
      y: layerY,
      width: maxRight - layerX + LAYER_CONTENT_PADDING_X,
      height: maxBottom - layerY + LAYER_CONTENT_PADDING_BOTTOM,
    };
    currentY = layerY + bounds.height + LAYOUT_GAP;
    layerLayouts[section.key] = {
      sgLayouts,
      subgroupKeys: section.sgKeys,
      bounds,
    };
  });

  // safety net: every resource needs a finite position
  let fallback = 0;
  resources.forEach((r) => {
    if (positions.has(r.id)) return;
    positions.set(r.id, {
      x: GRID_START_X + (fallback % GRID_MIN_COLS) * GRID_SPACING,
      y: currentY + Math.floor(fallback / GRID_MIN_COLS) * GRID_ROW_HEIGHT,
    });
    fallback++;
  });

  return {
    positions,
    containerBoxes,
    containerChildren,
    layerLayouts,
    network: networkLayout,
    networkIds,
    assignment,
    ranks,
    order,
  };
}

// ─── Export builders ──────────────────────────────────────────────────────────
// Pure functions: they take the visible resources plus the connections and
// return the text of an export file. Nothing here touches the DOM, so the
// generated files can be asserted without a browser.

// Characters that would break out of a Mermaid node or edge label.
const MERMAID_ESCAPES = {
  "&": "&amp;",
  '"': "#quot;",
  "#": "#35;",
  ";": "#59;",
  "{": "#123;",
  "}": "#125;",
  "[": "#91;",
  "]": "#93;",
  "(": "#40;",
  ")": "#41;",
  "|": "#124;",
};

/** Escape a label so quotes or braces cannot break the Mermaid file. */
function escapeMermaidLabel(text) {
  return String(text == null ? "" : text)
    .replace(/\s+/g, " ")
    .trim()
    .replace(/[&"#;{}()|[\]]/g, (ch) => MERMAID_ESCAPES[ch]);
}

/** Mermaid node ids allow only alphanumerics and underscores. */
function mermaidNodeId(resourceId, used) {
  const base = "n_" + String(resourceId).replace(/[^A-Za-z0-9_]/g, "_");
  let id = base;
  let suffix = 2;
  while (used.has(id)) id = `${base}_${suffix++}`;
  used.add(id);
  return id;
}

/**
 * Build a Mermaid flowchart: one node per resource, one edge per connection.
 * The link label is the connection type.
 */
function buildMermaid(resources, connections) {
  const used = new Set();
  const ids = new Map();
  const lines = ["graph TD"];

  resources.forEach((resource) => {
    const id = mermaidNodeId(resource.id, used);
    ids.set(resource.id, id);
    lines.push(`  ${id}["${escapeMermaidLabel(resource.name || resource.id)}"]`);
  });

  connections.forEach((connection) => {
    const sourceId = connection.source_id || connection.source;
    const targetId = connection.target_id || connection.target;
    const from = ids.get(sourceId);
    const to = ids.get(targetId);
    if (!from || !to) return;
    const label = escapeMermaidLabel(connection.type || "default");
    lines.push(`  ${from} -->|"${label}"| ${to}`);
  });

  return lines.join("\n") + "\n";
}

/** Escape a string so it can live inside a quoted Graphviz string. */
function escapeDotString(text) {
  return String(text == null ? "" : text)
    .replace(/\\/g, "\\\\")
    .replace(/"/g, '\\"')
    .replace(/\r?\n/g, "\\n");
}

/**
 * Build a Graphviz DOT digraph: one box per resource (label = name + type)
 * and one labelled edge per connection.
 */
function buildDot(resources, connections) {
  const ids = new Set(resources.map((r) => r.id));
  const lines = ['digraph "architecture" {'];

  resources.forEach((resource) => {
    const id = escapeDotString(resource.id);
    const label =
      escapeDotString(resource.name || resource.id) +
      "\\n" +
      escapeDotString(resource.type || "");
    lines.push(`  "${id}" [label="${label}", shape=box];`);
  });

  connections.forEach((connection) => {
    const sourceId = connection.source_id || connection.source;
    const targetId = connection.target_id || connection.target;
    if (!ids.has(sourceId) || !ids.has(targetId)) return;
    const label = escapeDotString(connection.type || "default");
    lines.push(
      `  "${escapeDotString(sourceId)}" -> "${escapeDotString(targetId)}" [label="${label}"];`,
    );
  });

  lines.push("}");
  return lines.join("\n") + "\n";
}

/** Rows of the edge tooltip: the metadata of a connection. */
function buildEdgeTooltipRows(connection) {
  const props = connection.properties || {};
  const rows = [];

  if (connection.description) {
    rows.push({ label: "Description", value: connection.description });
  }
  if (props.protocol) {
    rows.push({ label: "Protocol", value: String(props.protocol) });
  }
  if (props.port !== undefined && props.port !== null && props.port !== "") {
    rows.push({ label: "Port", value: String(props.port) });
  }
  if (props.from_port !== undefined && props.from_port !== null) {
    rows.push({ label: "From Port", value: String(props.from_port) });
  }
  if (props.to_port !== undefined && props.to_port !== null) {
    rows.push({ label: "To Port", value: String(props.to_port) });
  }
  if (Array.isArray(props.cidr_blocks) && props.cidr_blocks.length) {
    rows.push({ label: "CIDR", value: props.cidr_blocks.join(", ") });
  }

  const reasons = Array.isArray(props.reasons)
    ? props.reasons.filter((r) => typeof r === "string" && r.trim())
    : [];
  if (reasons.length > 1 || (reasons.length === 1 && !connection.description)) {
    reasons.forEach((reason, index) => {
      rows.push({
        label: reasons.length > 1 ? `Reason ${index + 1}` : "Reason",
        value: reason,
      });
    });
  }

  if (connection.bidirectional) {
    rows.push({ label: "Direction", value: "bidirectional" });
  }

  return rows;
}

// ─── Searchable Dropdown ───────────────────────────────────────────────────────

class SearchableDropdown {
  constructor(containerId, onChange) {
    this.container = document.getElementById(containerId);
    this.input = this.container.querySelector('.searchable-dropdown-input');
    this.list = this.container.querySelector('.searchable-dropdown-list');
    this.onChange = onChange;
    this.options = []; // [{value, label}]
    this.selectedValue = '';
    this.highlightIndex = -1;

    this.input.addEventListener('focus', () => this.open());
    this.input.addEventListener('input', () => this.filter());
    this.input.addEventListener('keydown', (e) => this.onKey(e));
    document.addEventListener('click', (e) => {
      if (!this.container.contains(e.target)) this.close();
    });
  }

  setOptions(options) {
    this.options = options; // [{value:'', label:'All Types'}, ...]
    this.selectedValue = '';
    this.input.value = '';
    this.render(this.options);
  }

  get value() {
    return this.selectedValue;
  }

  open() {
    this.container.classList.add('open');
    this.input.value = '';
    this.render(this.options);
  }

  close() {
    this.container.classList.remove('open');
    this.highlightIndex = -1;
    // Restore display text to selected label
    const sel = this.options.find(o => o.value === this.selectedValue);
    this.input.value = sel && sel.value ? sel.label : '';
  }

  filter() {
    const term = this.input.value.toLowerCase();
    const filtered = this.options.filter(o =>
      o.label.toLowerCase().includes(term)
    );
    this.highlightIndex = -1;
    this.render(filtered);
  }

  render(items) {
    while (this.list.firstChild) this.list.removeChild(this.list.firstChild);
    if (items.length === 0) {
      const li = document.createElement('li');
      li.className = 'searchable-dropdown-item no-results';
      li.textContent = 'No matches';
      this.list.appendChild(li);
      return;
    }
    items.forEach((opt, i) => {
      const li = document.createElement('li');
      li.className = 'searchable-dropdown-item';
      if (i === this.highlightIndex) li.classList.add('highlighted');
      li.textContent = opt.label;
      li.addEventListener('mousedown', (e) => {
        e.preventDefault(); // prevent blur before click registers
        this.select(opt);
      });
      this.list.appendChild(li);
    });
  }

  select(opt) {
    this.selectedValue = opt.value;
    this.input.value = opt.value ? opt.label : '';
    this.close();
    this.input.blur();
    this.onChange();
  }

  onKey(e) {
    const items = this.list.querySelectorAll('.searchable-dropdown-item:not(.no-results)');
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      this.highlightIndex = Math.min(this.highlightIndex + 1, items.length - 1);
      this.updateHighlight(items);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      this.highlightIndex = Math.max(this.highlightIndex - 1, 0);
      this.updateHighlight(items);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      if (this.highlightIndex >= 0 && items[this.highlightIndex]) {
        // Find the matching option from currently filtered list
        const term = this.input.value.toLowerCase();
        const filtered = this.options.filter(o => o.label.toLowerCase().includes(term));
        if (filtered[this.highlightIndex]) {
          this.select(filtered[this.highlightIndex]);
        }
      } else {
        // If typed text matches exactly one option, select it
        const term = this.input.value.toLowerCase();
        const filtered = this.options.filter(o => o.label.toLowerCase().includes(term));
        if (filtered.length === 1) {
          this.select(filtered[0]);
        } else {
          this.close();
        }
      }
    } else if (e.key === 'Escape') {
      this.close();
      this.input.blur();
    }
  }

  updateHighlight(items) {
    items.forEach((li, i) => {
      li.classList.toggle('highlighted', i === this.highlightIndex);
    });
    if (items[this.highlightIndex]) {
      items[this.highlightIndex].scrollIntoView({ block: 'nearest' });
    }
  }
}

// ─── Main Class ────────────────────────────────────────────────────────────────

class EnhancedDiagramViewer {
  constructor() {
    this.svg = null;
    this.minimapSvg = null;
    this.width = 0;
    this.height = 0;
    this.zoom = 1;
    this.panX = 0;
    this.panY = 0;
    this.isDragging = false;
    this.selectedResource = null;
    this._tooltipOwner = null;
    this.showLabels = true;
    this.showMinimap = true;
    this.showFlowAnimations = !window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    this.presentationMode = false;
    this.sidebarCollapsed = false;
    this.currentView = "diagram";

    this.resources = [];
    this.connections = [];
    this.filteredResources = [];

    // Fast lookup map: resource id -> resource object (built after data load)
    this._resourceMap = new Map();

    // Adjacency index of the relation graph: id -> [neighbour ids].
    // Rebuilt whenever the connections change.
    this._graphOut = new Map();
    this._graphIn = new Map();

    // How many hops the dependency trace follows (1 = direct neighbours).
    this.traceDepth = 2;

    // Layout output: nested container boxes and their children
    this._containerBoxes = null;
    this._containerChildren = new Map();
    this._networkLayout = null;
    this._layoutResult = null;

    // StackMap-inspired edge style config per connection type
    this.edgeStyles = {
      networking: {
        color: "#38BDF8",
        width: 1.8,
        dash: "",
        opacity: 0.75,
        flowDot: true,
        dotColor: "#38BDF8",
      },
      access: {
        color: "#ef4444",
        width: 1.8,
        dash: "",
        opacity: 0.75,
        flowDot: true,
        dotColor: "#ef4444",
      },
      data: {
        color: "#4ADE80",
        width: 1.8,
        dash: "",
        opacity: 0.75,
        flowDot: true,
        dotColor: "#4ADE80",
      },
      trigger: {
        color: "#FB923C",
        width: 2.0,
        dash: "",
        opacity: 0.8,
        flowDot: true,
        dotColor: "#FB923C",
      },
      dependency: {
        color: "#C084FC",
        width: 1.4,
        dash: "6,3",
        opacity: 0.6,
        flowDot: false,
        dotColor: "#C084FC",
      },
      reference: {
        color: "#888888",
        width: 1.0,
        dash: "4,4",
        opacity: 0.35,
        flowDot: false,
        dotColor: "#888888",
      },
      default: {
        color: "#888888",
        width: 1.4,
        dash: "",
        opacity: 0.5,
        flowDot: false,
        dotColor: "#888888",
      },
    };

    // AWS Service Groups (organized by actual AWS services, distinct colors)
    this.layers = {
      ec2: { name: "EC2", color: "#FF9900", resources: [], subgroups: {} },
      ecs: { name: "ECS", color: "#ED7100", resources: [], subgroups: {} },
      ecr: { name: "ECR", color: "#E86100", resources: [], subgroups: {} },
      lambda: {
        name: "Lambda",
        color: "#F59E0B",
        resources: [],
        subgroups: {},
      },
      apigateway: {
        name: "API Gateway",
        color: "#A855F7",
        resources: [],
        subgroups: {},
      },
      elb: {
        name: "Load Balancer",
        color: "#8B5CF6",
        resources: [],
        subgroups: {},
      },
      s3: { name: "S3", color: "#3F891A", resources: [], subgroups: {} },
      rds: { name: "RDS", color: "#3F48CC", resources: [], subgroups: {} },
      dynamodb: {
        name: "DynamoDB",
        color: "#2563EB",
        resources: [],
        subgroups: {},
      },
      eks: { name: "EKS", color: "#F3A13B", resources: [], subgroups: {} },
      apigatewayv2: {
        name: "API Gateway v2",
        color: "#C084FC",
        resources: [],
        subgroups: {},
      },
      sfn: {
        name: "Step Functions",
        color: "#E7157B",
        resources: [],
        subgroups: {},
      },
      sns: { name: "SNS", color: "#B7CA9D", resources: [], subgroups: {} },
      sqs: { name: "SQS", color: "#D97706", resources: [], subgroups: {} },
      logs: {
        name: "CloudWatch Logs",
        color: "#759C3E",
        resources: [],
        subgroups: {},
      },
      cloudwatch: {
        name: "CloudWatch",
        color: "#5B9A42",
        resources: [],
        subgroups: {},
      },
      iam: { name: "IAM", color: "#DD344C", resources: [], subgroups: {} },
      vpc: { name: "VPC", color: "#248814", resources: [], subgroups: {} },
      route53: {
        name: "Route 53",
        color: "#F58536",
        resources: [],
        subgroups: {},
      },
      acm: {
        name: "Certificate Manager",
        color: "#0EA5E9",
        resources: [],
        subgroups: {},
      },
      waf: { name: "WAF", color: "#E11D48", resources: [], subgroups: {} },
      secretsmanager: {
        name: "Secrets Manager",
        color: "#DC2626",
        resources: [],
        subgroups: {},
      },
      servicediscovery: {
        name: "Service Discovery",
        color: "#E5729D",
        resources: [],
        subgroups: {},
      },
      analytics: {
        name: "Analytics",
        color: "#871EBE",
        resources: [],
        subgroups: {},
      },
      "app-integration": {
        name: "App Integration",
        color: "#E01515",
        resources: [],
        subgroups: {},
      },
      database: {
        name: "Database",
        color: "#4338CA",
        resources: [],
        subgroups: {},
      },
      "management-governance": {
        name: "Management & Governance",
        color: "#64748B",
        resources: [],
        subgroups: {},
      },
      storage: {
        name: "Storage",
        color: "#16A34A",
        resources: [],
        subgroups: {},
      },
      compute: {
        name: "Compute",
        color: "#EA580C",
        resources: [],
        subgroups: {},
      },
      "networking-content-delivery": {
        name: "Networking & Content Delivery",
        color: "#059669",
        resources: [],
        subgroups: {},
      },
      "security-identity-compliance": {
        name: "Security & Compliance",
        color: "#BE123C",
        resources: [],
        subgroups: {},
      },
      "not-in-state-file": {
        name: "Not In State File",
        color: "#FB923C",
        resources: [],
        subgroups: {},
      },
      other: { name: "Other", color: "#64748B", resources: [], subgroups: {} },
    };

    this.init();
  }

  async init() {
    try {
      console.time("Total Loading Time");
      this.updateLoadingText("Loading diagram data...");

      await this.loadDiagram();
      this.updateLoadingText("Setting up canvas...");

      this.setupCanvas();
      this.updateLoadingText("Initializing controls...");

      this.setupControls();
      this.setupFilters();
      this.setupSearch();
      this.setupKeyboardShortcuts();

      this.updateLoadingText("Categorizing resources...");
      this.categorizeResources();

      this.updateLoadingText("Calculating layout...");
      this.autoLayout();

      this.updateLoadingText("Rendering diagram...");
      this.renderDiagram();

      this.updateLoadingText("Finalizing...");
      this.updateMinimap();

      document.getElementById("loading-overlay").classList.add("hidden");
      console.timeEnd("Total Loading Time");

      // Auto-fit the view after everything is rendered
      setTimeout(() => this.fitToView(), FIT_VIEW_DELAY_MS);
    } catch (error) {
      console.error("Failed to initialize diagram:", error);
      this.showError("Failed to load diagram");
    }
  }

  updateLoadingText(message) {
    const loadingText = document.querySelector(".loading-text");
    if (loadingText) {
      loadingText.textContent = message;
    }
  }

  async loadDiagram() {
    const response = await fetch("/api/diagram");
    if (!response.ok) {
      if (response.status === 404) {
        // No diagram loaded yet (e.g. server started without a file).
        // Set empty state so the UI remains functional for S3 import.
        this.resources = [];
        this.connections = [];
        this.filteredResources = [];
        this._resourceMap.clear();
        this.buildGraphIndex();
        console.log("No diagram loaded yet");
        this.updateEmptyState();
        return;
      }
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }
    const data = await response.json();

    this.resources = data.diagram.resources || [];
    this.connections = data.diagram.connections || [];
    this.filteredResources = [...this.resources];

    // Build fast lookup map
    this._resourceMap.clear();
    this.resources.forEach((r) => this._resourceMap.set(r.id, r));

    this.buildGraphIndex();

    console.log(
      `Loaded ${this.resources.length} resources and ${this.connections.length} connections`,
    );

    this.updateFilters();
    this.updateResourceList();
    this.updateEmptyState();
  }

  // Toggle the empty-state overlay based on whether resources are loaded.
  updateEmptyState() {
    const el = document.getElementById("empty-state");
    if (!el) return;
    if (this.resources.length > 0) {
      el.classList.add("hidden");
    } else {
      el.classList.remove("hidden");
    }
  }

  // Reload diagram data from server and re-render in place (no page reload).
  async reloadDiagram() {
    await this.loadDiagram();
    this.categorizeResources();
    this.autoLayout();
    this.renderDiagram();
    this.updateMinimap();
    setTimeout(() => this.fitToView(), 100);

    // Update header stats
    const rc = document.getElementById("resource-count");
    const cc = document.getElementById("connection-count");
    const vc = document.getElementById("visible-count");
    if (rc) rc.textContent = this.resources.length;
    if (cc) cc.textContent = this.connections.length;
    if (vc) vc.textContent = this.resources.length;
    this.updateEmptyState();
  }

  setupCanvas() {
    const container = document.querySelector(".canvas-container");
    this.width = container.clientWidth;
    this.height = container.clientHeight;

    this.svg = document.getElementById("diagram-canvas");
    this.svg.setAttribute("width", this.width);
    this.svg.setAttribute("height", this.height);
    this.svg.setAttribute("viewBox", `0 0 ${this.width} ${this.height}`);

    this.minimapSvg = document.getElementById("minimap-canvas");

    // Interactive minimap: click to navigate
    this.minimapSvg.addEventListener("click", (e) => {
      if (!this._minimapTransform || this.resources.length === 0) return;
      const rect = this.minimapSvg.getBoundingClientRect();
      const clickX = e.clientX - rect.left;
      const clickY = e.clientY - rect.top;
      const { minX, minY, scale } = this._minimapTransform;
      // Convert minimap click to diagram coordinates
      const diagramX = (clickX - 10) / scale + minX;
      const diagramY = (clickY - 10) / scale + minY;
      // Center the view on the clicked point
      this.panX = this.width / 2 - diagramX * this.zoom;
      this.panY = this.height / 2 - diagramY * this.zoom;
      this.updateTransform();
    });

    this.svg.addEventListener("mousedown", this.onMouseDown.bind(this));
    this.svg.addEventListener("mousemove", this.onMouseMove.bind(this));
    this.svg.addEventListener("mouseup", this.onMouseUp.bind(this));
    this.svg.addEventListener("wheel", this.onWheel.bind(this), {
      passive: false,
    });

    window.addEventListener(
      "resize",
      debounce(() => this.onResize(), RESIZE_DEBOUNCE_MS),
    );
    this.svg.addEventListener("contextmenu", (e) => e.preventDefault());
  }

  setupControls() {
    document
      .getElementById("zoom-in")
      .addEventListener("click", () => this.zoomIn());
    document
      .getElementById("zoom-out")
      .addEventListener("click", () => this.zoomOut());
    document
      .getElementById("fit-view")
      .addEventListener("click", () => this.fitToView());
    const resetBtn = document.getElementById("reset-layout");
    if (resetBtn) {
      resetBtn.addEventListener("click", () => this.resetLayout());
    }

    document
      .getElementById("toggle-labels")
      .addEventListener("click", () => this.toggleLabels());
    document
      .getElementById("toggle-minimap")
      .addEventListener("click", () => this.toggleMinimap());
    document
      .getElementById("presentation-mode")
      .addEventListener("click", () => this.togglePresentationMode());
    document
      .getElementById("sidebar-toggle")
      .addEventListener("click", () => this.toggleSidebar());

    const flowBtn = document.getElementById("toggle-flow");
    if (flowBtn) {
      flowBtn.addEventListener("click", () => this.toggleFlowAnimations());
    }

    document
      .getElementById("export-png")
      .addEventListener("click", () => this.exportToPNG());
    document
      .getElementById("export-svg")
      .addEventListener("click", () => this.exportToSVG());
    document
      .getElementById("export-csv")
      .addEventListener("click", () => this.exportToCSV());
    document
      .getElementById("export-mermaid")
      .addEventListener("click", () => this.exportToMermaid());
    document
      .getElementById("export-dot")
      .addEventListener("click", () => this.exportToDOT());

    document
      .getElementById("diagram-view-btn")
      .addEventListener("click", () => this.setView("diagram"));
    document
      .getElementById("list-view-btn")
      .addEventListener("click", () => this.setView("list"));

    this.setupThemeSettings();

    // Toggle Grid
    const gridBtn = document.getElementById("toggle-grid");
    if (gridBtn) {
      gridBtn.addEventListener("click", () => {
        const root = document.documentElement;
        const current = getComputedStyle(root).getPropertyValue('--sm-grid-dot').trim();
        if (current === 'transparent') {
          root.style.removeProperty('--sm-grid-dot');
          gridBtn.classList.add('active');
        } else {
          root.style.setProperty('--sm-grid-dot', 'transparent');
          gridBtn.classList.remove('active');
        }
      });
    }

    // Legend popover
    const legendBtn = document.getElementById("toggle-legend");
    const legendPopover = document.getElementById("legend-popover");
    if (legendBtn && legendPopover) {
      legendBtn.addEventListener("click", () => {
        legendPopover.classList.toggle('visible');
        legendBtn.classList.toggle('active');
      });
      // Close on click outside
      document.addEventListener("click", (e) => {
        if (!legendBtn.contains(e.target) && !legendPopover.contains(e.target)) {
          legendPopover.classList.remove('visible');
          legendBtn.classList.remove('active');
        }
      });
    }

    // Deployment Diff toggle
    const diffBtn = document.getElementById("deployment-diff");
    if (diffBtn) {
      diffBtn.addEventListener("click", () => {
        diffBtn.classList.toggle('diff-active');
      });
    }
  }

  setupThemeSettings() {
    const settingsBtn = document.getElementById("toggle-settings");
    const popover = document.getElementById("settings-popover");
    const options = [...document.querySelectorAll("[data-theme-value]")];
    const validThemes = new Set(options.map((option) => option.dataset.themeValue));
    const currentTheme = validThemes.has(document.documentElement.dataset.theme)
      ? document.documentElement.dataset.theme
      : "dark";

    const applyTheme = (theme) => {
      document.documentElement.dataset.theme = theme;
      options.forEach((option) => {
        const selected = option.dataset.themeValue === theme;
        option.classList.toggle("active", selected);
        option.setAttribute("aria-checked", String(selected));
      });
      try {
        localStorage.setItem("terraviz-theme", theme);
      } catch (_) {
        // Theme still applies when browser storage is unavailable.
      }
    };

    applyTheme(currentTheme);
    settingsBtn.addEventListener("click", (event) => {
      event.stopPropagation();
      const visible = popover.classList.toggle("visible");
      settingsBtn.classList.toggle("active", visible);
      settingsBtn.setAttribute("aria-expanded", String(visible));
    });
    options.forEach((option) => {
      option.addEventListener("click", () => applyTheme(option.dataset.themeValue));
    });
    document.addEventListener("click", (event) => {
      if (!settingsBtn.contains(event.target) && !popover.contains(event.target)) {
        popover.classList.remove("visible");
        settingsBtn.classList.remove("active");
        settingsBtn.setAttribute("aria-expanded", "false");
      }
    });
  }

  setupFilters() {
    this.dropdowns = {
      type: new SearchableDropdown('type-filter-dropdown', () => this.applyFilters()),
      provider: new SearchableDropdown('provider-filter-dropdown', () => this.applyFilters()),
      region: new SearchableDropdown('region-filter-dropdown', () => this.applyFilters()),
      state: new SearchableDropdown('state-filter-dropdown', () => this.applyFilters()),
    };
    // State has fixed options
    this.dropdowns.state.setOptions([
      { value: '', label: 'All States' },
      { value: 'active', label: 'Active' },
      { value: 'inactive', label: 'Inactive' },
      { value: 'pending', label: 'Pending' },
      { value: 'failed', label: 'Failed' },
      { value: 'terminated', label: 'Terminated' },
      { value: 'unknown', label: 'Unknown' },
    ]);
    // Populate type/provider/region from already-loaded resources
    this.updateFilters();
  }

  setupSearch() {
    const searchInput = document.getElementById("search-input");
    let searchTimeout;

    searchInput.addEventListener("input", () => {
      clearTimeout(searchTimeout);
      searchTimeout = setTimeout(() => {
        this.applyFilters();
      }, SEARCH_DEBOUNCE_MS);
    });
  }

  setupKeyboardShortcuts() {
    document.addEventListener("keydown", (e) => {
      if (e.target.tagName === "INPUT" || e.target.tagName === "SELECT") return;

      switch (e.key) {
        case "f":
          e.preventDefault();
          this.fitToView();
          break;
        case "r":
          e.preventDefault();
          this.resetLayout();
          break;
        case "l":
          e.preventDefault();
          this.toggleLabels();
          break;
        case "m":
          e.preventDefault();
          this.toggleMinimap();
          break;
        case "p":
          e.preventDefault();
          this.togglePresentationMode();
          break;
        case "a":
          e.preventDefault();
          this.toggleFlowAnimations();
          break;
        case "t":
          e.preventDefault();
          this.toggleSidebar();
          break;
        case "Escape":
          if (this.presentationMode) this.togglePresentationMode();
          break;
        case "/":
          e.preventDefault();
          document.getElementById("search-input").focus();
          break;
      }
    });
  }

  // ─── Resource Categorization ───────────────────────────────────────

  categorizeResources() {
    // Reset all layers
    Object.values(this.layers).forEach((layer) => {
      layer.resources = [];
      layer.subgroups = {};
    });

    this.resources.forEach((resource) => {
      // Resource type format: "aws:<service>:<subtype>". Resources with an
      // unknown type are grouped by the service segment of their Terraform
      // type (aws_dynamodb_table -> dynamodb).
      const { layer: layerKey, subgroup: subgroupKey } = classifyResource(
        resource,
        this.layers,
      );

      const layer = this.layers[layerKey];
      if (!layer) return;

      // Add to flat resources list
      layer.resources.push(resource);
      resource.layer = layerKey;
      resource.subgroup = subgroupKey;

      // Add to subgroup
      if (!layer.subgroups[subgroupKey]) {
        layer.subgroups[subgroupKey] = { resources: [] };
      }
      layer.subgroups[subgroupKey].resources.push(resource);
    });

    // Summary only
    const summary = Object.entries(this.layers)
      .filter(([, layer]) => layer.resources.length > 0)
      .map(([key, layer]) => `${key}(${layer.resources.length})`)
      .join(", ");
    console.log("Layer distribution:", summary);
  }

  // ─── Layout ────────────────────────────────────────────────────────

  /**
   * Graph-aware layout (AWS architecture diagram style).
   *
   * computeDiagramLayout() does the work:
   *  1. ranks the resources over the flow edges (containment edges excluded)
   *  2. derives the VPC / subnet nesting from the containment edges
   *  3. orders the service layers, their sub-groups and their cards by rank
   *     and by the barycentre of the already placed neighbours
   *  4. stacks the sections top to bottom, most upstream first
   */
  autoLayout() {
    const canvasWidth = Math.max(this.width, LAYOUT_MIN_CANVAS_WIDTH);

    if (this.resources.length === 0) {
      this.simpleGridLayout();
      return;
    }

    const layout = computeDiagramLayout(
      this.resources,
      this.connections,
      this.layers,
      canvasWidth,
    );

    this.resources.forEach((resource) => {
      const p = layout.positions.get(resource.id);
      if (!p) return;
      resource.x = p.x;
      resource.y = p.y;
    });

    this._containerBoxes = layout.containerBoxes;
    this._containerChildren = layout.containerChildren;
    this._networkLayout = layout.network;
    this._layoutResult = layout;

    this.applyLayerLayouts(layout);
  }

  /**
   * Rebuild the layer membership from the computed layout: resources drawn
   * inside a VPC / subnet box do not belong to a service layer box.
   */
  applyLayerLayouts(layout) {
    Object.values(this.layers).forEach((layer) => {
      layer.resources = [];
      layer.subgroups = {};
    });

    this.resources.forEach((resource) => {
      const assignment = layout.assignment.get(resource.id);
      if (!assignment) return;
      resource.layer = assignment.layer;
      resource.subgroup = assignment.subgroup;

      if (layout.networkIds.has(resource.id)) return;

      const layer = this.layers[assignment.layer];
      if (!layer) return;
      layer.resources.push(resource);
      if (!layer.subgroups[assignment.subgroup]) {
        layer.subgroups[assignment.subgroup] = { resources: [] };
      }
      layer.subgroups[assignment.subgroup].resources.push(resource);
    });

    Object.entries(this.layers).forEach(([key, layer]) => {
      layer._layout = layout.layerLayouts[key] || {
        sgLayouts: {},
        subgroupKeys: [],
        bounds: null,
      };
    });
  }

  simpleGridLayout() {
    const resourcesPerRow = Math.max(
      GRID_MIN_COLS,
      Math.floor(this.width / GRID_SPACING),
    );

    this.resources.forEach((resource, index) => {
      const row = Math.floor(index / resourcesPerRow);
      const col = index % resourcesPerRow;
      resource.x = GRID_START_X + col * GRID_SPACING;
      resource.y = GRID_START_Y + row * GRID_ROW_HEIGHT;
    });
  }

  // ─── Rendering ─────────────────────────────────────────────────────

  renderDiagram() {
    // Clear SVG using DOM API instead of innerHTML (proper SVG namespace handling)
    while (this.svg.firstChild) this.svg.removeChild(this.svg.firstChild);

    const defsGroup = svgEl("defs");
    const connectionsGroup = svgEl("g");
    const layersGroup = svgEl("g");
    const nodesGroup = svgEl("g");

    connectionsGroup.classList.add("connections");
    layersGroup.classList.add("layers");
    nodesGroup.classList.add("nodes");

    this.svg.appendChild(defsGroup);
    this.svg.appendChild(layersGroup);
    this.svg.appendChild(connectionsGroup);
    this.svg.appendChild(nodesGroup);

    this.createDefs(defsGroup);
    this.renderLayers(layersGroup);
    this.renderConnections(connectionsGroup);
    this.renderNodes(nodesGroup);
  }

  createDefs(defsGroup) {
    // Glow filter (static content, safe to build with DOM API)
    const glowFilter = svgEl("filter", { id: "glow" });
    const blur = svgEl("feGaussianBlur", {
      stdDeviation: "3",
      result: "coloredBlur",
    });
    glowFilter.appendChild(blur);
    const merge = svgEl("feMerge");
    merge.appendChild(svgEl("feMergeNode", { in: "coloredBlur" }));
    merge.appendChild(svgEl("feMergeNode", { in: "SourceGraphic" }));
    glowFilter.appendChild(merge);
    defsGroup.appendChild(glowFilter);

    // Layer gradients
    Object.entries(this.layers).forEach(([layerKey, layer]) => {
      const gradient = svgEl("linearGradient", {
        id: `gradient-${layerKey}`,
        x1: "0%",
        y1: "0%",
        x2: "100%",
        y2: "100%",
      });
      const stop1 = svgEl("stop", {
        offset: "0%",
        "stop-color": layer.color,
        "stop-opacity": "0.1",
      });
      const stop2 = svgEl("stop", {
        offset: "100%",
        "stop-color": layer.color,
        "stop-opacity": "0.05",
      });
      gradient.appendChild(stop1);
      gradient.appendChild(stop2);
      defsGroup.appendChild(gradient);
    });

    // Arrow markers for each edge type
    Object.entries(this.edgeStyles).forEach(([type, style]) => {
      const marker = svgEl("marker", {
        id: `arrow-${type}`,
        viewBox: "0 0 10 10",
        refX: "8",
        refY: "5",
        markerWidth: "6",
        markerHeight: "6",
        orient: "auto-start-reverse",
        markerUnits: "strokeWidth",
      });
      const arrowPath = svgEl("path", {
        d: "M 0 1 L 8 5 L 0 9 Z",
        fill: style.color,
        "fill-opacity": String(style.opacity),
      });
      marker.appendChild(arrowPath);
      defsGroup.appendChild(marker);
    });
  }

  renderLayers(layersGroup) {
    // Build a Set for fast filtered-resource membership checks
    const filteredSet = new Set(this.filteredResources);

    // Nested VPC / subnet boxes sit behind the service boxes and the nodes.
    this.renderContainerBoxes(layersGroup, filteredSet);

    Object.entries(this.layers).forEach(([layerKey, layer]) => {
      if (layer.resources.length === 0) return;

      const visible = layer.resources.filter((r) => filteredSet.has(r));
      if (visible.length === 0) return;

      const bounds = layer._layout && layer._layout.bounds;
      if (!bounds) return;

      // Outer service box — dashed border (AWS architecture style)
      const serviceBox = svgEl("rect", {
        x: bounds.x,
        y: bounds.y,
        width: bounds.width,
        height: bounds.height,
        fill: `url(#gradient-${layerKey})`,
        stroke: layer.color,
        "stroke-width": "1.5",
        "stroke-opacity": "0.5",
        "stroke-dasharray": "8,4",
        rx: "14",
      });
      layersGroup.appendChild(serviceBox);

      // Service label badge — colored pill at top-left
      const labelText = layer.name.toUpperCase();
      const labelWidth =
        labelText.length * 7.5 + LAYER_LABEL_PAD_X * 2;

      const labelBadge = svgEl("rect", {
        x: bounds.x + 8,
        y: bounds.y + 6,
        width: labelWidth,
        height: LAYER_LABEL_H,
        rx: LAYER_LABEL_RX,
        fill: layer.color,
        "fill-opacity": "0.15",
        stroke: layer.color,
        "stroke-width": "1",
        "stroke-opacity": "0.3",
      });
      layersGroup.appendChild(labelBadge);

      const serviceLabel = svgEl("text", {
        x: bounds.x + 8 + labelWidth / 2,
        y: bounds.y + 6 + LAYER_LABEL_H / 2 + 1,
        fill: layer.color,
        "font-size": String(LAYER_LABEL_FONT_SIZE),
        "font-weight": "700",
        "font-family": "'Inter', sans-serif",
        "text-anchor": "middle",
        "dominant-baseline": "central",
        opacity: "0.9",
      });
      serviceLabel.textContent = labelText;
      layersGroup.appendChild(serviceLabel);

      // Inner sub-group boxes — solid border (contrast with dashed outer)
      if (layer.subgroups) {
        Object.entries(layer.subgroups).forEach(([sgKey, sg]) => {
          const sgVisible = sg.resources.filter((r) => filteredSet.has(r));
          if (sgVisible.length === 0) return;

          const sgBounds = layer._layout.sgLayouts[sgKey];

          const sgBox = svgEl("rect", {
            x: sgBounds.x,
            y: sgBounds.y,
            width: sgBounds.width,
            height: sgBounds.height,
            fill: layer.color,
            "fill-opacity": "0.06",
            stroke: layer.color,
            "stroke-width": "1",
            "stroke-opacity": "0.3",
            rx: "8",
          });
          layersGroup.appendChild(sgBox);

          // Sub-group label
          const sgLabel = svgEl("text", {
            x: sgBounds.x + 10,
            y: sgBounds.y + 20,
            fill: layer.color,
            "font-size": "10",
            "font-weight": "600",
            "font-family": TYPE_LABEL_FONT,
            opacity: "0.7",
            "letter-spacing": "0.8",
          });
          sgLabel.textContent = sgKey.toUpperCase();
          layersGroup.appendChild(sgLabel);
        });
      }
    });
  }

  /**
   * Draw the nested AWS-style boxes (VPC > subnet > cards).
   *
   * A VPC or subnet is drawn as a box instead of a card, so the resource
   * itself stays selectable: the box carries the resource id.
   */
  renderContainerBoxes(layersGroup, filteredSet) {
    if (!this._containerBoxes) return;
    const vpcColor = (this.layers.vpc && this.layers.vpc.color) || "#248814";

    this._containerBoxes.forEach((box, id) => {
      const resource = this._resourceMap.get(id);
      if (!resource) return;
      if (
        !filteredSet.has(id) &&
        !this.hasVisibleDescendant(id, filteredSet)
      ) {
        return;
      }

      const isVpc = box.kind === "vpc";
      const color = isVpc ? vpcColor : subnetColor(resource);
      const strokeWidth = isVpc ? "1.5" : "1";
      const strokeOpacity = isVpc ? "0.5" : "0.35";
      const x = resource.x - box.width / 2;
      const y = resource.y - box.height / 2;

      const group = svgEl("g", { "data-id": id });
      group.classList.add("container-box");
      group.classList.add(isVpc ? "vpc-box" : "subnet-box");
      group.dataset.baseStrokeWidth = strokeWidth;
      group.dataset.baseStrokeOpacity = strokeOpacity;

      group.appendChild(
        svgEl("rect", {
          x: String(x),
          y: String(y),
          width: String(box.width),
          height: String(box.height),
          rx: String(isVpc ? 14 : 10),
          fill: color,
          "fill-opacity": isVpc ? "0.05" : "0.08",
          stroke: color,
          "stroke-width": strokeWidth,
          "stroke-opacity": strokeOpacity,
          "stroke-dasharray": isVpc ? "" : "4,3",
        }),
      );

      const label = svgEl("text", {
        x: String(x + 12),
        y: String(y + (isVpc ? 20 : 17)),
        fill: color,
        "font-size": isVpc ? "12" : "10",
        "font-weight": "700",
        "font-family": "'Inter', sans-serif",
        opacity: "0.9",
        "pointer-events": "none",
      });
      label.textContent = truncateText(box.label, 32);
      group.appendChild(label);

      if (box.sublabel) {
        const sub = svgEl("text", {
          x: String(x + 12),
          y: String(y + (isVpc ? 33 : 28)),
          fill: color,
          "font-size": "9",
          "font-family": TYPE_LABEL_FONT,
          opacity: "0.7",
          "pointer-events": "none",
        });
        sub.textContent = truncateText(box.sublabel, 40);
        group.appendChild(sub);
      }

      group.addEventListener("click", (e) => {
        e.stopPropagation();
        this.selectResource(resource);
      });
      group.addEventListener("mouseenter", (e) =>
        this.showTooltip(e, resource, group),
      );
      group.addEventListener("mouseleave", () => this.hideTooltip(group));

      layersGroup.appendChild(group);
    });
  }

  /** True when a nested resource of the container is currently visible. */
  hasVisibleDescendant(id, filteredSet) {
    const children = this._containerChildren && this._containerChildren.get(id);
    if (!children || !children.length) return false;
    return children.some((childId) => {
      if (filteredSet.has(childId)) return true;
      return this.hasVisibleDescendant(childId, filteredSet);
    });
  }

  renderConnections(connectionsGroup) {
    // Build a Set of filtered resource IDs for fast membership checks
    const filteredIds = new Set(this.filteredResources.map((r) => r.id));

    // Count edges between each ordered node pair for multi-edge offset
    const edgeCounts = new Map(); // "srcId->tgtId" -> count
    const edgeIndex = new Map(); // "srcId->tgtId" -> current index
    this.connections.forEach((connection) => {
      const sourceId = connection.source_id || connection.source;
      const targetId = connection.target_id || connection.target;
      // Use sorted key so both directions share the same offset group
      const pairKey =
        sourceId < targetId
          ? `${sourceId}->${targetId}`
          : `${targetId}->${sourceId}`;
      edgeCounts.set(pairKey, (edgeCounts.get(pairKey) || 0) + 1);
    });

    // Track placed label positions for collision avoidance
    const placedLabels = [];

    this.connections.forEach((connection) => {
      const sourceId = connection.source_id || connection.source;
      const targetId = connection.target_id || connection.target;

      // Use Map lookup instead of Array.find()
      const source = this._resourceMap.get(sourceId);
      const target = this._resourceMap.get(targetId);

      if (!source || !target) return;
      if (!filteredIds.has(sourceId) || !filteredIds.has(targetId)) return;
      if (source.x === target.x && source.y === target.y) return;

      const edgeType =
        connection.type && this.edgeStyles[connection.type]
          ? connection.type
          : "default";
      const style = this.edgeStyles[edgeType];

      const dx = target.x - source.x;
      const dy = target.y - source.y;
      const dist = Math.sqrt(dx * dx + dy * dy);
      const perpDx = -dy / dist;
      const perpDy = dx / dist;

      // Multi-edge offset: fan out parallel edges between the same node pair
      const pairKey =
        sourceId < targetId
          ? `${sourceId}->${targetId}`
          : `${targetId}->${sourceId}`;
      const totalEdges = edgeCounts.get(pairKey) || 1;
      const currentIndex = edgeIndex.get(pairKey) || 0;
      edgeIndex.set(pairKey, currentIndex + 1);

      // Compute perpendicular offset for multi-edges
      let offsetAmount = 0;
      if (totalEdges > 1) {
        const spread = 20; // pixels between parallel edges
        offsetAmount =
          (currentIndex - (totalEdges - 1) / 2) * spread;
      }

      const halfW = NODE_CARD_W / 2;
      const halfH = NODE_CARD_H / 2;

      // Nested containers are boxes: anchor the edge on their border.
      const sourceBox = this._containerBoxes && this._containerBoxes.get(sourceId);
      const targetBox = this._containerBoxes && this._containerBoxes.get(targetId);
      const srcHalfW = sourceBox ? sourceBox.width / 2 : halfW;
      const srcHalfH = sourceBox ? sourceBox.height / 2 : halfH;
      const tgtHalfW = targetBox ? targetBox.width / 2 : halfW;
      const tgtHalfH = targetBox ? targetBox.height / 2 : halfH;

      // Compute anchor points on card edges (rect intersection)
      // For orthogonal routing we pick the nearest horizontal or vertical edge
      const absDx = Math.abs(dx);
      const absDy = Math.abs(dy);

      let sx, sy, tx, ty;
      if (absDx > absDy) {
        // Primarily horizontal: exit from left/right edges
        sx = source.x + Math.sign(dx) * srcHalfW;
        sy = source.y + offsetAmount;
        tx = target.x - Math.sign(dx) * tgtHalfW;
        ty = target.y + offsetAmount;
      } else {
        // Primarily vertical: exit from top/bottom edges
        sx = source.x + offsetAmount;
        sy = source.y + Math.sign(dy) * srcHalfH;
        tx = target.x + offsetAmount;
        ty = target.y - Math.sign(dy) * tgtHalfH;
      }

      let pathData;
      const r = CONN_ELBOW_RADIUS;

      if (dist < CONN_CLOSE_DIST) {
        // Short connections: straight line
        pathData = `M ${sx},${sy} L ${tx},${ty}`;
      } else if (absDx > absDy) {
        // Primarily horizontal: elbow at midX with rounded corners
        const midX = (sx + tx) / 2;
        const dxSign = Math.sign(tx - sx);
        const dySign = Math.sign(ty - sy);
        const vGap = Math.abs(ty - sy);
        const hHalf = Math.abs(midX - sx);
        // Clamp radius to half the shorter segment
        const cr = Math.min(r, vGap / 2, hHalf);
        if (vGap < 1) {
          // Same Y: straight line
          pathData = `M ${sx},${sy} L ${tx},${ty}`;
        } else {
          pathData = `M ${sx},${sy}`
            + ` L ${midX - dxSign * cr},${sy}`
            + ` Q ${midX},${sy} ${midX},${sy + dySign * cr}`
            + ` L ${midX},${ty - dySign * cr}`
            + ` Q ${midX},${ty} ${midX + dxSign * cr},${ty}`
            + ` L ${tx},${ty}`;
        }
      } else {
        // Primarily vertical: elbow at midY with rounded corners
        const midY = (sy + ty) / 2;
        const dxSign = Math.sign(tx - sx);
        const dySign = Math.sign(ty - sy);
        const hGap = Math.abs(tx - sx);
        const vHalf = Math.abs(midY - sy);
        const cr = Math.min(r, hGap / 2, vHalf);
        if (hGap < 1) {
          // Same X: straight line
          pathData = `M ${sx},${sy} L ${tx},${ty}`;
        } else {
          pathData = `M ${sx},${sy}`
            + ` L ${sx},${midY - dySign * cr}`
            + ` Q ${sx},${midY} ${sx + dxSign * cr},${midY}`
            + ` L ${tx - dxSign * cr},${midY}`
            + ` Q ${tx},${midY} ${tx},${midY + dySign * cr}`
            + ` L ${tx},${ty}`;
        }
      }

      const pathId = `conn-path-${connection.id}`;

      const lineAttrs = {
        id: pathId,
        d: pathData,
        "data-connection": connection.id,
        stroke: style.color,
        "stroke-width": String(style.width),
        fill: "none",
        opacity: String(style.opacity),
        "stroke-linecap": "round",
        "stroke-linejoin": "round",
        "marker-end": `url(#arrow-${edgeType})`,
      };
      if (style.dash) {
        lineAttrs["stroke-dasharray"] = style.dash;
      }
      const line = svgEl("path", lineAttrs);
      if (connection.bidirectional) {
        line.setAttribute("marker-start", `url(#arrow-${edgeType})`);
      }
      connectionsGroup.appendChild(line);

      // Invisible wide path on top of the edge: it makes the thin edge
      // hoverable and carries the edge tooltip.
      const hitArea = svgEl("path", {
        d: pathData,
        fill: "none",
        stroke: "transparent",
        "stroke-width": "14",
        "pointer-events": "stroke",
      });
      hitArea.classList.add("connection-hit");
      hitArea.addEventListener("mouseenter", (e) =>
        this.showEdgeTooltip(e, connection, hitArea),
      );
      hitArea.addEventListener("mouseleave", () => this.hideTooltip(hitArea));
      hitArea.addEventListener("click", (e) => {
        e.stopPropagation();
        this.showEdgeTooltip(e, connection, hitArea);
      });
      connectionsGroup.appendChild(hitArea);

      // Edge label with collision avoidance
      if (connection.description) {
        let labelX = (source.x + target.x) / 2;
        let labelY = (source.y + target.y) / 2 - 8;
        const labelText = truncateText(
          connection.description,
          CONN_LABEL_MAX_CHARS,
        );
        const textWidth =
          labelText.length * CONN_LABEL_CHAR_WIDTH + CONN_LABEL_PAD;

        // Apply multi-edge offset to labels too
        labelX += perpDx * offsetAmount;
        labelY += perpDy * offsetAmount;

        // Collision avoidance: shift label if overlapping with existing labels
        for (let attempt = 0; attempt < 5; attempt++) {
          let collision = false;
          for (const placed of placedLabels) {
            const overlapX =
              Math.abs(labelX - placed.x) < (textWidth + placed.w) / 2;
            const overlapY =
              Math.abs(labelY - placed.y) < CONN_LABEL_HEIGHT + 2;
            if (overlapX && overlapY) {
              collision = true;
              labelY -= CONN_LABEL_HEIGHT + 4;
              break;
            }
          }
          if (!collision) break;
        }

        placedLabels.push({ x: labelX, y: labelY, w: textWidth });

        const labelBg = svgEl("rect", {
          x: String(labelX - textWidth / 2),
          y: String(labelY - 8),
          width: String(textWidth),
          height: String(CONN_LABEL_HEIGHT),
          rx: "4",
          fill: "var(--sm-terminal-bg)",
          "fill-opacity": "0.85",
          stroke: style.color,
          "stroke-width": "0.5",
          "stroke-opacity": "0.4",
          "pointer-events": "none",
        });
        connectionsGroup.appendChild(labelBg);

        const label = svgEl("text", {
          x: String(labelX),
          y: String(labelY + 1),
          "text-anchor": "middle",
          "dominant-baseline": "central",
          "font-size": CONN_LABEL_FONT_SIZE,
          "font-family": TYPE_LABEL_FONT,
          fill: style.color,
          "fill-opacity": "0.7",
          "pointer-events": "none",
        });
        label.textContent = labelText;
        connectionsGroup.appendChild(label);
      }

      // Animated flow dot
      if (style.flowDot && this.showFlowAnimations) {
        const dot = svgEl("circle", {
          r: String(FLOW_DOT_RADIUS),
          fill: style.dotColor,
          opacity: "0.9",
          "pointer-events": "none",
        });
        dot.classList.add("flow-dot");

        const animateMotion = svgEl("animateMotion", {
          dur: dist < FLOW_LONG_DIST ? FLOW_DOT_SHORT_DUR : FLOW_DOT_LONG_DUR,
          repeatCount: "indefinite",
        });
        const mpath = svgEl("mpath");
        mpath.setAttributeNS(
          "http://www.w3.org/1999/xlink",
          "xlink:href",
          `#${pathId}`,
        );
        mpath.setAttribute("href", `#${pathId}`);
        animateMotion.appendChild(mpath);
        dot.appendChild(animateMotion);
        connectionsGroup.appendChild(dot);

        // Second dot for longer connections
        if (dist > FLOW_LONG_DIST) {
          const dot2 = svgEl("circle", {
            r: String(FLOW_DOT2_RADIUS),
            fill: style.dotColor,
            opacity: "0.6",
            "pointer-events": "none",
          });
          dot2.classList.add("flow-dot");

          const animateMotion2 = svgEl("animateMotion", {
            dur: FLOW_DOT_LONG_DUR,
            repeatCount: "indefinite",
            begin: "-1.25s",
          });
          const mpath2 = svgEl("mpath");
          mpath2.setAttributeNS(
            "http://www.w3.org/1999/xlink",
            "xlink:href",
            `#${pathId}`,
          );
          mpath2.setAttribute("href", `#${pathId}`);
          animateMotion2.appendChild(mpath2);
          dot2.appendChild(animateMotion2);
          connectionsGroup.appendChild(dot2);
        }
      }
    });
  }

  renderNodes(nodesGroup) {
    this.filteredResources.forEach((resource) => {
      // VPCs and subnets are drawn as boxes, not as cards.
      if (this._containerBoxes && this._containerBoxes.has(resource.id)) return;

      const nodeGroup = svgEl("g", {
        transform: `translate(${resource.x}, ${resource.y})`,
        "data-id": resource.id,
      });
      nodeGroup.classList.add("resource-node");

      const layerColor = this.getResourceColor(resource);
      const halfW = NODE_CARD_W / 2;
      const halfH = NODE_CARD_H / 2;

      // Outer glow rectangle
      const glowRect = svgEl("rect", {
        x: String(-halfW - 6),
        y: String(-halfH - 6),
        width: String(NODE_CARD_W + 12),
        height: String(NODE_CARD_H + 12),
        rx: String(NODE_CARD_RX + 4),
        fill: "none",
        stroke: layerColor,
        "stroke-width": "0.5",
        "stroke-opacity": "0.15",
      });
      nodeGroup.appendChild(glowRect);

      // Card background rectangle
      const cardBg = svgEl("rect", {
        x: String(-halfW),
        y: String(-halfH),
        width: String(NODE_CARD_W),
        height: String(NODE_CARD_H),
        rx: String(NODE_CARD_RX),
        fill: layerColor,
        "fill-opacity": "0.08",
        stroke: layerColor,
        "stroke-width": "1.5",
        "stroke-opacity": "0.4",
      });
      if (resource.in_state_file === false) {
        cardBg.setAttribute("stroke-dasharray", "6,3");
        cardBg.setAttribute("stroke-width", "2");
      }
      nodeGroup.appendChild(cardBg);

      // Icon (centered horizontally, positioned in upper portion of card)
      const iconPath =
        resource.icon_url || this.getResourceIconPath(resource.type);
      const icon = svgEl("image", {
        href: iconPath,
        x: String(-ICON_OFFSET),
        y: String(NODE_ICON_Y - ICON_OFFSET),
        width: String(ICON_SIZE),
        height: String(ICON_SIZE),
        preserveAspectRatio: "xMidYMid meet",
      });
      nodeGroup.appendChild(icon);

      // Labels (positioned in lower portion of card)
      if (this.showLabels) {
        const label = svgEl("text", {
          x: "0",
          y: String(LABEL_Y_OFFSET),
          "text-anchor": "middle",
        });
        label.classList.add("resource-label");
        label.textContent = truncateText(resource.name, LABEL_MAX_CHARS);
        nodeGroup.appendChild(label);

        const typeLabel = svgEl("text", {
          x: "0",
          y: String(TYPE_LABEL_Y_OFFSET),
          "text-anchor": "middle",
          "font-size": TYPE_LABEL_FONT_SIZE,
          fill: "var(--sm-node-type)",
          "font-family": TYPE_LABEL_FONT,
        });
        typeLabel.textContent = truncateText(
          resource.type,
          TYPE_LABEL_MAX_CHARS,
        );
        nodeGroup.appendChild(typeLabel);
      }

      nodeGroup.addEventListener("click", (e) => {
        e.stopPropagation();
        this.selectResource(resource);
      });
      nodeGroup.addEventListener("mouseenter", (e) =>
        this.showTooltip(e, resource, nodeGroup),
      );
      nodeGroup.addEventListener("mouseleave", () => this.hideTooltip(nodeGroup));

      nodesGroup.appendChild(nodeGroup);
    });
  }

  // ─── Icons ─────────────────────────────────────────────────────────

  getResourceIconPath(resourceType) {
    const t = (resourceType || "").toLowerCase();
    const iconMap = {
      // Compute
      "aws:ec2:instance": "/icons/Compute/EC2.svg",
      "aws:lambda:function": "/icons/Compute/Lambda.svg",
      // Containers
      "aws:ecs:cluster": "/icons/Containers/Elastic-Container-Service.svg",
      "aws:ecs:service": "/icons/Containers/Elastic-Container-Service.svg",
      "aws:ecs:taskdefinition":
        "/icons/Containers/Elastic-Container-Service.svg",
      "aws:ecr:repository": "/icons/Containers/Elastic-Container-Registry.svg",
      // API Gateway
      "aws:apigateway:restapi": "/icons/App-Integration/API-Gateway.svg",
      "aws:apigateway:resource": "/icons/App-Integration/API-Gateway.svg",
      "aws:apigateway:method": "/icons/App-Integration/API-Gateway.svg",
      "aws:apigateway:integration": "/icons/App-Integration/API-Gateway.svg",
      "aws:apigateway:stage": "/icons/App-Integration/API-Gateway.svg",
      "aws:apigateway:deployment": "/icons/App-Integration/API-Gateway.svg",
      "aws:apigateway:domainname": "/icons/App-Integration/API-Gateway.svg",
      "aws:apigateway:usageplan": "/icons/App-Integration/API-Gateway.svg",
      "aws:apigateway:vpclink": "/icons/App-Integration/API-Gateway.svg",
      "aws:apigateway:basepathmapping":
        "/icons/App-Integration/API-Gateway.svg",
      "aws:apigateway:methodresponse": "/icons/App-Integration/API-Gateway.svg",
      "aws:apigateway:integrationresponse":
        "/icons/App-Integration/API-Gateway.svg",
      // Load Balancing
      "aws:elb:loadbalancer":
        "/icons/Networking-Content-Delivery/Elastic-Load-Balancing.svg",
      "aws:elb:listener":
        "/icons/Networking-Content-Delivery/Elastic-Load-Balancing.svg",
      "aws:elb:targetgroup":
        "/icons/Networking-Content-Delivery/Elastic-Load-Balancing.svg",
      "aws:elb:listenerrule":
        "/icons/Networking-Content-Delivery/Elastic-Load-Balancing.svg",
      "aws:elb:targetgroupattachment":
        "/icons/Networking-Content-Delivery/Elastic-Load-Balancing.svg",
      // Storage
      "aws:s3:bucket": "/icons/Storage/Simple-Storage-Service.svg",
      // Database
      "aws:rds:instance": "/icons/Database/RDS.svg",
      "aws:dynamodb:table": "/icons/Database/DynamoDB.svg",
      // Messaging
      "aws:sns:topic": "/icons/App-Integration/Simple-Notification-Service.svg",
      "aws:sns:subscription":
        "/icons/App-Integration/Simple-Notification-Service.svg",
      "aws:sns:topicpolicy":
        "/icons/App-Integration/Simple-Notification-Service.svg",
      "aws:sqs:queue": "/icons/App-Integration/Simple-Queue-Service.svg",
      "aws:sqs:queuepolicy": "/icons/App-Integration/Simple-Queue-Service.svg",
      // Monitoring
      "aws:logs:loggroup": "/icons/Management-Governance/CloudWatch.svg",
      "aws:cloudwatch:alarm": "/icons/Management-Governance/CloudWatch.svg",
      // IAM
      "aws:iam:role":
        "/icons/Security-Identity-Compliance/Identity-and-Access-Management.svg",
      "aws:iam:policy":
        "/icons/Security-Identity-Compliance/Identity-and-Access-Management.svg",
      "aws:iam:user":
        "/icons/Security-Identity-Compliance/Identity-and-Access-Management.svg",
      "aws:iam:rolepolicyattachment":
        "/icons/Security-Identity-Compliance/Identity-and-Access-Management.svg",
      // Security
      "aws:acm:certificate":
        "/icons/Security-Identity-Compliance/Certificate-Manager.svg",
      "aws:acm:certificatevalidation":
        "/icons/Security-Identity-Compliance/Certificate-Manager.svg",
      "aws:waf:webacl": "/icons/Security-Identity-Compliance/WAF.svg",
      "aws:waf:ipset": "/icons/Security-Identity-Compliance/WAF.svg",
      "aws:waf:webaclassociation":
        "/icons/Security-Identity-Compliance/WAF.svg",
      "aws:secretsmanager:secret":
        "/icons/Security-Identity-Compliance/Secrets-Manager.svg",
      "aws:kms:key":
        "/icons/Security-Identity-Compliance/Key-Management-Service.svg",
      // Networking
      "aws:ec2:vpc":
        "/icons/Networking-Content-Delivery/Virtual-Private-Cloud.svg",
      "aws:ec2:subnet":
        "/icons/Networking-Content-Delivery/Virtual-Private-Cloud.svg",
      "aws:ec2:securitygroup":
        "/icons/Networking-Content-Delivery/Virtual-Private-Cloud.svg",
      "aws:route53:record": "/icons/Networking-Content-Delivery/Route-53.svg",
      "aws:route53:hostedzone":
        "/icons/Networking-Content-Delivery/Route-53.svg",
      "aws:cloudfront:distribution":
        "/icons/Networking-Content-Delivery/CloudFront.svg",
      // Service Discovery
      "aws:servicediscovery:namespace":
        "/icons/Networking-Content-Delivery/Cloud-Map.svg",
      "aws:servicediscovery:service":
        "/icons/Networking-Content-Delivery/Cloud-Map.svg",
    };
    return iconMap[t] || "/icons/General-Icons/Marketplace_Dark.svg";
  }

  getResourceColor(resource) {
    const layer = this.layers[resource.layer];
    return layer ? layer.color : "#888";
  }

  // ─── Minimap ───────────────────────────────────────────────────────

  updateMinimap() {
    if (!this.showMinimap) return;

    const allResources = this.resources;
    if (allResources.length === 0) return;

    const minX = safeMin(allResources.map((r) => r.x)) - MINIMAP_PADDING;
    const maxX = safeMax(allResources.map((r) => r.x)) + MINIMAP_PADDING;
    const minY = safeMin(allResources.map((r) => r.y)) - MINIMAP_PADDING;
    const maxY = safeMax(allResources.map((r) => r.y)) + MINIMAP_PADDING;

    const scaleX = MINIMAP_WIDTH / (maxX - minX);
    const scaleY = MINIMAP_HEIGHT / (maxY - minY);
    const scale = Math.min(scaleX, scaleY) * MINIMAP_SCALE_FACTOR;

    while (this.minimapSvg.firstChild)
      this.minimapSvg.removeChild(this.minimapSvg.firstChild);
    this.minimapSvg.setAttribute(
      "viewBox",
      `0 0 ${MINIMAP_WIDTH} ${MINIMAP_HEIGHT}`,
    );

    const filteredSet = new Set(this.filteredResources);

    allResources.forEach((resource) => {
      const x = (resource.x - minX) * scale + 10;
      const y = (resource.y - minY) * scale + 10;
      const isVisible = filteredSet.has(resource);

      const dot = svgEl("circle", {
        cx: x,
        cy: y,
        r: isVisible
          ? String(MINIMAP_DOT_SIZE)
          : String(MINIMAP_DOT_HIDDEN_SIZE),
        fill: isVisible ? this.getResourceColor(resource) : "#666",
        opacity: isVisible ? "1" : "0.3",
      });
      this.minimapSvg.appendChild(dot);
    });

    const viewportRect = svgEl("rect", {
      x: "5",
      y: "5",
      width: MINIMAP_WIDTH - 10,
      height: MINIMAP_HEIGHT - 10,
      fill: "none",
      stroke: "#4ADE80",
      "stroke-width": "1",
      opacity: "0.6",
    });

    // Compute actual viewport rectangle from zoom/pan state
    if (this.width && this.height && scale > 0) {
      const diagramWidth = maxX - minX;
      const diagramHeight = maxY - minY;

      // Convert screen viewport to diagram coordinates
      const viewLeft = (-this.panX / this.zoom - minX) * scale + 10;
      const viewTop = (-this.panY / this.zoom - minY) * scale + 10;
      const viewW = (this.width / this.zoom) * scale;
      const viewH = (this.height / this.zoom) * scale;

      // Clamp to minimap bounds
      viewportRect.setAttribute("x", Math.max(0, Math.min(viewLeft, MINIMAP_WIDTH)));
      viewportRect.setAttribute("y", Math.max(0, Math.min(viewTop, MINIMAP_HEIGHT)));
      viewportRect.setAttribute("width", Math.min(viewW, MINIMAP_WIDTH));
      viewportRect.setAttribute("height", Math.min(viewH, MINIMAP_HEIGHT));
    }

    this.minimapSvg.appendChild(viewportRect);

    // Store minimap transform data for interactive click-to-navigate
    this._minimapTransform = { minX, minY, scale };
  }

  // ─── UI Controls ───────────────────────────────────────────────────

  toggleSidebar() {
    const sidebar = document.getElementById("filter-sidebar");
    const toggle = document.getElementById("sidebar-toggle");

    this.sidebarCollapsed = !this.sidebarCollapsed;

    if (this.sidebarCollapsed) {
      sidebar.classList.add("collapsed");
      // The CSS transform only moves the sidebar visually; collapse its
      // flex footprint so the canvas reclaims the space.
      sidebar.style.width = "0";
      sidebar.style.minWidth = "0";
      sidebar.style.overflow = "hidden";
      sidebar.style.borderRight = "none";
      toggle.textContent = "\u2630"; // hamburger menu character
    } else {
      sidebar.classList.remove("collapsed");
      sidebar.style.width = "";
      sidebar.style.minWidth = "";
      sidebar.style.overflow = "";
      sidebar.style.borderRight = "";
      toggle.textContent = "\u2715"; // multiplication sign (close)
    }

    // Recalculate canvas dimensions after sidebar change
    this.onResize();
  }

  toggleLabels() {
    this.showLabels = !this.showLabels;
    document.getElementById("toggle-labels").classList.toggle("active");
    // Toggle label visibility via CSS class instead of full re-render
    const nodesGroup = this.svg.querySelector(".nodes");
    if (nodesGroup) {
      nodesGroup.classList.toggle("hide-labels", !this.showLabels);
    }
  }

  toggleMinimap() {
    this.showMinimap = !this.showMinimap;
    document.getElementById("minimap").classList.toggle("hidden");
    document.getElementById("toggle-minimap").classList.toggle("active");
  }

  togglePresentationMode() {
    this.presentationMode = !this.presentationMode;
    document.body.classList.toggle("presentation-mode");
    document.getElementById("presentation-mode").classList.toggle("active");
  }

  toggleFlowAnimations() {
    this.showFlowAnimations = !this.showFlowAnimations;
    const btn = document.getElementById("toggle-flow");
    if (btn) btn.classList.toggle("active");

    const dots = this.svg.querySelectorAll(".flow-dot");
    if (dots.length > 0 && !this.showFlowAnimations) {
      dots.forEach((d) => d.setAttribute("visibility", "hidden"));
    } else if (dots.length > 0 && this.showFlowAnimations) {
      dots.forEach((d) => d.setAttribute("visibility", "visible"));
    } else if (this.showFlowAnimations) {
      this.renderDiagram();
      this.updateTransform();
    }
  }

  // ─── Zoom & Pan ────────────────────────────────────────────────────

  zoomIn() {
    this.zoom = Math.min(this.zoom * 1.2, ZOOM_MAX);
    this.updateTransform();
  }

  zoomOut() {
    this.zoom = Math.max(this.zoom / 1.2, ZOOM_MIN);
    this.updateTransform();
  }

  fitToView() {
    if (this.filteredResources.length === 0) return;

    // Nested containers are bigger than their centre point, so the box
    // extents have to be part of the fitted area.
    let minX = Infinity;
    let maxX = -Infinity;
    let minY = Infinity;
    let maxY = -Infinity;
    this.filteredResources.forEach((resource) => {
      const box = this._containerBoxes && this._containerBoxes.get(resource.id);
      const halfW = box ? box.width / 2 : 0;
      const halfH = box ? box.height / 2 : 0;
      minX = Math.min(minX, resource.x - halfW);
      maxX = Math.max(maxX, resource.x + halfW);
      minY = Math.min(minY, resource.y - halfH);
      maxY = Math.max(maxY, resource.y + halfH);
    });

    minX -= FIT_VIEW_MARGIN;
    maxX += FIT_VIEW_MARGIN;
    minY -= FIT_VIEW_MARGIN;
    maxY += FIT_VIEW_MARGIN;

    const contentWidth = maxX - minX;
    const contentHeight = maxY - minY;

    const scaleX = this.width / contentWidth;
    const scaleY = this.height / contentHeight;
    this.zoom = Math.min(scaleX, scaleY, FIT_VIEW_MAX_ZOOM);

    this.panX = (this.width - contentWidth * this.zoom) / 2 - minX * this.zoom;
    this.panY =
      (this.height - contentHeight * this.zoom) / 2 - minY * this.zoom;

    this.updateTransform();
  }

  resetLayout() {
    this.categorizeResources();
    this.autoLayout();
    this.renderDiagram();
    this.updateMinimap();
    setTimeout(() => this.fitToView(), RESET_FIT_DELAY_MS);
  }

  updateTransform() {
    const nodesGroup = this.svg.querySelector(".nodes");
    const connectionsGroup = this.svg.querySelector(".connections");
    const layersGroup = this.svg.querySelector(".layers");

    const transform = `translate(${this.panX}, ${this.panY}) scale(${this.zoom})`;

    if (nodesGroup) nodesGroup.setAttribute("transform", transform);
    if (connectionsGroup) connectionsGroup.setAttribute("transform", transform);
    if (layersGroup) layersGroup.setAttribute("transform", transform);

    // Update minimap viewport to reflect current zoom/pan
    this.updateMinimap();
  }

  // ─── Mouse Events ──────────────────────────────────────────────────

  onMouseDown(e) {
    // Check if clicking on a node group (for drag-and-drop)
    const nodeGroup = e.target.closest("[data-id]");
    if (nodeGroup && e.button === 0) {
      const resourceId = nodeGroup.getAttribute("data-id");
      const resource = this._resourceMap.get(resourceId);
      if (resource) {
        this._draggingNode = resource;
        this._draggingNodeGroup = nodeGroup;
        this.lastMouseX = e.clientX;
        this.lastMouseY = e.clientY;
        this.svg.style.cursor = "move";
        e.preventDefault();
        return;
      }
    }

    if (e.target === this.svg || e.target.closest(".layers")) {
      this.isDragging = true;
      this.lastMouseX = e.clientX;
      this.lastMouseY = e.clientY;
      this.svg.style.cursor = "grabbing";
    }
  }

  onMouseMove(e) {
    // Node dragging
    if (this._draggingNode) {
      const deltaX = (e.clientX - this.lastMouseX) / this.zoom;
      const deltaY = (e.clientY - this.lastMouseY) / this.zoom;
      this._draggingNode.x += deltaX;
      this._draggingNode.y += deltaY;
      // A dragged VPC / subnet takes its nested resources with it
      this.moveNestedResources(
        this._draggingNode.id,
        deltaX,
        deltaY,
        new Set([this._draggingNode.id]),
      );
      this.lastMouseX = e.clientX;
      this.lastMouseY = e.clientY;

      if (!this._rafPending) {
        this._rafPending = true;
        requestAnimationFrame(() => {
          this._rafPending = false;
          // Re-render to update node position and connections
          this.renderDiagram();
          this.updateTransform();
        });
      }
      return;
    }

    // Canvas panning
    if (this.isDragging) {
      const deltaX = e.clientX - this.lastMouseX;
      const deltaY = e.clientY - this.lastMouseY;

      this.panX += deltaX;
      this.panY += deltaY;

      this.lastMouseX = e.clientX;
      this.lastMouseY = e.clientY;

      if (!this._rafPending) {
        this._rafPending = true;
        requestAnimationFrame(() => {
          this._rafPending = false;
          this.updateTransform();
        });
      }
    }
  }

  onMouseUp() {
    if (this._draggingNode) {
      this._draggingNode = null;
      this._draggingNodeGroup = null;
      this.svg.style.cursor = "grab";
      return;
    }
    this.isDragging = false;
    this.svg.style.cursor = "grab";
  }

  /** Move the resources nested inside a dragged container along with it. */
  moveNestedResources(id, deltaX, deltaY, seen) {
    const children = this._containerChildren && this._containerChildren.get(id);
    if (!children || !children.length) return;
    children.forEach((childId) => {
      if (seen.has(childId)) return;
      seen.add(childId);
      const child = this._resourceMap.get(childId);
      if (!child) return;
      child.x += deltaX;
      child.y += deltaY;
      this.moveNestedResources(childId, deltaX, deltaY, seen);
    });
  }

  onWheel(e) {
    e.preventDefault();

    if (this._wheelRafPending) return;
    this._wheelRafPending = true;

    const rect = this.svg.getBoundingClientRect();
    const mouseX = e.clientX - rect.left;
    const mouseY = e.clientY - rect.top;

    const oldZoom = this.zoom;
    const zoomFactor = e.deltaY > 0 ? ZOOM_FACTOR_OUT : ZOOM_FACTOR_IN;
    this.zoom = Math.min(Math.max(this.zoom * zoomFactor, ZOOM_MIN), ZOOM_MAX);

    // Zoom towards cursor position
    const zoomRatio = this.zoom / oldZoom;
    this.panX = mouseX - (mouseX - this.panX) * zoomRatio;
    this.panY = mouseY - (mouseY - this.panY) * zoomRatio;

    requestAnimationFrame(() => {
      this._wheelRafPending = false;
      this.updateTransform();
    });
  }

  onResize() {
    const container = document.querySelector(".canvas-container");
    this.width = container.clientWidth;
    this.height = container.clientHeight;

    this.svg.setAttribute("width", this.width);
    this.svg.setAttribute("height", this.height);
    this.svg.setAttribute("viewBox", `0 0 ${this.width} ${this.height}`);
  }

  // ─── Filters & Search ──────────────────────────────────────────────

  updateFilters() {
    if (!this.dropdowns) return;
    const types = [...new Set(this.resources.map((r) => r.type))].sort();
    const providers = [
      ...new Set(this.resources.map((r) => r.provider || "Unknown")),
    ].sort();
    const regions = [
      ...new Set(this.resources.map((r) => r.region || "Unknown")),
    ].sort();

    this.dropdowns.type.setOptions([
      { value: '', label: 'All Types' },
      ...types.map(t => ({ value: t, label: t })),
    ]);
    this.dropdowns.provider.setOptions([
      { value: '', label: 'All Providers' },
      ...providers.map(p => ({ value: p, label: p })),
    ]);
    this.dropdowns.region.setOptions([
      { value: '', label: 'All Regions' },
      ...regions.map(r => ({ value: r, label: r })),
    ]);
  }

  applyFilters() {
    const typeFilter = this.dropdowns.type.value;
    const providerFilter = this.dropdowns.provider.value;
    const regionFilter = this.dropdowns.region.value;
    const stateFilter = this.dropdowns.state.value;
    const searchTerm = document
      .getElementById("search-input")
      .value.toLowerCase();

    this.filteredResources = this.resources.filter((resource) => {
      return (
        (!typeFilter || resource.type === typeFilter) &&
        (!providerFilter || resource.provider === providerFilter) &&
        (!regionFilter || resource.region === regionFilter) &&
        (!stateFilter || resource.state === stateFilter) &&
        (!searchTerm || resource.name.toLowerCase().includes(searchTerm))
      );
    });

    this.updateResourceList();
    this.renderDiagram();
    this.updateMinimap();
    this.updateStats();
    this.updateTransform();
  }

  updateResourceList() {
    const resourceList = document.getElementById("resource-list");
    // Clear using DOM API instead of innerHTML
    while (resourceList.firstChild)
      resourceList.removeChild(resourceList.firstChild);

    // Build all items in a DocumentFragment to minimize reflows
    const fragment = document.createDocumentFragment();

    this.filteredResources.forEach((resource) => {
      const item = document.createElement("div");
      item.className = "resource-item";
      item.onclick = () => this.selectResource(resource);

      const name = document.createElement("div");
      name.className = "resource-name";

      const iconImg = document.createElement("img");
      iconImg.src =
        resource.icon_url || this.getResourceIconPath(resource.type);

      iconImg.width = 16;
      iconImg.height = 16;
      iconImg.style.cssText =
        "vertical-align:middle; margin-right:6px; flex-shrink:0;";

      const nameText = document.createTextNode(resource.name);
      name.appendChild(iconImg);
      name.appendChild(nameText);

      const type = document.createElement("div");
      type.className = "resource-type";
      type.textContent = resource.type;

      item.appendChild(name);
      item.appendChild(type);

      if (resource.tags && Object.keys(resource.tags).length > 0) {
        const tagsDiv = document.createElement("div");
        tagsDiv.className = "resource-tags";

        Object.entries(resource.tags)
          .slice(0, 3)
          .forEach(([key, value]) => {
            const tag = document.createElement("span");
            tag.className = "resource-tag";
            tag.textContent = `${key}: ${value}`;
            tagsDiv.appendChild(tag);
          });

        item.appendChild(tagsDiv);
      }

      fragment.appendChild(item);
    });

    resourceList.appendChild(fragment);
    this.updateResourceTable();
  }

  updateResourceTable() {
    const body = document.getElementById("resource-table-body");
    while (body.firstChild) body.removeChild(body.firstChild);

    if (this.filteredResources.length === 0) {
      const row = body.insertRow();
      const cell = row.insertCell();
      cell.colSpan = 8;
      cell.className = "resource-table-empty";
      cell.textContent = "No resources match the current filters.";
      return;
    }

    const fragment = document.createDocumentFragment();
    this.filteredResources.forEach((resource) => {
      const row = document.createElement("tr");
      row.addEventListener("click", () => this.selectResource(resource, false));

      const nameCell = document.createElement("td");
      const name = document.createElement("div");
      name.className = "resource-table-name";
      const icon = document.createElement("img");
      icon.src = resource.icon_url || this.getResourceIconPath(resource.type);
      icon.alt = "";
      name.appendChild(icon);
      name.appendChild(document.createTextNode(resource.name || ""));
      nameCell.appendChild(name);
      row.appendChild(nameCell);

      ["type", "provider", "region", "account", "state", "source"].forEach((key) => {
        const cell = document.createElement("td");
        cell.textContent = resource[key] || "-";
        if (key === "type" || key === "account") cell.className = "resource-table-code";
        row.appendChild(cell);
      });
      const presenceCell = document.createElement("td");
      const presenceBadge = document.createElement("span");
      presenceBadge.className = `state-file-badge${resource.in_state_file === false ? " external" : ""}`;
      presenceBadge.textContent = resource.in_state_file === false ? "No" : "Yes";
      presenceCell.appendChild(presenceBadge);
      row.appendChild(presenceCell);
      fragment.appendChild(row);
    });
    body.appendChild(fragment);
  }

  setView(view) {
    this.currentView = view;
    const listVisible = view === "list";
    document.getElementById("resource-table-view").classList.toggle("visible", listVisible);
    document.getElementById("resource-table-view").setAttribute("aria-hidden", String(!listVisible));
    document.getElementById("diagram-view-btn").classList.toggle("active", !listVisible);
    document.getElementById("diagram-view-btn").setAttribute("aria-pressed", String(!listVisible));
    document.getElementById("list-view-btn").classList.toggle("active", listVisible);
    document.getElementById("list-view-btn").setAttribute("aria-pressed", String(listVisible));
  }

  updateStats() {
    document.getElementById("visible-count").textContent =
      this.filteredResources.length;
  }

  // ─── Selection & Dependencies ──────────────────────────────────────

  selectResource(resource, panToResource = true) {
    if (this.selectedResource) {
      document.querySelectorAll(".resource-item.selected").forEach((item) => {
        item.classList.remove("selected");
      });
      this.clearDependencyHighlighting();
    }

    this.selectedResource = resource;

    const resourceItems = document.querySelectorAll(".resource-item");
    resourceItems.forEach((item, index) => {
      if (this.filteredResources[index] === resource) {
        item.classList.add("selected");
      }
    });

    this.highlightDependencies(resource);
    this.updateResourceDetailsPanel(resource);
    if (panToResource) this.panToResource(resource);
  }

  panToResource(resource) {
    const centerX = this.width / 2;
    const centerY = this.height / 2;

    this.panX = centerX - resource.x * this.zoom;
    this.panY = centerY - resource.y * this.zoom;

    this.updateTransform();
  }

  highlightDependencies(resource) {
    const trace = this.getTrace(resource);

    // Build lookup maps for DOM elements to avoid querySelector per resource/connection
    const nodeElements = new Map();
    this.svg.querySelectorAll("[data-id]").forEach((el) => {
      nodeElements.set(el.getAttribute("data-id"), el);
    });
    const connElements = new Map();
    this.svg.querySelectorAll("path[data-connection]").forEach((el) => {
      connElements.set(el.getAttribute("data-connection"), el);
    });

    const upstreamIds = trace.upstreamIds;
    const downstreamIds = trace.downstreamIds;
    const tracedIds = new Set([resource.id, ...upstreamIds, ...downstreamIds]);

    this.resources.forEach((r) => {
      const nodeElement = nodeElements.get(r.id);
      if (!nodeElement) return;

      if (r.id === resource.id) {
        nodeElement.classList.add("selected-resource");
        // Apply selection glow directly to avoid full re-render
        this.applySelectedStyle(nodeElement, true);
      } else if (upstreamIds.has(r.id)) {
        nodeElement.classList.add("upstream-dependency");
      } else if (downstreamIds.has(r.id)) {
        nodeElement.classList.add("downstream-dependency");
      } else {
        nodeElement.classList.add("unrelated-resource");
      }
    });

    this.connections.forEach((conn) => {
      const pathElement = connElements.get(conn.id);
      if (!pathElement) return;

      // Every edge inside the traced subgraph stays lit, the rest fades.
      if (
        tracedIds.has(conn.source_id) &&
        tracedIds.has(conn.target_id)
      ) {
        pathElement.classList.add("highlighted-connection");
      } else {
        pathElement.classList.add("dimmed-connection");
      }
    });
  }

  /**
   * Selection glow for a node. Cards use a glow ring plus a card background,
   * nested container boxes only have a single rectangle.
   */
  applySelectedStyle(element, selected) {
    if (element.classList.contains("container-box")) {
      const rect = element.querySelector("rect");
      if (rect) {
        rect.setAttribute(
          "stroke-width",
          selected ? "2.5" : element.dataset.baseStrokeWidth || "1.5",
        );
        rect.setAttribute(
          "stroke-opacity",
          selected ? "1" : element.dataset.baseStrokeOpacity || "0.45",
        );
      }
      return;
    }

    const rects = element.querySelectorAll("rect");
    if (rects.length >= 2) {
      // First rect is glow ring, second is card background
      rects[0].setAttribute("stroke-width", selected ? "1" : "0.5");
      rects[0].setAttribute("stroke-opacity", selected ? "0.5" : "0.15");
      rects[1].setAttribute("stroke-width", selected ? "2.5" : "1.5");
      rects[1].setAttribute("stroke-opacity", selected ? "1" : "0.4");
    }
  }

  clearDependencyHighlighting() {
    document
      .querySelectorAll(".selected-resource")
      .forEach((el) => {
        // Reset selection glow attributes to defaults
        this.applySelectedStyle(el, false);
      });

    document
      .querySelectorAll(
        ".selected-resource, .upstream-dependency, .downstream-dependency, .unrelated-resource",
      )
      .forEach((el) => {
        el.classList.remove(
          "selected-resource",
          "upstream-dependency",
          "downstream-dependency",
          "unrelated-resource",
        );
      });

    document
      .querySelectorAll(".highlighted-connection, .dimmed-connection")
      .forEach((el) => {
        el.classList.remove("highlighted-connection", "dimmed-connection");
      });
  }

  /** Build the adjacency index of the relation graph. */
  buildGraphIndex() {
    this._graphOut = new Map();
    this._graphIn = new Map();
    this.connections.forEach((conn) => {
      const source = conn.source_id || conn.source;
      const target = conn.target_id || conn.target;
      if (!source || !target) return;
      if (!this._graphOut.has(source)) this._graphOut.set(source, []);
      if (!this._graphIn.has(target)) this._graphIn.set(target, []);
      this._graphOut.get(source).push(target);
      this._graphIn.get(target).push(source);
    });
  }

  /**
   * Walk the relation graph in one direction, breadth first, up to `depth` hops.
   * Returns the ids that were reached (the start node is never included).
   */
  traceDirection(startId, neighbours, depth) {
    const seen = new Set([startId]);
    const reached = new Set();
    let frontier = [startId];

    for (let hop = 0; hop < depth; hop++) {
      const next = [];
      frontier.forEach((id) => {
        (neighbours.get(id) || []).forEach((other) => {
          if (seen.has(other)) return;
          seen.add(other);
          reached.add(other);
          next.push(other);
        });
      });
      if (!next.length) break;
      frontier = next;
    }

    return reached;
  }

  /**
   * Multi hop trace around the selected resource: everything that reaches it
   * (upstream) and everything it reaches (downstream), up to `depth` hops.
   */
  getTrace(resource, depth = this.traceDepth) {
    // depth 0 means "follow every hop".
    const hops = depth === 0 ? Infinity : depth;
    const upstreamIds = this.traceDirection(resource.id, this._graphIn, hops);
    const downstreamIds = this.traceDirection(resource.id, this._graphOut, hops);

    const toResources = (ids) =>
      Array.from(ids)
        .map((id) => this._resourceMap.get(id))
        .filter(Boolean);

    return {
      upstream: toResources(upstreamIds),
      downstream: toResources(downstreamIds),
      upstreamIds,
      downstreamIds,
      depth,
    };
  }

  /**
   * Build the resource details panel entirely via DOM API (no innerHTML with
   * user-controlled data) to prevent XSS. The dependency items use closures
   * that capture `this` directly, avoiding the fragile global `diagram` reference.
   */
  updateResourceDetailsPanel(resource) {
    let panel = document.getElementById("resource-details-panel");
    if (!panel) {
      panel = this.createResourceDetailsPanel();
    }

    const trace = this.getTrace(resource);

    // Clear panel
    while (panel.firstChild) panel.removeChild(panel.firstChild);

    // ── Panel header ──
    const header = document.createElement("div");
    header.className = "panel-header";

    const h3 = document.createElement("h3");
    h3.textContent = "Resource Details";
    header.appendChild(h3);

    const closeBtn = document.createElement("button");
    closeBtn.id = "close-details";
    closeBtn.className = "close-btn";
    closeBtn.textContent = "\u00D7"; // &times;
    closeBtn.onclick = () => {
      panel.classList.remove("visible");
      this.clearDependencyHighlighting();
      this.selectedResource = null;
    };
    header.appendChild(closeBtn);
    panel.appendChild(header);

    // ── Resource info ──
    const infoDiv = document.createElement("div");
    infoDiv.className = "resource-info";

    const titleDiv = document.createElement("div");
    titleDiv.className = "resource-title";
    const iconSpan = document.createElement("span");
    iconSpan.className = "resource-icon";
    const iconImg = document.createElement("img");
    iconImg.src = resource.icon_url || this.getResourceIconPath(resource.type);

    iconImg.width = 24;
    iconImg.height = 24;
    iconImg.style.verticalAlign = "middle";
    iconSpan.appendChild(iconImg);
    titleDiv.appendChild(iconSpan);
    const nameSpan = document.createElement("span");
    nameSpan.className = "resource-name";
    nameSpan.textContent = resource.name;
    titleDiv.appendChild(nameSpan);
    infoDiv.appendChild(titleDiv);

    const typeDiv = document.createElement("div");
    typeDiv.className = "resource-type";
    typeDiv.textContent = resource.type;
    infoDiv.appendChild(typeDiv);

    const layerDiv = document.createElement("div");
    layerDiv.className = "resource-layer";
    layerDiv.textContent = `Group: ${resource.layer || "Unknown"}`;
    infoDiv.appendChild(layerDiv);

    const stateDiv = document.createElement("div");
    stateDiv.className = "resource-state";
    stateDiv.textContent = `State: ${resource.state || "Unknown"}`;
    infoDiv.appendChild(stateDiv);

    const presenceDiv = document.createElement("div");
    presenceDiv.className = "resource-state";
    presenceDiv.textContent = `In state file: ${resource.in_state_file === false ? "No" : "Yes"}`;
    infoDiv.appendChild(presenceDiv);

    panel.appendChild(infoDiv);

    // ── Dependencies section ──
    const depsSection = document.createElement("div");
    depsSection.className = "dependencies-section";

    const depsH4 = document.createElement("h4");
    depsH4.textContent = "Dependencies";
    depsSection.appendChild(depsH4);

    // ── Trace depth control ──
    const depthRow = document.createElement("div");
    depthRow.className = "trace-depth";

    const depthLabel = document.createElement("span");
    depthLabel.className = "trace-depth-label";
    depthLabel.textContent = "Trace depth";
    depthRow.appendChild(depthLabel);

    // 0 means "follow every hop".
    [1, 2, 3, 0].forEach((depth) => {
      const btn = document.createElement("button");
      btn.className = "trace-depth-btn";
      btn.textContent = depth === 0 ? "All" : String(depth);
      if ((this.traceDepth === 0 ? 0 : this.traceDepth) === depth) {
        btn.classList.add("active");
      }
      btn.onclick = () => {
        this.traceDepth = depth;
        this.highlightDependencies(resource);
        this.updateResourceDetailsPanel(resource);
      };
      depthRow.appendChild(btn);
    });

    depsSection.appendChild(depthRow);

    // Helper to create a dependency group
    const createDepGroup = (label, items, direction) => {
      const group = document.createElement("div");
      group.className = "dependency-group";

      const h5 = document.createElement("h5");
      h5.textContent = `${label} (${items.length})`;
      group.appendChild(h5);

      const list = document.createElement("div");
      list.className = "dependency-list";

      items.forEach((r) => {
        const depItem = document.createElement("div");
        depItem.className = `dependency-item ${direction}`;
        // Use a closure that captures `this` directly (no global `diagram`)
        depItem.onclick = () => this.selectResource(r);

        const depIcon = document.createElement("span");
        depIcon.className = "dep-icon";
        const depImg = document.createElement("img");
        depImg.src = r.icon_url || this.getResourceIconPath(r.type);
        depImg.width = 16;

        depImg.height = 16;
        depImg.style.verticalAlign = "middle";
        depIcon.appendChild(depImg);
        depItem.appendChild(depIcon);

        const depName = document.createElement("span");
        depName.className = "dep-name";
        depName.textContent = r.name;
        depItem.appendChild(depName);

        const depType = document.createElement("span");
        depType.className = "dep-type";
        depType.textContent = r.type;
        depItem.appendChild(depType);

        list.appendChild(depItem);
      });

      group.appendChild(list);
      return group;
    };

    depsSection.appendChild(
      createDepGroup("Upstream", trace.upstream, "upstream"),
    );
    depsSection.appendChild(
      createDepGroup("Downstream", trace.downstream, "downstream"),
    );
    panel.appendChild(depsSection);

    // ── Properties section ──
    const propsSection = document.createElement("div");
    propsSection.className = "properties-section";

    const propsH4 = document.createElement("h4");
    propsH4.textContent = "Properties";
    propsSection.appendChild(propsH4);

    const propsList = document.createElement("div");
    propsList.className = "properties-list";

    Object.entries(resource.properties || {})
      .slice(0, 10)
      .forEach(([key, value]) => {
        const propItem = document.createElement("div");
        propItem.className = "property-item";

        const propKey = document.createElement("span");
        propKey.className = "prop-key";
        propKey.textContent = `${key}:`;
        propItem.appendChild(propKey);

        const propValue = document.createElement("span");
        propValue.className = "prop-value";
        propValue.textContent =
          typeof value === "string"
            ? truncateText(value, 30)
            : JSON.stringify(value);
        propItem.appendChild(propValue);

        propsList.appendChild(propItem);
      });

    propsSection.appendChild(propsList);
    panel.appendChild(propsSection);

    panel.classList.add("visible");
  }

  createResourceDetailsPanel() {
    const panel = document.createElement("div");
    panel.id = "resource-details-panel";
    panel.className = "resource-details-panel";
    document.body.appendChild(panel);
    return panel;
  }

  // ─── Tooltip ───────────────────────────────────────────────────────

  showTooltip(event, resource, owner) {
    const rows = [];
    if (resource.provider) {
      rows.push({ label: "Provider", value: resource.provider });
    }
    if (resource.region) {
      rows.push({ label: "Region", value: resource.region });
    }
    if (resource.state) {
      rows.push({ label: "State", value: resource.state });
    }
    this.renderTooltip(event, {
      title: resource.name,
      subtitle: resource.type,
      rows,
    });
    this._tooltipOwner = owner || null;
  }

  /** Tooltip of an edge: its endpoints, its type and the connection metadata. */
  showEdgeTooltip(event, connection, owner) {
    const sourceId = connection.source_id || connection.source;
    const targetId = connection.target_id || connection.target;
    const source = this._resourceMap.get(sourceId);
    const target = this._resourceMap.get(targetId);
    this.renderTooltip(event, {
      title: `${source ? source.name : sourceId} → ${target ? target.name : targetId}`,
      subtitle: connection.type || "default",
      rows: buildEdgeTooltipRows(connection),
    });
    this._tooltipOwner = owner || null;
  }

  /** Render a "title + rows" payload into the tooltip element. */
  renderTooltip(event, payload) {
    const tooltip = document.getElementById("tooltip");

    // Build tooltip content safely using DOM API (no innerHTML with user data)
    while (tooltip.firstChild) tooltip.removeChild(tooltip.firstChild);

    const strong = document.createElement("strong");
    strong.textContent = payload.title;
    tooltip.appendChild(strong);

    if (payload.subtitle) {
      tooltip.appendChild(document.createElement("br"));

      const subtitleSpan = document.createElement("span");
      subtitleSpan.style.color = "rgba(245,245,247,0.6)";
      subtitleSpan.textContent = payload.subtitle;
      tooltip.appendChild(subtitleSpan);
    }

    (payload.rows || []).forEach((row) => {
      tooltip.appendChild(document.createElement("br"));

      const labelSpan = document.createElement("span");
      labelSpan.style.color = "rgba(245,245,247,0.6)";
      labelSpan.textContent = `${row.label}: `;
      tooltip.appendChild(labelSpan);

      tooltip.appendChild(document.createTextNode(String(row.value)));
    });

    tooltip.style.left = event.pageX + 10 + "px";
    tooltip.style.top = event.pageY + 10 + "px";
    tooltip.classList.add("show");
  }

  hideTooltip(owner) {
    // Moving the pointer from one element to another can deliver the leave
    // event after the enter event: only the owner of the visible tooltip may
    // hide it, otherwise the new tooltip would disappear right away.
    if (owner && this._tooltipOwner && this._tooltipOwner !== owner) return;
    this._tooltipOwner = null;
    document.getElementById("tooltip").classList.remove("show");
  }

  // ─── Export ─────────────────────────────────────────────────────────

  _createExportSvg() {
    const clone = this.svg.cloneNode(true);
    const exportBackground = getComputedStyle(document.documentElement)
      .getPropertyValue("--sm-bg")
      .trim();

    clone.setAttribute("xmlns", "http://www.w3.org/2000/svg");
    clone.setAttribute("xmlns:xlink", "http://www.w3.org/1999/xlink");
    clone.setAttribute("width", this.width);
    clone.setAttribute("height", this.height);

    const bgRect = svgEl("rect", {
      width: "100%",
      height: "100%",
      fill: exportBackground,
    });
    clone.insertBefore(bgRect, clone.firstChild);

    const propsToInline = [
      "fill",
      "fill-opacity",
      "stroke",
      "stroke-width",
      "stroke-opacity",
      "stroke-dasharray",
      "stroke-linecap",
      "stroke-linejoin",
      "opacity",
      "font-family",
      "font-size",
      "font-weight",
      "text-anchor",
      "dominant-baseline",
      "visibility",
      "display",
    ];

    const origElements = this.svg.querySelectorAll("*");
    const cloneElements = clone.querySelectorAll("*");

    for (let i = 0; i < origElements.length && i < cloneElements.length; i++) {
      const orig = origElements[i];
      const cloned = cloneElements[i];
      const computed = window.getComputedStyle(orig);

      for (const prop of propsToInline) {
        const val = computed.getPropertyValue(prop);
        if (val && val !== "" && val !== "none" && val !== "normal") {
          if (!cloned.getAttribute(prop)) {
            cloned.setAttribute(prop, val);
          }
        }
      }

      if (cloned.hasAttribute("style")) {
        let inlineStyle = cloned.getAttribute("style");
        if (inlineStyle.includes("var(")) {
          inlineStyle = inlineStyle.replace(/var\(--[^)]+\)/g, (match) => {
            const varName = match.slice(4, -1).trim();
            return (
              getComputedStyle(document.documentElement)
                .getPropertyValue(varName)
                .trim() || match
            );
          });
          cloned.setAttribute("style", inlineStyle);
        }
      }
    }

    clone.querySelectorAll("animateMotion").forEach((el) => el.remove());
    clone.querySelectorAll(".flow-dot").forEach((el) => el.remove());

    return clone;
  }

  exportToPNG() {
    const exportSvg = this._createExportSvg();
    const exportBackground = getComputedStyle(document.documentElement)
      .getPropertyValue("--sm-bg")
      .trim();
    const svgData = new XMLSerializer().serializeToString(exportSvg);
    const svgBlob = new Blob([svgData], {
      type: "image/svg+xml;charset=utf-8",
    });
    const url = URL.createObjectURL(svgBlob);

    const canvas = document.createElement("canvas");
    const ctx = canvas.getContext("2d");
    canvas.width = this.width * EXPORT_SCALE;
    canvas.height = this.height * EXPORT_SCALE;

    const img = new Image();
    img.onload = () => {
      ctx.fillStyle = exportBackground;
      ctx.fillRect(0, 0, canvas.width, canvas.height);
      ctx.drawImage(img, 0, 0, canvas.width, canvas.height);

      const link = document.createElement("a");
      link.download = "architecture-diagram.png";
      link.href = canvas.toDataURL("image/png");
      link.click();

      URL.revokeObjectURL(url);
    };
    img.onerror = (err) => {
      console.error("PNG export failed:", err);
      URL.revokeObjectURL(url);
    };
    img.src = url;
  }

  exportToSVG() {
    const exportSvg = this._createExportSvg();
    const svgData = new XMLSerializer().serializeToString(exportSvg);
    const svgBlob = new Blob([svgData], {
      type: "image/svg+xml;charset=utf-8",
    });
    const url = URL.createObjectURL(svgBlob);

    const link = document.createElement("a");
    link.download = "architecture-diagram.svg";
    link.href = url;

    // Capture the URL before triggering the download, then revoke async
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  exportToCSV() {
    const headers = [
      "id", "name", "type", "provider", "region", "account", "state", "source",
      "in_state_file", "created_at", "updated_at", "icon_url", "x", "y", "hidden", "tags", "properties",
      "incoming_connections", "outgoing_connections",
    ];
    const rows = this.resources.map((resource) => {
      const incoming = [];
      const outgoing = [];
      this.connections.forEach((connection) => {
        const sourceId = connection.source_id || connection.source;
        const targetId = connection.target_id || connection.target;
        const summary = {
          ...connection,
          connected_resource_id: sourceId === resource.id ? targetId : sourceId,
        };
        if (targetId === resource.id) incoming.push(summary);
        if (sourceId === resource.id) outgoing.push(summary);
      });
      return [
        resource.id, resource.name, resource.type, resource.provider, resource.region,
        resource.account, resource.state, resource.source, resource.in_state_file !== false, resource.created_at,
        resource.updated_at, resource.icon_url, resource.x, resource.y,
        Boolean(resource.hidden), resource.tags || {}, resource.properties || {},
        incoming, outgoing,
      ];
    });
    const escapeCSV = (value) => {
      const text = typeof value === "object" ? JSON.stringify(value) : String(value ?? "");
      return `"${text.replace(/"/g, '""')}"`;
    };
    const csv = [headers, ...rows].map((row) => row.map(escapeCSV).join(",")).join("\r\n");
    const url = URL.createObjectURL(new Blob(["\uFEFF", csv], { type: "text/csv;charset=utf-8" }));
    const link = document.createElement("a");
    link.download = "architecture-resources.csv";
    link.href = url;
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  exportToMermaid() {
    const mermaid = buildMermaid(this.filteredResources, this.connections);
    downloadTextFile(
      "architecture-diagram.mmd",
      mermaid,
      "text/plain;charset=utf-8",
    );
  }

  exportToDOT() {
    const dot = buildDot(this.filteredResources, this.connections);
    downloadTextFile("architecture-diagram.dot", dot, "text/plain;charset=utf-8");
  }

  // ─── Utility ───────────────────────────────────────────────────────

  showError(message) {
    const loadingOverlay = document.getElementById("loading-overlay");
    // Build error display safely using DOM API
    while (loadingOverlay.firstChild)
      loadingOverlay.removeChild(loadingOverlay.firstChild);

    const card = document.createElement("div");
    card.className = "loading-card";

    const icon = document.createElement("div");
    icon.style.color = "var(--sm-danger)";
    icon.style.fontSize = "24px";
    icon.textContent = "!";
    card.appendChild(icon);

    const text = document.createElement("div");
    text.className = "loading-text";
    text.style.color = "var(--sm-danger)";
    text.textContent = message;
    card.appendChild(text);

    loadingOverlay.appendChild(card);
  }
}

// Alias for backward compatibility (HTML references EnhancedDiagram)
const EnhancedDiagram = EnhancedDiagramViewer;

// Expose the pure layout helpers so the layout maths can be driven from the
// console or from a test harness without instantiating the viewer.
EnhancedDiagramViewer.computeDiagramLayout = computeDiagramLayout;
EnhancedDiagramViewer.classifyResource = classifyResource;
EnhancedDiagramViewer.computeFlowRanks = computeFlowRanks;
EnhancedDiagramViewer.orderResourcesByBarycenter = orderResourcesByBarycenter;
EnhancedDiagramViewer.buildMermaid = buildMermaid;
EnhancedDiagramViewer.buildDot = buildDot;
EnhancedDiagramViewer.buildEdgeTooltipRows = buildEdgeTooltipRows;
EnhancedDiagramViewer.escapeMermaidLabel = escapeMermaidLabel;
