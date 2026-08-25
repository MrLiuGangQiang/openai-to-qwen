// Package proxy implements the byte-level reverse proxy used for OpenAI
// compatible text endpoints. The request/response bodies are never parsed.
package proxy

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// ExposedPrefix is the API prefix this gateway serves.
const ExposedPrefix = "/v1"

// NewTransport returns a tuned, shared http.Transport for upstream calls.
// responseHeaderTimeout <= 0 disables the response-header timeout, which is
// recommended for streaming endpoints where the first byte may be slow.
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
	}
}

// NewTextProxy builds a reverse proxy that forwards everything under
// /v1 (except /v1/images/*, routed elsewhere) to the Token Plan
// OpenAI-compatible endpoint, replacing the Authorization header.
func NewTextProxy(textBaseURL, apiKey string, timeout time.Duration, logger *log.Logger) (*httputil.ReverseProxy, error) {
	upstream, err := url.Parse(textBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid QWEN_TEXT_BASE_URL: %w", err)
	}
	if upstream.Scheme != "http" && upstream.Scheme != "https" {
		return nil, fmt.Errorf("invalid QWEN_TEXT_BASE_URL scheme %q", upstream.Scheme)
	}

	rp := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			rel := strings.TrimPrefix(req.URL.Path, ExposedPrefix)
			req.URL.Scheme = upstream.Scheme
			req.URL.Host = upstream.Host
			req.URL.Path = upstream.Path + rel
			req.Host = upstream.Host
			req.Header.Set("Authorization", "Bearer "+apiKey)
		},
		Transport:     NewTransport(0), // no header timeout: streaming must not be cut off
		FlushInterval: -1,              // immediate flush for SSE streaming
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			logger.Printf("text upstream error: %v", err)
			WriteJSONError(w, http.StatusBadGateway, "upstream request failed")
		},
		ErrorLog: logger,
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
