.PHONY: build test test-cover vet lint clean all docs docs-check debt-check parity

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

# 欠账红线：TODO(dep) 与 not-yet-implemented 桩相对基线只降不升
# 详见 scripts/debt-check.sh 与 docs/project/discussions/2026-08-29-process-retrospective.md（建议 1）
debt-check:
	./scripts/debt-check.sh

# 行为对比测试：与原版 TS 二进制（oracle）黑盒对比。
# 未设置 CCG_PARITY_ORACLE 时全部 SKIP，不影响常规 CI。
# 详见 test/parity/README.md（复盘建议 3）
parity: build
	CCG_PARITY_TARGET=bin/claude go test ./test/parity/... -v
