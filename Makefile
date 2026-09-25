.PHONY: compose-up compose-down dev-up dev-down dev-logs migrate test test-unit lint build run frontend-install frontend-dev frontend-build frontend-test build-all check
compose-up:
	docker compose up -d db
compose-down:
	docker compose down
dev-up:
	docker compose up --build
dev-down:
	docker compose down
dev-logs:
	docker compose logs -f
migrate:
	cd backend && go run ./cmd/apekeeper -migrate
test:
	cd backend && go test ./...
test-unit: test
lint:
	cd backend && test -z "$$(gofmt -l .)" && go vet ./...
build: frontend-build
	cd backend && go build ./...
frontend-install:
	cd frontend && npm install
frontend-dev:
	cd frontend && npm run dev
frontend-build:
	cd frontend && npm run build
frontend-test:
	cd frontend && npm run test
build-all: build
check:
	cd backend && test -z "$$(gofmt -l .)" && go vet ./...
	cd frontend && npm run check
run:
	cd backend && STATIC_DIR=$(CURDIR)/frontend/dist go run ./cmd/apekeeper
