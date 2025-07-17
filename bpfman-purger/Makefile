.PHONY: all build clean fmt vet test lint check deps help

BINARY_NAME := purge-bpfman
BUILD_DIR := .

all: check build

build: fmt vet
	go build -o $(BINARY_NAME) .

fmt:
	go fmt ./...

vet: fmt
	go vet ./...

test: fmt vet
	go test ./...

lint: fmt vet
	golangci-lint run

check: fmt vet test

deps:
	go mod tidy
	go mod verify

clean:
	rm -f $(BINARY_NAME)
	go clean

help:
	@echo "Available targets:"
	@echo "  all     - Run check and build (default)"
	@echo "  build   - Build the binary (runs fmt and vet first)"
	@echo "  fmt     - Format Go source code"
	@echo "  vet     - Run go vet"
	@echo "  test    - Run tests (runs fmt and vet first)"
	@echo "  lint    - Run golangci-lint (runs fmt and vet first)"
	@echo "  check   - Run fmt, vet, and test"
	@echo "  deps    - Tidy and verify dependencies"
	@echo "  clean   - Remove binary and clean build cache"
	@echo "  help    - Show this help message"