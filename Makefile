# SPDX-License-Identifier: GPL-2.0-only
VERSION ?= 0.1.0-dev
IMAGE ?= nulldevil/ps4-games-server
TAG ?= $(VERSION)
STABLE ?= 0
BUILDER ?= ps4-games-server-release
CONFIG ?= config.json
BINARY := bin/ps4-server
.PHONY: all build run test check fmt source-map release docker deploy logs down upload
all: build
build:
	CGO_ENABLED=0 go build -trimpath -ldflags '-s -w -X main.version=$(VERSION)' -o $(BINARY) ./cmd/ps4-server
run:
	go run ./cmd/ps4-server -config "$(CONFIG)"
test:
	go test ./...
check:
	@test -z "$$(gofmt -l cmd internal tools)" || (gofmt -l cmd internal tools; exit 1)
	go vet ./...
	go test ./...
	go test -race ./...
	go run ./tools/source-map.go -check
fmt:
	gofmt -w cmd internal tools
source-map:
	go run ./tools/source-map.go
release:
	go run ./tools/release -version $(VERSION)
docker:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(TAG) .
deploy:
	VERSION=$(VERSION) IMAGE=$(IMAGE) TAG=$(TAG) docker compose up -d --build --wait
logs:
	docker compose logs -f
down:
	docker compose down
upload:
	@test "$(TAG)" != latest || (echo 'Use a version tag; latest requires STABLE=1 in addition to a version tag'; exit 1)
	@docker buildx inspect "$(BUILDER)" >/dev/null 2>&1 || docker buildx create --name "$(BUILDER)" --driver docker-container
	@if [ "$(STABLE)" = 1 ]; then docker buildx build --builder "$(BUILDER)" --platform linux/amd64,linux/arm64 --build-arg VERSION=$(VERSION) -t $(IMAGE):$(TAG) -t $(IMAGE):latest --push .; else docker buildx build --builder "$(BUILDER)" --platform linux/amd64,linux/arm64 --build-arg VERSION=$(VERSION) -t $(IMAGE):$(TAG) --push .; fi
