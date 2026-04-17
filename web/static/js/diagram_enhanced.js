// Enhanced Cloud Architecture Visualizer - Modern StackMap-inspired UI
"use strict";

// ─── Constants ─────────────────────────────────────────────────────────────────

const ZOOM_MIN = 0.1;
const ZOOM_MAX = 5;
const ZOOM_FACTOR_IN = 1.1;
const ZOOM_FACTOR_OUT = 0.9;

const NODE_RADIUS = 22;
const GLOW_RING_RADIUS = 28;
const ICON_OFFSET = 14; // half of icon dimension (28/2)
const ICON_SIZE = 28;

const LABEL_Y_OFFSET = 40;
const TYPE_LABEL_Y_OFFSET = 54;
const LABEL_MAX_CHARS = 18;
const TYPE_LABEL_MAX_CHARS = 22;
const TYPE_LABEL_FONT_SIZE = "9";
const TYPE_LABEL_COLOR = "rgba(245, 245, 247, 0.3)";
const TYPE_LABEL_FONT = "'JetBrains Mono', monospace";

const LAYOUT_PADDING_X = 40;
const LAYOUT_PADDING_Y = 50;
const LAYOUT_GAP = 60;
const SUBGROUP_PADDING_X = 20;
const SUBGROUP_PADDING_Y = 36;
const SUBGROUP_GAP = 20;
const NODE_SPACING_X = 160;
const NODE_SPACING_Y = 110;
const NODES_PER_ROW = 5;
const LAYOUT_MIN_CANVAS_WIDTH = 1400;
const LAYOUT_START_Y = 80;

const GRID_SPACING = 180;
const GRID_MIN_COLS = 4;
const GRID_START_X = 100;
const GRID_START_Y = 150;
const GRID_ROW_HEIGHT = 150;

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

const LAYER_BOX_PAD = 30;
const LAYER_BOX_EXTRA_X = 50;
const LAYER_BOX_EXTRA_Y = 30;
const LAYER_BOX_MIN_W = 200;
const LAYER_BOX_MIN_H = 120;
const SUBGROUP_BOX_PAD = 18;
const SUBGROUP_BOX_EXTRA_X = 40;
const SUBGROUP_BOX_EXTRA_Y = 20;
const SUBGROUP_BOX_MIN_W = 140;
const SUBGROUP_BOX_MIN_H = 80;

const CONN_CLOSE_DIST = 200;
const CONN_MED_DIST = 500;
const CONN_CURVE_FACTOR = 0.2;
const CONN_ELBOW_RADIUS = 16;

