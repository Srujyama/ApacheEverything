package httpapi

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.yaml
var openapiYAML []byte

// rapidocHTML is a minimal HTML page that loads RapiDoc from a CDN and
// points it at /api/openapi.yaml. RapiDoc is a single web component, ~250
// KB gzipped from unpkg, way smaller than embedding Swagger UI.
const rapidocHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <title>Sunny API</title>
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <script type="module" src="https://unpkg.com/rapidoc@9.3.4/dist/rapidoc-min.js"></script>
  <style>
    body { margin: 0; }
    rapi-doc {
      --bg: #0a0e17;
      --primary-color: #5eead4;
      --nav-bg-color: #111827;
      --nav-text-color: #cbd5e1;
      --regular-font-size: 13px;
      --mono-font-size: 12px;
      height: 100vh;
    }
  </style>
</head>
<body>
  <rapi-doc
    spec-url="/api/openapi.yaml"
    theme="dark"
    show-header="false"
    render-style="read"
    nav-bg-color="#111827"
    schema-style="table"
    schema-description-expanded="true"
  ></rapi-doc>
</body>
</html>
`

func openapiSpecHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(openapiYAML)
}

func apiDocsHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(rapidocHTML))
}
