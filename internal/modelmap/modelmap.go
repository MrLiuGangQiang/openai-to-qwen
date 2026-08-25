// Package modelmap resolves the upstream image model for a request.
package modelmap

// ResolveImageModel returns the Qwen image model to call for an incoming
// request. The model name from the request is used as-is (no mapping), so the
// client is expected to send a real Qwen image model name (e.g. qwen-image-2.0,
// wan2.7-image). An explicit alias (MODEL_ALIAS_<name>) still takes precedence
// when configured; fallback is only used when the request carries no model.
func ResolveImageModel(name string, aliases map[string]string, fallback string) string {
	if name != "" {
		if m, ok := aliases[name]; ok && m != "" {
			return m
		}
		return name
	}
	return fallback
}
