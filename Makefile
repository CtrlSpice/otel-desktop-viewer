.DEFAULT_GOAL := help

.PHONY: install
install:
	cd desktopexporter/internal/frontend && npm install
	cd desktopexporter/internal/frontend && npx playwright install chromium

.PHONY: install-clean
install-clean:
	cd desktopexporter/internal/frontend && rm -rf node_modules package-lock.json && npm install
	cd desktopexporter/internal/frontend && npx playwright install chromium

.PHONY: build-go
build-go:
	go build -o otel-desktop-viewer

.PHONY: format-go
format-go:
	gofmt -w .

.PHONY: format-go-check
format-go-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "These files need gofmt:"; echo "$$unformatted"; exit 1; \
	fi

# Reapply macro DDL in dependency order to a DuckDB catalog.
#   make refresh-macros DB=path/to.db
.PHONY: refresh-macros
refresh-macros:
	@test -n "$(DB)" || { echo "usage: make refresh-macros DB=path/to.db"; exit 1; }
	@dir=desktopexporter/internal/store/queries/ddl/macros; \
	grep -v '^\#' $$dir/_order | grep -v '^$$' | while read -r f; do cat "$$dir/$$f"; echo ";"; done | duckdb "$(DB)"
	@echo "macros reapplied to $(DB)"

.PHONY: test-go
test-go:
	go test ./...

.PHONY: run-go
run-go:
	go run . --browser-port 8000

.PHONY: dev-ts
dev-ts:
	@echo "Starting Vite dev server..."
	@echo "Open http://localhost:3001 for development"
	@echo ""
	cd desktopexporter/internal/frontend && npm run dev

.PHONY: run-go-persist
run-go-persist:
	go run . --db duck.db

OTLP_DATASET ?= $(CURDIR)/testdata/otlp/demo
OTLP_ENDPOINT ?= http://localhost:4318

.PHONY: populate-traces populate-logs populate-metrics
populate-traces populate-logs populate-metrics:
	@set -eu; \
	endpoint="$(OTLP_ENDPOINT)"; endpoint="$${endpoint:-http://localhost:4318}"; \
	signal="$(@:populate-%=%)"; \
	for file in "$(OTLP_DATASET)/$$signal"*.json; do \
		test -f "$$file" || { printf 'No %s requests in %s\n' "$$signal" "$(OTLP_DATASET)" >&2; exit 1; }; \
		curl --fail-with-body --silent --show-error -H 'Content-Type: application/json' \
			--data-binary "@$$file" "$${endpoint%/}/v1/$$signal"; \
		printf '\nLoaded %s\n' "$$file"; \
	done

.PHONY: dev-go
dev-go: kill-port
	@go run . --browser-port 8000 & \
	PID=$$!; \
	echo "Waiting for server (pid $$PID) to start..."; \
	for i in $$(seq 1 30); do \
		if curl -s http://localhost:8000 > /dev/null 2>&1; then \
			echo "Server is up."; \
			break; \
		fi; \
		sleep 1; \
	done; \
	$(MAKE) populate-traces; \
	$(MAKE) populate-logs; \
	$(MAKE) populate-metrics; \
	echo "Server running (pid $$PID). Press Ctrl-C to stop."; \
	wait $$PID

.PHONY: build-ts
build-ts:
	cd desktopexporter/internal/frontend && npm run build && rm -rf ../../internal/server/static/* && cp -r dist/* ../../internal/server/static/

# Verify that the committed bundle matches the frontend sources and lockfile.
.PHONY: build-ts-check
build-ts-check:
	cd desktopexporter/internal/frontend && npm run build
	@diff -r --exclude=.gitkeep \
		desktopexporter/internal/frontend/dist \
		desktopexporter/internal/server/static \
		|| (echo ""; \
		    echo "internal/server/static is stale. Run 'make build-ts' and commit the result."; \
		    exit 1)

.PHONY: format-ts
format-ts:
	cd desktopexporter/internal/frontend && npm run format

.PHONY: format-ts-check
format-ts-check:
	cd desktopexporter/internal/frontend && npm run format:check

.PHONY: lint-ts
lint-ts:
	cd desktopexporter/internal/frontend && npm run lint

.PHONY: validate-ts
validate-ts:
	cd desktopexporter/internal/frontend && npm run check

.PHONY: validate-playwright
validate-playwright:
	cd desktopexporter/internal/frontend && npx tsc --project tsconfig.playwright.json

.PHONY: test-ts
test-ts:
	cd desktopexporter/internal/frontend && npm test

.PHONY: test-a11y
test-a11y:
	cd desktopexporter/internal/frontend && npm run test:a11y

.PHONY: build
build: build-ts build-go

.PHONY: run
run: build-ts
	go run . --browser-port 8000

# Run cheap formatting checks before the test suites.
.PHONY: test
test: format-go-check format-ts-check lint-ts validate-ts validate-playwright build-ts-check test-go test-ts test-a11y

.PHONY: release-dry-run
release-dry-run:
	gh workflow run "Release" --ref $$(git branch --show-current)

.PHONY: kill-port
kill-port:
	@echo "Killing processes on ports 8000, 4317, 4318..."
	@lsof -ti:8000,4317,4318 | xargs kill -9 2>/dev/null || echo "No process found on ports 8000, 4317, 4318"

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
	@echo "  install           - Install frontend dependencies and Playwright Chromium"
	@echo "  install-clean     - Clean install, including Playwright Chromium"
	@echo "  build-ts          - Build frontend"
	@echo "  format-ts         - Format frontend code (Prettier)"
	@echo "  format-ts-check   - Fail if any frontend file needs Prettier"
	@echo "  lint-ts           - Lint frontend code (Oxlint and zero-debt policy rules)"
	@echo "  validate-ts       - Type check frontend"
	@echo "  validate-playwright - Type check Playwright tests"
	@echo "  test-ts           - Run frontend unit tests (Vitest)"
	@echo "  test-a11y         - Run browser accessibility tests (Playwright + axe)"
	@echo "  dev-ts            - Start frontend dev server (Vite)"
	@echo ""
	@echo "Server:"
	@echo "  build-go          - Build Go binary"
	@echo "  format-go         - Format Go code (gofmt)"
	@echo "  format-go-check   - Fail if any Go file needs gofmt"
	@echo "  refresh-macros    - Reapply macro DDL to a .db file (DB=path)"
	@echo "  test-go           - Run Go tests"
	@echo "  run-go            - Run server (in-memory, data lost on exit)"
	@echo "  run-go-persist    - Run server with persistent DB file (data retained)"
	@echo "  populate-traces   - POST sample traces to OTLP HTTP (default localhost:4318)"
	@echo "  populate-logs     - POST sample logs to OTLP HTTP (run after populate-traces to link logs to real traces)"
	@echo "  populate-metrics  - POST sample metrics to OTLP HTTP (default localhost:4318)"
	@echo ""
	@echo "Convenience:"
	@echo "  build             - Build frontend and Go binary"
	@echo "  run               - Build frontend, then run server (in-memory)"
	@echo "  test              - Run the primary local formatting, build, and test gate"
	@echo "  dev-go            - Kill port, start server, seed traces + logs + metrics"
	@echo ""
	@echo "Other:"
	@echo "  release-dry-run      - Trigger release workflow (dry run)"
	@echo "  kill-port            - Kill processes on ports 8000, 4317, 4318"
	@echo "  stop              - Stop Go server and Vite dev server"
