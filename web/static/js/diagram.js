// Cloud Architecture Visualizer - Interactive Diagram (Basic View)
'use strict';

// ─── Constants ─────────────────────────────────────────────────────────────────

const NODE_RADIUS = 26;
const ICON_SIZE = 48;
const ICON_SCALE = 0.8;
const LABEL_FONT_SIZE = 11;
const TYPE_FONT_SIZE = 8;
const LABEL_MAX_CHARS = 12;
const CONNECTION_LABEL_FONT_SIZE = 10;

const ZOOM_MIN = 0.1;
const ZOOM_MAX = 3;
const ZOOM_STEP = 1.2;

const LAYOUT_ITERATIONS = 50;
const LAYOUT_REPULSION = 1000;
const LAYOUT_ATTRACTION = 0.1;
const LAYOUT_IDEAL_DISTANCE = 100;
const LAYOUT_DAMPING = 0.01;
const LAYOUT_MARGIN = 50;

const RESIZE_DEBOUNCE_MS = 150;

const CONNECTION_COLORS = Object.freeze({
    networking: '#3498db',
    access:     '#e74c3c',
    data:       '#2ecc71',
    trigger:    '#f39c12',
    dependency: '#9b59b6',
    reference:  '#95a5a6',
});
const DEFAULT_CONNECTION_COLOR = '#95a5a6';

// ─── Helpers ───────────────────────────────────────────────────────────────────

/** Escape HTML entities to prevent XSS when inserting user-provided strings. */
function escapeHTML(str) {
    const div = document.createElement('div');
    div.appendChild(document.createTextNode(str));
    return div.innerHTML;
}

/** Create an SVG element in the SVG namespace. */
function svgEl(tag, attrs) {
    const el = document.createElementNS('http://www.w3.org/2000/svg', tag);
    if (attrs) {
        for (const [k, v] of Object.entries(attrs)) {
            el.setAttribute(k, String(v));
        }
    }
    return el;
}

function truncateText(text, maxLength) {
    return text.length > maxLength ? text.substring(0, maxLength) + '...' : text;
}

function formatResourceType(type) {
    return type.split(':').pop()
        .replace(/([a-z])([A-Z])/g, '$1 $2')
        .replace(/^\w/, c => c.toUpperCase());
}

// ─── Inline Icon SVG markup per resource type ──────────────────────────────────

