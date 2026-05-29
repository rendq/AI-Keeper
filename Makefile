# AI-Keeper mono-repo Makefile.
#
# Verification entry-points required by `tasks.md` 1.1:
#   make bootstrap   -> install dev toolchain (gofumpt / ruff / eslint / yamllint / kubeconform / pre-commit)
#   make lint        -> run all linters across Go / Python / YAML / Node / k8s manifests
#
# Subsequent tasks add: manifests, kind-up, ci-local, audit-e2e, e2e-up, ...

SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

# ---------- Pinned tool versions -------------------------------------------------
GO              ?= go
GOFUMPT_VERSION ?= v0.6.0
KUBECONFORM_VERSION ?= v0.6.7
CONTROLLER_GEN_VERSION ?= v0.16.5
KUSTOMIZE_VERSION ?= v5.4.3
BUF_VERSION     ?= v1.45.0
PROTOC_GEN_GO_VERSION       ?= v1.34.2
PROTOC_GEN_GO_GRPC_VERSION  ?= v1.5.1
HELM_VERSION    ?= v3.16.2

# ---------- kind / local cluster --------------------------------------------------
KIND_CLUSTER_NAME ?= aik-dev
KIND_CONFIG       ?= hack/kind/kind-cluster.yaml
KIND_REGISTRY_NAME ?= kind-registry
KIND_REGISTRY_PORT ?= 5001

# ---------- Local tool dirs ------------------------------------------------------
ROOT_DIR := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
LOCAL_BIN := $(ROOT_DIR)/.local-tools/bin
export PATH := $(LOCAL_BIN):$(shell go env GOPATH 2>/dev/null)/bin:$(PATH)

# ---------- Helpers --------------------------------------------------------------
.PHONY: help
help: ## Show this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage: make \033[36m<target>\033[0m\n\nTargets:\n"} \
	  /^[a-zA-Z0-9_.-]+:.*?##/ { printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

$(LOCAL_BIN):
	@mkdir -p $(LOCAL_BIN)

# ---------- Bootstrap ------------------------------------------------------------
.PHONY: bootstrap
bootstrap: bootstrap-go bootstrap-proto bootstrap-python bootstrap-node bootstrap-k8s bootstrap-precommit ## Install the full dev toolchain.
	@echo "✅ bootstrap complete"

.PHONY: bootstrap-go
bootstrap-go: $(LOCAL_BIN) ## Install Go dev tooling (gofumpt / controller-gen / kustomize).
	@echo "→ installing Go tooling"
	GOBIN=$(LOCAL_BIN) $(GO) install mvdan.cc/gofumpt@$(GOFUMPT_VERSION)
	GOBIN=$(LOCAL_BIN) $(GO) install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)
	GOBIN=$(LOCAL_BIN) $(GO) install sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION)

