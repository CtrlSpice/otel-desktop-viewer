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
	SERVE_FROM_FS=true cd desktopcollector; go run ./...

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