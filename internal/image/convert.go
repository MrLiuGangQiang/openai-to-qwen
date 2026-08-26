// Package image converts between the OpenAI Images API and Qwen's
// multimodal-generation protocol.
package image

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"openai-to-qwen/internal/modelmap"
)

const maxN = 6

// openAIGenerationsRequest is the subset of POST /v1/images/generations we
// consume. Unknown fields are ignored on purpose.
type openAIGenerationsRequest struct {
	Model             string `json:"model"`
	Prompt            string `json:"prompt"`
	N                 *int   `json:"n"`
	Size              string `json:"size"`
	Quality           string `json:"quality"`
	Style             string `json:"style"`
	ResponseFormat    string `json:"response_format"`
	User              string `json:"user"`
	OutputFormat      string `json:"output_format"`
	Background        string `json:"background"`
	Moderation        string `json:"moderation"`
	OutputCompression *int   `json:"output_compression"`
	Thinking          string `json:"thinking"`
}

// droppedFields returns the OpenAI-only fields that were parsed but have no
// Qwen equivalent, so callers can surface them in logs instead of silently
// losing information.
func (r openAIGenerationsRequest) droppedFields() []string {
	var d []string
	if r.Style != "" {
		d = append(d, "style="+r.Style)
	}
	if r.User != "" {
		d = append(d, "user="+r.User)
	}
	if r.OutputFormat != "" {
		d = append(d, "output_format="+r.OutputFormat)
	}
	if r.Background != "" {
		d = append(d, "background="+r.Background)
	}
	if r.Moderation != "" {
		d = append(d, "moderation="+r.Moderation)
	}
	if r.OutputCompression != nil {
		d = append(d, fmt.Sprintf("output_compression=%d", *r.OutputCompression))
	}
	return d
}

