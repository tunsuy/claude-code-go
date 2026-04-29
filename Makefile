.PHONY: build test test-cover vet lint clean all docs docs-check

build:
	go build -o bin/claude ./cmd/claude

test:
	go test -race ./...

test-cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

vet:
	go vet ./...

lint:
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed. Run: brew install golangci-lint" && exit 1)
	golangci-lint run ./...

docs:
	go run ./cmd/docgen -out docs/generated ./internal ./pkg ./cmd

docs-check:
	go run ./cmd/docgen -out docs/generated -check ./internal ./pkg ./cmd

clean:
	rm -rf bin/ coverage.out coverage.html

all: vet test build
