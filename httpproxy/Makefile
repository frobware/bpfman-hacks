# Get version from git, fallback to 'dev' if no tags exist
VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo "dev")

# Build flags
LDFLAGS = -X main.version=$(VERSION)

# Default target
.PHONY: all
all: test

# Run tests
.PHONY: test
test:
	go test -v ./...

# Format check
.PHONY: fmt-check
fmt-check:
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "Error: Go code is not properly formatted. Run 'make fmt' to fix:"; \
		gofmt -l .; \
		exit 1; \
	fi

# Format code
.PHONY: fmt
fmt:
	gofmt -w .

# Vet code
.PHONY: vet
vet:
	go vet ./...

# Check for whitespace issues
.PHONY: whitespace-check
whitespace-check:
	@if git diff --check --cached | grep .; then \
		exit 1; \
	fi
	@if git rev-parse --verify HEAD >/dev/null 2>&1; then \
		if git diff --check HEAD | grep .; then \
			exit 1; \
		fi; \
	fi

# Verify go.mod and go.sum are tidy
.PHONY: mod-tidy-check
mod-tidy-check:
	@cp go.mod go.mod.backup
	@cp go.sum go.sum.backup
	@go mod tidy
	@if ! diff go.mod go.mod.backup || ! diff go.sum go.sum.backup; then \
		echo "Error: go.mod or go.sum needs updating. Run 'go mod tidy'"; \
		mv go.mod.backup go.mod; \
		mv go.sum.backup go.sum; \
		exit 1; \
	fi
	@rm go.mod.backup go.sum.backup

# Run all checks
.PHONY: check
check: whitespace-check fmt-check vet mod-tidy-check

# CI target: run all checks and build
.PHONY: ci
ci: check test build

# Show version that would be built
.PHONY: version
version:
	@echo $(VERSION)

# Development build (same as build, but explicit)
.PHONY: dev
dev: build

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  all            - Build the binary (default)"
	@echo "  clean          - Remove build artifacts"
	@echo "  test           - Run tests"
	@echo "  fmt            - Format code"
	@echo "  fmt-check      - Check code formatting"
	@echo "  vet            - Run go vet"
	@echo "  mod-tidy-check - Check if go.mod/go.sum need tidying"
	@echo "  check          - Run all checks (format, vet, mod-tidy, whitespace)"
	@echo "  ci             - Run all checks, tests, and build"
	@echo "  version        - Show version that would be built"
	@echo "  dev            - Development build (alias for build)"
	@echo "  help           - Show this help"
