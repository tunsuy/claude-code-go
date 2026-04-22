# Changelog

All notable changes to Claude Code Go will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/tunsuy/claude-code-go/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/tunsuy/claude-code-go/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/tunsuy/claude-code-go/releases/tag/v0.1.1
