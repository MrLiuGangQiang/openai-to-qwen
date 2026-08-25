package server_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestTextStreaming verifies that SSE chunks are flushed incrementally through
// the gateway (the logging middleware must not break http.Flusher) and that the
// terminal [DONE] event is preserved end-to-end.
func TestTextStreaming(t *testing.T) {
	const chunks = 8
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		fl, _ := w.(http.Flusher)
		for i := 0; i < chunks; i++ {
			_, _ = fmt.Fprintf(w, "data: {\"id\":\"chunk-%d\"}\n\n", i)
			if fl != nil {
				fl.Flush()
			}
			time.Sleep(200 * time.Millisecond)
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		if fl != nil {
			fl.Flush()
		}
	}))
	defer upstream.Close()

	cfg := newTestConfig(upstream.URL+"/compatible-mode/v1", "http://img.invalid", "sk-sp-test", "")
	ts := newGateway(t, cfg)

	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"qwen3.8-max","messages":[{"role":"user","content":"hi"}],"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("content-type = %q, want text/event-stream", ct)
	}

	// The first chunk must arrive while the upstream is still streaming
	// (total upstream duration = 8*200ms = 1.6s). If the middleware breaks
	// http.Flusher, nothing arrives until the stream finishes.
	type result struct {
		data string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		buf := make([]byte, 2048)
		n, err := resp.Body.Read(buf)
		ch <- result{data: string(buf[:n]), err: err}
	}()
	var first result
	select {
	case first = <-ch:
		if first.err != nil {
			t.Fatalf("first read error: %v", first.err)
		}
		if !strings.Contains(first.data, "chunk-0") {
			t.Fatalf("first data = %q, want chunk-0", first.data)
		}
	case <-time.After(time.Second):
		t.Fatal("no data received while upstream is still streaming (Flusher broken?)")
	}

	// Drain the rest; the terminal [DONE] must be present.
	rest, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	all := first.data + string(rest) // first read already consumed chunk-0
	if !strings.Contains(all, "[DONE]") {
		t.Errorf("missing terminal [DONE], got: %q", all)
	}
	for i := 0; i < chunks; i++ {
		want := fmt.Sprintf("chunk-%d", i)
		if !strings.Contains(all, want) {
			t.Errorf("missing %s in stream", want)
		}
	}
}

// TestTextNonStreaming verifies plain JSON responses pass through untouched.
func TestTextNonStreaming(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chatcmpl-9","choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer upstream.Close()

	cfg := newTestConfig(upstream.URL+"/compatible-mode/v1", "http://img.invalid", "sk-sp-test", "")
	ts := newGateway(t, cfg)

	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"qwen3.8-max","messages":[{"role":"user","content":"hi"}],"stream":false}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"id":"chatcmpl-9","choices":[{"message":{"content":"ok"}}]}` {
		t.Errorf("body = %s", body)
	}
}
