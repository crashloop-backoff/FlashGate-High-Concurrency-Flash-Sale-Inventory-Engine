.PHONY: up down test lint migrate

ifneq (,$(wildcard ./.env))
include .env
export
endif

DB_HOST_LOCAL ?= localhost
DB_HOST_PORT ?= 5433
DB_SSLMODE ?= disable
GOOSE_VERSION := v3.22.1
GOLANGCI_LINT_IMAGE := golangci/golangci-lint:v1.61.0

up:
	docker compose up -d --build

down:
	docker compose down

test:
	go test -race -v ./...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		docker run --rm -v "$(CURDIR):/app" -w /app $(GOLANGCI_LINT_IMAGE) golangci-lint run; \
	fi

migrate:
	@if [ -z "$(DB_USER)" ] || [ -z "$(DB_PASSWORD)" ] || [ -z "$(DB_NAME)" ]; then \
		echo "Error: DB_USER, DB_PASSWORD, and DB_NAME must be set (copy .env.example to .env)."; \
		exit 1; \
	fi
	@DSN="postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST_LOCAL):$(DB_HOST_PORT)/$(DB_NAME)?sslmode=$(DB_SSLMODE)"; \
	if command -v goose >/dev/null 2>&1; then \
		goose -dir db/migrations postgres "$$DSN" up; \
	else \
		go run github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION) -dir db/migrations postgres "$$DSN" up; \
	fi
