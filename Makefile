.PHONY: all build run setup test test-unit test-verbose test-coverage test-integration \
        test-e2e test-e2e-run test-all lint fmt tidy \
        docker-build docker-push helm-lint helm-template clean vuln mocks help

# Variables
BINARY_NAME    = telekube
BUILD_DIR      = bin
CMD_DIR        = cmd/telekube
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
REGISTRY      ?= ghcr.io/leanhdoan
IMAGE_NAME    ?= telekube
LDFLAGS        = -ldflags "-s -w -X github.com/d9042n/telekube/pkg/version.Version=$(VERSION)"

# ── Build ──────────────────────────────────────────────────
build:
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)
	@echo "  Binary: $(BUILD_DIR)/$(BINARY_NAME)"

# ── Run ────────────────────────────────────────────────────
run: build
	./$(BUILD_DIR)/$(BINARY_NAME) serve --config configs/config.yaml

# ── Setup (interactive wizard) ─────────────────────────────
setup: build
	./$(BUILD_DIR)/$(BINARY_NAME) setup

# ── Tests ──────────────────────────────────────────────────
test:
	go test ./... -race -cover -count=1 -timeout=5m

# Layer 1+2: unit tests — no Docker required
test-unit:
	go test ./internal/... ./pkg/... -race -count=1 -timeout=5m

test-verbose:
	go test ./... -race -cover -count=1 -v -timeout=5m

test-coverage:
	go test ./... -race -coverprofile=coverage.out -count=1 -timeout=5m
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-integration:
	docker compose -f test/docker-compose.yaml up -d
	@echo "Waiting for services to be healthy..."
	@sleep 5
	go test ./test/integration/... -v -tags=integration -timeout=10m
	docker compose -f test/docker-compose.yaml down

# Layer 3: E2E tests — requires Docker (k3s auto start/stop)
test-e2e:
	go test ./test/e2e/... -v -tags=e2e -timeout=20m -count=1

# Fast smoke E2E tests — NO Docker/k3s required; only fake Telegram + real bot
test-e2e-smoke:
	E2E_SKIP_CLUSTER=true go test ./test/e2e/... -v -tags=e2e -timeout=5m -count=1

# Run a single E2E test: make test-e2e-run TEST=TestE2E_RBAC_UnknownUserBlocked
test-e2e-run:
	go test ./test/e2e/... -v -tags=e2e -run $(TEST) -timeout=10m

# All layers
test-all: test-unit test-integration test-e2e

# ── Lint ───────────────────────────────────────────────────
lint:
	golangci-lint run ./...

# ── Format ─────────────────────────────────────────────────
fmt:
	go fmt ./...
	goimports -w .

# ── Tidy ───────────────────────────────────────────────────
tidy:
	go mod tidy

# ── Vulnerability Check ────────────────────────────────────
vuln:
	@which govulncheck > /dev/null 2>&1 || go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

# ── Generate Mocks ─────────────────────────────────────────
mocks:
	go generate ./...

# ── Docker ─────────────────────────────────────────────────
docker-build:
	docker build \
		-f deploy/docker/Dockerfile \
		-t $(REGISTRY)/$(IMAGE_NAME):$(VERSION) \
		--build-arg VERSION=$(VERSION) \
		.

docker-push: docker-build
	docker push $(REGISTRY)/$(IMAGE_NAME):$(VERSION)

docker-run:
	docker run --rm -it \
		-e TELEKUBE_TELEGRAM_TOKEN=$${TELEKUBE_TELEGRAM_TOKEN} \
		-e TELEKUBE_TELEGRAM_ADMIN_IDS=$${TELEKUBE_TELEGRAM_ADMIN_IDS} \
		$(REGISTRY)/$(IMAGE_NAME):$(VERSION)

# ── Helm ───────────────────────────────────────────────────
helm-lint:
	helm lint deploy/helm/telekube

helm-template:
	helm template telekube deploy/helm/telekube

# ── Clean ──────────────────────────────────────────────────
clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# ── Help ───────────────────────────────────────────────────
help:
	@echo "Telekube — Makefile targets:"
	@echo ""
	@echo "  build              Build the binary (VERSION=$(VERSION))"
	@echo "  run                Build and run with config file"
	@echo "  setup              Run interactive setup wizard"
	@echo "  test               Run all tests with race detection"
	@echo "  test-unit          Run unit tests only (no Docker required)"
	@echo "  test-verbose       Run tests with verbose output"
	@echo "  test-coverage      Run tests and generate HTML coverage report"
	@echo "  test-integration   Run integration tests (requires Docker)"
	@echo "  test-e2e           Run E2E tests (requires Docker)"
	@echo "  test-e2e-run       Run a single E2E test: make test-e2e-run TEST=TestName"
	@echo "  test-all           Run all layers: unit + integration + e2e"
	@echo "  lint               Run golangci-lint"
	@echo "  fmt                Format code with gofmt and goimports"
	@echo "  tidy               Tidy go modules"
	@echo "  vuln               Run govulncheck for vulnerabilities"
	@echo "  mocks              Regenerate all mocks (go generate)"
	@echo "  docker-build       Build Docker image"
	@echo "  docker-push        Build and push Docker image"
	@echo "  docker-run         Run the Docker image locally"
	@echo "  helm-lint          Lint the Helm chart"
	@echo "  helm-template      Template the Helm chart (dry-run)"
	@echo "  clean              Remove build artifacts"
	@echo ""
	@echo "  Variables:"
	@echo "    VERSION=$(VERSION)"
	@echo "    REGISTRY=$(REGISTRY)"
	@echo "    IMAGE_NAME=$(IMAGE_NAME)"
