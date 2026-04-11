BINARY := polaris
CONFIG ?= ./config/polaris.yaml
STACK ?= local

.DEFAULT_GOAL := help

.PHONY: help dev build run test lint migrate docker-build \
	local-up local-down local-restart local-logs local-ps local-config \
	stack-up stack-down stack-restart stack-logs stack-ps stack-config stack-pull

help:
	@printf "\nPolaris developer commands\n\n"
	@printf "  make dev            Run Polaris locally with CONFIG=%s\n" "$(CONFIG)"
	@printf "  make build          Build ./bin/$(BINARY)\n"
	@printf "  make run            Build then run ./bin/$(BINARY)\n"
	@printf "  make test           Run go test -race ./...\n"
	@printf "  make lint           Run golangci-lint\n"
	@printf "  make docker-build   Build the Polaris Docker image\n"
	@printf "  make stack-up       Start Docker stack STACK=%s\n" "$(STACK)"
	@printf "  make stack-down     Stop Docker stack STACK=%s\n" "$(STACK)"
	@printf "  make stack-logs     Follow logs for stack STACK=%s\n" "$(STACK)"
	@printf "  make stack-ps       Show status for stack STACK=%s\n" "$(STACK)"
	@printf "  make stack-config   Render Compose config for STACK=%s\n" "$(STACK)"
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
	golangci-lint run ./...

migrate:
	@echo "scripts/migrate.go is a placeholder until Phase 1 store wiring lands"

docker-build:
	docker build -f deployments/Dockerfile -t polaris:dev .

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

stack-pull:
	STACK=$(STACK) ./scripts/stack.sh pull
