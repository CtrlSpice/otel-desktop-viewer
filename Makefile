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

.PHONY: release-dry-run
release-dry-run:
	gh workflow run "Release" --ref $$(git branch --show-current) -f test_mode=true

.PHONY: kill-port
kill-port:
	@echo "Killing process on port 8888..."
	@lsof -ti:8888 | xargs kill -9 2>/dev/null || echo "No process found on port 8888"
