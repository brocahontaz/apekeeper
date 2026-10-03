.PHONY: compose-up compose-down dev-up dev-down dev-logs migrate migrate-check api-contract test test-unit lint build run frontend-install frontend-dev frontend-build frontend-test frontend-e2e frontend-format build-all check
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
migrate-check:
	python3 scripts/check_migrations.py
api-contract:
	python3 scripts/validate_openapi.py
	cd backend && go test ./internal/httpapi -run '^TestRouteManifestMatchesOpenAPIRoutes$$'
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
frontend-e2e:
	cd frontend && npm run e2e
frontend-format:
	cd frontend && npm run format
build-all: build
check:
	cd backend && test -z "$$(gofmt -l .)" && go vet ./...
	cd frontend && npm run check
	cd frontend && npm run format:check
	python3 scripts/validate_openapi.py
run:
	cd backend && STATIC_DIR=$(CURDIR)/frontend/dist go run ./cmd/apekeeper