const ICON_MAP = Object.freeze({
    'aws:ec2:instance': `
        <rect fill="#FF9900" width="48" height="48" rx="4"/>
        <g fill="white">
            <rect x="8" y="12" width="32" height="20" rx="2" fill="none" stroke="white" stroke-width="2"/>
            <rect x="12" y="16" width="6" height="4"/>
            <rect x="20" y="16" width="6" height="4"/>
            <rect x="28" y="16" width="6" height="4"/>
            <rect x="12" y="22" width="6" height="4"/>
            <rect x="20" y="22" width="6" height="4"/>
            <rect x="28" y="22" width="6" height="4"/>
            <circle cx="38" cy="14" r="2"/>
        </g>
        <text x="24" y="40" text-anchor="middle" font-family="Arial" font-size="8" fill="white">EC2</text>`,
    'aws:s3:bucket': `
        <rect fill="#569A31" width="48" height="48" rx="4"/>
        <g fill="white">
            <ellipse cx="24" cy="12" rx="14" ry="4"/>
            <rect x="10" y="12" width="28" height="20" fill="none" stroke="white" stroke-width="2"/>
            <ellipse cx="24" cy="32" rx="14" ry="4"/>
            <line x1="10" y1="18" x2="38" y2="18" stroke="white" stroke-width="1"/>
            <line x1="10" y1="24" x2="38" y2="24" stroke="white" stroke-width="1"/>
        </g>
        <text x="24" y="42" text-anchor="middle" font-family="Arial" font-size="8" fill="white">S3</text>`,
    'aws:rds:instance': `
        <rect fill="#3F48CC" width="48" height="48" rx="4"/>
        <g fill="white">
            <rect x="8" y="10" width="32" height="24" rx="4" fill="none" stroke="white" stroke-width="2"/>
            <rect x="12" y="14" width="24" height="2"/>
            <rect x="12" y="18" width="24" height="2"/>
            <rect x="12" y="22" width="24" height="2"/>
            <rect x="12" y="26" width="24" height="2"/>
            <circle cx="18" cy="30" r="1"/>
            <circle cx="22" cy="30" r="1"/>
            <circle cx="26" cy="30" r="1"/>
            <circle cx="30" cy="30" r="1"/>
        </g>
        <text x="24" y="42" text-anchor="middle" font-family="Arial" font-size="8" fill="white">RDS</text>`,
    'aws:lambda:function': `
        <rect fill="#FF9900" width="48" height="48" rx="4"/>
        <g fill="white">
            <path d="M12 34 L20 14 L24 14 L30 24 L26 24 L22 16 L16 34 Z"/>
            <path d="M26 24 L30 24 L36 34 L32 34 Z"/>
        </g>
        <text x="24" y="42" text-anchor="middle" font-family="Arial" font-size="7" fill="white">Lambda</text>`,
    'aws:ec2:vpc': `
        <rect fill="#248814" width="48" height="48" rx="4"/>
        <g stroke="white" stroke-width="2" fill="none">
            <rect x="6" y="10" width="36" height="24" rx="4"/>
            <rect x="10" y="14" width="12" height="8" rx="2"/>
            <rect x="26" y="14" width="12" height="8" rx="2"/>
            <rect x="10" y="26" width="12" height="4" rx="1"/>
            <rect x="26" y="26" width="12" height="4" rx="1"/>
        </g>
        <text x="24" y="42" text-anchor="middle" font-family="Arial" font-size="8" fill="white">VPC</text>`,
    'aws:ec2:subnet': `
        <rect fill="#248814" width="48" height="48" rx="4"/>
        <g stroke="white" stroke-width="1.5" fill="none">
            <rect x="8" y="12" width="32" height="20" rx="3"/>
            <rect x="12" y="16" width="10" height="6" rx="1"/>
            <rect x="26" y="16" width="10" height="6" rx="1"/>
            <rect x="12" y="24" width="24" height="4" rx="1"/>
        </g>
        <text x="24" y="40" text-anchor="middle" font-family="Arial" font-size="7" fill="white">Subnet</text>`,
    'aws:ec2:securitygroup': `
        <rect fill="#FF4B4B" width="48" height="48" rx="4"/>
        <g fill="white">
            <path d="M24 8 L32 12 L32 24 C32 30 28 34 24 36 C20 34 16 30 16 24 L16 12 Z"/>
            <circle cx="24" cy="22" r="3" fill="none" stroke="white" stroke-width="2"/>
            <rect x="22" y="24" width="4" height="6"/>
        </g>
        <text x="24" y="44" text-anchor="middle" font-family="Arial" font-size="6" fill="white">Security</text>`,
    'aws:elb:loadbalancer': `
        <rect fill="#B0084D" width="48" height="48" rx="4"/>
        <g fill="white">
            <circle cx="24" cy="16" r="6"/>
            <circle cx="14" cy="30" r="4"/>
            <circle cx="34" cy="30" r="4"/>
            <line x1="20" y1="20" x2="16" y2="26" stroke="white" stroke-width="2"/>
            <line x1="28" y1="20" x2="32" y2="26" stroke="white" stroke-width="2"/>
            <path d="M21 16 L24 14 L27 16" stroke="white" stroke-width="2" fill="none"/>
        </g>
        <text x="24" y="42" text-anchor="middle" font-family="Arial" font-size="8" fill="white">ELB</text>`,
});

const DEFAULT_ICON = `
    <rect fill="#95a5a6" width="48" height="48" rx="4"/>
    <g fill="white">
        <circle cx="24" cy="20" r="8"/>
        <rect x="16" y="28" width="16" height="8" rx="2"/>
    </g>
    <text x="24" y="42" text-anchor="middle" font-family="Arial" font-size="7" fill="white">Unknown</text>`;

// ─── DiagramViewer ─────────────────────────────────────────────────────────────

