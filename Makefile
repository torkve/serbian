GO_BIN ?= go
PORT ?= 8089

# Docker knobs (only used by the docker-* targets).
DOCKER ?= docker
COMPOSE ?= $(DOCKER) compose
WHISPER_MODEL ?= medium
SERBIAN_PORT ?= $(PORT)
GPU ?= 0

# Layer the cuda overlay file when GPU=1.
COMPOSE_FILES := -f docker-compose.yml
ifeq ($(GPU),1)
COMPOSE_FILES += -f docker-compose.cuda.yml
endif
# Layer a local nginx (subpath + TLS) in front of the app when WITH_NGINX=1.
WITH_NGINX ?= 0
ifeq ($(WITH_NGINX),1)
COMPOSE_FILES += -f docker-compose.nginx.yml
endif

.PHONY: all build run backup pregen vapid fmt vet test tidy clean \
        docker-build docker-up docker-down docker-restart docker-logs \
        docker-ps docker-shell docker-clean docker-import whisper-model \
        nginx-cert

all: build

build: bin/serbian bin/pregen bin/vapid

bin/serbian: $(shell find cmd/serbian internal web migrations prompts -type f 2>/dev/null)
	@mkdir -p bin
	$(GO_BIN) build -o bin/serbian ./cmd/serbian

bin/pregen: $(shell find cmd/pregen internal prompts -type f 2>/dev/null)
	@mkdir -p bin
	$(GO_BIN) build -o bin/pregen ./cmd/pregen

bin/vapid: $(shell find cmd/vapid -type f 2>/dev/null)
	@mkdir -p bin
	$(GO_BIN) build -o bin/vapid ./cmd/vapid

run: bin/serbian
	./bin/serbian -addr :$(PORT)

backup: bin/serbian
	./bin/serbian -backup

pregen: bin/pregen
	./bin/pregen $(ARGS)

vapid: bin/vapid
	./bin/vapid

fmt:
	$(GO_BIN) fmt ./...

vet:
	$(GO_BIN) vet ./...

test:
	$(GO_BIN) test ./...

tidy:
	$(GO_BIN) mod tidy

clean:
	rm -rf bin

# ---- Docker ---------------------------------------------------------------
# Two-container stack: the Go PWA + a whisper.cpp STT sidecar. Both images
# are built locally. The host's pinned $(GO_BIN) is NOT used inside the
# containers — the app image uses golang:1.25-alpine internally for build
# reproducibility.
#
# Typical first-time flow:
#   make whisper-model        # ~1.5 GB download into ./models/
#   make docker-build         # build both images (~5-10 min first time)
#   make docker-up            # start both services in the background
#   make docker-logs          # find the /setup?token=... link in the output
#
# Switch to a CUDA whisper build:
#   make docker-build GPU=1
#   make docker-up GPU=1
#
# Switch model size:
#   make whisper-model WHISPER_MODEL=large-v3

# Fetch a whisper ggml model into ./models/ on the host.
whisper-model:
	WHISPER_MODEL=$(WHISPER_MODEL) ./scripts/fetch-whisper-model.sh

# Implicit dependency: docker-up triggers a download if the model is missing.
models/ggml-$(WHISPER_MODEL).bin:
	$(MAKE) whisper-model WHISPER_MODEL=$(WHISPER_MODEL)

docker-build:
	$(COMPOSE) $(COMPOSE_FILES) build

docker-up: models/ggml-$(WHISPER_MODEL).bin
	SERBIAN_PORT=$(SERBIAN_PORT) UID=$$(id -u) GID=$$(id -g) \
	    $(COMPOSE) $(COMPOSE_FILES) up -d
	@echo
	@echo "Running. Tail logs to find the setup link:"
	@echo "  make docker-logs"

docker-down:
	$(COMPOSE) $(COMPOSE_FILES) down

docker-restart:
	$(COMPOSE) $(COMPOSE_FILES) restart

docker-logs:
	$(COMPOSE) $(COMPOSE_FILES) logs -f --tail=200

docker-ps:
	$(COMPOSE) $(COMPOSE_FILES) ps

# Open a shell inside the app container. NB: the default app image is
# distroless and has no shell — override the image or rebuild with an alpine
# base if you need this often. Whisper's runtime image has /bin/sh.
docker-shell:
	$(COMPOSE) $(COMPOSE_FILES) exec whisper /bin/sh

# One-shot task import inside the app container. Usage:
#   make docker-import FILE=path/to/batch.json
# The file is copied into ./data/imports/ first so the existing data bind
# mount makes it visible inside the container.
docker-import:
	@if [ -z "$(FILE)" ]; then \
		echo "usage: make docker-import FILE=path/to/batch.json"; exit 2; \
	fi
	@mkdir -p data/imports
	@cp "$(FILE)" data/imports/
	$(COMPOSE) $(COMPOSE_FILES) run --rm --entrypoint /app/pregen app \
		-config /app/data/config.json \
		-import /app/data/imports/$$(basename "$(FILE)")

# Remove containers and locally-built images. Leaves ./data and ./models on
# the host so nothing is lost.
docker-clean:
	$(COMPOSE) $(COMPOSE_FILES) down --rmi local --remove-orphans

# Generate a self-signed cert for the local nginx overlay. One-time setup;
# re-uses the existing files if both already exist.
nginx-cert: docker/nginx/certs/server.crt
docker/nginx/certs/server.crt:
	@mkdir -p docker/nginx/certs
	openssl req -x509 -nodes -newkey rsa:2048 -days 365 \
		-subj "/CN=serbian-local" \
		-addext "subjectAltName=DNS:localhost,IP:127.0.0.1" \
		-keyout docker/nginx/certs/server.key \
		-out docker/nginx/certs/server.crt
	@chmod 600 docker/nginx/certs/server.key
	@echo
	@echo "Self-signed cert written to docker/nginx/certs/server.crt"
	@echo "  → for desktop testing: click past the browser warning"
	@echo "  → for iPhone testing: install docker/nginx/certs/server.crt as trusted"
