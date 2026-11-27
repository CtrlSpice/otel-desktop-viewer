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
test-go run-go: build-ts
endif

OTLP_DATASET ?= $(CURDIR)/testdata/otlp/small
OTLP_ENDPOINT ?= http://localhost:4318

.PHONY: install-clean build build-ts build-ts-check run-go dev-ts
install-clean:
	cd $(FRONTEND) && npm ci
	cd $(FRONTEND) && npx playwright install chromium

build: build-ts
	go build $(GO_TAGS) -o otel-desktop-viewer

build-ts:
	cd $(FRONTEND) && node scripts/check-lockfile.ts && npm run build
	rm -rf $(STATIC)/*
	mkdir -p $(STATIC)
	cp -R $(FRONTEND_DIST)/. $(STATIC)/

# Used by CI and `make test`. Only EVALS=0 assets are committed.
build-ts-check:
	cd $(FRONTEND) && node scripts/check-lockfile.ts && npm run build
ifeq ($(EVALS),0)
	@diff -r --exclude=.gitkeep \
		$(FRONTEND_DIST) \
		$(STATIC) \
		|| (echo ""; \
		    echo "$(STATIC) is stale. Run 'make build-ts EVALS=0' and commit the result."; \
		    exit 1)
endif

run-go:
	go run $(GO_TAGS) . --browser-port 8000

dev-ts:
	@echo "Starting Vite dev server at http://localhost:3001"
	cd $(FRONTEND) && npm run dev

.PHONY: populate populate-traces populate-logs populate-metrics
populate: populate-traces populate-logs populate-metrics

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

.PHONY: format format-go-check lint test-go test-ts test-a11y test refresh-macros help
format:
	gofmt -w .
	cd $(FRONTEND) && npm run format

# Used by CI and `make lint`.
format-go-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "These files need gofmt:"; echo "$$unformatted"; exit 1; \
	fi

lint: format-go-check
	cd $(FRONTEND) && npm run format:check
	cd $(FRONTEND) && npm run lint
	cd $(FRONTEND) && npm run check
	cd $(FRONTEND) && npx tsc --project config/tsconfig.playwright.json

test-go:
	go test $(GO_TAGS) ./...

test-ts:
	cd $(FRONTEND) && npm test
	cd $(FRONTEND) && npm run test:variant

test-a11y:
	cd $(FRONTEND) && npm run test:a11y

test: lint build-ts-check test-go test-ts test-a11y

# Reapply macro DDL in dependency order to a DuckDB catalog.
#   make refresh-macros DB=path/to.duckdb
refresh-macros:
	@test -n "$(DB)" || { echo "usage: make refresh-macros DB=path/to.duckdb"; exit 1; }
	@dir=desktopexporter/internal/store/queries/ddl/macros; \
	grep -v '^\#' $$dir/_order | grep -v '^$$' | while read -r f; do cat "$$dir/$$f"; echo ";"; done | duckdb "$(DB)"
	@echo "macros reapplied to $(DB)"

help:
	@echo "Available targets (EVALS=0 default public build; EVALS=1 includes Evals):"
	@echo ""
	@echo "Develop:"
	@echo "  install-clean  - Install locked frontend dependencies and Playwright Chromium"
	@echo "  build          - Build embedded frontend and Go binary"
	@echo "  build-ts       - Check lockfile sync, build frontend, and copy embedded assets"
	@echo "  run-go         - Run server with managed sessions (port 8000)"
	@echo "  dev-ts         - Start Vite frontend dev server (port 3001)"
	@echo "  populate       - POST sample traces, logs, and metrics to OTLP HTTP"
	@echo ""
	@echo "Check & tools:"
	@echo "  format         - Format Go and frontend code (gofmt + Prettier)"
	@echo "  lint           - Run Go/TS format checks, Oxlint, svelte-check, and Playwright tsc"
	@echo "  test-go        - Run Go tests"
	@echo "  test-ts        - Run frontend unit tests and Arrow variant checks"
	@echo "  test-a11y      - Run Playwright browser and accessibility tests"
	@echo "  test           - Run full local check, build, and test gate"
	@echo "  refresh-macros - Reapply macro DDL to a DuckDB file (DB=path)"
