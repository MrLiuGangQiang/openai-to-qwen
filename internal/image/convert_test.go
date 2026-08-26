package image

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
	q, respFmt, dropped, err := ConvertGenerations(body, aliases, "qwen-image-2.0")
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
	// thinking maps only for qwen-image-3.0 targets (as enable_thinking)
	if _, ok := q.Parameters["thinking"]; ok {
		t.Errorf("thinking should be dropped for non-3.0 model, got %v", q.Parameters["thinking"])
	}
	if _, ok := q.Parameters["enable_thinking"]; ok {
		t.Errorf("enable_thinking should be dropped for non-3.0 model, got %v", q.Parameters["enable_thinking"])
	}
	if respFmt != "b64_json" {
		t.Errorf("response_format = %q, want b64_json", respFmt)
	}
	if len(dropped) != 0 {
		t.Errorf("dropped = %v, want none", dropped)
	}
}

func TestConvertGenerationsEnableThinking(t *testing.T) {
	// qwen-image-3.0 series maps the thinking switch to enable_thinking,
	// matching the official API reference.
	body := []byte(`{"model":"qwen-image-3.0-pro","prompt":"a cat","thinking":"on"}`)
	q, _, _, err := ConvertGenerations(body, nil, "qwen-image-2.0")
	if err != nil {
		t.Fatal(err)
	}
	if got := q.Parameters["enable_thinking"]; got != true {
		t.Errorf("enable_thinking = %v, want true", got)
	}

	body = []byte(`{"model":"qwen-image-3.0","prompt":"a cat","thinking":"off"}`)
	q, _, _, err = ConvertGenerations(body, nil, "qwen-image-2.0")
	if err != nil {
		t.Fatal(err)
	}
	if got := q.Parameters["enable_thinking"]; got != false {
		t.Errorf("enable_thinking = %v, want false", got)
	}
}

func TestConvertGenerationsDroppedFields(t *testing.T) {
	// OpenAI-only fields with no Qwen equivalent must be reported, not silently
	// dropped, so the gateway can log them.
	body := []byte(`{
		"model":"gpt-image-1","prompt":"a cat",
		"style":"vivid","user":"u-1","output_format":"webp",
		"background":"transparent","moderation":"low","output_compression":50
	}`)
	_, _, dropped, err := ConvertGenerations(body, nil, "qwen-image-2.0")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"style=vivid", "user=u-1", "output_format=webp", "background=transparent", "moderation=low", "output_compression=50"}
	if len(dropped) != len(want) {
		t.Fatalf("dropped = %v, want %v", dropped, want)
	}
	for i := range want {
		if dropped[i] != want[i] {
			t.Errorf("dropped[%d] = %q, want %q", i, dropped[i], want[i])
		}
	}
}

func TestConvertGenerationsQwenModelPassthrough(t *testing.T) {
	body := []byte(`{"model":"qwen-image-2.0","prompt":"x"}`)
	q, _, _, err := ConvertGenerations(body, nil, "qwen-image-2.0")
	if err != nil {
		t.Fatal(err)
	}
	if q.Model != "qwen-image-2.0" {
		t.Errorf("model = %q, want qwen-image-2.0 passthrough", q.Model)
	}
}

func TestConvertGenerationsUsesRequestModel(t *testing.T) {
	// The model from the request is passed through as-is, no mapping/fallback.
	body := []byte(`{"model":"wan2.7-image","prompt":"a cat"}`)
	q, _, _, err := ConvertGenerations(body, nil, "qwen-image-2.0")
	if err != nil {
		t.Fatal(err)
	}
	if q.Model != "wan2.7-image" {
		t.Errorf("model = %q, want wan2.7-image (request model as-is)", q.Model)
	}
}

func TestConvertGenerationsFallbackWhenNoModel(t *testing.T) {
	// Empty model falls back to the configured default.
	body := []byte(`{"prompt":"a cat"}`)
	q, _, _, err := ConvertGenerations(body, nil, "qwen-image-2.0")
	if err != nil {
		t.Fatal(err)
	}
	if q.Model != "qwen-image-2.0" {
		t.Errorf("model = %q, want qwen-image-2.0 (fallback)", q.Model)
	}
}

func TestConvertGenerationsMissingPrompt(t *testing.T) {
	if _, _, _, err := ConvertGenerations([]byte(`{"model":"dall-e-3"}`), nil, "qwen-image-2.0"); err == nil {
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
	if out.RequestID != "req-1" {
		t.Errorf("request_id = %q, want req-1", out.RequestID)
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

func TestConvertResponsePreservesQwenUsage(t *testing.T) {
	// The upstream usage object (shape varies by model series) must be echoed
	// verbatim so no usage information is lost.
	usage2 := json.RawMessage(`{"width":1024,"height":1024,"image_count":2}`)
	qresp := &QwenResponse{
		Output: QwenOutput{Choices: []QwenChoice{{
			Message: QwenMsg{Content: []QwenContent{{Image: "https://a/1.png"}}},
		}}},
		Usage:     usage2,
		RequestID: "req-usage-2",
	}
	out, err := ConvertResponse(context.Background(), qresp, "url", fakeFetcher{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if string(out.QwenUsage) != string(usage2) {
		t.Errorf("qwen_usage = %s, want %s", out.QwenUsage, usage2)
	}
	if out.RequestID != "req-usage-2" {
		t.Errorf("request_id = %q, want req-usage-2", out.RequestID)
	}

	// 3.0 series usage shape must pass through unchanged as well.
	usage3 := json.RawMessage(`{"output_width":2048,"output_height":2048,"output_image_count":1,"input_image_count":0}`)
	qresp.Usage = usage3
	out, err = ConvertResponse(context.Background(), qresp, "b64_json", fakeFetcher{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if string(out.QwenUsage) != string(usage3) {
		t.Errorf("qwen_usage = %s, want %s", out.QwenUsage, usage3)
	}
}

func TestConvertResponseNoImages(t *testing.T) {
	qresp := &QwenResponse{Output: QwenOutput{Choices: []QwenChoice{{Message: QwenMsg{}}}}}
	if _, err := ConvertResponse(context.Background(), qresp, "url", fakeFetcher{}, 1); err == nil {
		t.Error("expected error when upstream returns no images")
	}
}
