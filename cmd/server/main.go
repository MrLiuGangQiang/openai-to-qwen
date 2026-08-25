// Command server runs the OpenAI 鈫?Qwen Token Plan protocol gateway.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"openai-to-qwen/internal/config"
	"openai-to-qwen/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}
	if cfg.ExposedAPIKey == "" {
		log.Println("WARNING: EXPOSED_API_KEY is not set, authentication is disabled")
	}

	srv, err := server.New(cfg)
	if err != nil {
		log.Fatalf("server init error: %v", err)
	}

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	if cfg.LogLevel == "off" {
		log.Println("access/upstream logging disabled (LOG_LEVEL=off); set LOG_LEVEL=info or debug to enable")
	}

	go func() {
		log.Printf("openai-to-qwen listening on %s", cfg.ListenAddr)
		log.Printf("  text upstream : %s", cfg.QwenTextBaseURL)
		log.Printf("  image upstream: %s", cfg.QwenImageBaseURL)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
}