class DiagramViewer {
    constructor() {
        this.svg = null;
        this.width = 0;
        this.height = 0;
        this.zoom = 1;
        this.panX = 0;
        this.panY = 0;
        this.isDragging = false;
        this.lastMouseX = 0;
        this.lastMouseY = 0;
        this.selectedResource = null;
        this.showLabels = true;

        this.resources = [];
        this.connections = [];
        this.filteredResources = [];

        // Fast lookup: resource id -> resource object.
        this._resourceMap = new Map();

        this._resizeTimer = null;

        this.init();
    }

    async init() {
        await this.loadDiagram();
        this.setupCanvas();
        this.setupControls();
        this.setupFilters();
        this.setupSearch();
        this.renderDiagram();

        document.getElementById('loading').classList.add('hidden');
        document.getElementById('diagram-canvas').classList.remove('hidden');
    }

    async loadDiagram() {
        try {
            const response = await fetch('/api/diagram');
            if (!response.ok) {
                throw new Error(`Server returned ${response.status}: ${response.statusText}`);
            }
            const data = await response.json();

            this.resources = data.diagram.resources || [];
            this.connections = data.diagram.connections || [];
            this.filteredResources = [...this.resources];

            // Build fast lookup map.
            this._resourceMap.clear();
            for (const r of this.resources) {
                this._resourceMap.set(r.id, r);
            }

            this.autoLayout();
            this.updateFilters();
            this.updateResourceList();
        } catch (error) {
            console.error('Error loading diagram:', error);
            this.showError('Failed to load diagram data');
        }
    }

    // ─── Canvas setup ──────────────────────────────────────────────────

    setupCanvas() {
        const container = document.querySelector('.diagram-container');
        this.width = container.clientWidth;
        this.height = container.clientHeight;

        this.svg = document.getElementById('diagram-canvas');
        this.svg.setAttribute('width', this.width);
        this.svg.setAttribute('height', this.height);
        this.svg.setAttribute('viewBox', `0 0 ${this.width} ${this.height}`);

        this.svg.addEventListener('mousedown', this.onMouseDown.bind(this));
        this.svg.addEventListener('mousemove', this.onMouseMove.bind(this));
        this.svg.addEventListener('mouseup', this.onMouseUp.bind(this));
        this.svg.addEventListener('wheel', this.onWheel.bind(this), { passive: false });

        window.addEventListener('resize', this.onResize.bind(this));
    }

    setupControls() {
        document.getElementById('zoom-in').addEventListener('click', () => this.zoomIn());
        document.getElementById('zoom-out').addEventListener('click', () => this.zoomOut());
        document.getElementById('fit-view').addEventListener('click', () => this.fitToView());
        document.getElementById('reset-layout').addEventListener('click', () => this.resetLayout());
        document.getElementById('toggle-labels').addEventListener('click', () => this.toggleLabels());
        document.getElementById('export-png').addEventListener('click', () => this.exportToPNG());
        document.getElementById('export-svg').addEventListener('click', () => this.exportToSVG());
    }

    setupFilters() {
        const ids = ['type-filter', 'provider-filter', 'region-filter', 'state-filter'];
        ids.forEach(id => {
            document.getElementById(id).addEventListener('change', () => this.applyFilters());
        });
    }

    setupSearch() {
        const searchInput = document.getElementById('search-input');
        searchInput.addEventListener('input', (e) => {
            this.searchResources(e.target.value);
        });
    }

    // ─── Layout ────────────────────────────────────────────────────────

