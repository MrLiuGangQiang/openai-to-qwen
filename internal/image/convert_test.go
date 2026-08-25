package image

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"testing"
)

func TestNormalizeSize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"1024x1024", "1024*1024"},
		{"1792x1024", "1792*1024"},
		{"1024X1536", "1024*1536"},
		{"1536*1024", "1536*1024"},
		{" 1024 x 1024 ", "1024*1024"},
		{"", ""},
		{"abc", ""},
		{"1024", ""},
		{"1024x", ""},
		{"x1024", ""},
	}
	for _, c := range cases {
		if got := NormalizeSize(c.in); got != c.want {
			t.Errorf("NormalizeSize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestConvertGenerations(t *testing.T) {
	aliases := map[string]string{"gpt-image-1": "qwen-image-2.0-pro"}
	body := []byte(`{
		"model": "gpt-image-1",
		"prompt": "a red cat",
		"n": 9,
		"size": "1024x1536",
		"quality": "high",
		"thinking": "medium",
		"response_format": "b64_json"
	}`)
	q, respFmt, err := ConvertGenerations(body, aliases, "qwen-image-2.0")
	if err != nil {
		t.Fatal(err)
	}
	if q.Model != "qwen-image-2.0-pro" {
		t.Errorf("model = %q, want qwen-image-2.0-pro", q.Model)
	}
	msg := q.Input.Messages[0]
	if msg.Role != "user" {
		t.Errorf("role = %q, want user", msg.Role)
	}
	if len(msg.Content) != 1 || msg.Content[0].Text != "a red cat" {
		t.Errorf("content = %+v, want single text prompt", msg.Content)
	}
	if got := q.Parameters["n"]; got != 6 {
		t.Errorf("n = %v, want 6 (clamped)", got)
	}
	if got := q.Parameters["size"]; got != "1024*1536" {
		t.Errorf("size = %v, want 1024*1536", got)
	}
	if got := q.Parameters["prompt_extend"]; got != true {
		t.Errorf("prompt_extend = %v, want true", got)
	}
	// thinking maps only for qwen-image-3.0 targets
	if _, ok := q.Parameters["thinking"]; ok {
		t.Errorf("thinking should be dropped for non-3.0 model, got %v", q.Parameters["thinking"])
	}
	if respFmt != "b64_json" {
		t.Errorf("response_format = %q, want b64_json", respFmt)
	}
}

func TestConvertGenerationsQwenModelPassthrough(t *testing.T) {
	body := []byte(`{"model":"qwen-image-2.0","prompt":"x"}`)
	q, _, err := ConvertGenerations(body, nil, "qwen-image-2.0")
	if err != nil {
		t.Fatal(err)
	}
	if q.Model != "qwen-image-2.0" {
		t.Errorf("model = %q, want qwen-image-2.0 passthrough", q.Model)
	}
}

func TestConvertGenerationsMissingPrompt(t *testing.T) {
	if _, _, err := ConvertGenerations([]byte(`{"model":"dall-e-3"}`), nil, "qwen-image-2.0"); err == nil {
		t.Error("expected error for missing prompt")
	}
}

func TestConvertResponseURL(t *testing.T) {
	qresp := &QwenResponse{
		Output: QwenOutput{Choices: []QwenChoice{{
			FinishReason: "stop",
			Message: QwenMsg{Content: []QwenContent{
				{Image: "https://a/1.png"},
				{Image: "https://a/2.png"},
			}},
		}}},
		RequestID: "req-1",
	}
	d := NewDownloader(http.DefaultClient, 1<<20)
	out, err := ConvertResponse(context.Background(), qresp, "url", d, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Data) != 2 {
		t.Fatalf("len(data) = %d, want 2", len(out.Data))
	}
	if out.Data[0].URL != "https://a/1.png" || out.Data[0].B64JSON != "" {
		t.Errorf("data[0] = %+v, want URL only", out.Data[0])
	}
	if out.Data[1].URL != "https://a/2.png" {
		t.Errorf("data[1].url = %q", out.Data[1].URL)
	}
	if out.Created == 0 {
		t.Error("created must be set")
	}
}

type fakeFetcher struct{}

func (fakeFetcher) Fetch(_ context.Context, url string) ([]byte, error) {
	switch url {
	case "https://a/1.png":
		return []byte("img1"), nil
	case "https://a/2.png":
		return []byte("img2"), nil
	}
	return nil, fmt.Errorf("unexpected url %q", url)
}

func TestConvertResponseB64(t *testing.T) {
	qresp := &QwenResponse{
		Output: QwenOutput{Choices: []QwenChoice{{
			Message: QwenMsg{Content: []QwenContent{{Image: "https://a/1.png"}, {Image: "https://a/2.png"}}},
		}}},
	}
	out, err := ConvertResponse(context.Background(), qresp, "b64_json", fakeFetcher{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Data) != 2 {
		t.Fatalf("len(data) = %d, want 2", len(out.Data))
	}
	if out.Data[0].B64JSON != base64.StdEncoding.EncodeToString([]byte("img1")) {
		t.Errorf("b64_json[0] = %q", out.Data[0].B64JSON)
	}
	if out.Data[1].B64JSON != base64.StdEncoding.EncodeToString([]byte("img2")) {
		t.Errorf("b64_json[1] = %q", out.Data[1].B64JSON)
	}
	if out.Data[0].URL != "" {
		t.Error("url must be empty in b64_json mode")
	}
}

func TestConvertResponseNoImages(t *testing.T) {
	qresp := &QwenResponse{Output: QwenOutput{Choices: []QwenChoice{{Message: QwenMsg{}}}}}
	if _, err := ConvertResponse(context.Background(), qresp, "url", fakeFetcher{}, 1); err == nil {
		t.Error("expected error when upstream returns no images")
	}
}
