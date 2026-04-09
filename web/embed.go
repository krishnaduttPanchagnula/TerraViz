// Package web embeds all frontend assets (templates, JS, fonts, icons) so the
// binary is fully self-contained and requires no external files at runtime.
package web

import "embed"

// EmbeddedFiles contains the complete web/ directory tree:
//   - templates/*.html   — Go HTML templates
//   - static/js/         — Frontend JavaScript
//   - static/fonts/      — Inter and JetBrains Mono WOFF2 fonts
//   - static/icons/      — Legacy icon SVGs (unused, kept for compatibility)
//   - icons/             — AWS service icon packs (26 categories, ~620 SVGs)
//
//go:embed templates static icons
var EmbeddedFiles embed.FS