    autoLayout() {
        const centerX = this.width / 2 || 600;
        const centerY = this.height / 2 || 400;
        const radius = Math.min(centerX, centerY) * 0.67;
        const count = this.resources.length;

        // Place resources that have no position in a circle.
        this.resources.forEach((resource, index) => {
            if (resource.x === 0 && resource.y === 0) {
                const angle = (index / count) * 2 * Math.PI;
                const r = radius + (Math.random() - 0.5) * radius * 0.5;
                resource.x = centerX + Math.cos(angle) * r;
                resource.y = centerY + Math.sin(angle) * r;
            }
        });

        // Pre-build a connection adjacency list for O(1) lookups during layout.
        const adjacency = new Map(); // resourceId -> [{other: Resource, ...}]
        for (const conn of this.connections) {
            const src = this._resourceMap.get(conn.source_id);
            const tgt = this._resourceMap.get(conn.target_id);
            if (!src || !tgt) continue;
            if (!adjacency.has(conn.source_id)) adjacency.set(conn.source_id, []);
            if (!adjacency.has(conn.target_id)) adjacency.set(conn.target_id, []);
            adjacency.get(conn.source_id).push(tgt);
            adjacency.get(conn.target_id).push(src);
        }

        const resources = this.resources;
        const w = this.width || 1200;
        const h = this.height || 800;

        for (let iter = 0; iter < LAYOUT_ITERATIONS; iter++) {
            for (const resource of resources) {
                let fx = 0;
                let fy = 0;

                // Repulsion from other nodes.
                for (const other of resources) {
                    if (other === resource) continue;
                    const dx = resource.x - other.x;
                    const dy = resource.y - other.y;
                    const distSq = dx * dx + dy * dy;
                    const dist = Math.sqrt(distSq);
                    if (dist > 0) {
                        const force = LAYOUT_REPULSION / distSq;
                        fx += (dx / dist) * force;
                        fy += (dy / dist) * force;
                    }
                }

                // Attraction to connected nodes.
                const neighbors = adjacency.get(resource.id);
                if (neighbors) {
                    for (const other of neighbors) {
                        const dx = other.x - resource.x;
                        const dy = other.y - resource.y;
                        const dist = Math.sqrt(dx * dx + dy * dy);
                        if (dist > 0) {
                            const force = (dist - LAYOUT_IDEAL_DISTANCE) * LAYOUT_ATTRACTION;
                            fx += (dx / dist) * force;
                            fy += (dy / dist) * force;
                        }
                    }
                }

                resource.x += fx * LAYOUT_DAMPING;
                resource.y += fy * LAYOUT_DAMPING;

                // Keep within bounds.
                resource.x = Math.max(LAYOUT_MARGIN, Math.min(w - LAYOUT_MARGIN, resource.x));
                resource.y = Math.max(LAYOUT_MARGIN, Math.min(h - LAYOUT_MARGIN, resource.y));
            }
        }
    }

    // ─── Filters & search ──────────────────────────────────────────────

    updateFilters() {
        const typeFilter = document.getElementById('type-filter');
        const providerFilter = document.getElementById('provider-filter');
        const regionFilter = document.getElementById('region-filter');

        const types = [...new Set(this.resources.map(r => r.type))].sort();
        const providers = [...new Set(this.resources.map(r => r.provider))].sort();
        const regions = [...new Set(this.resources.map(r => r.region))].filter(Boolean).sort();

        typeFilter.innerHTML = '<option value="">All Types</option>';
        for (const type of types) {
            const opt = document.createElement('option');
            opt.value = type;
            opt.textContent = formatResourceType(type);
            typeFilter.appendChild(opt);
        }

        providerFilter.innerHTML = '<option value="">All Providers</option>';
        for (const provider of providers) {
            const opt = document.createElement('option');
            opt.value = provider;
            opt.textContent = provider.toUpperCase();
            providerFilter.appendChild(opt);
        }

        regionFilter.innerHTML = '<option value="">All Regions</option>';
        for (const region of regions) {
            const opt = document.createElement('option');
            opt.value = region;
            opt.textContent = region;
            regionFilter.appendChild(opt);
        }
    }

    applyFilters() {
        const typeVal = document.getElementById('type-filter').value;
        const providerVal = document.getElementById('provider-filter').value;
        const regionVal = document.getElementById('region-filter').value;
        const stateVal = document.getElementById('state-filter').value;

        this.filteredResources = this.resources.filter(r => {
            if (typeVal && r.type !== typeVal) return false;
            if (providerVal && r.provider !== providerVal) return false;
            if (regionVal && r.region !== regionVal) return false;
            if (stateVal && r.state !== stateVal) return false;
            return true;
        });

        this.updateResourceList();
        this.renderDiagram();
        this.updateStats();
    }

