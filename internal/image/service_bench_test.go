package image

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"openai-to-qwen/internal/config"
	"openai-to-qwen/internal/logger"
	"openai-to-qwen/internal/proxy"
)

// BenchmarkHandleGenerations drives the full handler (parse -> convert ->
// upstream round trip -> response conversion) against an in-process upstream,
// with logging disabled (the default production setting).
func BenchmarkHandleGenerations(b *testing.B) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"choices":[{"message":{"content":[{"image":"https://cdn.example/1.png"}]}}]},"request_id":"bench"}`))
	}))
	defer upstream.Close()

	cfg := &config.Config{
		QwenImageBaseURL:         upstream.URL,
		QwenImageModel:           "qwen-image-2.0",
		ModelAliases:             map[string]string{},
		ImageDownloadConcurrency: 4,
		ImageMaxBytes:            20 << 20,
		UpstreamTimeout:          30 * time.Second,
		LogLevel:                 "off",
	}
	client := &http.Client{Timeout: 30 * time.Second, Transport: proxy.NewTransport(30 * time.Second)}
	svc := NewService(cfg, client, logger.New(logger.Parse("off"), io.Discard))

	small := `{"model":"gpt-image-1","prompt":"a cat","n":1,"size":"1024x1024"}`
	large := `{"model":"gpt-image-1","prompt":"` + strings.Repeat("x", 512<<10) + `","n":1}`

	for _, tc := range []struct{ name, body string }{
		{"small", small},
		{"512KB-prompt", large},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(tc.body)))
			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(tc.body))
				rec := httptest.NewRecorder()
				svc.HandleGenerations(rec, req)
				if rec.Code != http.StatusOK {
					b.Fatalf("status %d: %s", rec.Code, rec.Body.String())
				}
			}
		})
	}
}
