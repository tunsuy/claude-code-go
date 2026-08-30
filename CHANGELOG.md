# Changelog

All notable changes to Claude Code Go will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.8.0] - 2026-08-30

### Added

- **Maintenance-mode mechanisms (PR #20)**: CI debt red lines (`make debt-check`, `scripts/debt-check.sh`) capping `TODO(dep)` and user-visible `not yet implemented` stubs at monotonically decreasing baselines; sealed CLAUDE.md registry; parity testing skeleton (`test/parity/`) with three-tier oracle comparison.
- **Behavior parity against the TS original (PR #21)**: `--version` now prints `<semver> (Claude Code)` (parity case A1); unknown-subcommand semantics documented and waived with a Go-side guard test (`TestTargetUnknownSubcommandError`).
- Subcommand inventory vs oracle 2.1.251 (11 top-level + 9 second-level gaps, `test/parity/cases.md` §B11).
- Memory system and permission system gap closures (T-060..T-064, PR #19).
- Multi-agent gap closures: model selection, custom agents, task CRUD (PR #18).

### Fixed

- **Release binaries now report the real version**: goreleaser ldflags targeted a nonexistent `main.version` variable, so all releases v0.1.0–v0.7.0 printed `claude 0.1.0` from `--version`. Now injects `bootstrap.appVersion` — released binaries print `<semver> (Claude Code)`.
- Removed dead homebrew-tap publish config (repository/token never existed).

### Changed

- Project entered maintenance mode (2026-08-29): single-owner, gap-analysis-driven, feat branch + PR. Multi-agent build-era process docs are historical archive only.

## [0.7.0] - 2026-04-29

### Added

- Auto-generated API context docs (`make docs`, cmd/docgen): per-package `CONTEXT.md` with exported signatures, change impact chains, dependency graphs; `make docs-check` verifies sync and requires Design Notes.

## [0.6.0] - 2026-04-28

### Added

- Multi-agent system enhancements (P0/P1/P2 from gap analysis).

## [0.5.0] - 2026-04-28

### Added

- Mid-session message processing: unified command queue, query guard, mid-turn drain, between-turn drain.

## [0.4.1] - 2026-04-28

### Fixed

- Memory system P0+P1 fixes: forked agent extraction, multi-scope CLAUDE.md, lint errors.

## [0.4.0] - 2026-04-23

### Added

- Multi-agent coordination system (spawn, message routing, coordinator panel).

## [0.3.0] - 2026-04-22

### Added

- (Backfilled 2026-08-30 — see git history `git log v0.2.1..v0.3.0` for the authoritative record.)

## [0.2.1] - 2026-04-23

### Added
- **Permission System Integration**: Full integration of permission checker with engine orchestration
- Permission ask/response message types for TUI communication
- Permission dialog handling in TUI layer
- Security enhancement GitHub Issue template
- Permission system gap analysis documentation (Go vs TypeScript comparison)

### Changed
- Refactored engine to call permission checker before tool execution
- Wired permission checker into bootstrap initialization
- Reorganized analysis documentation into `docs/analysis/origin/` subdirectory
- Updated ROADMAP with detailed permission system security enhancement tasks

### Documentation
- Added `docs/analysis/permission-system-gap-analysis.md` - comprehensive gap analysis
- Added `docs/analysis/permission-system-issues.md` - GitHub Issue templates
- Added `.github/ISSUE_TEMPLATE/security_enhancement.yml`

## [0.2.0] - 2026-04-22

### Added
- **OpenAI Provider Support**: Full integration with OpenAI API, including GPT-4o, GPT-4-turbo, and other OpenAI models
- Multi-provider architecture with unified interface for LLM backends
- OpenAI streaming (SSE) response handling with delta accumulation
- OpenAI tool calling support with format conversion
- `/provider` command to switch between Anthropic and OpenAI at runtime
- Debug logging for API requests/responses (enable with `CLAUDE_DEBUG=1`)
- Welcome screen with quick start guide and feature highlights
- Enhanced TUI with improved message rendering and status display

### Changed
- Refactored API client layer to support pluggable providers
- Updated configuration to support multiple API keys (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`)
- Improved error handling and retry logic for API calls
- Enhanced mouse event handling using new Bubble Tea API

### Fixed
- Fixed golangci-lint errors (errcheck, ineffassign, staticcheck)
- Fixed deprecated Bubble Tea mouse event API usage
- Various TUI rendering improvements

## [0.1.1] - 2026-04-20

### Added
- Full TUI layer with Bubble Tea MVU architecture (dark/light themes, Vim mode, coordinator panel)
- 18 built-in slash commands (`/help`, `/clear`, `/compact`, `/commit`, `/diff`, `/review`, `/mcp`, `/memory`, `/session`, `/status`, `/cost`, `/model`, `/theme`, `/vim`, `/config`, `/init`, `/resume`, `/terminal-setup`)
- Multi-agent coordinator for parallel background tasks
- MCP (Model Context Protocol) server support
- CLAUDE.md memory file auto-loading from directory tree
- Session persistence and resumption (`--resume`, `--session`)
- OAuth2 authentication flow alongside API key auth
- Permission layer for tool use (allow/deny/always-allow)
- Streaming responses with real-time token display and thinking-block rendering
- Conversation compaction (`/compact`) to reduce context usage
- Built-in tools: file read/write/list, shell execution, web search, grep, glob
- Hook system for pre/post-tool extensibility
- Plugin API for custom tool registration
- Comprehensive unit tests across all layers (≥60% coverage target)
- GitHub Actions CI workflow (build, test, vet)
- Open-source documentation (README, LICENSE, CONTRIBUTING, CODE_OF_CONDUCT, SECURITY)

[Unreleased]: https://github.com/tunsuy/claude-code-go/compare/v0.8.0...HEAD
[0.8.0]: https://github.com/tunsuy/claude-code-go/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/tunsuy/claude-code-go/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/tunsuy/claude-code-go/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/tunsuy/claude-code-go/compare/v0.4.1...v0.5.0
[0.4.1]: https://github.com/tunsuy/claude-code-go/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/tunsuy/claude-code-go/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/tunsuy/claude-code-go/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/tunsuy/claude-code-go/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/tunsuy/claude-code-go/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/tunsuy/claude-code-go/releases/tag/v0.1.1
