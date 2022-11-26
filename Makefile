.PHONY: install
install:
	cd desktop-exporter; npm install

.PHONY: build-go
build-go:
	go build ./...

.PHONY: test-go
test-go:
	go test ./...
	
.PHONY: run-go
run-go:
	go run ./... --config config.yaml

.PHONY: build-js
build-js:
	cd desktop-exporter; npx esbuild --bundle app/main.jsx app/main.css --outdir=static

.PHONY: watch-js
watch-js:
	cd desktop-exporter; npx esbuild --watch --bundle app/main.jsx app/main.css --outdir=static

.PHONY: format-js
format-js:
	cd desktop-exporter; npx prettier -w app
