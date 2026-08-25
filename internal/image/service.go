package image

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"openai-to-qwen/internal/config"
	"openai-to-qwen/internal/proxy"
)

const maxRequestBytes = 1 << 20 // 1 MiB JSON request cap

// Service handles the /v1/images/* conversion endpoints.
type Service struct {
	cfg        *config.Config
	client     *http.Client
	downloader *Downloader
}

// NewService builds the image service.
func NewService(cfg *config.Config, client *http.Client) *Service {
	return &Service{
		cfg:        cfg,
		client:     client,
		downloader: NewDownloader(client, cfg.ImageMaxBytes),
	}
}

// HandleGenerations converts POST /v1/images/generations.
func (s *Service) HandleGenerations(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes+1))
	if err != nil {
		proxy.WriteJSONError(w, http.StatusBadRequest, "read request body failed")
		return
	}
	if len(body) > maxRequestBytes {
		proxy.WriteJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}

	qreq, respFormat, err := ConvertGenerations(body, s.cfg.ModelAliases, s.cfg.QwenImageModel)
	if err != nil {
		proxy.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.forward(w, r, qreq, respFormat)
}

// HandleEdits converts POST /v1/images/edits (multipart).
func (s *Service) HandleEdits(w http.ResponseWriter, r *http.Request) {
	qreq, respFormat, err := ConvertEdits(r, s.cfg.ImageMaxBytes, s.cfg.ModelAliases, s.cfg.QwenImageModel)
	if err != nil {
		proxy.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.forward(w, r, qreq, respFormat)
}

// forward posts the converted request to Qwen and converts the response back.
func (s *Service) forward(w http.ResponseWriter, r *http.Request, qreq *QwenRequest, respFormat string) {
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

	resp, err := s.client.Do(upstreamReq)
	if err != nil {
		proxy.WriteJSONError(w, http.StatusBadGateway, "upstream image request failed: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Pass the upstream error body and status through unchanged.
		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return
	}

	var qresp QwenResponse
	if err := json.NewDecoder(resp.Body).Decode(&qresp); err != nil {
		proxy.WriteJSONError(w, http.StatusBadGateway, "decode upstream response failed: "+err.Error())
		return
	}

	out, err := ConvertResponse(r.Context(), &qresp, respFormat, s.downloader, s.cfg.ImageDownloadConcurrency)
	if err != nil {
		proxy.WriteJSONError(w, http.StatusBadGateway, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if qresp.RequestID != "" {
		w.Header().Set("X-Request-Id", qresp.RequestID)
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}
