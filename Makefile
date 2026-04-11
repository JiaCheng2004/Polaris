BINARY := polaris
CONFIG ?= ./config/polaris.yaml

.PHONY: build run test lint migrate docker-build

build:
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
