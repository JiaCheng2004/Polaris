BINARY := polaris
CONFIG ?= ./config/polaris.yaml
STACK ?= local
LIVE_SMOKE_TIMEOUT ?= 45m
LOAD_CHECK_TIMEOUT ?= 60m
GOLANGCI_LINT_VERSION ?= v2.11.4
GOLANGCI_LINT_MODULE := github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
GOLANGCI_LINT ?= go run $(GOLANGCI_LINT_MODULE)
GOSEC_VERSION ?= v2.25.0
GOSEC_MODULE := github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION)
GOSEC ?= go run $(GOSEC_MODULE)
GOSEC_ALLOWLIST ?= ./config/security/gosec_allowlist.json

# Version metadata stamped into `make build` binaries so `--version` is meaningful
# locally. The release artifacts are stamped by GoReleaser (.goreleaser.yaml) and the
# Dockerfiles inject the same three vars at image build; this is best-effort from git.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)

.DEFAULT_GOAL := help

.PHONY: help dev build run test bench lint check check-layering security-check \
	license-check reuse-check sdk-ts web-console build-console doc-check openapi-lint \
	docs-config-check migrate docker-build verify-models verify-models-json live-smoke \
	load-check config-check contract-check release-check panic-scan fmt-check \
	stack-up stack-down stack-restart stack-logs stack-ps stack-config stack-validate stack-pull

# `help` is generated from the `## ` doc-comments and `##@ ` group headers below, so it
# can never drift from the real targets. Only the user-facing targets carry a comment;
# the granular gates stay callable (and CI calls them by name) without cluttering the menu.
help: ## Show this help
	@printf "\nPolaris — make targets\n"
	@awk 'BEGIN {FS = ":.*## "} \
		/^##@ / {printf "\n\033[1m%s\033[0m\n", substr($$0, 5); next} \
		/^[a-zA-Z0-9_-]+:.*## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@printf "\n  Vars: CONFIG=%s   STACK=%s (local|prod|dev)\n\n" "$(CONFIG)" "$(STACK)"

##@ Develop
dev: ## Run the gateway from source (CONFIG=...)
	go run ./cmd/polaris --config $(CONFIG)

build: ## Build ./bin/polaris (version-stamped)
	mkdir -p ./bin
	go build -ldflags "$(LDFLAGS)" -o ./bin/$(BINARY) ./cmd/polaris

run: build ## Build then run ./bin/polaris
	./bin/$(BINARY) --config $(CONFIG)

migrate: ## Run configured store migrations
	go run ./cmd/polaris --config $(CONFIG) --migrate

# Batteries-included binary: builds the console, embeds its assets, and compiles with
# the `console` build tag so `--console` serves it. The default `make build` stays lean
# and node-free. The embed dir is restored to its committed placeholder afterwards (the
# binary already embedded the real assets at compile time).
build-console: ## Build the binary with the embedded admin console (--console)
	mkdir -p ./bin
	cd web/console && npm ci && npm run build
	rm -rf internal/console/dist/assets
	cp -R web/console/dist/. internal/console/dist/
	go build -tags console -ldflags "$(LDFLAGS)" -o ./bin/$(BINARY) ./cmd/polaris
	@cp internal/console/placeholder.html internal/console/dist/index.html
	@rm -rf internal/console/dist/assets
	@echo "Built ./bin/$(BINARY) with the embedded console — run: ./bin/$(BINARY) --config $(CONFIG) --console"

##@ Quality gates
test: ## Run all tests with the race detector
	go test -race ./...

lint: ## Run pinned golangci-lint
	$(GOLANGCI_LINT) run ./...

check: ## Run all fast static gates (fmt, lint, layering, security, licenses, docs, contract)
	$(MAKE) fmt-check
	$(MAKE) lint
	$(MAKE) check-layering
	$(MAKE) security-check
	$(MAKE) reuse-check
	$(MAKE) license-check
	$(MAKE) doc-check
	$(MAKE) panic-scan
	$(MAKE) openapi-lint
	$(MAKE) docs-config-check
	$(MAKE) config-check
	$(MAKE) contract-check

# Full repo-local release gate (runbook precondition; run by release.yml). Layered:
# static gates -> race tests -> build -> Compose validation -> image. `config-check`
# already runs `verify-models` against the default config, so it is not repeated here.
release-check: ## Full repo-local release gate (check + tests + build + docker + compose)
	$(MAKE) check
	$(MAKE) test
	$(MAKE) build
	$(MAKE) stack-validate STACK=local
	$(MAKE) stack-validate STACK=prod
	$(MAKE) stack-validate STACK=dev
	$(MAKE) docker-build

# ---- Individual gates: composed by `check`/`release-check`; CI also calls them by name ----
fmt-check:
	test -z "$$(gofmt -l .)"

