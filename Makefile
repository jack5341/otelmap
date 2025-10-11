SHELL := /bin/bash

APP := server
GO ?= go

.PHONY: tidy build run docker-build up down logs

tidy:
	$(GO) mod tidy

build:
	CGO_ENABLED=0 $(GO) build -o bin/$(APP) ./cmd/server

run:
	PORT?=8080 CLICKHOUSE_DSN?=clickhouse://default:@localhost:9000/default?dial_timeout=5s\&compress=true \
	LOG_LEVEL?=info SHUTDOWN_TIMEOUT_SECONDS?=10 \
		./bin/$(APP)

docker-build:
	docker build -t project-pompa-server:local .

up:
	docker compose up -d

down:
	docker compose down -v

logs:
	docker compose logs -f clickhouse


