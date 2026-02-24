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
## GDK_BACKEND=wayland: this is a pure Wayland session (no Xwayland / no .Xauthority).
## Without it, Wails forces GDK_BACKEND=x11 (because XDG_SESSION_TYPE is unset)
## and GTK panics trying to connect to an X server that doesn't exist.
dev:
	GDK_BACKEND=wayland wails dev -tags $(BUILD_TAGS)

## build — Produce a release binary in build/bin/
build:
	GDK_BACKEND=wayland wails build -tags $(BUILD_TAGS) -clean

## build-debug — Debug build (not optimised; includes DevTools)
build-debug:
	GDK_BACKEND=wayland wails build -tags $(BUILD_TAGS) -debug

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
