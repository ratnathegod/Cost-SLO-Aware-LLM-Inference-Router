package docs

import (
	"embed"
	"html/template"
	"net/http"
	"strings"
	
	"github.com/rs/zerolog/log"
)

//go:embed swagger-ui/* openapi.yaml
var assets embed.FS

// SwaggerUIHandler serves the Swagger UI for API documentation
func SwaggerUIHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		
		// Serve the main Swagger UI page
		if path == "/docs" || path == "/docs/" {
			serveSwaggerHTML(w, r)
			return
		}
		
		// Serve OpenAPI spec
		if path == "/docs/openapi.yaml" {
			w.Header().Set("Content-Type", "application/yaml")
			spec, err := assets.ReadFile("openapi.yaml")
			if err != nil {
				http.Error(w, "OpenAPI spec not found", http.StatusNotFound)
				return
			}
			w.Write(spec)
			return
		}
		
		// Serve Swagger UI static assets
		if strings.HasPrefix(path, "/docs/swagger-ui/") {
			assetPath := strings.TrimPrefix(path, "/docs/")
			data, err := assets.ReadFile(assetPath)
			if err != nil {
				http.Error(w, "Asset not found", http.StatusNotFound)
				return
			}
			
			// Set appropriate content type
			contentType := getContentType(assetPath)
			w.Header().Set("Content-Type", contentType)
			w.Write(data)
			return
		}
		
		http.NotFound(w, r)
	})
}

func serveSwaggerHTML(w http.ResponseWriter, r *http.Request) {
	tmpl := `<!DOCTYPE html>
<html>
<head>
	<title>LLM Router API Documentation</title>
	<link rel="stylesheet" type="text/css" href="/docs/swagger-ui/swagger-ui-bundle.css" />
	<link rel="stylesheet" type="text/css" href="/docs/swagger-ui/swagger-ui-standalone-preset.css" />
	<style>
		html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
		*, *:before, *:after { box-sizing: inherit; }
		body { margin:0; background: #fafafa; }
		.swagger-ui .topbar { display: none; }
		.admin-endpoint { position: relative; }
		.admin-endpoint::after {
			content: "🔒 Admin Only";
			position: absolute;
			top: 10px;
			right: 20px;
			background: #ff6b6b;
			color: white;
			padding: 2px 8px;
			border-radius: 12px;
			font-size: 11px;
			font-weight: bold;
		}
		.swagger-ui .info .title { color: #3b4151; }
		.custom-header {
			background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
			color: white;
			padding: 20px;
			text-align: center;
			margin-bottom: 20px;
		}
		.custom-header h1 { margin: 0; font-size: 2em; }
		.custom-header p { margin: 10px 0 0 0; opacity: 0.9; }
	</style>
</head>
<body>
	<div class="custom-header">
		<h1>🚀 LLM Router API</h1>
		<p>Cost and SLO-aware LLM inference routing with multi-tenant support</p>
	</div>
	<div id="swagger-ui"></div>
	<script src="/docs/swagger-ui/swagger-ui-bundle.js"></script>
	<script src="/docs/swagger-ui/swagger-ui-standalone-preset.js"></script>
	<script>
		window.onload = function() {
			const ui = SwaggerUIBundle({
				url: '/docs/openapi.yaml',
				dom_id: '#swagger-ui',
				deepLinking: true,
				presets: [
					SwaggerUIBundle.presets.apis,
					SwaggerUIStandalonePreset
				],
				plugins: [
					SwaggerUIBundle.plugins.DownloadUrl
				],
				layout: "StandaloneLayout",
				tryItOutEnabled: true,
				supportedSubmitMethods: ['get', 'post'],
				onComplete: function() {
					// Add admin badges to admin endpoints
					setTimeout(() => {
						const adminPaths = [
							'/v1/admin/status',
							'/v1/admin/canary/status',
							'/v1/admin/canary/advance',
							'/v1/admin/canary/rollback',
							'/v1/admin/policy',
							'/v1/admin/providers/reload',
							'/v1/admin/tenants',
							'/v1/admin/tenants/{tenant_id}/usage'
						];
						
						adminPaths.forEach(path => {
							const elements = document.querySelectorAll('[data-path="' + path + '"]');
							elements.forEach(el => {
								if (el.classList) {
									el.classList.add('admin-endpoint');
								}
							});
						});
					}, 500);
				},
				requestInterceptor: function(req) {
					// Add helpful hints in request examples
					if (req.headers['X-API-Key']) {
						req.headers['X-API-Key'] = 'your_tenant_api_key_here';
					}
					if (req.headers['Authorization']) {
						req.headers['Authorization'] = 'Bearer your_admin_token_here';
					}
					return req;
				}
			});
		}
	</script>
</body>
</html>`
	
	t, err := template.New("swagger").Parse(tmpl)
	if err != nil {
		log.Error().Err(err).Msg("failed to parse swagger template")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "text/html")
	if err := t.Execute(w, nil); err != nil {
		log.Error().Err(err).Msg("failed to execute swagger template")
	}
}

func getContentType(path string) string {
	if strings.HasSuffix(path, ".css") {
		return "text/css"
	}
	if strings.HasSuffix(path, ".js") {
		return "application/javascript"
	}
	if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
		return "application/yaml"
	}
	return "text/plain"
}