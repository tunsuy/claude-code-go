# Changelog

All notable changes to Claude Code Go will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/anthropics/claude-code-go/compare/HEAD...HEAD
