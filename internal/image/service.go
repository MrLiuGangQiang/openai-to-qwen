package image

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"openai-to-qwen/internal/config"
	"openai-to-qwen/internal/proxy"
)

const maxRequestBytes = 1 << 20 // 1 MiB JSON request cap

// Service handles the /v1/images/* conversion endpoints.
type Service struct {
	cfg        *config.Config
	client     *http.Client
	downloader *Downloader
	logger     *log.Logger
}

// NewService builds the image service.
func NewService(cfg *config.Config, client *http.Client, logger *log.Logger) *Service {
	return &Service{
		cfg:        cfg,
		client:     client,
		downloader: NewDownloader(client, cfg.ImageMaxBytes),
		logger:     logger,
	}
}

// HandleGenerations converts POST /v1/images/generations.
func (s *Service) HandleGenerations(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes+1))
	if err != nil {
		s.logger.Printf("image generations: read body failed: %v", err)
		proxy.WriteJSONError(w, http.StatusBadRequest, "read request body failed")
		return
	}
	if len(body) > maxRequestBytes {
		proxy.WriteJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}

	qreq, respFormat, err := ConvertGenerations(body, s.cfg.ModelAliases, s.cfg.QwenImageModel)
	if err != nil {
		s.logger.Printf("image generations: convert failed: %v", err)
		proxy.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.logger.Printf("image generations: model=%s params=%v response_format=%q prompt_len=%d",
		qreq.Model, qreq.Parameters, respFormat, promptLen(qreq))
	s.forward(w, r, qreq, respFormat, start)
}

// HandleEdits converts POST /v1/images/edits (multipart).
func (s *Service) HandleEdits(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	qreq, respFormat, err := ConvertEdits(r, s.cfg.ImageMaxBytes, s.cfg.ModelAliases, s.cfg.QwenImageModel)
	if err != nil {
		s.logger.Printf("image edits: convert failed: %v", err)
		proxy.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.logger.Printf("image edits: model=%s images=%d params=%v response_format=%q",
		qreq.Model, imageCount(qreq), qreq.Parameters, respFormat)
	s.forward(w, r, qreq, respFormat, start)
}

// forward posts the converted request to Qwen and converts the response back.
// The log line breaks down where time goes: upstream = gateway->Qwen round
// trip, total = entire request (includes image download for b64_json).
func (s *Service) forward(w http.ResponseWriter, r *http.Request, qreq *QwenRequest, respFormat string, start time.Time) {
	payload, err := json.Marshal(qreq)
	if err != nil {
		proxy.WriteJSONError(w, http.StatusInternalServerError, "encode upstream request failed")
		return
	}

	upstreamReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, s.cfg.QwenImageBaseURL, bytes.NewReader(payload))
	if err != nil {
		proxy.WriteJSONError(w, http.StatusInternalServerError, "build upstream request failed")
		return
	}
	upstreamReq.Header.Set("Authorization", "Bearer "+s.cfg.QwenAPIKey)
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Accept", "application/json")
	upstreamReq.Header.Set("User-Agent", "openai-to-qwen")

	upstreamStart := time.Now()
	resp, err := s.client.Do(upstreamReq)
	upstreamDur := time.Since(upstreamStart)
	if err != nil {
		s.logger.Printf("image upstream error: model=%s upstream=%s duration=%s err=%v",
			qreq.Model, s.cfg.QwenImageBaseURL, upstreamDur, err)
		proxy.WriteJSONError(w, http.StatusBadGateway, "upstream image request failed: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		s.logger.Printf("image upstream non-2xx: model=%s status=%d duration=%s request_id=%s body=%s",
			qreq.Model, resp.StatusCode, upstreamDur, resp.Header.Get("X-Request-Id"), string(body))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(body)
		return
	}

	var qresp QwenResponse
	if err := json.NewDecoder(resp.Body).Decode(&qresp); err != nil {
		s.logger.Printf("image upstream decode error: model=%s status=%d duration=%s err=%v",
			qreq.Model, resp.StatusCode, upstreamDur, err)
		proxy.WriteJSONError(w, http.StatusBadGateway, "decode upstream response failed: "+err.Error())
		return
	}

	out, err := ConvertResponse(r.Context(), &qresp, respFormat, s.downloader, s.cfg.ImageDownloadConcurrency)
	if err != nil {
		s.logger.Printf("image response convert error: model=%s status=%d duration=%s err=%v",
			qreq.Model, resp.StatusCode, upstreamDur, err)
		proxy.WriteJSONError(w, http.StatusBadGateway, err.Error())
		return
	}

	s.logger.Printf("image ok: model=%s status=%d upstream=%s total=%s images=%d request_id=%s",
		qreq.Model, resp.StatusCode, upstreamDur, time.Since(start), len(out.Data), qresp.RequestID)
	w.Header().Set("Content-Type", "application/json")
	if qresp.RequestID != "" {
		w.Header().Set("X-Request-Id", qresp.RequestID)
	}
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
