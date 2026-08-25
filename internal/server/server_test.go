package server_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"openai-to-qwen/internal/config"
	"openai-to-qwen/internal/server"
)

func newTestConfig(textURL, imageURL, apiKey, exposed string) *config.Config {
	return &config.Config{
		QwenAPIKey:               apiKey,
		QwenTextBaseURL:          textURL,
		QwenImageBaseURL:         imageURL,
		ExposedAPIKey:            exposed,
		ListenAddr:               ":0",
		QwenImageModel:           "qwen-image-2.0",
		ModelAliases:             map[string]string{},
		ImageDownloadConcurrency: 4,
		ImageMaxBytes:            1 << 20,
		UpstreamTimeout:          10 * time.Second,
		LogLevel:                 "info",
	}
}

func newGateway(t *testing.T, cfg *config.Config) *httptest.Server {
	t.Helper()
	srv, err := server.New(cfg)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestHealthz(t *testing.T) {
	ts := newGateway(t, newTestConfig("http://text.invalid", "http://img.invalid", "sk-sp-test", ""))
	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestTextPassThrough(t *testing.T) {
	var gotPath, gotAuth atomic.Value
	var gotBody atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath.Store(r.URL.Path)
		gotAuth.Store(r.Header.Get("Authorization"))
		b, _ := io.ReadAll(r.Body)
		gotBody.Store(string(b))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"id":"chatcmpl-1","object":"chat.completion","choices":[]}`)
	}))
	defer upstream.Close()

	cfg := newTestConfig(upstream.URL+"/compatible-mode/v1", "http://img.invalid", "sk-sp-test", "")
	ts := newGateway(t, cfg)

	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"qwen3.8-max","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"id":"chatcmpl-1","object":"chat.completion","choices":[]}` {
		t.Errorf("body = %s", body)
	}
	if gotPath.Load() != "/compatible-mode/v1/chat/completions" {
		t.Errorf("upstream path = %v, want /compatible-mode/v1/chat/completions", gotPath.Load())
	}
	if gotAuth.Load() != "Bearer sk-sp-test" {
		t.Errorf("upstream auth = %v, want Bearer sk-sp-test", gotAuth.Load())
	}
	if !strings.Contains(gotBody.Load().(string), "qwen3.8-max") {
		t.Errorf("upstream body = %v", gotBody.Load())
	}
}

func TestImagesGenerations(t *testing.T) {
	var captured atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/services/aigc/multimodal-generation/generation" {
			t.Errorf("upstream path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-sp-test" {
			t.Errorf("auth = %q", got)
		}
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		captured.Store(m)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"output":{"choices":[{"finish_reason":"stop","message":{"content":[{"image":"https://img.example/1.png"}]}}]},"request_id":"req-abc"}`)
	}))
	defer upstream.Close()

	cfg := newTestConfig("http://text.invalid", upstream.URL+"/api/v1/services/aigc/multimodal-generation/generation", "sk-sp-test", "")
	ts := newGateway(t, cfg)

	resp, err := http.Post(ts.URL+"/v1/images/generations", "application/json",
		strings.NewReader(`{"model":"dall-e-3","prompt":"a cat","n":1,"size":"1024x1024","response_format":"url"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, b)
	}
	if got := resp.Header.Get("X-Request-Id"); got != "req-abc" {
		t.Errorf("X-Request-Id = %q", got)
	}

	m, _ := captured.Load().(map[string]any)
	if m == nil {
		t.Fatal("upstream request not captured")
	}
	if m["model"] != "dall-e-3" {
		t.Errorf("model = %v, want dall-e-3 (request model as-is)", m["model"])
	}
	in := m["input"].(map[string]any)
	msgs := in["messages"].([]any)
	content := msgs[0].(map[string]any)["content"].([]any)
	if content[0].(map[string]any)["text"] != "a cat" {
		t.Errorf("text = %v", content[0])
	}
	params := m["parameters"].(map[string]any)
	if params["size"] != "1024*1024" {
		t.Errorf("size = %v, want 1024*1024", params["size"])
	}

	var out struct {
		Created int64 `json:"created"`
		Data    []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Created == 0 {
		t.Error("created not set")
	}
	if len(out.Data) != 1 || out.Data[0].URL != "https://img.example/1.png" {
		t.Errorf("data = %+v", out.Data)
	}
}

func TestImagesGenerationsB64(t *testing.T) {
	imgBytes := []byte("fake-image-bytes-0123456789")
	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imgBytes)
	}))
	defer imgSrv.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"output":{"choices":[{"message":{"content":[{"image":%q}]}}]}}`, imgSrv.URL+"/img.png")
	}))
	defer upstream.Close()

	cfg := newTestConfig("http://text.invalid", upstream.URL+"/api/v1/services/aigc/multimodal-generation/generation", "sk-sp-test", "")
	ts := newGateway(t, cfg)

	resp, err := http.Post(ts.URL+"/v1/images/generations", "application/json",
		strings.NewReader(`{"model":"dall-e-3","prompt":"a cat","response_format":"b64_json"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, b)
	}

	var out struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	want := base64.StdEncoding.EncodeToString(imgBytes)
	if len(out.Data) != 1 || out.Data[0].B64JSON != want {
		t.Errorf("data = %+v, want b64_json %q", out.Data, want)
	}
	if out.Data[0].URL != "" {
		t.Error("url should be empty in b64_json mode")
	}
}

func TestImagesEdits(t *testing.T) {
	var captured atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		captured.Store(m)
		_, _ = io.WriteString(w, `{"output":{"choices":[{"message":{"content":[{"image":"https://img.example/out.png"}]}}]}}`)
	}))
	defer upstream.Close()

	cfg := newTestConfig("http://text.invalid", upstream.URL+"/api/v1/services/aigc/multimodal-generation/generation", "sk-sp-test", "")
	ts := newGateway(t, cfg)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("image", "cat.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte("\x89PNG\r\n\x1a\nfake"))
	_ = mw.WriteField("prompt", "make it red")
	_ = mw.WriteField("model", "dall-e-2")
	_ = mw.WriteField("size", "1024x1024")
	_ = mw.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/images/edits", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, b)
	}

	m, _ := captured.Load().(map[string]any)
	if m == nil {
		t.Fatal("upstream request not captured")
	}
	in := m["input"].(map[string]any)
	content := in["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("content len = %d, want 2 (image + text)", len(content))
	}
	img := content[0].(map[string]any)["image"].(string)
	if !strings.HasPrefix(img, "data:image/png;base64,") {
		t.Errorf("image data URL = %q", img)
	}
	text := content[1].(map[string]any)["text"].(string)
	if text != "make it red" {
		t.Errorf("text = %q", text)
	}
}

func TestAuthRequired(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()

	cfg := newTestConfig(upstream.URL, "http://img.invalid", "sk-sp-test", "sk-exposed")
	ts := newGateway(t, cfg)

	// no key -> 401
	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no-key status = %d, want 401", resp.StatusCode)
	}

	// correct key -> 200
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer sk-exposed")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("with-key status = %d, want 200", resp2.StatusCode)
	}

	// wrong key -> 401
	req3, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(`{}`))
	req3.Header.Set("Authorization", "Bearer wrong")
	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp3.Body)
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong-key status = %d, want 401", resp3.StatusCode)
	}
}

func TestImagesVariationsNotFound(t *testing.T) {
	cfg := newTestConfig("http://text.invalid", "http://img.invalid", "sk-sp-test", "")
	ts := newGateway(t, cfg)
	resp, err := http.Post(ts.URL+"/v1/images/variations", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}
