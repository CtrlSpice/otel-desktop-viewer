.DEFAULT_GOAL := help

.PHONY: install
install:
	cd desktopexporter/internal/frontend && npm install

.PHONY: install-clean
install-clean:
	cd desktopexporter/internal/frontend && rm -rf node_modules package-lock.json && npm install

.PHONY: build-go
build-go:
	go build -o otel-desktop-viewer

.PHONY: test-go
test-go:
	cd desktopexporter && go test ./...

.PHONY: run-go
run-go:
	STATIC_ASSETS_DIR=$(abspath ./desktopexporter/internal/frontend/dist/) go run . --browser-port 8000

.PHONY: dev-ts
dev-ts:
	@echo "Starting Vite dev server..."
	@echo "Open http://localhost:3001 for development"
	@echo ""
	cd desktopexporter/internal/frontend && npm run dev

.PHONY: run-go-persist
run-go-persist:
	STATIC_ASSETS_DIR=$(abspath ./desktopexporter/internal/frontend/dist/) go run . --db duck.db

.PHONY: build-ts
build-ts:
	cd desktopexporter/internal/frontend && npm run build && cp -r dist/* ../../internal/server/static/

.PHONY: format-ts
format-ts:
	cd desktopexporter/internal/frontend && npm run format

.PHONY: validate-ts
validate-ts:
	cd desktopexporter/internal/frontend && npm run check

.PHONY: build
build: build-ts build-go

.PHONY: run
run: build-ts
	STATIC_ASSETS_DIR=$(abspath ./desktopexporter/internal/frontend/dist/) go run . --browser-port 8000

.PHONY: test
test: test-go validate-ts

.PHONY: release-dry-run
release-dry-run:
	gh workflow run "Release" --ref $$(git branch --show-current) -f test_mode=true

.PHONY: kill-port
kill-port:
	@echo "Killing process on port 8888..."
	@lsof -ti:8888 | xargs kill -9 2>/dev/null || echo "No process found on port 8888"

.PHONY: stop
stop:
	@echo "Stopping Go server (port 8000) and Vite dev server (port 3001)..."
	@lsof -ti:8000 | xargs kill -9 2>/dev/null || true
	@lsof -ti:3001 | xargs kill -9 2>/dev/null || true
	@echo "done"

.PHONY: help
help:
	@echo "Available targets:"
	@echo ""
	@echo "Frontend:"
	@echo "  install           - Install frontend dependencies"
	@echo "  install-clean     - Clean install (removes node_modules first)"
	@echo "  build-ts          - Build frontend"
	@echo "  format-ts         - Format frontend code (Prettier)"
	@echo "  validate-ts       - Type check frontend"
	@echo "  dev-ts            - Start frontend dev server (Vite)"
	@echo ""
	@echo "Server:"
	@echo "  build-go          - Build Go binary"
	@echo "  test-go           - Run Go tests"
	@echo "  run-go            - Run server (in-memory, data lost on exit)"
	@echo "  run-go-persist    - Run server with persistent DB file (data retained)"
	@echo ""
	@echo "Convenience:"
	@echo "  build             - Build frontend and Go binary"
	@echo "  run               - Build frontend, then run server (in-memory)"
	@echo "  test              - Run Go tests and type check frontend"
	@echo ""
	@echo "Other:"
	@echo "  release-dry-run   - Trigger release workflow (dry run)"
	@echo "  kill-port         - Kill process on port 8888"
	@echo "  stop              - Stop Go server and Vite dev server"
