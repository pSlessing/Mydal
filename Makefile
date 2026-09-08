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

## test: run the whole suite with the race detector
##       Integration tests need Postgres and MinIO; they skip when
##       TEST_DATABASE_URL / TEST_MINIO_ENDPOINT (or DATABASE_URL /
##       MINIO_ENDPOINT) are unset.
test:
	go test -race $(PKGS)

## test-unit: run only the tests that need no services
test-unit:
	go test -race -short $(PKGS)

## test-cover: run the suite and report coverage per package
test-cover:
	go test -race -cover $(PKGS)

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

.PHONY: help run build migrate swagger test test-unit test-cover vet lint fmt tidy up down down-clean logs clean
