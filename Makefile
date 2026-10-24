.DEFAULT_GOAL := help

EVALS ?= 0
ifneq ($(EVALS),0)
ifneq ($(EVALS),1)
$(error EVALS must be 0 or 1)
endif
endif
export EVALS

FRONTEND := desktopexporter/internal/frontend
FRONTEND_DIST := $(FRONTEND)/dist
STATIC := desktopexporter/internal/server/static
GO_TAGS := -tags=
ifeq ($(EVALS),1)
FRONTEND_DIST := $(FRONTEND)/dist-evals
STATIC := desktopexporter/internal/server/static-evals
GO_TAGS := -tags=evals
# The enabled embed directory is generated, not committed.
test-go run-go run-go-persist: build-ts
endif

.PHONY: install
install:
	cd desktopexporter/internal/frontend && npm install
	cd desktopexporter/internal/frontend && npx playwright install chromium

.PHONY: install-clean
install-clean:
	cd desktopexporter/internal/frontend && npm ci
	cd desktopexporter/internal/frontend && npx playwright install chromium

.PHONY: build-go
build-go: build-ts
	go build $(GO_TAGS) -o otel-desktop-viewer

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
	go test $(GO_TAGS) ./...

.PHONY: run-go
run-go:
	go run $(GO_TAGS) . --browser-port 8000

.PHONY: dev-ts
dev-ts:
	@echo "Starting Vite dev server..."
	@echo "Open http://localhost:3001 for development"
	@echo ""
	cd desktopexporter/internal/frontend && npm run dev

.PHONY: run-go-persist
run-go-persist:
	go run $(GO_TAGS) . --db duck.db

OTLP_DATASET ?= $(CURDIR)/testdata/otlp/small
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

.PHONY: frontend-build build-ts
frontend-build:
	cd $(FRONTEND) && npm run build

build-ts: frontend-build
	rm -rf $(STATIC)/*
	mkdir -p $(STATIC)
	cp -R $(FRONTEND_DIST)/. $(STATIC)/

# Only the public bundle is committed; enabled builds verify compilation only.
.PHONY: build-ts-check
build-ts-check: frontend-build
ifeq ($(EVALS),0)
	@diff -r --exclude=.gitkeep \
		$(FRONTEND_DIST) \
		$(STATIC) \
		|| (echo ""; \
		    echo "$(STATIC) is stale. Run 'make build-ts EVALS=0' and commit the result."; \
		    exit 1)
endif

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
build: build-go

.PHONY: run
run: build-ts
	go run $(GO_TAGS) . --browser-port 8000

# Run cheap formatting checks before the test suites.
.PHONY: test
test: format-go-check format-ts-check lint-ts validate-ts validate-playwright build-ts-check test-go test-ts test-a11y

.PHONY: release-dry-run
release-dry-run:
	gh workflow run "Release" --ref $$(git branch --show-current)

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  EVALS=0 (default): public build; EVALS=1: include Evals"
	@echo ""
	@echo "Frontend:"
	@echo "  install           - Install frontend dependencies and Playwright Chromium"
	@echo "  install-clean     - Install locked frontend dependencies and Playwright Chromium"
	@echo "  build-ts          - Build frontend and copy selected embedded assets"
	@echo "  build-ts-check    - Build frontend; check committed public bundle when EVALS=0"
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
	@echo "  build-go          - Build selected embedded assets, then Go binary"
	@echo "  format-go         - Format Go code (gofmt)"
	@echo "  format-go-check   - Fail if any Go file needs gofmt"
	@echo "  refresh-macros    - Reapply macro DDL to a .db file (DB=path)"
	@echo "  test-go           - Run Go tests"
	@echo "  run-go            - Run server (in-memory, data lost on exit)"
	@echo "  run-go-persist    - Run server with persistent DB file (data retained)"
	@echo "  populate-traces   - POST sample traces to OTLP HTTP (default localhost:4318)"
	@echo "  populate-logs     - POST sample logs to OTLP HTTP (default localhost:4318)"
	@echo "  populate-metrics  - POST sample metrics to OTLP HTTP (default localhost:4318)"
	@echo ""
	@echo "Convenience:"
	@echo "  build             - Build frontend and Go binary"
	@echo "  run               - Build frontend, then run server (in-memory)"
	@echo "  test              - Run the primary local formatting, build, and test gate"
	@echo ""
	@echo "Other:"
	@echo "  release-dry-run   - Trigger release workflow (snapshot)"
