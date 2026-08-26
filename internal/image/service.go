package image

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"openai-to-qwen/internal/config"
	"openai-to-qwen/internal/logger"
	"openai-to-qwen/internal/proxy"
)

const maxRequestBytes = 1 << 20 // 1 MiB JSON request cap

// Service handles the /v1/images/* conversion endpoints.
type Service struct {
	cfg        *config.Config
	client     *http.Client
	downloader *Downloader
	log        *logger.Logger
}

// NewService builds the image service.
func NewService(cfg *config.Config, client *http.Client, lg *logger.Logger) *Service {
	return &Service{
		cfg:        cfg,
		client:     client,
		downloader: NewDownloader(client, cfg.ImageMaxBytes),
		log:        lg,
	}
}

// HandleGenerations converts POST /v1/images/generations.
func (s *Service) HandleGenerations(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	start := time.Now()
	// Pre-size the buffer from Content-Length so large bodies are read with
	// one allocation instead of ReadAll's doubling-growth copies.
	buf := bytes.NewBuffer(make([]byte, 0, bodyHint(r)))
	_, err := buf.ReadFrom(io.LimitReader(r.Body, maxRequestBytes+1))
	body := buf.Bytes()
	if err != nil {
		s.log.Errorf("image generations: read body failed: %v", err)
		proxy.WriteJSONError(w, http.StatusBadRequest, "read request body failed")
		return
	}
	if len(body) > maxRequestBytes {
		s.log.Errorf("image generations: request body too large (%d bytes)", len(body))
		proxy.WriteJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	// Debug arguments are eager, so guard the body copy behind the level
	// check: with logging off (default) the hot path must not copy the
	// request body just to discard it.
	if s.log.Enabled(logger.LevelDebug) {
		s.log.Debugf("image generations incoming path=%s body=%s", r.URL.Path, truncate(string(body), 4096))
	}

	qreq, respFormat, dropped, err := ConvertGenerations(body, s.cfg.ModelAliases, s.cfg.QwenImageModel)
	if err != nil {
		s.log.Errorf("image generations: convert failed: %v", err)
		proxy.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.log.Infof("image generations: model=%s params=%v response_format=%q prompt_len=%d",
		qreq.Model, qreq.Parameters, respFormat, promptLen(qreq))
	if len(dropped) > 0 {
		s.log.Infof("image generations: no Qwen equivalent, dropped: %s", strings.Join(dropped, ", "))
	}
	s.forward(w, r, qreq, respFormat, start)
}

// HandleEdits converts POST /v1/images/edits (multipart).
func (s *Service) HandleEdits(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	start := time.Now()
	qreq, respFormat, dropped, err := ConvertEdits(r, s.cfg.ImageMaxBytes, s.cfg.ModelAliases, s.cfg.QwenImageModel)
	if err != nil {
		s.log.Errorf("image edits: convert failed: %v", err)
		proxy.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.log.Infof("image edits: model=%s images=%d params=%v response_format=%q",
		qreq.Model, imageCount(qreq), qreq.Parameters, respFormat)
	if len(dropped) > 0 {
		s.log.Infof("image edits: no Qwen equivalent, dropped: %s", strings.Join(dropped, ", "))
	}
	s.forward(w, r, qreq, respFormat, start)
}

// forward posts the converted request to Qwen and converts the response back.
// The log line breaks down where time goes: upstream = gateway->Qwen round
// trip, total = entire request (includes image download for b64_json).
func (s *Service) forward(w http.ResponseWriter, r *http.Request, qreq *QwenRequest, respFormat string, start time.Time) {
	payload, err := json.Marshal(qreq)
	if err != nil {
		s.log.Errorf("image upstream: encode request failed: %v", err)
		proxy.WriteJSONError(w, http.StatusInternalServerError, "encode upstream request failed")
		return
	}
	s.log.Infof("image upstream request url=%s model=%s", s.cfg.QwenImageBaseURL, qreq.Model)
	if s.log.Enabled(logger.LevelDebug) {
		s.log.Debugf("image upstream request body=%s", truncate(string(payload), 4096))
	}

	upstreamReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, s.cfg.QwenImageBaseURL, bytes.NewReader(payload))
	if err != nil {
		s.log.Errorf("image upstream: build request failed: %v", err)
		proxy.WriteJSONError(w, http.StatusInternalServerError, "build upstream request failed")
		return
	}
	// Key passthrough: forward the client's Authorization header unchanged.
	if auth := r.Header.Get("Authorization"); auth != "" {
		upstreamReq.Header.Set("Authorization", auth)
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Accept", "application/json")
	upstreamReq.Header.Set("User-Agent", "openai-to-qwen")

	upstreamStart := time.Now()
	resp, err := s.client.Do(upstreamReq)
	upstreamDur := time.Since(upstreamStart)
	if err != nil {
		s.log.Errorf("image upstream error: model=%s upstream=%s duration=%s err=%v",
			qreq.Model, s.cfg.QwenImageBaseURL, upstreamDur, err)
		proxy.WriteJSONError(w, http.StatusBadGateway, "upstream image request failed: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		// Drain a bounded remainder so the upstream connection can return
		// to the keep-alive pool instead of being discarded after this reply
		// (bounded in case upstream misbehaves and streams an error body).
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		s.log.Errorf("image upstream non-2xx: model=%s status=%d duration=%s request_id=%s request=%s body=%s",
			qreq.Model, resp.StatusCode, upstreamDur, resp.Header.Get("X-Request-Id"), truncate(string(payload), 2048), truncate(string(body), 2048))
		// Pass the upstream error through verbatim (headers included) so the
		// client sees the real error code/message and can trace the request.
		copyUpstreamHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(body)
		return
	}

	var qresp QwenResponse
	if err := json.NewDecoder(resp.Body).Decode(&qresp); err != nil {
		s.log.Errorf("image upstream decode error: model=%s status=%d duration=%s err=%v",
			qreq.Model, resp.StatusCode, upstreamDur, err)
		proxy.WriteJSONError(w, http.StatusBadGateway, "decode upstream response failed: "+err.Error())
		return
	}

	out, err := ConvertResponse(r.Context(), &qresp, respFormat, s.downloader, s.cfg.ImageDownloadConcurrency)
	if err != nil {
		s.log.Errorf("image response convert error: model=%s status=%d duration=%s err=%v",
			qreq.Model, resp.StatusCode, upstreamDur, err)
		proxy.WriteJSONError(w, http.StatusBadGateway, err.Error())
		return
	}

	firstURL := "-"
	if len(out.Data) > 0 && out.Data[0].URL != "" {
		firstURL = truncate(out.Data[0].URL, 200)
	}
	s.log.Infof("image ok: model=%s status=%d upstream=%s total=%s images=%d request_id=%s first_url=%s",
		qreq.Model, resp.StatusCode, upstreamDur, time.Since(start), len(out.Data), qresp.RequestID, firstURL)
	copyUpstreamHeaders(w.Header(), resp.Header)
	if qresp.RequestID != "" {
		w.Header().Set("X-Request-Id", qresp.RequestID)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func promptLen(q *QwenRequest) int {
	if len(q.Input.Messages) == 0 {
		return 0
	}
	for _, c := range q.Input.Messages[0].Content {
		if c.Text != "" {
			return len(c.Text)
		}
	}
	return 0
}

func imageCount(q *QwenRequest) int {
	if len(q.Input.Messages) == 0 {
		return 0
	}
	n := 0
	for _, c := range q.Input.Messages[0].Content {
		if c.Image != "" {
			n++
		}
	}
	return n
}

// bodyHint picks an initial read-buffer size for a request body: the
// declared Content-Length when sane, capped at maxRequestBytes+1 so a lying
// client cannot force a huge preallocation.
func bodyHint(r *http.Request) int {
	if r.ContentLength > 0 && r.ContentLength <= maxRequestBytes+1 {
		return int(r.ContentLength)
	}
	return 512
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