// QwenRequest is the Qwen multimodal-generation request body.
type QwenRequest struct {
	Model      string         `json:"model"`
	Input      QwenInput      `json:"input"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

// QwenInput holds the message list.
type QwenInput struct {
	Messages []QwenMessage `json:"messages"`
}

// QwenMessage is a single-turn user message.
type QwenMessage struct {
	Role    string        `json:"role"`
	Content []QwenContent `json:"content"`
}

// QwenContent is one content item: text and/or image.
type QwenContent struct {
	Text  string `json:"text,omitempty"`
	Image string `json:"image,omitempty"`
}

// ConvertGenerations parses an OpenAI images/generations JSON body into a
// Qwen request. It returns the Qwen request, the requested response_format
// (used later for response conversion), and the names of OpenAI-only fields
// that have no Qwen equivalent (for logging; never nil).
func ConvertGenerations(body []byte, aliases map[string]string, fallbackModel string) (*QwenRequest, string, []string, error) {
	var req openAIGenerationsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, "", nil, fmt.Errorf("invalid request body: %w", err)
	}
	if req.Prompt == "" {
		return nil, "", nil, fmt.Errorf("prompt is required")
	}
	model := modelmap.ResolveImageModel(req.Model, aliases, fallbackModel)
	q := &QwenRequest{
		Model: model,
		Input: QwenInput{
			Messages: []QwenMessage{{
				Role:    "user",
				Content: []QwenContent{{Text: req.Prompt}},
			}},
		},
		Parameters: buildParameters(req.N, req.Size, req.Quality, req.Thinking, model),
	}
	return q, req.ResponseFormat, req.droppedFields(), nil
}

// buildParameters maps optional OpenAI fields into Qwen parameters.
func buildParameters(n *int, size, quality, thinking, model string) map[string]any {
	params := make(map[string]any, 4)
	if n != nil {
		params["n"] = clampN(*n)
	}
	if s := NormalizeSize(size); s != "" {
		params["size"] = s
	}
	switch quality {
	case "high":
		params["prompt_extend"] = true
	case "low":
		params["prompt_extend"] = false
	}
	// qwen-image-3.0 series exposes thinking mode as enable_thinking
	// (see the official Qwen Image Generation and Editing 3.0 API reference).
	if thinking != "" && strings.HasPrefix(model, "qwen-image-3.0") {
		params["enable_thinking"] = thinking != "off"
	}
	return params
}

// clampN clamps OpenAI's n (1..10) into Qwen's supported 1..6.
func clampN(n int) int {
	if n < 1 {
		return 1
	}
	if n > maxN {
		return maxN
	}
	return n
}

// NormalizeSize converts an OpenAI "WxH" size into Qwen "W*H".
// It returns "" when the size is missing or unparseable so Qwen uses its default.
func NormalizeSize(size string) string {
	s := strings.TrimSpace(size)
	if s == "" {
		return ""
	}
	sep := strings.IndexAny(s, "xX*")
	if sep <= 0 || sep == len(s)-1 {
		return ""
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(s[:sep]))
	h, err2 := strconv.Atoi(strings.TrimSpace(s[sep+1:]))
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return ""
	}
	return strconv.Itoa(w) + "*" + strconv.Itoa(h)
}

// QwenResponse is the Qwen multimodal-generation response body.
type QwenResponse struct {
	Output    QwenOutput      `json:"output"`
	Usage     json.RawMessage `json:"usage,omitempty"`
	RequestID string          `json:"request_id"`
}

// QwenOutput wraps the result choices.
type QwenOutput struct {
	Choices []QwenChoice `json:"choices"`
}

// QwenChoice is one result.
type QwenChoice struct {
	FinishReason string  `json:"finish_reason"`
	Message      QwenMsg `json:"message"`
}

// QwenMsg is the assistant message.
type QwenMsg struct {
	Role    string        `json:"role"`
	Content []QwenContent `json:"content"`
}

// OpenAIResponse is the OpenAI images/generations response shape.
//
// QwenUsage and RequestID are extra namespaced fields that preserve upstream
// information without breaking OpenAI clients: the OpenAI SDK type-checks the
// standard `usage` field (token-based) and would reject Qwen's pixel-based
// usage object, but unknown fields such as `qwen_usage` are ignored.
type OpenAIResponse struct {
	Created int64             `json:"created"`
	Data    []OpenAIImageData `json:"data"`
	// QwenUsage mirrors the upstream usage object verbatim, e.g.
	// {"width":1024,"height":1024,"image_count":1} for the 2.0 series or
	// {"output_width":...,"output_image_count":...} for the 3.0 series.
	QwenUsage json.RawMessage `json:"qwen_usage,omitempty"`
	// RequestID mirrors the upstream request_id so clients can trace calls
	// without relying on the X-Request-Id header alone.
	RequestID string `json:"request_id,omitempty"`
}

// OpenAIImageData is one generated image.
type OpenAIImageData struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// ConvertResponse turns a Qwen multimodal-generation response into an OpenAI
// images/generations response.
//   - responseFormat "" or "url": image URLs are returned directly (no download).
//   - responseFormat "b64_json": images are downloaded concurrently and
//     returned as base64 strings.
func ConvertResponse(ctx context.Context, qresp *QwenResponse, responseFormat string, f Fetcher, concurrency int) (*OpenAIResponse, error) {
	images := make([]string, 0, 2)
	for _, ch := range qresp.Output.Choices {
		for _, c := range ch.Message.Content {
			if c.Image != "" {
				images = append(images, c.Image)
			}
		}
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("upstream returned no images")
	}

	out := &OpenAIResponse{
		Created:   time.Now().Unix(),
		Data:      make([]OpenAIImageData, 0, len(images)),
		QwenUsage: qresp.Usage,
		RequestID: qresp.RequestID,
	}

	if responseFormat == "b64_json" {
		datas, err := fetchAll(ctx, images, f, concurrency)
		if err != nil {
			return nil, err
		}
		for _, data := range datas {
			out.Data = append(out.Data, OpenAIImageData{B64JSON: base64Encode(data)})
		}
		return out, nil
	}

	for _, u := range images {
		out.Data = append(out.Data, OpenAIImageData{URL: u})
	}
	return out, nil
}
