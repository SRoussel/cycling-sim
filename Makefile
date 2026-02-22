# ─────────────────────────────────────────────────────────────────────────────
# Cycling Sim — Build & Development Targets
# ─────────────────────────────────────────────────────────────────────────────
# Requires: wails v2 (https://wails.io), Go 1.24+, Node.js 18+
# Linux: needs webkit2gtk-4.1-dev  (pkg: libwebkit2gtk-4.1-dev on Debian/Ubuntu)
# ─────────────────────────────────────────────────────────────────────────────

BINARY   := cycling-sim
BUILD_TAGS := webkit2_41
PLATFORM ?= linux/amd64

.PHONY: all dev build build-debug test seed tidy clean help

all: build

## dev — Start Wails dev server with hot-reload
dev:
	wails dev -tags $(BUILD_TAGS)

## build — Produce a release binary in build/bin/
build:
	wails build -tags $(BUILD_TAGS) -clean

## build-debug — Debug build (not optimised; includes DevTools)
build-debug:
	wails build -tags $(BUILD_TAGS) -debug

## build-appimage — Package as an AppImage (requires appimagetool on PATH)
build-appimage: build
	@echo "AppImage packaging: copy build/bin/$(BINARY) into an AppDir and run appimagetool"

## test — Run all Go tests
test:
	go test ./... -count=1

## test-verbose — Run all Go tests with verbose output
test-verbose:
	go test ./... -v -count=1

## seed — Seed the database with riders, courses, and races
seed:
	go run ./cmd/cycling-sim seed

## tidy — Tidy Go modules and install frontend dependencies
tidy:
	go mod tidy
	cd gui && npm install

## clean — Remove build artefacts
clean:
	rm -rf build/bin/*
	go clean -testcache

## help — Show this help message
help:
	@grep -E '^## ' Makefile | sed 's/^## /  /'
