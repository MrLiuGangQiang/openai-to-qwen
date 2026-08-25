// Package proxy implements the byte-level reverse proxy used for OpenAI
// compatible text endpoints. The request/response bodies are never parsed.
package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"openai-to-qwen/internal/logger"
)

// ExposedPrefix is the API prefix this gateway serves.
const ExposedPrefix = "/v1"

// reqStartKey carries the upstream start time in the request context so
// ModifyResponse / ErrorHandler can log the upstream round-trip duration.
// The context is only allocated when info logging is enabled.
type reqStartKey struct{}

// NewTransport returns a tuned, shared http.Transport for upstream calls.
// responseHeaderTimeout <= 0 disables the response-header timeout, which is
// recommended for streaming endpoints where the first byte may be slow.
// DisableCompression avoids gzip encode/decode CPU on small JSON/SSE payloads.
func NewTransport(responseHeaderTimeout time.Duration) *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: responseHeaderTimeout,
		DisableCompression:    true,
	}
}

// NewTextProxy builds a reverse proxy that forwards everything under
// /v1 (except /v1/images/*, routed elsewhere) to the Token Plan
// OpenAI-compatible endpoint, replacing the Authorization header.
func NewTextProxy(textBaseURL string, timeout time.Duration, lg *logger.Logger) (*httputil.ReverseProxy, error) {
	upstream, err := url.Parse(textBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid QWEN_TEXT_BASE_URL: %w", err)
	}
	if upstream.Scheme != "http" && upstream.Scheme != "https" {
		return nil, fmt.Errorf("invalid QWEN_TEXT_BASE_URL scheme %q", upstream.Scheme)
	}

	infoEnabled := lg.Enabled(logger.LevelInfo)

	rp := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			rel := strings.TrimPrefix(req.URL.Path, ExposedPrefix)
			req.URL.Scheme = upstream.Scheme
			req.URL.Host = upstream.Host
			req.URL.Path = upstream.Path + rel
			req.Host = upstream.Host
			// Key passthrough: the client's Authorization header is forwarded
			// unchanged (ReverseProxy copies inbound headers automatically).
			if infoEnabled {
				*req = *req.WithContext(context.WithValue(req.Context(), reqStartKey{}, time.Now()))
			}
		},
		Transport:     NewTransport(0), // no header timeout: streaming must not be cut off
		FlushInterval: -1,              // immediate flush for SSE streaming
		ModifyResponse: func(resp *http.Response) error {
			if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
				// SSE: ask proxies/nginx not to buffer so chunks flow through
				// immediately instead of arriving as one burst.
				resp.Header.Set("X-Accel-Buffering", "no")
				resp.Header.Set("Cache-Control", "no-cache")
			}
			if infoEnabled {
				if start, ok := resp.Request.Context().Value(reqStartKey{}).(time.Time); ok {
					lg.Infof("text upstream url=%s status=%d duration=%s content_type=%s",
						resp.Request.URL.String(), resp.StatusCode, time.Since(start), resp.Header.Get("Content-Type"))
				}
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			lg.Errorf("text upstream error: %s err=%v", req.URL.Path, err)
			WriteJSONError(w, http.StatusBadGateway, "upstream request failed")
		},
		ErrorLog: lg.StdLogger(logger.LevelError),
	}
	return rp, nil
}

// WriteJSONError writes an OpenAI-style error response.
func WriteJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"message": msg,
			"type":    "invalid_request_error",
			"code":    status,
		},
	})
}
