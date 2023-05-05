# SRC_ROOT is the top of the source tree.
SRC_ROOT := $(shell git rev-parse --show-toplevel)

TOOLS_MOD_DIR    := $(SRC_ROOT)/internal/tools
TOOLS_MOD_REGEX  := "\s+_\s+\".*\""
TOOLS_PKG_NAMES  := $(shell grep -E $(TOOLS_MOD_REGEX) < $(TOOLS_MOD_DIR)/tools.go | tr -d " _\"")
TOOLS_BIN_DIR    := $(SRC_ROOT)/.tools
TOOLS_BIN_NAMES  := $(addprefix $(TOOLS_BIN_DIR)/, $(notdir $(TOOLS_PKG_NAMES)))

GOCMD?= go
GOTEST=$(GOCMD) test
GOOS=$(shell $(GOCMD) env GOOS)
GOARCH=$(shell $(GOCMD) env GOARCH)

IMAGE_NAME=codeboten/collector-with-viewer

.PHONY: install-tools
install-tools: $(TOOLS_BIN_NAMES)

$(TOOLS_BIN_DIR):
	mkdir -p $@

$(TOOLS_BIN_NAMES): $(TOOLS_BIN_DIR) $(TOOLS_MOD_DIR)/go.mod
	cd $(TOOLS_MOD_DIR) && $(GOCMD) build -o $@ -trimpath $(filter %/$(notdir $@),$(TOOLS_PKG_NAMES))

BUILDER             := $(TOOLS_BIN_DIR)/builder

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

.PHONY: build-collector
build-collector: $(BUILDER)
	mkdir -p ./distribution/linux
	GOOS=linux GOARCH=amd64 $(BUILDER) --config ./distribution/manifest.yaml --output-path ./distribution/linux/amd64
	GOOS=linux GOARCH=arm64 $(BUILDER) --config ./distribution/manifest.yaml --output-path ./distribution/linux/arm64
	docker rmi -f ${IMAGE_NAME}:latest
	docker buildx build -t ${IMAGE_NAME}:latest --platform=linux/arm64,linux/amd64 distribution/. --push
