package image

import (
	"context"
	"net/http"
	"testing"
)

func BenchmarkConvertGenerations(b *testing.B) {
	body := []byte(`{"model":"gpt-image-1","prompt":"a cat on the moon","n":2,"size":"1024x1024","quality":"high","response_format":"url"}`)
	aliases := map[string]string{"gpt-image-1": "qwen-image-2.0-pro"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, _, err := ConvertGenerations(body, aliases, "qwen-image-2.0"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConvertResponseURL(b *testing.B) {
	qresp := &QwenResponse{
		Output: QwenOutput{Choices: []QwenChoice{{
			Message: QwenMsg{Content: []QwenContent{
				{Image: "https://a/1.png"},
				{Image: "https://a/2.png"},
			}},
		}}},
	}
	d := NewDownloader(http.DefaultClient, 1<<20)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := ConvertResponse(context.Background(), qresp, "url", d, 4); err != nil {
			b.Fatal(err)
		}
	}
}
