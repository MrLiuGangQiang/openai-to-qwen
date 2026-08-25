# syntax=docker/dockerfile:1
# ---- build stage ----
FROM golang:1.24-alpine AS builder
WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/openai-to-qwen ./cmd/server

# ---- runtime stage ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
    && adduser -D -u 10001 -h /home/app app
USER app
WORKDIR /home/app
COPY --from=builder /out/openai-to-qwen /openai-to-qwen
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
ENTRYPOINT ["/openai-to-qwen"]