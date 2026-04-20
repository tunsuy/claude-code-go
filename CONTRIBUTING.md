# Contributing to Claude Code Go

Thank you for your interest in contributing! This document explains how to set up your development environment, our coding standards, and the pull request process.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Making Changes](#making-changes)
- [Testing](#testing)
- [Pull Request Process](#pull-request-process)
- [Coding Standards](#coding-standards)
- [Architecture Overview](#architecture-overview)
- [Reporting Bugs](#reporting-bugs)
- [Requesting Features](#requesting-features)

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior to [opensource@anthropic.com](mailto:opensource@anthropic.com).

## Getting Started

1. **Fork** the repository on GitHub.
2. **Clone** your fork locally:
   ```bash
   git clone https://github.com/<your-username>/claude-code-go.git
   cd claude-code-go
   ```
3. **Add the upstream remote**:
   ```bash
   git remote add upstream https://github.com/anthropics/claude-code-go.git
   ```

## Development Setup

**Requirements:**
- Go 1.21 or later
- `git`
- (Optional) [`golangci-lint`](https://golangci-lint.run/usage/install/) for linting

**Build and verify everything works:**

```bash
make all    # runs build + test + vet
```

**Individual targets:**

```bash
make build        # compile → bin/claude
make test         # run all tests with -race
make test-cover   # run tests and open HTML coverage report
make vet          # go vet ./...
make lint         # golangci-lint run (requires golangci-lint)
make clean        # remove build artifacts
```

## Making Changes

1. **Create a feature branch** from `main`:
   ```bash
   git checkout -b feat/my-feature
   ```
   Branch naming convention:
   - `feat/<description>` — new features
   - `fix/<description>` — bug fixes
   - `docs/<description>` — documentation only
   - `refactor/<description>` — code refactoring
   - `test/<description>` — adding or improving tests

2. **Make your changes.** Keep commits focused; one logical change per commit.

3. **Write or update tests.** New functionality must be covered by unit tests. Bug fixes should include a regression test.

4. **Run the test suite** and ensure everything passes:
   ```bash
   make test
   make vet
   ```

5. **Keep your branch up to date**:
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

## Testing

Tests live alongside the code they test (e.g., `internal/tui/tui_test.go`).

```bash
# Run all tests
make test

# Run tests for a specific package
go test ./internal/tui/...

# Run a specific test
go test -run TestRenderSystemMessage ./internal/tui/...

# Generate coverage report
make test-cover
```

**Coverage targets:**
- All new packages should aim for ≥ 60% test coverage.
- Pull requests that significantly reduce coverage may be asked to add more tests.

**Notes on TUI testing:**
- The TUI layer (`internal/tui`) uses the Bubble Tea MVU pattern with pure value semantics, making it straightforward to unit-test `Update()` and rendering functions without a real terminal.
- Use `lipgloss`-agnostic assertions when checking rendered output (check for individual words rather than exact strings, because ANSI codes may be injected between tokens).

## Pull Request Process

1. **Open a draft PR** early if you want feedback before the work is complete.
2. Fill in the **PR template** completely — describe what changed, why, and how to test it.
3. Ensure all **CI checks pass** (build, test, vet).
4. Request a review from at least one maintainer.
5. Address review comments. When all conversations are resolved and CI is green, a maintainer will merge your PR.

**What we look for in reviews:**
- Correctness and test coverage
- Adherence to the layered architecture (see below)
- Clear, idiomatic Go code
- No unnecessary dependencies added
- Documentation updated where relevant

## Coding Standards

- **Format**: All code must be formatted with `gofmt`. CI will fail on unformatted files.
- **Imports**: Group imports as stdlib / external / internal, separated by blank lines.
- **Error handling**: Return errors; do not swallow them silently. Wrap with `fmt.Errorf("context: %w", err)`.
- **Comments**: All exported identifiers must have doc comments.
- **No global mutable state** outside of `internal/config` and `internal/state`.
- **Context propagation**: Pass `context.Context` as the first argument to functions that do I/O or may be cancelled.
- **Concurrency**: Prefer channels over shared memory. Protect shared state with mutexes and document which fields are guarded.

## Architecture Overview

The project is split into six layers. Changes should respect layer boundaries — lower layers must not import from higher ones:

```
CLI  →  TUI  →  Tools  →  Engine  →  Services  →  Infra
```

| Layer | Package(s) | Responsibility |
|-------|-----------|----------------|
| Infra | `pkg/types`, `internal/config`, `internal/state`, `internal/session`, `internal/hooks` | Types, config, persistence, plugin API |
| Services | `internal/api`, `internal/oauth`, `internal/mcp`, `internal/compact` | Anthropic API client, OAuth, MCP, compaction |
| Engine | `internal/engine` | Streaming query loop, tool dispatch, multi-agent coordinator |
| Tools | `internal/tools`, `internal/tool`, `internal/permissions` | Built-in tools (file, shell, search), permission gating |
| TUI | `internal/tui`, `internal/commands` | Bubble Tea UI, slash commands |
| CLI | `cmd/claude`, `internal/bootstrap` | Cobra entry point, app initialisation |

For a deeper dive see [`docs/project/architecture.md`](docs/project/architecture.md).

## Reporting Bugs

Please use the [bug report template](.github/ISSUE_TEMPLATE/bug_report.md) when opening a bug issue. Include:
- Claude Code Go version (`claude --version`)
- Go version (`go version`)
- OS and terminal emulator
- Steps to reproduce
- Expected vs actual behaviour
- Any relevant logs or screenshots

## Requesting Features

Use the [feature request template](.github/ISSUE_TEMPLATE/feature_request.md). Describe the problem you are trying to solve and your proposed solution. For large changes, consider opening a discussion first to align on approach before investing time in implementation.

## Questions?

Feel free to open a [GitHub Discussion](https://github.com/tunsuy/claude-code-go/discussions) for questions, ideas, or general feedback.
