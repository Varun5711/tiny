package handlers

import (
	"net/http"
)

// SwaggerHandler serves the interactive API documentation UI and the raw
// OpenAPI specification file. It uses the swagger-ui-dist CDN bundle so the
// gateway binary does not need to embed any static assets beyond the spec YAML.
type SwaggerHandler struct {
	specPath string // specPath is the filesystem path to the OpenAPI YAML spec file.
}

// NewSwaggerHandler creates a SwaggerHandler that reads the OpenAPI spec from
// the given file path. The spec is served on each request (not cached), so
// changes to the file are reflected without a restart.
func NewSwaggerHandler(specPath string) *SwaggerHandler {
	return &SwaggerHandler{
		specPath: specPath,
	}
}

// ServeSwaggerUI renders the Swagger UI single-page application. The HTML is
// compiled into the binary as a constant to avoid filesystem dependencies for
// the UI itself; only the OpenAPI spec is loaded from disk.
func (h *SwaggerHandler) ServeSwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerUIHTML))
}

// ServeSpec serves the raw OpenAPI YAML specification. The Access-Control-Allow-Origin
// header is set to "*" so the Swagger UI JavaScript (loaded from a CDN) can
// fetch the spec without running into CORS restrictions.
func (h *SwaggerHandler) ServeSpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.ServeFile(w, r, h.specPath)
}

// RegisterRoutes mounts the Swagger UI and spec endpoints on the given mux.
// Both "/docs" and "/docs/" serve the UI to handle trailing-slash variance.
func (h *SwaggerHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/docs", h.ServeSwaggerUI)
	mux.HandleFunc("/docs/", h.ServeSwaggerUI)
	mux.HandleFunc("/openapi.yaml", h.ServeSpec)
}

const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>API Documentation - Tiny URL Shortener</title>
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css">
    <link rel="icon" type="image/png" href="https://unpkg.com/swagger-ui-dist@5.11.0/favicon-32x32.png">
    <style>
        html { box-sizing: border-box; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin: 0; background: #fafafa; }
        .swagger-ui .topbar { display: none; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-standalone-preset.js"></script>
    <script>
        window.onload = function() {
            window.ui = SwaggerUIBundle({
                url: "/openapi.yaml",
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                plugins: [SwaggerUIBundle.plugins.DownloadUrl],
                layout: "StandaloneLayout",
                docExpansion: "list",
                filter: true,
                showRequestDuration: true,
                persistAuthorization: true
            });
        };
    </script>
</body>
</html>`
