# syntax=docker/dockerfile:1.6
#
# App image: three static Go binaries (modernc.org/sqlite is pure-Go, no CGo).
# Embeds web/, migrations/, prompts/ via embed.go at compile time.
#
# Bundled binaries:
#   /app/serbian — the PWA server (default ENTRYPOINT)
#   /app/pregen  — task pre-generator: -import <file.json> or -kind <…> via Anthropic
#   /app/vapid   — one-shot VAPID key-pair generator for web-push setup

FROM golang:1.25-alpine AS builder
WORKDIR /src

# Cache deps separately from sources.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# CGO disabled — keeps the binaries fully static so /distroless/static works.
ENV CGO_ENABLED=0 GOOS=linux
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/serbian ./cmd/serbian && \
    go build -trimpath -ldflags="-s -w" -o /out/pregen  ./cmd/pregen  && \
    go build -trimpath -ldflags="-s -w" -o /out/vapid   ./cmd/vapid

# ---- runtime --------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot AS runtime
WORKDIR /app

# Data dir is bind-mounted by compose; declared here so the volume mount-point
# exists in the image and the distroless user can write to it.
COPY --from=builder --chown=nonroot:nonroot /out/serbian /app/serbian
COPY --from=builder --chown=nonroot:nonroot /out/pregen  /app/pregen
COPY --from=builder --chown=nonroot:nonroot /out/vapid   /app/vapid

USER nonroot:nonroot
EXPOSE 8089
VOLUME ["/app/data"]

ENTRYPOINT ["/app/serbian", "-config", "/app/data/config.json", "-backup-dir", "/app/data/backups"]