    searchResources(query) {
        if (!query) {
            this.applyFilters();
            return;
        }

        const lower = query.toLowerCase();
        this.filteredResources = this.resources.filter(r =>
            r.name.toLowerCase().includes(lower) ||
            r.id.toLowerCase().includes(lower) ||
            r.type.toLowerCase().includes(lower)
        );

        this.updateResourceList();
        this.renderDiagram();
        this.updateStats();
    }

    // ─── Resource list (sidebar) ───────────────────────────────────────

    updateResourceList() {
        const list = document.getElementById('resource-list');
        list.innerHTML = '';

        for (const resource of this.filteredResources) {
            const item = document.createElement('div');
            item.className = 'resource-item';
            item.dataset.resourceId = resource.id;

            const name = document.createElement('div');
            name.className = 'resource-name';
            name.textContent = resource.name || resource.id;

            const type = document.createElement('div');
            type.className = 'resource-type';
            type.textContent = formatResourceType(resource.type);

            const tags = document.createElement('div');
            tags.className = 'resource-tags';
            for (const [key, value] of Object.entries(resource.tags || {})) {
                const tag = document.createElement('span');
                tag.className = 'tag';
                tag.textContent = `${key}: ${value}`;
                tags.appendChild(tag);
            }

            item.appendChild(name);
            item.appendChild(type);
            item.appendChild(tags);

            item.addEventListener('click', () => this.selectResource(resource.id));
            list.appendChild(item);
        }
    }

    selectResource(resourceId) {
        document.querySelectorAll('.resource-item').forEach(item => {
            item.classList.remove('selected');
        });

        const selectedItem = document.querySelector(`[data-resource-id="${resourceId}"]`);
        if (selectedItem) {
            selectedItem.classList.add('selected');
        }

        this.selectedResource = resourceId;
        this.renderDiagram();
    }

    // ─── Rendering ─────────────────────────────────────────────────────

    renderDiagram() {
        this.svg.innerHTML = '';

        const mainGroup = svgEl('g', {
            transform: `translate(${this.panX}, ${this.panY}) scale(${this.zoom})`
        });
        mainGroup.classList.add('main-group');
        this.svg.appendChild(mainGroup);

        // Connections first (behind nodes).
        this.renderConnections(mainGroup);
        // Nodes on top.
        this.renderResources(mainGroup);
    }

    renderConnections(container) {
        // Build a Set of filtered IDs for fast membership checks.
        const filteredIds = new Set(this.filteredResources.map(r => r.id));

        for (const connection of this.connections) {
            if (connection.hidden) continue;

            const source = this._resourceMap.get(connection.source_id);
            const target = this._resourceMap.get(connection.target_id);
            if (!source || !target) continue;
            if (!filteredIds.has(source.id) || !filteredIds.has(target.id)) continue;

            const line = svgEl('line', {
                x1: source.x,
                y1: source.y,
                x2: target.x,
                y2: target.y,
                class: `connection-line ${connection.type}`,
                stroke: connection.color || (CONNECTION_COLORS[connection.type] || DEFAULT_CONNECTION_COLOR),
                'stroke-width': 2,
                opacity: 0.7,
            });

            if (connection.style === 'dashed') {
                line.setAttribute('stroke-dasharray', '5,5');
            }

            container.appendChild(line);

            // Connection label (rendered after the line so it appears on top).
            if (connection.description) {
                const midX = (source.x + target.x) / 2;
                const midY = (source.y + target.y) / 2;

                // Approximate text width for background sizing.
                const labelStr = connection.type;
                const approxWidth = labelStr.length * 6 + 8;

                const bg = svgEl('rect', {
                    x: midX - approxWidth / 2,
                    y: midY - 12,
                    width: approxWidth,
                    height: 14,
                    fill: 'white',
                    opacity: 0.8,
                    rx: 2,
                });

                const label = svgEl('text', {
                    x: midX,
                    y: midY - 3,
                    'text-anchor': 'middle',
                    'font-size': CONNECTION_LABEL_FONT_SIZE,
                    fill: '#666',
                    opacity: 0.8,
                });
                label.textContent = labelStr;

                container.appendChild(bg);
                container.appendChild(label);
            }
        }
    }

