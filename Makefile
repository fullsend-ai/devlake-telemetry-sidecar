BINARY  := bin/devlake-telemetry-sidecar
# Existing image repository retained for deployment compatibility.
IMAGE   ?= quay.io/dprod/devlake-sidecar
TAG     ?= latest
GO      := go
GOFLAGS ?=
GOLANGCI_LINT ?= golangci-lint

METRICS_PORT ?= 9090

.PHONY: build test vet fmt-check lint clean image push query run

build: ## Build the exporter binary
	$(GO) build $(GOFLAGS) -buildvcs=false -o $(BINARY) ./cmd/devlake-telemetry-sidecar/

test: ## Run all tests
	$(GO) test ./... -race -count=1

vet: ## Run go vet
	$(GO) vet ./...

fmt-check: ## Check that all files are gofmt'd
	@test -z "$$(gofmt -l .)" || { gofmt -l . ; echo "above files are not gofmt'd"; exit 1; }

lint: ## Run golangci-lint (must be built with a Go version >= go.mod)
	$(GOLANGCI_LINT) run

image: ## Build container image
	podman build -t $(IMAGE):$(TAG) .

push: image ## Build and push container image to the selected repository
	podman push $(IMAGE):$(TAG)

query: ## Query the running sidecar's endpoints
	@echo "=== /health ==="
	@curl -sf http://localhost:$(METRICS_PORT)/health | jq .
	@echo "\n=== /metrics (devlake_* only) ==="
	@curl -sf http://localhost:$(METRICS_PORT)/metrics | grep ^devlake_

run: build ## Build and run the sidecar locally using environment variables
	./$(BINARY)

clean: ## Remove build artifacts
	rm -rf bin/

help: ## Show this help
	@grep -E '^[a-z_-]+:.*## ' $(MAKEFILE_LIST) | awk -F ':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'