.PHONY: bootstrap-proto
bootstrap-proto: $(LOCAL_BIN) ## Install buf + protoc-gen-go + protoc-gen-go-grpc.
	@echo "→ installing proto tooling"
	@if [ ! -x $(LOCAL_BIN)/buf ]; then \
	  set -e; \
	  os=$$(uname -s); arch=$$(uname -m); \
	  case "$$arch" in x86_64) arch=x86_64;; aarch64|arm64) arch=arm64;; esac; \
	  url="https://github.com/bufbuild/buf/releases/download/$(BUF_VERSION)/buf-$$os-$$arch"; \
	  echo "  ↳ downloading $$url"; \
	  curl -fsSL "$$url" -o $(LOCAL_BIN)/buf; \
	  chmod +x $(LOCAL_BIN)/buf; \
	fi
	$(LOCAL_BIN)/buf --version
	GOBIN=$(LOCAL_BIN) $(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	GOBIN=$(LOCAL_BIN) $(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)

.PHONY: bootstrap-python
bootstrap-python: ## Install Python dev tooling via uv.
	@echo "→ installing Python tooling (uv)"
	@command -v uv >/dev/null 2>&1 || { echo "uv not found. Install from https://docs.astral.sh/uv/" >&2; exit 1; }
	uv sync --extra dev

.PHONY: bootstrap-node
bootstrap-node: ## Install Node dev tooling via pnpm.
	@echo "→ installing Node tooling (pnpm)"
	@command -v pnpm >/dev/null 2>&1 || { echo "pnpm not found. Install from https://pnpm.io/installation" >&2; exit 1; }
	pnpm install --frozen-lockfile=false

.PHONY: bootstrap-k8s
bootstrap-k8s: $(LOCAL_BIN) ## Install kubeconform + helm.
	@echo "→ installing kubeconform"
	@if [ ! -x $(LOCAL_BIN)/kubeconform ]; then \
	  set -e; \
	  os=$$(uname -s | tr '[:upper:]' '[:lower:]'); \
	  arch=$$(uname -m); case "$$arch" in x86_64) arch=amd64;; aarch64|arm64) arch=arm64;; esac; \
	  url="https://github.com/yannh/kubeconform/releases/download/$(KUBECONFORM_VERSION)/kubeconform-$$os-$$arch.tar.gz"; \
	  echo "  ↳ downloading $$url"; \
	  tmp=$$(mktemp -d); \
	  curl -fsSL "$$url" | tar -xz -C "$$tmp"; \
	  mv "$$tmp/kubeconform" $(LOCAL_BIN)/kubeconform; \
	  rm -rf "$$tmp"; \
	fi
	$(LOCAL_BIN)/kubeconform -v
	@echo "→ installing helm"
	@if [ ! -x $(LOCAL_BIN)/helm ]; then \
	  set -e; \
	  os=$$(uname -s | tr '[:upper:]' '[:lower:]'); \
	  arch=$$(uname -m); case "$$arch" in x86_64) arch=amd64;; aarch64|arm64) arch=arm64;; esac; \
	  url="https://get.helm.sh/helm-$(HELM_VERSION)-$$os-$$arch.tar.gz"; \
	  echo "  ↳ downloading $$url"; \
	  tmp=$$(mktemp -d); \
	  curl -fsSL "$$url" | tar -xz -C "$$tmp"; \
	  mv "$$tmp/$$os-$$arch/helm" $(LOCAL_BIN)/helm; \
	  rm -rf "$$tmp"; \
	fi
	$(LOCAL_BIN)/helm version --short

.PHONY: bootstrap-precommit
bootstrap-precommit: ## Install pre-commit git hooks (skipped if not a git repo).
	@echo "→ installing pre-commit hooks"
	@if [ -d .git ]; then \
	  uv run pre-commit install --install-hooks; \
	else \
	  echo "  (no .git directory — pre-commit tool installed, hooks skipped. Run 'git init && make bootstrap-precommit' to enable.)"; \
	fi

# ---------- Lint -----------------------------------------------------------------
.PHONY: lint
lint: lint-go lint-python lint-yaml lint-node lint-k8s ## Run every linter.
	@echo "✅ lint complete"

.PHONY: lint-go
lint-go: ## Run gofumpt + go vet on all Go packages.
	@echo "→ go vet"
	$(GO) vet ./...
	@echo "→ gofumpt"
	@if ! command -v gofumpt >/dev/null 2>&1 && [ ! -x $(LOCAL_BIN)/gofumpt ]; then \
	  echo "gofumpt not found. Run 'make bootstrap-go' first." >&2; exit 1; \
	fi
	@# Restrict gofumpt to first-party Go sources; exclude vendored / non-module
	@# / generated stubs.
	@files=$$(find api controllers internal dataplane proto cmd -type f -name '*.go' \
	  ! -name 'zz_generated.*.go' \
	  ! -name '*.pb.go' \
	  ! -name '*_grpc.pb.go' 2>/dev/null); \
	if [ -z "$$files" ]; then \
	  echo "  (no Go sources yet — skipping)"; \
	else \
	  diff=$$(echo "$$files" | xargs gofumpt -l -extra); \
	  if [ -n "$$diff" ]; then \
	    echo "✗ gofumpt found unformatted files:"; echo "$$diff"; exit 1; \
	  fi; \
	fi

