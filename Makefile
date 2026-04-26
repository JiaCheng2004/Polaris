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

.DEFAULT_GOAL := help

.PHONY: help dev build run test lint security-check migrate docker-build verify-models verify-models-json live-smoke live-smoke-strict live-smoke-opt-in load-check config-check contract-check release-check panic-scan fmt-check \
	local-up local-down local-restart local-logs local-ps local-config \
	stack-up stack-down stack-restart stack-logs stack-ps stack-config stack-validate stack-pull

help:
	@printf "\nPolaris developer commands\n\n"
	@printf "  make dev            Run Polaris locally with CONFIG=%s\n" "$(CONFIG)"
	@printf "  make build          Build ./bin/$(BINARY)\n"
	@printf "  make run            Build then run ./bin/$(BINARY)\n"
	@printf "  make test           Run go test -race ./...\n"
	@printf "  make lint           Run pinned golangci-lint\n"
	@printf "  make security-check Run pinned gosec with exact audited allowlist\n"
	@printf "  make migrate        Run configured store migrations\n"
	@printf "  make docker-build   Build the Polaris Docker image\n"
	@printf "  make verify-models  Print configured model verification summary\n"
	@printf "  make verify-models-json  Print configured model verification summary as JSON\n"
	@printf "  make live-smoke     Run env-gated live provider smoke tests\n"
	@printf "  make live-smoke-strict  Run strict live smoke for release-blocking models\n"
	@printf "  make live-smoke-opt-in  Run live smoke including opt-in models\n"
	@printf "  make load-check     Run env-gated load validation with SQLite + memory cache\n"
	@printf "  make config-check   Validate config loader, modular YAML, and model catalog wiring\n"
	@printf "  make contract-check Validate OpenAPI route coverage and golden HTTP fixtures\n"
	@printf "  make release-check  Run the current repo-local release validation gate\n"
	@printf "  make stack-up       Start Docker stack STACK=%s\n" "$(STACK)"
	@printf "  make stack-down     Stop Docker stack STACK=%s\n" "$(STACK)"
	@printf "  make stack-logs     Follow logs for stack STACK=%s\n" "$(STACK)"
	@printf "  make stack-ps       Show status for stack STACK=%s\n" "$(STACK)"
	@printf "  make stack-config   Render Compose config for STACK=%s\n" "$(STACK)"
	@printf "  make stack-validate Validate Compose config for STACK=%s without rendering secrets\n" "$(STACK)"
	@printf "  make stack-pull     Pull images for stack STACK=%s\n" "$(STACK)"
	@printf "\n"
	@printf "  stacks: local | prod | dev\n"
	@printf "\n"

dev:
	go run ./cmd/polaris --config $(CONFIG)

build:
	mkdir -p ./bin
	go build -o ./bin/$(BINARY) ./cmd/polaris

run: build
	./bin/$(BINARY) --config $(CONFIG)

test:
	go test -race ./...

lint:
	$(GOLANGCI_LINT) run ./...

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

migrate:
	go run ./cmd/polaris --config $(CONFIG) --migrate

docker-build:
	docker build -f deployments/Dockerfile -t polaris:dev .

verify-models:
	go run ./cmd/polaris --config $(CONFIG) --verify-models

verify-models-json:
	go run ./cmd/polaris --config $(CONFIG) --verify-models-json

live-smoke:
	POLARIS_LIVE_SMOKE=1 go test -count=1 -timeout $(LIVE_SMOKE_TIMEOUT) ./tests/e2e -run TestLiveSmokeMatrix

live-smoke-strict:
	POLARIS_LIVE_SMOKE=1 POLARIS_LIVE_SMOKE_STRICT=1 go test -count=1 -timeout $(LIVE_SMOKE_TIMEOUT) ./tests/e2e -run TestLiveSmokeMatrix

live-smoke-opt-in:
	POLARIS_LIVE_SMOKE=1 POLARIS_LIVE_SMOKE_INCLUDE_OPT_IN=1 go test -count=1 -timeout $(LIVE_SMOKE_TIMEOUT) ./tests/e2e -run TestLiveSmokeMatrix

load-check:
	POLARIS_LOAD_CHECK=1 go test -count=1 -timeout $(LOAD_CHECK_TIMEOUT) ./tests/e2e -run TestLoadCheckMatrix

config-check:
	go test -count=1 ./internal/config ./internal/provider/catalog
	go run ./cmd/polaris --config ./config/polaris.yaml --verify-models
	DATABASE_URL='postgres://polaris:polaris@localhost:5432/polaris?sslmode=disable' REDIS_URL='redis://localhost:6379/0' POLARIS_BOOTSTRAP_ADMIN_KEY_HASH='sha256:example' MINIMAX_BASE_URL='https://api.minimax.io' go run ./cmd/polaris --config ./config/polaris.example.yaml --verify-models
	MINIMAX_BASE_URL='https://api.minimax.io' go run ./cmd/polaris --config ./config/polaris.live-smoke.yaml --verify-models

contract-check:
	go test -count=1 ./tests/contract

fmt-check:
	test -z "$$(gofmt -l .)"

panic-scan:
	! rg -n "panic\\(" internal --glob '!**/*_test.go'

release-check:
	$(MAKE) fmt-check
	$(MAKE) lint
	$(MAKE) security-check
	$(MAKE) panic-scan
	$(MAKE) config-check
	$(MAKE) verify-models CONFIG=$(CONFIG)
	$(MAKE) contract-check
	go test -race ./...
	$(MAKE) build
	$(MAKE) stack-validate STACK=local
	$(MAKE) stack-validate STACK=prod
	$(MAKE) stack-validate STACK=dev
	$(MAKE) docker-build

local-up:
	STACK=local ./scripts/stack.sh up

local-down:
	STACK=local ./scripts/stack.sh down

local-restart:
	STACK=local ./scripts/stack.sh restart

local-logs:
	STACK=local ./scripts/stack.sh logs

local-ps:
	STACK=local ./scripts/stack.sh ps

local-config:
	STACK=local ./scripts/stack.sh config

stack-up:
	STACK=$(STACK) ./scripts/stack.sh up

stack-down:
	STACK=$(STACK) ./scripts/stack.sh down

stack-restart:
	STACK=$(STACK) ./scripts/stack.sh restart

stack-logs:
	STACK=$(STACK) ./scripts/stack.sh logs

stack-ps:
	STACK=$(STACK) ./scripts/stack.sh ps

stack-config:
	STACK=$(STACK) ./scripts/stack.sh config

stack-validate:
	STACK=$(STACK) ./scripts/stack.sh validate

stack-pull:
	STACK=$(STACK) ./scripts/stack.sh pull
