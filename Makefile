PACKAGE_NAME         := github.com/CtrlSpice/otel-desktop-viewer
GOLANG_CROSS_VERSION ?= v1.24.2

SYSROOT_DIR     ?= sysroots
SYSROOT_ARCHIVE ?= sysroots.tar.bz2

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

.PHONY: sysroot-pack
sysroot-pack:
	@tar cf - $(SYSROOT_DIR) -P | pv -s $[$(du -sk $(SYSROOT_DIR) | awk '{print $1}') * 1024] | pbzip2 > $(SYSROOT_ARCHIVE)

.PHONY: sysroot-unpack
sysroot-unpack:
	@pv $(SYSROOT_ARCHIVE) | pbzip2 -cd | tar -xf -

.PHONY: release-dry-run
release-dry-run:
	@docker run \
		--rm \
		-e CGO_ENABLED=1 \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/$(PACKAGE_NAME) \
		-v `pwd`/sysroot:/sysroot \
		-w /go/src/$(PACKAGE_NAME) \
		ghcr.io/goreleaser/goreleaser-cross:${GOLANG_CROSS_VERSION} \
		--clean --skip=validate --skip=publish