    renderResources(container) {
        for (const resource of this.filteredResources) {
            if (resource.hidden) continue;

            const halfIcon = ICON_SIZE / 2;

            const group = svgEl('g', {
                class: 'resource-node',
                transform: `translate(${resource.x - halfIcon}, ${resource.y - halfIcon})`,
            });

            if (resource.id === this.selectedResource) {
                group.classList.add('selected');
            }

            // Background circle.
            const bg = svgEl('circle', {
                cx: halfIcon, cy: halfIcon, r: NODE_RADIUS,
                fill: 'white', stroke: '#ddd', 'stroke-width': 2, opacity: 0.9,
            });

            // Inline SVG icon, or <image> from API icon_url.
            const iconGroup = svgEl('g', { transform: `scale(${ICON_SCALE})` });
            if (resource.icon_url) {
                const img = svgEl('image', {
                    href: resource.icon_url,
                    x: '0', y: '0',
                    width: String(ICON_SIZE), height: String(ICON_SIZE),
                    preserveAspectRatio: 'xMidYMid meet',
                });
                iconGroup.appendChild(img);
            } else {
                iconGroup.innerHTML = ICON_MAP[resource.type] || DEFAULT_ICON;
            }

            group.appendChild(bg);
            group.appendChild(iconGroup);

            // Labels (only when showLabels is on).
            if (this.showLabels) {
                const label = svgEl('text', {
                    class: 'resource-label',
                    x: halfIcon, y: halfIcon + 31,
                    'text-anchor': 'middle',
                    'font-size': LABEL_FONT_SIZE,
                    'font-weight': 500,
                });
                label.textContent = truncateText(resource.name || resource.id, LABEL_MAX_CHARS);

                const typeLabel = svgEl('text', {
                    class: 'resource-type-label',
                    x: halfIcon, y: halfIcon + 43,
                    'text-anchor': 'middle',
                    'font-size': TYPE_FONT_SIZE,
                    fill: '#666',
                });
                typeLabel.textContent = formatResourceType(resource.type);

                group.appendChild(label);
                group.appendChild(typeLabel);
            }

            group.addEventListener('click', () => this.selectResource(resource.id));
            group.addEventListener('mouseenter', (e) => this.showTooltip(e, resource));
            group.addEventListener('mouseleave', () => this.hideTooltip());

            container.appendChild(group);
        }
    }

    // ─── Transform (pan/zoom without full re-render) ───────────────────

    updateTransform() {
        const mainGroup = this.svg.querySelector('.main-group');
        if (mainGroup) {
            mainGroup.setAttribute('transform',
                `translate(${this.panX}, ${this.panY}) scale(${this.zoom})`);
        }
    }

    // ─── Tooltip (XSS-safe) ────────────────────────────────────────────

    showTooltip(event, resource) {
        const tooltip = document.getElementById('tooltip');
        const rect = this.svg.getBoundingClientRect();

        // Build tooltip content using safe escaping.
        const name = escapeHTML(resource.name || resource.id);
        const type = escapeHTML(formatResourceType(resource.type));
        const provider = escapeHTML(resource.provider || '');
        const region = escapeHTML(resource.region || 'N/A');
        const state = escapeHTML(resource.state || '');

        tooltip.innerHTML =
            `<strong>${name}</strong><br>` +
            `Type: ${type}<br>` +
            `Provider: ${provider}<br>` +
            `Region: ${region}<br>` +
            `State: ${state}`;

        tooltip.style.left = (event.clientX - rect.left + 10) + 'px';
        tooltip.style.top = (event.clientY - rect.top - 10) + 'px';
        tooltip.classList.remove('hidden');
    }

    hideTooltip() {
        document.getElementById('tooltip').classList.add('hidden');
    }

    updateStats() {
        document.getElementById('visible-count').textContent = this.filteredResources.length;
    }