.PHONY: lint-python
lint-python: ## Run ruff (lint + format check).
	@echo "→ ruff lint"
	uv run ruff check .
	@echo "→ ruff format check"
	uv run ruff format --check .

.PHONY: lint-yaml
lint-yaml: ## Run yamllint across the repo.
	@echo "→ yamllint"
	uv run yamllint -c .yamllint.yaml .

.PHONY: lint-node
lint-node: ## Run eslint across all TS/JS sources.
	@echo "→ eslint"
	@if [ ! -d node_modules ]; then echo "node_modules missing. Run 'make bootstrap-node' first." >&2; exit 1; fi
	pnpm -s lint

.PHONY: lint-k8s
lint-k8s: ## kubeconform validate any rendered CRD/Helm manifests.
	@echo "→ kubeconform"
	@if [ ! -x $(LOCAL_BIN)/kubeconform ] && ! command -v kubeconform >/dev/null 2>&1; then \
	  echo "kubeconform not installed. Run 'make bootstrap-k8s' first." >&2; exit 1; \
	fi
	@shopt -s nullglob; \
	files=(config/crd/bases/*.yaml); \
	if [ $${#files[@]} -eq 0 ]; then \
	  echo "  (no CRD manifests yet — skipping CRD validation)"; \
	else \
	  kubeconform -strict -summary -ignore-missing-schemas "$${files[@]}"; \
	fi
	@$(MAKE) --no-print-directory helm-validate

# ---------- Proto / buf ----------------------------------------------------------
.PHONY: proto
proto: ## Lint and generate code from proto/aip/v1/*.proto (Go + Python + TS).
	@echo "→ buf lint"
	@if [ ! -x $(LOCAL_BIN)/buf ]; then echo "buf not found. Run 'make bootstrap-proto' first." >&2; exit 1; fi
	$(LOCAL_BIN)/buf lint
	@echo "→ buf generate"
	$(LOCAL_BIN)/buf generate
	@echo "→ go mod tidy (post-proto)"
	$(GO) mod tidy
	@echo "→ go build ./proto/..."
	$(GO) build ./proto/...

.PHONY: proto-clean
proto-clean: ## Remove all generated proto stubs (keeps .proto sources).
	rm -rf proto/aip/v1/*.pb.go proto/aip/v1/*_grpc.pb.go proto/ts dataplane/runtime/aik_runtime/proto

# ---------- Convenience ----------------------------------------------------------
.PHONY: fmt
fmt: ## Auto-format Go + Python + Node sources.
	@echo "→ gofumpt -w"
	@files=$$(find api controllers internal dataplane proto cmd -type f -name '*.go' \
	  ! -name 'zz_generated.*.go' \
	  ! -name '*.pb.go' \
	  ! -name '*_grpc.pb.go' 2>/dev/null); \
	if [ -n "$$files" ]; then echo "$$files" | xargs gofumpt -w -extra; fi
	@echo "→ ruff format"
	uv run ruff format .
	@echo "→ ruff --fix"
	uv run ruff check --fix .
	@echo "→ eslint --fix"
	pnpm -s lint:fix

.PHONY: tidy
tidy: ## go mod tidy.
	$(GO) mod tidy

.PHONY: clean
clean: ## Remove caches and local toolchain.
	rm -rf $(LOCAL_BIN) bin dist build coverage .pytest_cache .ruff_cache .mypy_cache

# ---------- Helm chart -----------------------------------------------------------
.PHONY: helm-lint
helm-lint: ## helm lint the umbrella chart.
	@if [ ! -x $(LOCAL_BIN)/helm ] && ! command -v helm >/dev/null 2>&1; then \
	  echo "helm not installed. Run 'make bootstrap-k8s' first." >&2; exit 1; \
	fi
	$(LOCAL_BIN)/helm lint deploy/helm/ai-keeper

.PHONY: helm-template
helm-template: ## Render the umbrella chart to /tmp/aik-rendered.yaml.
	@if [ ! -x $(LOCAL_BIN)/helm ] && ! command -v helm >/dev/null 2>&1; then \
	  echo "helm not installed. Run 'make bootstrap-k8s' first." >&2; exit 1; \
	fi
	$(LOCAL_BIN)/helm template aik-test deploy/helm/ai-keeper > /tmp/aik-rendered.yaml
	@echo "  rendered → /tmp/aik-rendered.yaml ($$(wc -l < /tmp/aik-rendered.yaml) lines)"

.PHONY: helm-validate
helm-validate: helm-lint helm-template ## helm lint + render + kubeconform on rendered output.
	@echo "→ kubeconform on rendered helm chart"
	$(LOCAL_BIN)/kubeconform -strict -summary -ignore-missing-schemas /tmp/aik-rendered.yaml

# ---------- kind / local cluster -------------------------------------------------
.PHONY: kind-up
kind-up: ## Bring up the kind cluster + local registry + ingress-nginx.
	@command -v docker >/dev/null 2>&1 || { \
	  echo "✗ docker not found in PATH — kind cannot be started without docker." >&2; \
	  echo "  Install Docker Desktop (or colima) and re-run \`make kind-up\`." >&2; \
	  exit 1; \
	}
	@command -v kind >/dev/null 2>&1 || { \
	  echo "✗ kind not found in PATH." >&2; \
	  echo "  Install via 'brew install kind' or https://kind.sigs.k8s.io/docs/user/quick-start/#installation" >&2; \
	  exit 1; \
	}
	@command -v kubectl >/dev/null 2>&1 || { \
	  echo "✗ kubectl not found in PATH. Install via 'brew install kubectl'." >&2; \
	  exit 1; \
	}
	CLUSTER_NAME=$(KIND_CLUSTER_NAME) \
	REGISTRY_NAME=$(KIND_REGISTRY_NAME) \
	REGISTRY_PORT=$(KIND_REGISTRY_PORT) \
	KIND_CONFIG=$(KIND_CONFIG) \
	  bash hack/kind/kind-with-registry.sh
	@echo "→ installing ingress-nginx"
	bash hack/kind/install-ingress.sh

.PHONY: kind-down
kind-down: ## Tear down the kind cluster + local registry.
	CLUSTER_NAME=$(KIND_CLUSTER_NAME) \
	REGISTRY_NAME=$(KIND_REGISTRY_NAME) \
	  bash hack/kind/kind-teardown.sh

.PHONY: kind-config-validate
kind-config-validate: ## Sanity-check the kind config without bringing the cluster up.
	@bash -n hack/kind/kind-with-registry.sh
	@bash -n hack/kind/kind-teardown.sh
	@bash -n hack/kind/install-ingress.sh
	@if command -v kind >/dev/null 2>&1; then \
	  echo "  kind --version: $$(kind --version)"; \
	  kind create cluster --help >/dev/null; \
	else \
	  echo "  (kind not installed — skipping live validation, scripts syntax-checked OK)"; \
	fi
	@uv run yamllint -c .yamllint.yaml hack/kind/kind-cluster.yaml

# ---------- Unit tests -----------------------------------------------------------
.PHONY: unit
unit: unit-go unit-python ## Run all unit tests.
	@echo "✅ unit tests complete"

.PHONY: unit-go
unit-go: ## Run Go unit tests.
	@echo "→ go test ./..."
	@if find . -name '*_test.go' -not -path './node_modules/*' -not -path './.venv/*' -not -path './vendor/*' | grep -q .; then \
	  $(GO) test ./...; \
	else \
	  echo "  (no Go test files yet — skipping)"; \
	fi

.PHONY: webhook-test
webhook-test: ## Run AdmissionWebhook unit tests (task 2.3).
	@echo "→ go test ./internal/webhook/... -count=1 -race"
	$(GO) test ./internal/webhook/... -count=1 -race

.PHONY: unit-python
unit-python: ## Run Python unit tests with pytest.
	@echo "→ pytest"
	@if find . -name 'test_*.py' -o -name '*_test.py' 2>/dev/null \
	  | grep -v node_modules | grep -v .venv | grep -q .; then \
	  set +e; \
	  uv run pytest -q --no-header; rc=$$?; \
	  set -e; \
	  if [ $$rc -ne 0 ] && [ $$rc -ne 5 ]; then exit $$rc; fi; \
	else \
	  echo "  (no pytest tests yet — skipping)"; \
	fi

# ---------- Image build ----------------------------------------------------------
.PHONY: images
images: ## Placeholder for container image builds (real Dockerfiles land in component tasks).
	@echo "→ go build ./cmd/... (image build placeholder)"
	@cmds=$$(find cmd -mindepth 1 -maxdepth 1 -type d); \
	if [ -z "$$cmds" ]; then \
	  echo "  (no commands yet — skipping)"; \
	else \
	  for d in $$cmds; do \
	    if find "$$d" -name '*.go' -maxdepth 1 | grep -q .; then \
	      echo "  → building $$d"; \
	      $(GO) build -o /dev/null ./$$d; \
	    else \
	      echo "  → $$d (no Go sources yet, skipping)"; \
	    fi; \
	  done; \
	fi

# ---------- CI local rehearsal ---------------------------------------------------
.PHONY: ci-local
ci-local: lint unit helm-validate ## Full local CI rehearsal: lint + unit tests + helm validation.
	@echo "✅ ci-local complete"

# ---------- CRD / RBAC / Webhook manifests --------------------------------------
.PHONY: manifests
manifests: ## Run controller-gen to (re)generate CRD / RBAC / webhook manifests + deepcopy code.
	@if [ ! -x $(LOCAL_BIN)/controller-gen ]; then \
	  echo "controller-gen not installed. Run 'make bootstrap-go' first." >&2; exit 1; \
	fi
	@echo "→ controller-gen object (deepcopy)"
	$(LOCAL_BIN)/controller-gen object paths="./api/..."
	@echo "→ controller-gen rbac+crd+webhook"
	$(LOCAL_BIN)/controller-gen \
	  rbac:roleName=aik-manager-role \
	  crd:crdVersions=v1,allowDangerousTypes=true \
	  webhook \
	  paths="./api/..." \
	  output:crd:artifacts:config=config/crd/bases \
	  output:rbac:artifacts:config=config/rbac \
	  output:webhook:artifacts:config=config/webhook
	@echo "  generated CRDs:"
	@ls -1 config/crd/bases/*.yaml 2>/dev/null | sed 's/^/    /' || true

# ---------- Storage stack (task 16.1) --------------------------------------------
.PHONY: storage-up
storage-up: ## Deploy PostgreSQL + Redis + NATS JetStream to kind cluster (aik-system namespace).
	@command -v helm >/dev/null 2>&1 || { \
	  if [ -x $(LOCAL_BIN)/helm ]; then true; else \
	    echo "✗ helm not found. Run 'make bootstrap-k8s' first." >&2; exit 1; \
	  fi; \
	}
	$(LOCAL_BIN)/helm upgrade --install aik-storage deploy/helm/ai-keeper/charts/storage \
	  --namespace aik-system --create-namespace \
	  --wait --timeout 120s

.PHONY: storage-down
storage-down: ## Remove the storage stack from kind cluster.
	$(LOCAL_BIN)/helm uninstall aik-storage --namespace aik-system 2>/dev/null || true

# ---------- Audit storage stack (task 16.2) ---------------------------------------
.PHONY: audit-storage-up
audit-storage-up: ## Deploy ClickHouse + MinIO (S3 WORM) to kind cluster (aik-system namespace).
	@command -v helm >/dev/null 2>&1 || { \
	  if [ -x $(LOCAL_BIN)/helm ]; then true; else \
	    echo "✗ helm not found. Run 'make bootstrap-k8s' first." >&2; exit 1; \
	  fi; \
	}
	$(LOCAL_BIN)/helm upgrade --install aik-audit-storage deploy/helm/ai-keeper/charts/audit-storage \
	  --namespace aik-system --create-namespace \
	  --wait --timeout 120s

.PHONY: audit-storage-down
audit-storage-down: ## Remove the audit-storage stack from kind cluster.
	$(LOCAL_BIN)/helm uninstall aik-audit-storage --namespace aik-system 2>/dev/null || true

# ---------- E2E environment (task 20.1) ------------------------------------------
E2E_KIND_CLUSTER  ?= aik-e2e
E2E_KIND_CONFIG   ?= test/e2e/kind-config.yaml

.PHONY: e2e-up
e2e-up: ## Bring up E2E environment: kind cluster + storage + mock services.
	@command -v docker >/dev/null 2>&1 || { echo "✗ docker not found" >&2; exit 1; }
	@command -v kind >/dev/null 2>&1 || { echo "✗ kind not found" >&2; exit 1; }
	@command -v kubectl >/dev/null 2>&1 || { echo "✗ kubectl not found" >&2; exit 1; }
	@echo "→ creating kind cluster $(E2E_KIND_CLUSTER) (if not exists)"
	@kind get clusters 2>/dev/null | grep -q "^$(E2E_KIND_CLUSTER)$$" || \
	  kind create cluster --name $(E2E_KIND_CLUSTER) --config $(E2E_KIND_CONFIG) --wait 60s
	@kubectl cluster-info --context kind-$(E2E_KIND_CLUSTER)
	@echo "→ building mock images"
	docker build -t mock-llm:e2e -f test/e2e/fixtures/mock-llm/Dockerfile .
	docker build -t mock-idp:e2e -f test/e2e/fixtures/mock-idp/Dockerfile .
	docker build -t mock-feishu:e2e -f test/e2e/fixtures/mock-feishu/Dockerfile .
	@echo "→ loading mock images into kind"
	kind load docker-image mock-llm:e2e --name $(E2E_KIND_CLUSTER)
	kind load docker-image mock-idp:e2e --name $(E2E_KIND_CLUSTER)
	kind load docker-image mock-feishu:e2e --name $(E2E_KIND_CLUSTER)
	@echo "→ creating namespace"
	kubectl --context kind-$(E2E_KIND_CLUSTER) create namespace aik-system --dry-run=client -o yaml | \
	  kubectl --context kind-$(E2E_KIND_CLUSTER) apply -f -
	@echo "→ deploying storage (PostgreSQL + Redis + NATS)"
	$(LOCAL_BIN)/helm upgrade --install aik-storage deploy/helm/ai-keeper/charts/storage \
	  --namespace aik-system --kube-context kind-$(E2E_KIND_CLUSTER) \
	  --wait --timeout 120s
	@echo "→ deploying audit-storage (ClickHouse + MinIO)"
	$(LOCAL_BIN)/helm upgrade --install aik-audit-storage deploy/helm/ai-keeper/charts/audit-storage \
	  --namespace aik-system --kube-context kind-$(E2E_KIND_CLUSTER) \
	  --wait --timeout 120s
	@echo "→ deploying mock services"
	kubectl --context kind-$(E2E_KIND_CLUSTER) apply -f test/e2e/manifests/
	@echo "→ waiting for all ai-keeper pods to be ready"
	kubectl --context kind-$(E2E_KIND_CLUSTER) wait --for=condition=ready pod \
	  -l app.kubernetes.io/part-of=ai-keeper --all --namespace aik-system --timeout=300s
	@echo "✅ e2e-up complete"

.PHONY: e2e-down
e2e-down: ## Tear down the E2E kind cluster.
	kind delete cluster --name $(E2E_KIND_CLUSTER) 2>/dev/null || true

# ---------- Placeholders for later tasks -----------------------------------------
.PHONY: audit-e2e checkpoint-p0
audit-e2e checkpoint-p0:
	@echo "$@: not yet implemented (added in subsequent tasks)" >&2; exit 1