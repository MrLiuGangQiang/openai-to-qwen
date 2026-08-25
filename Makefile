GO ?= go

.PHONY: build run test bench docker docker-run clean

build:
	$(GO) build -trimpath -ldflags="-s -w" -o bin/openai-to-qwen ./cmd/server

run:
	$(GO) run ./cmd/server

test:
	$(GO) test ./...

bench:
	$(GO) test -bench=. -benchmem ./internal/image/

docker:
	docker build -t openai-to-qwen:latest .

docker-run:
	docker compose up -d

clean:
	rm -rf bin