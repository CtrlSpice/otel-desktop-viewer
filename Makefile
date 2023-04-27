.PHONY: install
install:
	cd desktopexporter; npm install

.PHONY: build-go
build-go:
	go build

.PHONY: test-go
test-go:
	go test ./...
	
.PHONY: run-go
run-go:
	SERVE_FROM_FS=true go run ./...

.PHONY: build-js
build-js:
	cd desktopexporter; npx esbuild --bundle app/main.tsx app/main.css --outdir=static

.PHONY: watch-js
watch-js:
	cd desktopexporter; npx esbuild --watch --bundle app/main.tsx app/main.css --outdir=static

.PHONY: format-js
format-js:
	cd desktopexporter; npx prettier -w app

# esbuild will compile typescript files but will not typecheck them. This runs the
# typescript typechecker but does not build the files.
.PHONY: validate-typescript
validate-typescript:
	cd desktopexporter; npx tsc --noEmit
