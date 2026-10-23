package exporters

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"regexp"
	"strings"

	"terraviz/internal/models"
	"terraviz/web"
)

// The embedded assets the exported file is built from.
const (
	htmlTemplatePath = "templates/index_enhanced.html"
	htmlScriptPath   = "static/js/diagram_enhanced.js"
	htmlScriptTag    = `<script src="/static/js/diagram_enhanced.js"></script>`
	htmlFontDir      = "static/fonts/"
	htmlIconDir      = "icons/"
)

var (
	// htmlFontRef finds the font files the template asks for.
	htmlFontRef = regexp.MustCompile(`/static/fonts/([A-Za-z0-9._-]+)`)
	// htmlIconRef finds the icon files the diagram and the script point at.
	htmlIconRef = regexp.MustCompile(`/icons/([A-Za-z0-9._/-]+\.svg)`)
	// htmlIconLiteral finds an icon path that stands on its own in the
	// script, so it can be replaced by a lookup instead of a data URI.
	htmlIconLiteral = regexp.MustCompile(`"/icons/([A-Za-z0-9._/-]+\.svg)"`)
)

// htmlFetchShim answers the diagram request of the app from the data that
// was inlined into the file, so the page works without a server.
const htmlFetchShim = `<script id="terraviz-fetch-shim">
(function () {
  var originalFetch = window.fetch ? window.fetch.bind(window) : null;
  window.fetch = function (input, init) {
    var url = typeof input === "string" ? input : (input && input.url) || "";
    if (url.indexOf("/api/diagram") !== -1) {
      var payload = window.__TERRAVIZ_DIAGRAM__;
      if (typeof Response === "function") {
        return Promise.resolve(new Response(JSON.stringify(payload), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }));
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        statusText: "OK",
        json: function () { return Promise.resolve(payload); },
      });
    }
    if (originalFetch) return originalFetch(input, init);
    return Promise.reject(new Error("The exported diagram is an offline file"));
  };
})();
</script>`