    // ─── Controls ──────────────────────────────────────────────────────

    zoomIn() {
        this.zoom = Math.min(this.zoom * ZOOM_STEP, ZOOM_MAX);
        this.updateTransform();
    }

    zoomOut() {
        this.zoom = Math.max(this.zoom / ZOOM_STEP, ZOOM_MIN);
        this.updateTransform();
    }

    fitToView() {
        const visible = this.filteredResources.filter(r => !r.hidden);
        if (visible.length === 0) return;

        const pad = LAYOUT_MARGIN;
        const bounds = visible.reduce((acc, r) => ({
            minX: Math.min(acc.minX, r.x - pad),
            maxX: Math.max(acc.maxX, r.x + pad),
            minY: Math.min(acc.minY, r.y - pad),
            maxY: Math.max(acc.maxY, r.y + pad),
        }), { minX: Infinity, maxX: -Infinity, minY: Infinity, maxY: -Infinity });

        const bw = bounds.maxX - bounds.minX;
        const bh = bounds.maxY - bounds.minY;
        const cx = bounds.minX + bw / 2;
        const cy = bounds.minY + bh / 2;

        this.zoom = Math.min(this.width / bw, this.height / bh) * 0.8;
        this.panX = this.width / 2 - cx * this.zoom;
        this.panY = this.height / 2 - cy * this.zoom;

        this.updateTransform();
    }

    resetLayout() {
        this.zoom = 1;
        this.panX = 0;
        this.panY = 0;
        this.autoLayout();
        this.renderDiagram();
    }

    toggleLabels() {
        this.showLabels = !this.showLabels;
        this.renderDiagram();
    }

    // ─── Mouse events ──────────────────────────────────────────────────

    onMouseDown(event) {
        if (event.target === this.svg) {
            this.isDragging = true;
            this.lastMouseX = event.clientX;
            this.lastMouseY = event.clientY;
        }
    }

    onMouseMove(event) {
        if (!this.isDragging) return;

        this.panX += event.clientX - this.lastMouseX;
        this.panY += event.clientY - this.lastMouseY;
        this.lastMouseX = event.clientX;
        this.lastMouseY = event.clientY;

        // Update transform only -- no full re-render.
        this.updateTransform();
    }

    onMouseUp() {
        this.isDragging = false;
    }

    onWheel(event) {
        event.preventDefault();

        // Zoom towards cursor position.
        const rect = this.svg.getBoundingClientRect();
        const mouseX = event.clientX - rect.left;
        const mouseY = event.clientY - rect.top;

        const oldZoom = this.zoom;
        const factor = event.deltaY > 0 ? (1 / ZOOM_STEP) : ZOOM_STEP;
        this.zoom = Math.max(ZOOM_MIN, Math.min(ZOOM_MAX, this.zoom * factor));

        const ratio = this.zoom / oldZoom;
        this.panX = mouseX - (mouseX - this.panX) * ratio;
        this.panY = mouseY - (mouseY - this.panY) * ratio;

        this.updateTransform();
    }

    onResize() {
        clearTimeout(this._resizeTimer);
        this._resizeTimer = setTimeout(() => {
            const container = document.querySelector('.diagram-container');
            this.width = container.clientWidth;
            this.height = container.clientHeight;

            this.svg.setAttribute('width', this.width);
            this.svg.setAttribute('height', this.height);
            this.svg.setAttribute('viewBox', `0 0 ${this.width} ${this.height}`);

            this.renderDiagram();
        }, RESIZE_DEBOUNCE_MS);
    }

    showError(message) {
        const loading = document.getElementById('loading');
        if (loading) {
            loading.textContent = '';
            const div = document.createElement('div');
            div.className = 'error';
            div.textContent = message;
            loading.appendChild(div);
        }
    }

    // ─── Export ─────────────────────────────────────────────────────────

