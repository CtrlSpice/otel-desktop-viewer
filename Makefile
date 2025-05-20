PACKAGE_NAME         := github.com/CtrlSpice/otel-desktop-viewer
GOLANG_CROSS_VERSION ?= v1.24.2

.PHONY: install
install:
	cd desktopexporter/internal/app; npm install

.PHONY: build-go
build-go:
	cd desktopcollector; go build -o ../otel-desktop-viewer

.PHONY: test-go
test-go:
	cd desktopexporter; go test ./...
	
.PHONY: run-go
run-go:
	cd desktopcollector; STATIC_ASSETS_DIR=$(abspath ./desktopexporter/internal/server/static/) go run ./...

.PHONY: run-db-go
run-db-go:
	cd desktopcollector; STATIC_ASSETS_DIR=$(abspath ./desktopexporter/internal/server/static/) go run ./... --db ../duck.db

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
