package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultQwenBaseURL      = "https://token-plan.cn-beijing.maas.aliyuncs.com"
	defaultListenAddr       = ":8080"
	defaultQwenImageModel   = "qwen-image-2.0"
	defaultImageConcurrency = 4
	defaultImageMaxBytes    = 20 << 20          // 20 MiB
	defaultUpstreamTimeout  = 300 * time.Second // image generation can take 60s+, be generous
	defaultLogLevel         = "off"             // logging off by default for maximum performance
)

// Config holds all runtime settings, sourced from environment variables.
type Config struct {
	QwenAPIKey       string
	QwenBaseURL      string
	QwenTextBaseURL  string
	QwenImageBaseURL string

	ExposedAPIKey string
	ListenAddr    string

	QwenImageModel string
	ModelAliases   map[string]string

	ImageDownloadConcurrency int
	ImageMaxBytes            int64
	UpstreamTimeout          time.Duration
	LogLevel                 string
}

// Load reads the environment and validates required values.
func Load() (*Config, error) {
	cfg := &Config{ModelAliases: map[string]string{}}

	cfg.QwenBaseURL = strings.TrimRight(getenv("QWEN_BASE_URL", defaultQwenBaseURL), "/")
	cfg.QwenAPIKey = os.Getenv("QWEN_API_KEY")
	if cfg.QwenAPIKey == "" {
		return nil, fmt.Errorf("QWEN_API_KEY is required (Token Plan key, sk-sp- prefix)")
	}
	cfg.QwenTextBaseURL = strings.TrimRight(getenv("QWEN_TEXT_BASE_URL", cfg.QwenBaseURL+"/compatible-mode/v1"), "/")
	cfg.QwenImageBaseURL = getenv("QWEN_IMAGE_BASE_URL", cfg.QwenBaseURL+"/api/v1/services/aigc/multimodal-generation/generation")
	cfg.ExposedAPIKey = os.Getenv("EXPOSED_API_KEY")
	cfg.ListenAddr = getenv("LISTEN_ADDR", defaultListenAddr)
	cfg.QwenImageModel = getenv("QWEN_IMAGE_MODEL", defaultQwenImageModel)
	cfg.LogLevel = strings.ToLower(getenv("LOG_LEVEL", defaultLogLevel))

	var err error
	if cfg.ImageDownloadConcurrency, err = getenvInt("IMAGE_DOWNLOAD_CONCURRENCY", defaultImageConcurrency); err != nil {
		return nil, err
	}
	if cfg.ImageDownloadConcurrency < 1 {
		cfg.ImageDownloadConcurrency = 1
	}
	if cfg.ImageMaxBytes, err = getenvInt64("IMAGE_MAX_BYTES", defaultImageMaxBytes); err != nil {
		return nil, err
	}
	if cfg.ImageMaxBytes < 1 {
		cfg.ImageMaxBytes = defaultImageMaxBytes
	}
	if cfg.UpstreamTimeout, err = getenvDuration("UPSTREAM_TIMEOUT", defaultUpstreamTimeout); err != nil {
		return nil, err
	}

	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "MODEL_ALIAS_") {
			continue
		}
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimPrefix(parts[0], "MODEL_ALIAS_")
		if name != "" {
			cfg.ModelAliases[name] = parts[1]
		}
	}
	return cfg, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return n, nil
}

func getenvInt64(key string, def int64) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return n, nil
}

func getenvDuration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration (e.g. 180s): %w", key, err)
	}
	return d, nil
}
