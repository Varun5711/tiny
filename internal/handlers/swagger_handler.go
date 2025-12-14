package handlers

import (
	"net/http"
)

type SwaggerHandler struct {
	specPath string
}

func NewSwaggerHandler(specPath string) *SwaggerHandler {
	return &SwaggerHandler{
		specPath: specPath,
	}
}

func (h *SwaggerHandler) ServeSwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(swaggerUIHTML))
}

func (h *SwaggerHandler) ServeSpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.ServeFile(w, r, h.specPath)
}

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
