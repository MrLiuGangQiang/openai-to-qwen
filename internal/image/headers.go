package image

import (
	"net/http"
	"strings"
)

// hopHeaders are connection-specific headers that must not be copied from the
// upstream response (Go's http client already handles the transfer itself).
var hopHeaders = map[string]bool{
	"Connection":          true,
	"Proxy-Connection":    true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

// framingHeaders describe the upstream body; since the gateway rewrites the
// response body, these must not be copied (Go recomputes Content-Length).
// Content-Type is intentionally NOT skipped here: error responses are passed
// through verbatim, and the success path overrides it explicitly.
var framingHeaders = map[string]bool{
	"Content-Length":   true,
	"Content-Encoding": true,
	"Content-Range":    true,
}

// copyUpstreamHeaders copies response headers from the upstream call to the
// client response, skipping hop-by-hop and body-framing headers so the gateway
// keeps full control of the client connection. This preserves rate-limit,
// retry-after, X-Request-Id, X-DashScope-* and any other diagnostic headers.
func copyUpstreamHeaders(dst, src http.Header) {
	// Headers named in the Connection header are also hop-by-hop (RFC 7230).
	extraHop := map[string]bool{}
	for _, v := range src.Values("Connection") {
		for _, name := range strings.Split(v, ",") {
			if n := strings.TrimSpace(name); n != "" {
				extraHop[http.CanonicalHeaderKey(n)] = true
			}
		}
	}
	for k, vv := range src {
		ck := http.CanonicalHeaderKey(k)
		if hopHeaders[ck] || extraHop[ck] || framingHeaders[ck] {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}