check-layering:
	@echo "Verifying provider/tooling packages never import the gateway layer..."
	@matches="$$(grep -rl 'JiaCheng2004/Polaris/internal/gateway' internal/provider internal/tooling 2>/dev/null || true)"; \
	if [ -n "$$matches" ]; then \
		echo "LAYERING VIOLATION: the following provider/tooling files import internal/gateway:" >&2; \
		echo "$$matches" >&2; \
		exit 1; \
	fi; \
	echo "OK: no provider/tooling -> gateway imports."

security-check:
	@mkdir -p ./tmp; \
	tmp="$$(mktemp ./tmp/gosec-report.XXXXXX.json)"; \
	log="$$(mktemp ./tmp/gosec-log.XXXXXX.txt)"; \
	set +e; \
	$(GOSEC) -quiet -exclude-generated -fmt=json -out "$$tmp" ./... 2>"$$log"; \
	set +e; \
	go run ./scripts/securitycheck -report "$$tmp" -allowlist "$(GOSEC_ALLOWLIST)"; \
	check_status=$$?; \
	set -e; \
	if [ $$check_status -ne 0 ]; then cat "$$log" >&2; fi; \
	rm -f "$$tmp" "$$log"; \
	exit $$check_status

reuse-check:
	python3 -m reuse lint

license-check:
	go run github.com/google/go-licenses/v2@latest check ./cmd/polaris ./pkg/client

doc-check:
	go run github.com/mgechev/revive@latest -config .revive-doc.toml -set_exit_status ./pkg/client/...

openapi-lint:
	go run github.com/daveshanley/vacuum@latest lint -r .vacuum.yaml --fail-severity error spec/openapi/polaris.v1.yaml

docs-config-check:
	bash scripts/docs-config-check.sh

config-check:
	go test -count=1 ./internal/config ./internal/provider/catalog
	go run ./cmd/polaris --config ./config/polaris.yaml --verify-models
	DATABASE_URL='postgres://polaris:polaris@localhost:5432/polaris?sslmode=disable' REDIS_URL='redis://localhost:6379/0' POLARIS_BOOTSTRAP_ADMIN_KEY_HASH='sha256:example' MINIMAX_BASE_URL='https://api.minimax.io' go run ./cmd/polaris --config ./config/polaris.example.yaml --verify-models
	MINIMAX_BASE_URL='https://api.minimax.io' go run ./cmd/polaris --config ./config/polaris.live-smoke.yaml --verify-models

contract-check:
	go test -count=1 ./tests/contract

panic-scan:
	! rg -n "panic\\(" internal --glob '!**/*_test.go'

bench:
	go test -run '^$$' -bench . -benchmem \
		./internal/gateway ./internal/guardrails ./internal/routing ./internal/semcache

##@ Providers & load
verify-models: ## Print the configured model verification summary
	go run ./cmd/polaris --config $(CONFIG) --verify-models

verify-models-json:
	go run ./cmd/polaris --config $(CONFIG) --verify-models-json

# Real-provider smoke; needs provider API keys in the environment. For the
# release-blocking strict matrix prefix POLARIS_LIVE_SMOKE_STRICT=1; add
# POLARIS_LIVE_SMOKE_INCLUDE_OPT_IN=1 to include opt-in models (env vars set before
# `make` are inherited by the recipe).
live-smoke: ## Run the env-gated real-provider smoke matrix (needs API keys)
	POLARIS_LIVE_SMOKE=1 go test -count=1 -timeout $(LIVE_SMOKE_TIMEOUT) ./tests/e2e -run TestLiveSmokeMatrix

load-check:
	POLARIS_LOAD_CHECK=1 go test -count=1 -timeout $(LOAD_CHECK_TIMEOUT) ./tests/e2e -run TestLoadCheckMatrix

# npm-based CI checks (mirrored by the sdk-typescript / web-console CI jobs).
sdk-ts:
	cd sdk/typescript && npm install && npm run typecheck && npm test && npm run build && npm publish --dry-run

web-console:
	cd web/console && npm ci && npm run typecheck && npm test && npm run build

##@ Docker & stack
docker-build: ## Build the Polaris Docker image
	docker build -f deployments/Dockerfile -t polaris:dev .

stack-up: ## Start the Docker stack (STACK=local|prod|dev)
	STACK=$(STACK) ./scripts/stack.sh up

stack-down: ## Stop the Docker stack
	STACK=$(STACK) ./scripts/stack.sh down

stack-logs: ## Follow logs for the Docker stack
	STACK=$(STACK) ./scripts/stack.sh logs

stack-validate: ## Validate the Compose config without rendering secrets
	STACK=$(STACK) ./scripts/stack.sh validate

stack-restart:
	STACK=$(STACK) ./scripts/stack.sh restart

stack-ps:
	STACK=$(STACK) ./scripts/stack.sh ps

stack-config:
	STACK=$(STACK) ./scripts/stack.sh config

stack-pull:
	STACK=$(STACK) ./scripts/stack.sh pull
