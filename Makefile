BINARY := bin/mydal
PKGS   := ./...

.DEFAULT_GOAL := help

## help: list the available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'

## run: run the server against the local .env
run:
	go run ./cmd/server

## build: build the server binary into bin/
build:
	go build -o $(BINARY) ./cmd/server

## migrate: apply database migrations and exit
migrate:
	go run ./cmd/server -migrate-only

## swagger: regenerate the OpenAPI docs from the handler annotations
swagger:
	swag init --generalInfo cmd/server/main.go --output cmd/server/docs --parseDependency --parseInternal

## test: run the test suite with the race detector
test:
	go test -race $(PKGS)

## vet: run go vet
vet:
	go vet $(PKGS)

## lint: run golangci-lint (see .golangci.yml)
lint:
	golangci-lint run

## fmt: format the tree
fmt:
	gofmt -w .

## tidy: prune and sync go.mod / go.sum
tidy:
	go mod tidy

## up: start the whole stack in the background, rebuilding the app image
up:
	docker compose up -d --build

## down: stop the stack, keeping the named volumes
down:
	docker compose down

## down-clean: stop the stack and delete its data volumes
down-clean:
	docker compose down -v

## logs: follow the app logs
logs:
	docker compose logs -f app

## clean: remove build artefacts
clean:
	rm -rf bin

.PHONY: help run build migrate swagger test vet lint fmt tidy up down down-clean logs clean
