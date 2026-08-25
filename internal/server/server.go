// Package server wires routing, auth, and logging around the text proxy and
// image conversion service.
package server

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
	"time"

	"openai-to-qwen/internal/config"
	"openai-to-qwen/internal/image"
	"openai-to-qwen/internal/logger"
	"openai-to-qwen/internal/proxy"
)

// Server is the gateway.
type Server struct {
	cfg       *config.Config
	lg        *logger.Logger
	textProxy http.Handler
	img       *image.Service
}

// New builds the server from configuration.
func New(cfg *config.Config) (*Server, error) {
	lg := logger.New(logger.Parse(cfg.LogLevel), os.Stdout)

	textProxy, err := proxy.NewTextProxy(cfg.QwenTextBaseURL, cfg.UpstreamTimeout, lg)
	if err != nil {
		return nil, err
	}

	imgClient := &http.Client{
		Timeout:   cfg.UpstreamTimeout,
		Transport: proxy.NewTransport(cfg.UpstreamTimeout),
	}
	return &Server{
		cfg:       cfg,
		lg:        lg,
		textProxy: textProxy,
		img:       image.NewService(cfg, imgClient, lg),
	}, nil
}

// Handler returns the root HTTP handler. When info logging is disabled the
// access-log middleware is not installed at all, so the hot path stays lean.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/v1/images/", s.handleImages)
	mux.Handle("/", s.textProxy)

	var h http.Handler = mux
	if s.lg.Enabled(logger.LevelInfo) {
		h = s.loggingMiddleware(h)
	}
	h = s.authMiddleware(h)
	return s.recoverMiddleware(h)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) handleImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		proxy.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/generations"):
		s.img.HandleGenerations(w, r)
	case strings.HasSuffix(r.URL.Path, "/edits"):
		s.img.HandleEdits(w, r)
	default:
		proxy.WriteJSONError(w, http.StatusNotFound, "unknown image endpoint")
	}
}

// authMiddleware enforces the exposed API key when configured. /healthz is
// always allowed so Docker HEALTHCHECK works without a key.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	if s.cfg.ExposedAPIKey == "" {
		return next
	}
	expected := []byte(s.cfg.ExposedAPIKey)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		provided := ""
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			provided = strings.TrimPrefix(auth, "Bearer ")
		} else if k := r.Header.Get("x-api-key"); k != "" {
			provided = k
		}
		if subtle.ConstantTimeCompare([]byte(provided), expected) != 1 {
			s.lg.Errorf("auth reject path=%s remote=%s", r.URL.Path, r.RemoteAddr)
			proxy.WriteJSONError(w, http.StatusUnauthorized, "invalid API key")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush makes the recorder transparent for streaming (SSE) responses so
// httputil.ReverseProxy can flush each chunk immediately.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// loggingMiddleware emits a detailed access log line per request.
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.lg.Infof("req method=%s path=%s query=%q remote=%s ua=%q status=%d duration=%s bytes_in=%d",
			r.Method, r.URL.Path, r.URL.RawQuery, r.RemoteAddr, r.UserAgent(), rec.status, time.Since(start), r.ContentLength)
	})
}

func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.lg.Errorf("panic: %v", rec)
				proxy.WriteJSONError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
