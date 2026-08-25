// Package modelmap resolves OpenAI-side model names to Qwen image models.
package modelmap

import "strings"

// ResolveImageModel maps an incoming model name to a Qwen image model.
// Precedence: explicit alias > Qwen-native model (pass through) > default fallback.
func ResolveImageModel(name string, aliases map[string]string, fallback string) string {
	if name == "" {
		return fallback
	}
	if m, ok := aliases[name]; ok && m != "" {
		return m
	}
	if IsQwenImageModel(name) {
		return name
	}
	return fallback
}

// IsQwenImageModel reports whether the model name belongs to Qwen's native
// multimodal-generation families and should be passed through unchanged.
func IsQwenImageModel(name string) bool {
	for _, p := range []string{"qwen-image", "wan", "z-image"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}