// HTML renders the diagram as a single self-contained page: the template, the
// script, the fonts and the icons are inlined, and the diagram itself is
// handed to the app by a fetch shim. The file opens from disk with no server
// and no network.
func HTML(d *models.Diagram) ([]byte, error) {
	if d == nil {
		return nil, fmt.Errorf("building HTML: no diagram")
	}

	raw, err := web.EmbeddedFiles.ReadFile(htmlTemplatePath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", htmlTemplatePath, err)
	}

	tmpl, err := template.New(htmlTemplatePath).Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", htmlTemplatePath, err)
	}

	var page bytes.Buffer
	if err := tmpl.Execute(&page, map[string]any{
		"title":            "Cloud Architecture Visualizer",
		"diagram_name":     diagramName(d),
		"resource_count":   len(d.Resources),
		"connection_count": len(d.Connections),
	}); err != nil {
		return nil, fmt.Errorf("rendering %s: %w", htmlTemplatePath, err)
	}
	html := page.String()

	// The fonts become data URIs, so the file needs no network for type.
	fontBytes := 0
	seenFonts := map[string]bool{}
	for _, match := range htmlFontRef.FindAllStringSubmatch(html, -1) {
		name := match[1]
		if seenFonts[name] {
			continue
		}
		seenFonts[name] = true

		font, err := web.EmbeddedFiles.ReadFile(htmlFontDir + name)
		if err != nil {
			return nil, fmt.Errorf("reading font %s: %w", name, err)
		}
		fontBytes += len(font)
		html = strings.ReplaceAll(html, "/static/fonts/"+name, fontDataURI(font))
	}

	// The script and the icons it falls back to. Every icon of the pack that
	// the script names is stored once, under its path, and the script looks
	// it up, so the file does not carry the same data URI over and over.
	script, err := web.EmbeddedFiles.ReadFile(htmlScriptPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", htmlScriptPath, err)
	}
	js := string(script)

	iconBytes := 0
	icons := map[string]string{}
	seenIcons := map[string]bool{}
	for _, match := range htmlIconRef.FindAllStringSubmatch(js, -1) {
		path := match[1]
		if seenIcons[path] {
			continue
		}
		seenIcons[path] = true

		icon, err := web.EmbeddedFiles.ReadFile(htmlIconDir + path)
		if err != nil {
			// The pack has no such icon: drop the path so the file keeps
			// pointing at nothing but itself.
			icons[path] = ""
			continue
		}
		iconBytes += len(icon)
		icons[path] = iconDataURI(icon)
	}

	// A path that stands on its own becomes a lookup, everything else is
	// replaced by the data URI itself.
	js = htmlIconLiteral.ReplaceAllStringFunc(js, func(literal string) string {
		path := htmlIconLiteral.FindStringSubmatch(literal)[1]
		return fmt.Sprintf("__terravizIcon(%q)", path)
	})
	js = htmlIconRef.ReplaceAllStringFunc(js, func(ref string) string {
		path := htmlIconRef.FindStringSubmatch(ref)[1]
		if data, ok := icons[path]; ok {
			return data
		}
		return ""
	})

	// The diagram, with the icons it uses inlined.
	inline := *d
	inline.Resources = make([]models.Resource, len(d.Resources))
	copy(inline.Resources, d.Resources)
	for i := range inline.Resources {
		resource := &inline.Resources[i]
		url := resource.IconURL
		if url == "" {
			url = resource.GetResourceIcon()
		}
		resource.IconURL = inlineIconURL(url)
	}

	payload, err := json.Marshal(struct {
		Diagram models.Diagram `json:"diagram"`
	}{Diagram: inline})
	if err != nil {
		return nil, fmt.Errorf("encoding the diagram: %w", err)
	}

	if !strings.Contains(html, htmlScriptTag) {
		return nil, fmt.Errorf("rendering HTML: %q not found in %s", htmlScriptTag, htmlTemplatePath)
	}
	iconMap, err := json.Marshal(icons)
	if err != nil {
		return nil, fmt.Errorf("encoding the icons: %w", err)
	}
	dataScript := "<script id=\"terraviz-diagram-data\">\n" +
		"window.__TERRAVIZ_ICONS__ = " + string(iconMap) + ";\n" +
		"window.__terravizIcon = function (path) { return window.__TERRAVIZ_ICONS__[path] || \"\"; };\n" +
		"window.__TERRAVIZ_DIAGRAM__ = " + string(payload) + ";\n</script>"
	appScript := "<script>\n" + strings.ReplaceAll(js, "</script", `<\/script`) + "\n</script>"
	html = strings.Replace(html, htmlScriptTag, dataScript+"\n"+htmlFetchShim+"\n"+appScript, 1)

	// Say what the file is made of, so the size of every part is visible.
	comment := fmt.Sprintf("<!-- terraviz self-contained export: js=%dB fonts=%dB icons=%dB -->",
		len(script), fontBytes, iconBytes)
	html = strings.Replace(html, "<!DOCTYPE html>", "<!DOCTYPE html>\n"+comment, 1)

	return []byte(html), nil
}

// inlineIconURL turns an icon path into a data URI. A path that is not an
// embedded icon is left alone.
func inlineIconURL(url string) string {
	match := htmlIconRef.FindStringSubmatch(url)
	if match == nil {
		return url
	}
	icon, err := web.EmbeddedFiles.ReadFile(htmlIconDir + match[1])
	if err != nil {
		return ""
	}
	return iconDataURI(icon)
}

// iconDataURI builds the data URI of an SVG icon.
func iconDataURI(icon []byte) string {
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(icon)
}

// fontDataURI builds the data URI of a WOFF2 font.
func fontDataURI(font []byte) string {
	return "data:font/woff2;base64," + base64.StdEncoding.EncodeToString(font)
}
