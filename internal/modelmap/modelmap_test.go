package modelmap

import "testing"

func TestResolveImageModel(t *testing.T) {
	aliases := map[string]string{"gpt-image-1": "qwen-image-2.0-pro"}

	cases := []struct {
		name     string
		want     string
		fallback string
	}{
		{"qwen-image-2.0", "qwen-image-2.0", "qwen-image-2.0"},
		{"wan2.7-image", "wan2.7-image", "qwen-image-2.0"},
		{"dall-e-3", "dall-e-3", "qwen-image-2.0"},
		{"gpt-image-1", "qwen-image-2.0-pro", "qwen-image-2.0"}, // alias wins
		{"", "qwen-image-2.0", "qwen-image-2.0"},                // empty -> fallback
	}
	for _, c := range cases {
		if got := ResolveImageModel(c.name, aliases, c.fallback); got != c.want {
			t.Errorf("ResolveImageModel(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestResolveImageModelNoAliases(t *testing.T) {
	// With no aliases configured, the request model is passed through unchanged.
	if got := ResolveImageModel("qwen-image-2.0-pro", nil, "qwen-image-2.0"); got != "qwen-image-2.0-pro" {
		t.Errorf("got %q, want qwen-image-2.0-pro", got)
	}
}