const EXPORT_SCALE = 2;
const EXPORT_BG_COLOR = "#0a0a0f";

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
    this.showLabels = true;
    this.showMinimap = true;
    this.showFlowAnimations = true;
    this.presentationMode = false;
    this.sidebarCollapsed = false;

    this.resources = [];
    this.connections = [];
    this.filteredResources = [];

    // Fast lookup map: resource id -> resource object (built after data load)
    this._resourceMap = new Map();

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

    // AWS Service Groups (organized by actual AWS services)
    this.layers = {
      ec2: { name: "EC2", color: "#FF9900", resources: [], subgroups: {} },
      ecs: { name: "ECS", color: "#FF9900", resources: [], subgroups: {} },
      ecr: { name: "ECR", color: "#FF9900", resources: [], subgroups: {} },
      lambda: {
        name: "Lambda",
        color: "#FF9900",
        resources: [],
        subgroups: {},
      },
      apigateway: {
        name: "API Gateway",
        color: "#FF9900",
        resources: [],
        subgroups: {},
      },
      elb: {
        name: "Load Balancer",
        color: "#FF9900",
        resources: [],
        subgroups: {},
      },
      s3: { name: "S3", color: "#3F891A", resources: [], subgroups: {} },
      rds: { name: "RDS", color: "#3F48CC", resources: [], subgroups: {} },
      dynamodb: {
        name: "DynamoDB",
        color: "#3F48CC",
        resources: [],
        subgroups: {},
      },
      sns: { name: "SNS", color: "#B7CA9D", resources: [], subgroups: {} },
      sqs: { name: "SQS", color: "#B7CA9D", resources: [], subgroups: {} },
      logs: {
        name: "CloudWatch Logs",
        color: "#759C3E",
        resources: [],
        subgroups: {},
      },
      cloudwatch: {
        name: "CloudWatch",
        color: "#759C3E",
        resources: [],
        subgroups: {},
      },
      iam: { name: "IAM", color: "#FF9900", resources: [], subgroups: {} },
      vpc: { name: "VPC", color: "#248814", resources: [], subgroups: {} },
      route53: {
        name: "Route 53",
        color: "#F58536",
        resources: [],
        subgroups: {},
      },
      acm: {
        name: "Certificate Manager",
        color: "#759C3E",
        resources: [],
        subgroups: {},
      },
      waf: { name: "WAF", color: "#FF9900", resources: [], subgroups: {} },
      secretsmanager: {
        name: "Secrets Manager",
        color: "#FF9900",
        resources: [],
        subgroups: {},
      },
      servicediscovery: {
        name: "Service Discovery",
        color: "#FF9900",
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
        color: "#3F48CC",
        resources: [],
        subgroups: {},
      },
      "management-governance": {
        name: "Management & Governance",
        color: "#B7CA9D",
        resources: [],
        subgroups: {},
      },
      storage: {
        name: "Storage",
        color: "#3F891A",
        resources: [],
        subgroups: {},
      },
      compute: {
        name: "Compute",
        color: "#FF9900",
        resources: [],
        subgroups: {},
      },
      "networking-content-delivery": {
        name: "Networking & Content Delivery",
        color: "#248814",
        resources: [],
        subgroups: {},
      },
      "security-identity-compliance": {
        name: "Security & Compliance",
        color: "#E01515",
        resources: [],
        subgroups: {},
      },
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
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }
    const data = await response.json();

    this.resources = data.diagram.resources || [];
    this.connections = data.diagram.connections || [];
    this.filteredResources = [...this.resources];

    // Build fast lookup map
    this._resourceMap.clear();
    this.resources.forEach((r) => this._resourceMap.set(r.id, r));

    console.log(
      `Loaded ${this.resources.length} resources and ${this.connections.length} connections`,
    );

    this.updateFilters();
    this.updateResourceList();
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
    document
      .getElementById("reset-layout")
      .addEventListener("click", () => this.resetLayout());

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
  }

  setupFilters() {
    const filters = [
      "type-filter",
      "provider-filter",
      "region-filter",
      "state-filter",
    ];
    filters.forEach((filterId) => {
      document
        .getElementById(filterId)
        .addEventListener("change", () => this.applyFilters());
    });
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
      // Resource type format: "aws:<service>:<subtype>"
      const parts = resource.type.split(":");
      let layerKey = parts.length >= 2 ? parts[1] : null;
      const subgroupKey = parts.length >= 3 ? parts[2] : "default";

      // Map type segments to layer keys (handle cloudwatch log group -> logs layer)
      if (!this.layers[layerKey]) {
        // Fallback: check common aliases
        const aliases = {
          cloudfront: "ec2",
          kms: "iam",
        };
        layerKey = aliases[layerKey] || "ec2";
      }

      const layer = this.layers[layerKey];

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

  autoLayout() {
    const canvasWidth = Math.max(this.width, LAYOUT_MIN_CANVAS_WIDTH);

    const hasLayeredResources = Object.values(this.layers).some(
      (l) => l.resources.length > 0,
    );
    if (!hasLayeredResources) {
      this.simpleGridLayout();
      return;
    }

    let currentY = LAYOUT_START_Y;

    Object.entries(this.layers).forEach(([, layer]) => {
      if (layer.resources.length === 0) return;

      const subgroupKeys = Object.keys(layer.subgroups);

      // First pass: compute dimensions of each sub-group
      const sgLayouts = {};
      subgroupKeys.forEach((sgKey) => {
        const sgResources = layer.subgroups[sgKey].resources;
        const cols = Math.min(sgResources.length, NODES_PER_ROW);
        const rows = Math.ceil(sgResources.length / NODES_PER_ROW);
        const innerW = cols * NODE_SPACING_X + SUBGROUP_PADDING_X * 2;
        const innerH =
          rows * NODE_SPACING_Y + SUBGROUP_PADDING_Y + SUBGROUP_PADDING_X;
        sgLayouts[sgKey] = {
          resources: sgResources,
          cols,
          rows,
          innerW,
          innerH,
        };
      });

      // Second pass: place sub-groups side by side (wrapping if wider than canvas)
      const maxLayerWidth = canvasWidth - LAYOUT_PADDING_X * 2;
      let rowX = LAYOUT_PADDING_X;
      let rowY = currentY + LAYOUT_PADDING_Y;
      let rowMaxH = 0;

      subgroupKeys.forEach((sgKey) => {
        const sg = sgLayouts[sgKey];

        // Wrap to next row if needed
        if (
          rowX + sg.innerW > LAYOUT_PADDING_X + maxLayerWidth &&
          rowX > LAYOUT_PADDING_X
        ) {
          rowY += rowMaxH + SUBGROUP_GAP;
          rowX = LAYOUT_PADDING_X;
          rowMaxH = 0;
        }

        // Store layout position
        sg.x = rowX;
        sg.y = rowY;

        // Position each resource within the sub-group
        sg.resources.forEach((resource, idx) => {
          const col = idx % NODES_PER_ROW;
          const row = Math.floor(idx / NODES_PER_ROW);
          resource.x =
            rowX +
            SUBGROUP_PADDING_X +
            col * NODE_SPACING_X +
            NODE_SPACING_X / 2;
          resource.y =
            rowY +
            SUBGROUP_PADDING_Y +
            row * NODE_SPACING_Y +
            NODE_SPACING_Y / 2;
        });

        rowX += sg.innerW + SUBGROUP_GAP;
        rowMaxH = Math.max(rowMaxH, sg.innerH);
      });

      // Track how far down this layer extends
      const maxY = Math.max(...layer.resources.map((r) => r.y));
      currentY = maxY + LAYOUT_PADDING_Y + LAYOUT_GAP;

      // Store layout metadata for renderLayers
      layer._layout = { sgLayouts, subgroupKeys };
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
    this.svg.innerHTML = "";

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

    Object.entries(this.layers).forEach(([layerKey, layer]) => {
      if (layer.resources.length === 0) return;

      const visible = layer.resources.filter((r) => filteredSet.has(r));
      if (visible.length === 0) return;

      const minX =
        Math.min(...visible.map((r) => r.x)) -
        LAYER_BOX_PAD -
        LAYER_BOX_EXTRA_X;
      const maxX =
        Math.max(...visible.map((r) => r.x)) +
        LAYER_BOX_PAD +
        LAYER_BOX_EXTRA_X;
      const minY =
        Math.min(...visible.map((r) => r.y)) -
        LAYER_BOX_PAD -
        LAYER_BOX_EXTRA_Y;
      const maxY =
        Math.max(...visible.map((r) => r.y)) +
        LAYER_BOX_PAD +
        LAYER_BOX_EXTRA_Y;

      // Outer service box
      const serviceBox = svgEl("rect", {
        x: minX,
        y: minY,
        width: Math.max(maxX - minX, LAYER_BOX_MIN_W),
        height: Math.max(maxY - minY, LAYER_BOX_MIN_H),
        fill: `url(#gradient-${layerKey})`,
        stroke: layer.color,
        "stroke-width": "1.5",
        "stroke-opacity": "0.35",
        rx: "14",
      });
      layersGroup.appendChild(serviceBox);

      // Service label (top-left)
      const serviceLabel = svgEl("text", {
        x: minX + 16,
        y: minY + 18,
        fill: layer.color,
        "font-size": "13",
        "font-weight": "700",
        "font-family": "'Inter', sans-serif",
        opacity: "0.9",
      });
      serviceLabel.textContent = layer.name.toUpperCase();
      layersGroup.appendChild(serviceLabel);

      // Inner sub-group boxes
      if (layer.subgroups) {
        Object.entries(layer.subgroups).forEach(([sgKey, sg]) => {
          const sgVisible = sg.resources.filter((r) => filteredSet.has(r));
          if (sgVisible.length === 0) return;

          const sgMinX =
            Math.min(...sgVisible.map((r) => r.x)) -
            SUBGROUP_BOX_PAD -
            SUBGROUP_BOX_EXTRA_X;
          const sgMaxX =
            Math.max(...sgVisible.map((r) => r.x)) +
            SUBGROUP_BOX_PAD +
            SUBGROUP_BOX_EXTRA_X;
          const sgMinY =
            Math.min(...sgVisible.map((r) => r.y)) -
            SUBGROUP_BOX_PAD -
            SUBGROUP_BOX_EXTRA_Y;
          const sgMaxY =
            Math.max(...sgVisible.map((r) => r.y)) +
            SUBGROUP_BOX_PAD +
            SUBGROUP_BOX_EXTRA_Y;

          const sgBox = svgEl("rect", {
            x: sgMinX,
            y: sgMinY,
            width: Math.max(sgMaxX - sgMinX, SUBGROUP_BOX_MIN_W),
            height: Math.max(sgMaxY - sgMinY, SUBGROUP_BOX_MIN_H),
            fill: layer.color,
            "fill-opacity": "0.04",
            stroke: layer.color,
            "stroke-width": "1",
            "stroke-opacity": "0.2",
            "stroke-dasharray": "5,3",
            rx: "8",
          });
          layersGroup.appendChild(sgBox);

          // Sub-group label
          const sgLabel = svgEl("text", {
            x: sgMinX + 10,
            y: sgMinY + 14,
            fill: layer.color,
            "font-size": "9",
            "font-weight": "600",
            "font-family": TYPE_LABEL_FONT,
            opacity: "0.55",
            "letter-spacing": "0.8",
          });
          sgLabel.textContent = sgKey.toUpperCase();
          layersGroup.appendChild(sgLabel);
        });
      }
    });
  }

  renderConnections(connectionsGroup) {
    // Build a Set of filtered resource IDs for fast membership checks
    const filteredIds = new Set(this.filteredResources.map((r) => r.id));

    this.connections.forEach((connection) => {
      const sourceId = connection.source_id || connection.source;
      const targetId = connection.target_id || connection.target;

      // Use Map lookup instead of Array.find()
      const source = this._resourceMap.get(sourceId);
      const target = this._resourceMap.get(targetId);

      if (!source || !target) return;
      if (!filteredIds.has(sourceId) || !filteredIds.has(targetId)) return;
      if (source.x === target.x && source.y === target.y) return;

      const edgeType = connection.type || "default";
      const style = this.edgeStyles[edgeType] || this.edgeStyles.default;

      const dx = target.x - source.x;
      const dy = target.y - source.y;
      const dist = Math.sqrt(dx * dx + dy * dy);

      const normDx = dx / dist;
      const normDy = dy / dist;
      const sx = source.x + normDx * NODE_RADIUS;
      const sy = source.y + normDy * NODE_RADIUS;
      const tx = target.x - normDx * NODE_RADIUS;
      const ty = target.y - normDy * NODE_RADIUS;

      let pathData;

      if (dist < CONN_CLOSE_DIST) {
        pathData = `M ${sx},${sy} L ${tx},${ty}`;
      } else if (dist < CONN_MED_DIST) {
        const cx = (sx + tx) / 2 - (ty - sy) * CONN_CURVE_FACTOR;
        const cy = (sy + ty) / 2 + (tx - sx) * CONN_CURVE_FACTOR;
        pathData = `M ${sx},${sy} Q ${cx},${cy} ${tx},${ty}`;
      } else {
        const midX = (sx + tx) / 2;
        const r = CONN_ELBOW_RADIUS;
        if (Math.abs(dy) > Math.abs(dx) * 0.3) {
          const midY = (sy + ty) / 2;
          pathData = `M ${sx},${sy} L ${sx},${midY - r} Q ${sx},${midY} ${sx + Math.sign(dx) * r},${midY} L ${tx - Math.sign(dx) * r},${midY} Q ${tx},${midY} ${tx},${midY + Math.sign(dy) * r} L ${tx},${ty}`;
        } else {
          pathData = `M ${sx},${sy} L ${midX - Math.sign(dx) * r},${sy} Q ${midX},${sy} ${midX},${sy + Math.sign(dy) * r} L ${midX},${ty - Math.sign(dy) * r} Q ${midX},${ty} ${midX + Math.sign(dx) * r},${ty} L ${tx},${ty}`;
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

      // Edge label
      if (connection.description) {
        const labelX = (source.x + target.x) / 2;
        const labelY = (source.y + target.y) / 2 - 8;
        const labelText = truncateText(
          connection.description,
          CONN_LABEL_MAX_CHARS,
        );
        const textWidth =
          labelText.length * CONN_LABEL_CHAR_WIDTH + CONN_LABEL_PAD;

        const labelBg = svgEl("rect", {
          x: String(labelX - textWidth / 2),
          y: String(labelY - 8),
          width: String(textWidth),
          height: String(CONN_LABEL_HEIGHT),
          rx: "4",
          fill: "#12121a",
          "fill-opacity": "0.85",
          stroke: style.color,
          "stroke-width": "0.5",
          "stroke-opacity": "0.4",
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
      const nodeGroup = svgEl("g", {
        transform: `translate(${resource.x}, ${resource.y})`,
        "data-id": resource.id,
      });
      nodeGroup.classList.add("resource-node");

      const layerColor = this.getResourceColor(resource);

      // Outer glow ring
      const glowCircle = svgEl("circle", {
        r: String(GLOW_RING_RADIUS),
        fill: "none",
        stroke: layerColor,
        "stroke-width": "0.5",
        "stroke-opacity": "0.25",
      });
      nodeGroup.appendChild(glowCircle);

      // Node background circle
      const nodeBg = svgEl("circle", {
        r: String(NODE_RADIUS),
        fill: layerColor,
        "fill-opacity": "0.15",
        stroke: layerColor,
        "stroke-width": "1.5",
        "stroke-opacity": "0.6",
      });
      nodeGroup.appendChild(nodeBg);

      // Icon
      const iconPath =
        resource.icon_url || this.getResourceIconPath(resource.type);
      const icon = svgEl("image", {
        href: iconPath,
        x: String(-ICON_OFFSET),
        y: String(-ICON_OFFSET),
        width: String(ICON_SIZE),
        height: String(ICON_SIZE),
        preserveAspectRatio: "xMidYMid meet",
      });
      nodeGroup.appendChild(icon);

      // Labels
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
          fill: TYPE_LABEL_COLOR,
          "font-family": TYPE_LABEL_FONT,
        });
        typeLabel.textContent = truncateText(
          resource.type,
          TYPE_LABEL_MAX_CHARS,
        );
        nodeGroup.appendChild(typeLabel);
      }

      // Highlight if selected
      if (this.selectedResource && this.selectedResource.id === resource.id) {
        nodeBg.setAttribute("stroke-width", "2.5");
        nodeBg.setAttribute("stroke-opacity", "1");
        glowCircle.setAttribute("stroke-width", "1");
        glowCircle.setAttribute("stroke-opacity", "0.5");
      }

      nodeGroup.addEventListener("click", (e) => {
        e.stopPropagation();
        this.selectResource(resource);
      });
      nodeGroup.addEventListener("mouseenter", (e) =>
        this.showTooltip(e, resource),
      );
      nodeGroup.addEventListener("mouseleave", () => this.hideTooltip());

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

    const minX = Math.min(...allResources.map((r) => r.x)) - MINIMAP_PADDING;
    const maxX = Math.max(...allResources.map((r) => r.x)) + MINIMAP_PADDING;
    const minY = Math.min(...allResources.map((r) => r.y)) - MINIMAP_PADDING;
    const maxY = Math.max(...allResources.map((r) => r.y)) + MINIMAP_PADDING;

    const scaleX = MINIMAP_WIDTH / (maxX - minX);
    const scaleY = MINIMAP_HEIGHT / (maxY - minY);
    const scale = Math.min(scaleX, scaleY) * MINIMAP_SCALE_FACTOR;

    this.minimapSvg.innerHTML = "";
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
    this.minimapSvg.appendChild(viewportRect);
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
    this.renderDiagram();
    this.updateTransform();
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

    const minX =
      Math.min(...this.filteredResources.map((r) => r.x)) - FIT_VIEW_MARGIN;
    const maxX =
      Math.max(...this.filteredResources.map((r) => r.x)) + FIT_VIEW_MARGIN;
    const minY =
      Math.min(...this.filteredResources.map((r) => r.y)) - FIT_VIEW_MARGIN;
    const maxY =
      Math.max(...this.filteredResources.map((r) => r.y)) + FIT_VIEW_MARGIN;

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
  }

  // ─── Mouse Events ──────────────────────────────────────────────────

  onMouseDown(e) {
    if (e.target === this.svg) {
      this.isDragging = true;
      this.lastMouseX = e.clientX;
      this.lastMouseY = e.clientY;
      this.svg.style.cursor = "grabbing";
    }
  }

  onMouseMove(e) {
    if (this.isDragging) {
      const deltaX = e.clientX - this.lastMouseX;
      const deltaY = e.clientY - this.lastMouseY;

      this.panX += deltaX;
      this.panY += deltaY;

      this.lastMouseX = e.clientX;
      this.lastMouseY = e.clientY;

      this.updateTransform();
    }
  }

  onMouseUp() {
    this.isDragging = false;
    this.svg.style.cursor = "grab";
  }

  onWheel(e) {
    e.preventDefault();

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

    this.updateTransform();
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
    const typeFilter = document.getElementById("type-filter");
    const providerFilter = document.getElementById("provider-filter");
    const regionFilter = document.getElementById("region-filter");

    const types = [...new Set(this.resources.map((r) => r.type))].sort();
    const providers = [
      ...new Set(this.resources.map((r) => r.provider || "Unknown")),
    ].sort();
    const regions = [
      ...new Set(this.resources.map((r) => r.region || "Unknown")),
    ].sort();

    // Clear and rebuild type filter
    while (typeFilter.firstChild) typeFilter.removeChild(typeFilter.firstChild);
    const typeDefault = document.createElement("option");
    typeDefault.value = "";
    typeDefault.textContent = "All Types";
    typeFilter.appendChild(typeDefault);
    types.forEach((type) => {
      const option = document.createElement("option");
      option.value = type;
      option.textContent = type;
      typeFilter.appendChild(option);
    });

    // Clear and rebuild provider filter
    while (providerFilter.firstChild)
      providerFilter.removeChild(providerFilter.firstChild);
    const provDefault = document.createElement("option");
    provDefault.value = "";
    provDefault.textContent = "All Providers";
    providerFilter.appendChild(provDefault);
    providers.forEach((provider) => {
      const option = document.createElement("option");
      option.value = provider;
      option.textContent = provider;
      providerFilter.appendChild(option);
    });

    // Clear and rebuild region filter
    while (regionFilter.firstChild)
      regionFilter.removeChild(regionFilter.firstChild);
    const regDefault = document.createElement("option");
    regDefault.value = "";
    regDefault.textContent = "All Regions";
    regionFilter.appendChild(regDefault);
    regions.forEach((region) => {
      const option = document.createElement("option");
      option.value = region;
      option.textContent = region;
      regionFilter.appendChild(option);
    });
  }

  applyFilters() {
    const typeFilter = document.getElementById("type-filter").value;
    const providerFilter = document.getElementById("provider-filter").value;
    const regionFilter = document.getElementById("region-filter").value;
    const stateFilter = document.getElementById("state-filter").value;
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

      resourceList.appendChild(item);
    });
  }

  updateStats() {
    document.getElementById("visible-count").textContent =
      this.filteredResources.length;
  }

  // ─── Selection & Dependencies ──────────────────────────────────────

  selectResource(resource) {
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
    this.panToResource(resource);
    this.renderDiagram();
    this.updateTransform();
  }

  panToResource(resource) {
    const centerX = this.width / 2;
    const centerY = this.height / 2;

    this.panX = centerX - resource.x * this.zoom;
    this.panY = centerY - resource.y * this.zoom;

    this.updateTransform();
  }

  highlightDependencies(resource) {
    const connectedResources = this.getConnectedResources(resource);

    this.resources.forEach((r) => {
      const nodeElement = document.querySelector(`[data-id="${r.id}"]`);
      if (!nodeElement) return;

      if (r.id === resource.id) {
        nodeElement.classList.add("selected-resource");
      } else if (connectedResources.upstream.some((cr) => cr.id === r.id)) {
        nodeElement.classList.add("upstream-dependency");
      } else if (connectedResources.downstream.some((cr) => cr.id === r.id)) {
        nodeElement.classList.add("downstream-dependency");
      } else {
        nodeElement.classList.add("unrelated-resource");
      }
    });

    this.connections.forEach((conn) => {
      const pathElement = document.querySelector(
        `path[data-connection="${conn.id}"]`,
      );
      if (!pathElement) return;

      if (conn.source_id === resource.id || conn.target_id === resource.id) {
        pathElement.classList.add("highlighted-connection");
      } else {
        pathElement.classList.add("dimmed-connection");
      }
    });
  }

  clearDependencyHighlighting() {
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

  getConnectedResources(resource) {
    const upstream = [];
    const downstream = [];

    this.connections.forEach((conn) => {
      if (conn.target_id === resource.id) {
        const sourceResource = this._resourceMap.get(conn.source_id);
        if (sourceResource) upstream.push(sourceResource);
      }
      if (conn.source_id === resource.id) {
        const targetResource = this._resourceMap.get(conn.target_id);
        if (targetResource) downstream.push(targetResource);
      }
    });

    return { upstream, downstream };
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

    const connectedResources = this.getConnectedResources(resource);

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
      this.renderDiagram();
      this.updateTransform();
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

    panel.appendChild(infoDiv);

    // ── Dependencies section ──
    const depsSection = document.createElement("div");
    depsSection.className = "dependencies-section";

    const depsH4 = document.createElement("h4");
    depsH4.textContent = "Dependencies";
    depsSection.appendChild(depsH4);

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
      createDepGroup("Upstream", connectedResources.upstream, "upstream"),
    );
    depsSection.appendChild(
      createDepGroup("Downstream", connectedResources.downstream, "downstream"),
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

  showTooltip(event, resource) {
    const tooltip = document.getElementById("tooltip");

    // Build tooltip content safely using DOM API (no innerHTML with user data)
    while (tooltip.firstChild) tooltip.removeChild(tooltip.firstChild);

    const strong = document.createElement("strong");
    strong.textContent = resource.name;
    tooltip.appendChild(strong);
    tooltip.appendChild(document.createElement("br"));

    const typeSpan = document.createElement("span");
    typeSpan.style.color = "rgba(245,245,247,0.6)";
    typeSpan.textContent = resource.type;
    tooltip.appendChild(typeSpan);
    tooltip.appendChild(document.createElement("br"));

    if (resource.provider) {
      tooltip.appendChild(
        document.createTextNode(`Provider: ${resource.provider}`),
      );
      tooltip.appendChild(document.createElement("br"));
    }
    if (resource.region) {
      tooltip.appendChild(
        document.createTextNode(`Region: ${resource.region}`),
      );
      tooltip.appendChild(document.createElement("br"));
    }
    if (resource.state) {
      tooltip.appendChild(document.createTextNode(`State: ${resource.state}`));
    }

    tooltip.style.left = event.pageX + 10 + "px";
    tooltip.style.top = event.pageY + 10 + "px";
    tooltip.classList.add("show");
  }

  hideTooltip() {
    document.getElementById("tooltip").classList.remove("show");
  }

  // ─── Export ─────────────────────────────────────────────────────────

  _createExportSvg() {
    const clone = this.svg.cloneNode(true);

    clone.setAttribute("xmlns", "http://www.w3.org/2000/svg");
    clone.setAttribute("xmlns:xlink", "http://www.w3.org/1999/xlink");
    clone.setAttribute("width", this.width);
    clone.setAttribute("height", this.height);

    const bgRect = svgEl("rect", {
      width: "100%",
      height: "100%",
      fill: EXPORT_BG_COLOR,
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
      ctx.fillStyle = EXPORT_BG_COLOR;
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
