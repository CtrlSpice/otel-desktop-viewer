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

.PHONY: format-go
format-go:
	gofmt -w .

.PHONY: format-go-check
format-go-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "These files need gofmt:"; echo "$$unformatted"; exit 1; \
	fi

# Reapply the macro DDL to an existing database file.
#
# Macros live in the DuckDB catalog, so a .db file keeps whatever definitions it
# was last opened with. NewStore re-runs `create or replace` on every open, so
# the app self-heals -- but a file inspected straight from the duckdb CLI can
# show a stale macro, or none at all if it predates one. That is a confusing
# thing to debug cold, and the fix is to reapply them.
#
# The files carry no trailing semicolons (one statement per file, so they read
# cleanly), hence the echo between them.
#
#   make refresh-macros DB=path/to.db
.PHONY: refresh-macros
refresh-macros:
	@test -n "$(DB)" || { echo "usage: make refresh-macros DB=path/to.db"; exit 1; }
	@for f in desktopexporter/internal/store/queries/ddl/macros/*.sql; do cat $$f; echo ";"; done | duckdb "$(DB)"
	@echo "macros reapplied to $(DB)"

.PHONY: test-go
test-go:
	cd desktopexporter && go test ./...

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

.PHONY: populate-traces
populate-traces:
	OTLP_ENDPOINT="$(OTLP_ENDPOINT)" perl "$(CURDIR)/scripts/seed.pl" --traces

.PHONY: populate-logs
populate-logs:
	OTLP_ENDPOINT="$(OTLP_ENDPOINT)" perl "$(CURDIR)/scripts/seed.pl" --logs

.PHONY: populate-metrics
populate-metrics:
	OTLP_ENDPOINT="$(OTLP_ENDPOINT)" perl "$(CURDIR)/scripts/seed.pl" --metrics

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

.PHONY: format-ts
format-ts:
	cd desktopexporter/internal/frontend && npm run format

.PHONY: format-ts-check
format-ts-check:
	cd desktopexporter/internal/frontend && npm run format:check

.PHONY: validate-ts
validate-ts:
	cd desktopexporter/internal/frontend && npm run check

.PHONY: test-ts
test-ts:
	cd desktopexporter/internal/frontend && npm test

.PHONY: build
build: build-ts build-go

.PHONY: run
run: build-ts
	go run . --browser-port 8000

# Mirrors what CI enforces, so a green `make test` means a green PR. The
# format checks run first because they cost seconds and the test suites cost
# minutes -- and because the formatting gap is what actually bit: prettier is
# part of the frontend CI job, `format-ts` only rewrites files, and nothing
# local ran `format:check`, so seven unformatted files went out across several
# commits before CI caught them.
.PHONY: test
test: format-go-check format-ts-check validate-ts test-go test-ts

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
	@echo "  install           - Install frontend dependencies"
	@echo "  install-clean     - Clean install (removes node_modules first)"
	@echo "  build-ts          - Build frontend"
	@echo "  format-ts         - Format frontend code (Prettier)"
	@echo "  format-ts-check   - Fail if any frontend file needs Prettier"
	@echo "  validate-ts       - Type check frontend"
	@echo "  test-ts           - Run frontend unit tests (Vitest)"
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
	@echo "  test              - Run Go tests, frontend type check, and frontend unit tests"
	@echo "  dev-go            - Kill port, start server, seed traces + logs + metrics"
	@echo ""
	@echo "Other:"
	@echo "  release-dry-run      - Trigger release workflow (dry run)"
	@echo "  kill-port            - Kill processes on ports 8000, 4317, 4318"
	@echo "  stop              - Stop Go server and Vite dev server"