    calculateDiagramBounds() {
        const visible = this.filteredResources.filter(r => !r.hidden);
        if (visible.length === 0) {
            return { x: 0, y: 0, width: this.width, height: this.height };
        }

        const pad = 60;
        const bounds = visible.reduce((acc, r) => ({
            minX: Math.min(acc.minX, r.x - pad),
            maxX: Math.max(acc.maxX, r.x + pad),
            minY: Math.min(acc.minY, r.y - pad),
            maxY: Math.max(acc.maxY, r.y + pad),
        }), { minX: Infinity, maxX: -Infinity, minY: Infinity, maxY: -Infinity });

        return {
            x: bounds.minX,
            y: bounds.minY,
            width: bounds.maxX - bounds.minX,
            height: bounds.maxY - bounds.minY,
        };
    }

    exportToSVG() {
        const svgClone = this.svg.cloneNode(true);
        const bounds = this.calculateDiagramBounds();
        const pad = 50;

        svgClone.setAttribute('width', bounds.width + pad * 2);
        svgClone.setAttribute('height', bounds.height + pad * 2);
        svgClone.setAttribute('viewBox',
            `${bounds.x - pad} ${bounds.y - pad} ${bounds.width + pad * 2} ${bounds.height + pad * 2}`);

        const bg = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
        bg.setAttribute('x', bounds.x - pad);
        bg.setAttribute('y', bounds.y - pad);
        bg.setAttribute('width', bounds.width + pad * 2);
        bg.setAttribute('height', bounds.height + pad * 2);
        bg.setAttribute('fill', 'white');
        svgClone.insertBefore(bg, svgClone.firstChild);

        const serializer = new XMLSerializer();
        const svgString = serializer.serializeToString(svgClone);
        const svgBlob = new Blob([svgString], { type: 'image/svg+xml;charset=utf-8' });

        // Capture URL before click to avoid revoke-after-navigation issues.
        const url = URL.createObjectURL(svgBlob);
        const link = document.createElement('a');
        link.href = url;
        link.download = 'architecture-diagram.svg';
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        URL.revokeObjectURL(url);
    }

    exportToPNG() {
        const bounds = this.calculateDiagramBounds();
        const pad = 50;
        const scale = 2;

        const canvas = document.createElement('canvas');
        const ctx = canvas.getContext('2d');
        canvas.width = (bounds.width + pad * 2) * scale;
        canvas.height = (bounds.height + pad * 2) * scale;

        ctx.fillStyle = 'white';
        ctx.fillRect(0, 0, canvas.width, canvas.height);

        const svgClone = this.svg.cloneNode(true);
        svgClone.setAttribute('width', bounds.width + pad * 2);
        svgClone.setAttribute('height', bounds.height + pad * 2);
        svgClone.setAttribute('viewBox',
            `${bounds.x - pad} ${bounds.y - pad} ${bounds.width + pad * 2} ${bounds.height + pad * 2}`);

        const bg = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
        bg.setAttribute('x', bounds.x - pad);
        bg.setAttribute('y', bounds.y - pad);
        bg.setAttribute('width', bounds.width + pad * 2);
        bg.setAttribute('height', bounds.height + pad * 2);
        bg.setAttribute('fill', 'white');
        svgClone.insertBefore(bg, svgClone.firstChild);

        const serializer = new XMLSerializer();
        const svgString = serializer.serializeToString(svgClone);
        const svgBlob = new Blob([svgString], { type: 'image/svg+xml;charset=utf-8' });
        const url = URL.createObjectURL(svgBlob);

        const img = new Image();
        img.onload = () => {
            ctx.drawImage(img, 0, 0, canvas.width, canvas.height);
            canvas.toBlob((blob) => {
                const blobUrl = URL.createObjectURL(blob);
                const link = document.createElement('a');
                link.href = blobUrl;
                link.download = 'architecture-diagram.png';
                document.body.appendChild(link);
                link.click();
                document.body.removeChild(link);
                URL.revokeObjectURL(blobUrl);
            }, 'image/png');
            URL.revokeObjectURL(url);
        };
        img.onerror = () => {
            console.error('PNG export failed');
            URL.revokeObjectURL(url);
        };
        img.src = url;
    }
}

// Initialize when DOM is loaded.
document.addEventListener('DOMContentLoaded', () => {
    new DiagramViewer();
});
