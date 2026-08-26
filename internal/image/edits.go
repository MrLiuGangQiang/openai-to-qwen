package image

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"openai-to-qwen/internal/modelmap"
)

const maxFormMemory = 32 << 20 // 32 MiB in-memory before spilling to disk

// ConvertEdits parses an OpenAI /v1/images/edits multipart request into a
// Qwen multimodal-generation request (image-to-image).
// Returns the Qwen request, the requested response_format, and the names of
// OpenAI-only fields that have no Qwen equivalent (for logging; never nil).
func ConvertEdits(r *http.Request, maxFileBytes int64, aliases map[string]string, fallbackModel string) (*QwenRequest, string, []string, error) {
	if err := r.ParseMultipartForm(maxFormMemory); err != nil {
		return nil, "", nil, fmt.Errorf("invalid multipart form: %w", err)
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	prompt := r.FormValue("prompt")
	if prompt == "" {
		return nil, "", nil, fmt.Errorf("prompt is required")
	}
	responseFormat := r.FormValue("response_format")

	content := make([]QwenContent, 0, 3)

	imageFH := firstFile(r, "image")
	if imageFH == nil {
		return nil, "", nil, fmt.Errorf("image file is required")
	}
	dataURL, err := fileToDataURL(imageFH, maxFileBytes)
	if err != nil {
		return nil, "", nil, err
	}
	content = append(content, QwenContent{Image: dataURL})

	if maskFH := firstFile(r, "mask"); maskFH != nil {
		maskURL, err := fileToDataURL(maskFH, maxFileBytes)
		if err != nil {
			return nil, "", nil, err
		}
		content = append(content, QwenContent{Image: maskURL})
	}
	content = append(content, QwenContent{Text: prompt})

	model := modelmap.ResolveImageModel(r.FormValue("model"), aliases, fallbackModel)

	q := &QwenRequest{
		Model: model,
		Input: QwenInput{
			Messages: []QwenMessage{{Role: "user", Content: content}},
		},
		Parameters: formParameters(r, model),
	}
	return q, responseFormat, droppedEditFields(r), nil
}

// droppedEditFields reports multipart fields that OpenAI accepts but Qwen has
// no equivalent for (for logging; never nil).
func droppedEditFields(r *http.Request) []string {
	var d []string
	for _, f := range []struct{ name, val string }{
		{"user", r.FormValue("user")},
		{"output_format", r.FormValue("output_format")},
		{"background", r.FormValue("background")},
		{"output_compression", r.FormValue("output_compression")},
	} {
		if f.val != "" {
			d = append(d, f.name+"="+f.val)
		}
	}
	return d
}

// formParameters maps multipart form fields (n, size, quality, thinking) into
// Qwen parameters.
func formParameters(r *http.Request, model string) map[string]any {
	params := make(map[string]any, 4)
	if n := r.FormValue("n"); n != "" {
		if v, err := strconv.Atoi(n); err == nil {
			params["n"] = clampN(v)
		}
	}
	if s := NormalizeSize(r.FormValue("size")); s != "" {
		params["size"] = s
	}
	switch r.FormValue("quality") {
	case "high":
		params["prompt_extend"] = true
	case "low":
		params["prompt_extend"] = false
	}
	if t := r.FormValue("thinking"); t != "" && strings.HasPrefix(model, "qwen-image-3.0") {
		params["enable_thinking"] = t != "off"
	}
	return params
}

// firstFile returns the first uploaded file for a multipart field, if any.
func firstFile(r *http.Request, field string) *multipart.FileHeader {
	if r.MultipartForm == nil || r.MultipartForm.File == nil {
		return nil
	}
	files := r.MultipartForm.File[field]
	if len(files) == 0 {
		return nil
	}
	return files[0]
}

// fileToDataURL reads an uploaded file and encodes it as a data URL, which is
// what Qwen accepts for image input.
func fileToDataURL(fh *multipart.FileHeader, maxBytes int64) (string, error) {
	f, err := fh.Open()
	if err != nil {
		return "", fmt.Errorf("open %s: %w", fh.Filename, err)
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", fh.Filename, err)
	}
	if int64(len(data)) > maxBytes {
		return "", fmt.Errorf("file %s exceeds max size of %d bytes", fh.Filename, maxBytes)
	}

	mime := fh.Header.Get("Content-Type")
	if mime == "" || !validMIME(mime) {
		mime = http.DetectContentType(data)
	}
	return "data:" + mime + ";base64," + base64Encode(data), nil
}

// validMIME reports whether the client-provided content type looks like an
// image MIME type; otherwise we sniff the bytes instead.
func validMIME(mime string) bool {
	if len(mime) < 6 || mime[:6] != "image/" {
		return false
	}
	for _, c := range mime[6:] {
		if !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && c != '-' && c != '+' && c != '.' {
			return false
		}
	}
	return true
}
