PACKAGE_NAME         := github.com/CtrlSpice/otel-desktop-viewer

.PHONY: install
install:
	cd desktopexporter/internal/app; npm install

.PHONY: build-go
build-go:
	go build -o otel-desktop-viewer

.PHONY: test-go
test-go:
	cd desktopexporter; go test ./...
	
.PHONY: run-go
run-go:
	V2_STATIC_ASSETS_DIR=$(abspath ./desktopexporter/internal/app-v2/dist/) USE_V2_FRONTEND=true go run . --browser-port 8000

.PHONY: dev-go
dev-go:
	@echo "Starting Vite dev server for v2 frontend..."
	@echo "Open http://localhost:3001 for development"
	@echo "The Go server will run on http://localhost:8000"
	@echo ""
	cd desktopexporter/internal/app-v2 && npm run dev

.PHONY: run-go-v1
run-go-v1:
	STATIC_ASSETS_DIR=$(abspath ./desktopexporter/internal/server/static/) go run .

.PHONY: run-db-go
run-db-go:
	STATIC_ASSETS_DIR=$(abspath ./desktopexporter/internal/server/static/) go run . --db duck.db

.PHONY: build-js
build-js:
	cd desktopexporter/internal/app; npx esbuild --bundle main.tsx main.css --outdir=../server/static

.PHONY: watch-js
watch-js:
	cd desktopexporter/internal/app; npx esbuild --watch --bundle main.tsx main.css --outdir=../server/static

.PHONY: format-js
format-js:
	cd desktopexporter/internal/app; npx prettier -w app

# esbuild will compile typescript files but will not typecheck them. This runs the
# typescript typechecker but does not build the files.
.PHONY: validate-typescript
validate-typescript:
	cd desktopexporter/internal/app; npx tsc --noEmit

# V2 Frontend targets
.PHONY: install-v2
install-v2:
	cd desktopexporter/internal/app-v2 && rm -rf node_modules package-lock.json && npm install

.PHONY: build-js-v2
build-js-v2:
	cd desktopexporter/internal/app-v2; npm run build
	cp -r desktopexporter/internal/app-v2/dist/* desktopexporter/internal/server/static-v2/

.PHONY: watch-js-v2
watch-js-v2:
	cd desktopexporter/internal/app-v2; npm run dev

.PHONY: validate-typescript-v2
validate-typescript-v2:
	cd desktopexporter/internal/app-v2; npm run check

.PHONY: run-go-v2
run-go-v2:
	V2_STATIC_ASSETS_DIR=$(abspath ./desktopexporter/internal/app-v2/dist/) USE_V2_FRONTEND=true go run .

.PHONY: run-db-go-v2
run-db-go-v2:
	V2_STATIC_ASSETS_DIR=$(abspath ./desktopexporter/internal/app-v2/dist/) USE_V2_FRONTEND=true go run . --db duck.db

.PHONY: release-dry-run
release-dry-run:
	gh workflow run "Release" --ref $$(git branch --show-current) -f test_mode=true

.PHONY: kill-port
kill-port:
	@echo "Killing process on port 8888..."
	@lsof -ti:8888 | xargs kill -9 2>/dev/null || echo "No process found on port 8888"

# Convenience targets for both frontends
.PHONY: install-all
install-all: install install-v2

.PHONY: build-all
build-all: build-js build-js-v2

.PHONY: validate-all
validate-all: validate-typescript validate-typescript-v2

# Help target
.PHONY: help
help:
	@echo "Available targets:"
	@echo ""
	@echo "V1 Frontend (React + Chakra UI):"
	@echo "  install          - Install v1 frontend dependencies"
	@echo "  build-js         - Build v1 frontend"
	@echo "  watch-js         - Watch and rebuild v1 frontend"
	@echo "  validate-typescript - Type check v1 frontend"
	@echo "  run-go           - Run Go server with v1 frontend"
	@echo "  run-db-go        - Run Go server with v1 frontend and database"
	@echo ""
	@echo "V2 Frontend (Svelte + DaisyUI):"
	@echo "  install-v2       - Install v2 frontend dependencies"
	@echo "  build-js-v2      - Build v2 frontend"
	@echo "  watch-js-v2      - Watch and rebuild v2 frontend (dev server)"
	@echo "  validate-typescript-v2 - Type check v2 frontend"
	@echo "  run-go-v2        - Run Go server with v2 frontend"
	@echo "  run-db-go-v2     - Run Go server with v2 frontend and database"
	@echo ""
	@echo "Convenience:"
	@echo "  install-all      - Install both frontend dependencies"
	@echo "  build-all        - Build both frontends"
	@echo "  validate-all     - Type check both frontends"
	@echo ""
	@echo "Other:"
	@echo "  build-go         - Build Go binary"
	@echo "  test-go          - Run Go tests"
	@echo "  kill-port        - Kill process on port 8888"
