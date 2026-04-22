<p align="center">
  <img src="assets/logo.png" alt="Claude Code Go Logo" width="200">
</p>

<h1 align="center">Claude Code Go</h1>

<p align="center">
  <strong>🤖 Claude Code의 Go 언어 구현 — 터미널에서 작동하는 AI 코딩 어시스턴트</strong>
</p>

<p align="center">
  <a href="https://golang.org/dl/"><img src="https://img.shields.io/badge/go-1.21+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go Version"></a>
  <a href="https://goreportcard.com/report/github.com/tunsuy/claude-code-go"><img src="https://goreportcard.com/badge/github.com/tunsuy/claude-code-go?style=flat-square" alt="Go Report Card"></a>
  <a href="https://codecov.io/gh/tunsuy/claude-code-go"><img src="https://codecov.io/gh/tunsuy/claude-code-go/branch/main/graph/badge.svg?style=flat-square" alt="커버리지"></a>
  <a href="https://pkg.go.dev/github.com/tunsuy/claude-code-go"><img src="https://pkg.go.dev/badge/github.com/tunsuy/claude-code-go.svg" alt="Go Reference"></a>
  <a href="https://github.com/tunsuy/claude-code-go/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/tunsuy/claude-code-go/ci.yml?branch=main&style=flat-square&logo=github&label=CI" alt="CI"></a>
  <a href="https://github.com/tunsuy/claude-code-go/releases"><img src="https://img.shields.io/github/v/release/tunsuy/claude-code-go?style=flat-square&logo=github" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green?style=flat-square" alt="License"></a>
  <a href="https://github.com/tunsuy/claude-code-go/pulls"><img src="https://img.shields.io/badge/PRs-welcome-brightgreen?style=flat-square" alt="PRs Welcome"></a>
</p>

<p align="center">
  <a href="README.md">English</a> •
  <a href="README.zh-CN.md">简体中文</a> •
  <a href="README.ja.md">日本語</a> •
  <a href="README.ko.md">한국어</a> •
  <a href="README.es.md">Español</a> •
  <a href="README.fr.md">Français</a>
</p>

---

<p align="center">
  <img src="assets/demo.png" alt="Claude Code Go 데모" width="800">
</p>

---

## 소개

이 프로젝트는 [Claude Code](https://claude.ai/code)(Anthropic의 공식 TypeScript CLI)를 **Go 언어로 완전히 재구현**한 것입니다. TUI 인터페이스, 도구 호출, 권한 시스템, 멀티 에이전트 조정, MCP 프로토콜, 세션 관리 등 모든 핵심 기능을 포함합니다.

### AI 에이전트로 개발 — 인간이 작성한 코드 없음

> **이 저장소에는 인간이 작성한 프로덕션 코드가 단 한 줄도 없습니다.**

전체 프로젝트(아키텍처 설계, 상세 설계 문서, 병렬 구현, 코드 리뷰, QA, 통합 테스트)는 구조화된 멀티 에이전트 워크플로우에서 협력하는 **9개의 Claude AI 에이전트**에 의해 생성되었습니다:

```
PM Agent          →  프로젝트 계획, 마일스톤, 작업 스케줄링
Tech Lead Agent   →  아키텍처 설계, 설계 문서 리뷰, 코드 리뷰
Agent-Infra       →  인프라 계층(타입, 설정, 상태, 세션)
Agent-Services    →  서비스 계층(API 클라이언트, OAuth, MCP, 압축)
Agent-Core        →  코어 엔진(LLM 루프, 도구 디스패치, 코디네이터)
Agent-Tools       →  도구 계층(파일, 셸, 검색, 웹 — 18개 도구)
Agent-TUI         →  UI 계층(Bubble Tea MVU, 테마, Vim 모드)
Agent-CLI         →  진입점 계층(Cobra CLI, DI, 부트스트랩 단계)
QA Agent          →  테스트 전략, 계층별 승인, 통합 테스트
```

결과: 약 **7,000줄의 프로덕션 코드 + 전체 테스트 스위트**, `go test -race ./...` 통과.

---

## 기능

- **대화형 TUI** — [Bubble Tea](https://github.com/charmbracelet/bubbletea)로 구축된 완전한 터미널 UI, 다크/라이트 테마 지원
- **에이전트 도구 사용** — 파일 읽기/쓰기, 셸 실행, 검색 등, 모두 권한 계층을 통해 중재
- **멀티 에이전트 조정** — 병렬 작업을 위한 백그라운드 서브 에이전트 생성
- **MCP 지원** — [Model Context Protocol](https://modelcontextprotocol.io)을 통한 외부 도구 연결
- **CLAUDE.md 메모리** — 디렉터리 트리의 `CLAUDE.md` 파일에서 프로젝트 컨텍스트 자동 로드
- **세션 관리** — 이전 대화 재개; 긴 기록 자동 압축
- **Vim 모드** — 입력 영역에서 선택적 Vim 키 바인딩
- **OAuth + API 키 인증** — Anthropic OAuth로 로그인하거나 `ANTHROPIC_API_KEY` 제공
- **18개의 내장 슬래시 명령** — `/help`, `/clear`, `/compact`, `/commit`, `/diff`, `/review`, `/mcp` 등
- **스트리밍 응답** — thinking 블록 표시와 함께 실시간 토큰 스트리밍

## 아키텍처

Claude Code Go는 6개 계층으로 구성됩니다:

```
┌─────────────────────────────────────┐
│  CLI (cmd/claude)                   │  Cobra 진입점
├─────────────────────────────────────┤
│  TUI (internal/tui)                 │  Bubble Tea MVU 인터페이스
├─────────────────────────────────────┤
│  Tools (internal/tools)             │  파일, 셸, 검색, MCP 도구
├─────────────────────────────────────┤
│  Core Engine (internal/engine)      │  스트리밍, 도구 디스패치, 코디네이터
├─────────────────────────────────────┤
│  Services (internal/api, oauth,     │  Anthropic API, OAuth, MCP 클라이언트
│            mcp, compact)            │
├─────────────────────────────────────┤
│  Infra (pkg/types, internal/config, │  타입, 설정, 상태, 훅, 플러그인
│         state, session, hooks)      │
└─────────────────────────────────────┘
```

자세한 내용은 [`docs/project/architecture.md`](docs/project/architecture.md)를 참조하세요.

## 요구 사항

- Go 1.21 이상
- [Anthropic API 키](https://console.anthropic.com/) **또는** Claude.ai 계정(OAuth)

## 설치

### 소스에서 빌드

```bash
git clone https://github.com/tunsuy/claude-code-go.git
cd claude-code-go
make build
# 바이너리는 ./bin/claude에 위치합니다
```

`PATH`에 추가:

```bash
export PATH="$PATH:$(pwd)/bin"
```

### `go install` 사용

```bash
go install github.com/tunsuy/claude-code-go/cmd/claude@latest
```

## 빠른 시작

```bash
# API 키 설정(또는 아래 인증 섹션을 참조하여 OAuth 사용)
export ANTHROPIC_API_KEY=sk-ant-...

# 현재 디렉터리에서 대화형 세션 시작
claude

# 한 번 질문하고 종료
claude -p "이 프로젝트의 메인 진입점을 설명해주세요"

# 가장 최근 세션 재개
claude --resume
```

## 인증

**API 키(CI/스크립트에 권장):**

```bash
export ANTHROPIC_API_KEY=sk-ant-...
```

**OAuth(대화형 사용에 권장):**

```bash
claude /config    # 브라우저에서 OAuth 플로우 열기
```

## API 프로바이더

Claude Code Go는 여러 API 프로바이더를 지원하여 Anthropic의 API뿐만 아니라 OpenAI 호환 API도 사용할 수 있습니다.

### 지원되는 프로바이더

| 프로바이더 | 설명 | 환경 변수 |
|-----------|------|----------|
| `direct` (기본값) | Anthropic Direct API | `ANTHROPIC_API_KEY`, `ANTHROPIC_BASE_URL` |
| `openai` | OpenAI 및 호환 API | `OPENAI_API_KEY`, `OPENAI_BASE_URL` |
| `bedrock` | AWS Bedrock | 환경 변수를 통한 AWS 자격 증명 |
| `vertex` | Google Cloud Vertex AI | 환경 변수를 통한 GCP 자격 증명 |

### OpenAI 호환 API 사용

OpenAI, DeepSeek, Qwen, Moonshot 또는 OpenAI 호환 API를 사용하려면:

**방법 1: 환경 변수**

```bash
# 프로바이더를 openai로 설정
export CLAUDE_PROVIDER=openai

# API 키 설정
export OPENAI_API_KEY=sk-xxx

# 선택사항: 커스텀 Base URL 설정 (OpenAI 호환 서비스용)
export OPENAI_BASE_URL=https://api.deepseek.com  # DeepSeek
# export OPENAI_BASE_URL=https://api.moonshot.cn/v1  # Moonshot
# export OPENAI_BASE_URL=http://localhost:11434/v1  # Ollama

# 모델 설정
export OPENAI_MODEL=deepseek-chat

# Claude Code 실행
claude
```

**방법 2: 설정 파일**

`~/.config/claude-code/settings.json`을 생성하거나 편집:

```json
{
  "provider": "openai",
  "apiKey": "sk-xxx",
  "baseUrl": "https://api.openai.com",
  "model": "gpt-4-turbo",
  "openaiOrganization": "org-xxx",
  "openaiProject": "proj-xxx"
}
```

### 프로바이더별 설정 예시

**OpenAI:**
- 모든 GPT-4 및 GPT-3.5 모델 지원
- 완전한 도구/함수 호출 지원
- 스트리밍 응답

**DeepSeek:**
```bash
export CLAUDE_PROVIDER=openai
export OPENAI_API_KEY=sk-xxx
export OPENAI_BASE_URL=https://api.deepseek.com
export OPENAI_MODEL=deepseek-chat
```

**Ollama (로컬):**
```bash
export CLAUDE_PROVIDER=openai
export OPENAI_BASE_URL=http://localhost:11434/v1
export OPENAI_MODEL=llama3
```

**Azure OpenAI:**
```bash
export CLAUDE_PROVIDER=openai
export OPENAI_API_KEY=your-azure-key
export OPENAI_BASE_URL=https://your-resource.openai.azure.com
export OPENAI_MODEL=your-deployment-name
```

## 사용법

### 대화형 모드

```
claude [flags]
```

| 플래그 | 설명 |
|--------|------|
| `--resume` | 가장 최근 세션 재개 |
| `--session <id>` | ID로 특정 세션 재개 |
| `--model <name>` | 기본 Claude 모델 재정의 |
| `--dark` / `--light` | 다크 또는 라이트 테마 강제 적용 |
| `--vim` | Vim 키 바인딩 활성화 |
| `-p, --print <prompt>` | 비대화형: 단일 프롬프트 실행 후 종료 |

### 슬래시 명령

입력창에서 `/`를 입력하면 사용 가능한 모든 명령을 볼 수 있습니다:

| 명령 | 설명 |
|------|------|
| `/help` | 모든 명령 표시 |
| `/clear` | 대화 기록 지우기 |
| `/compact` | 기록을 요약하여 컨텍스트 사용량 감소 |
| `/exit` | Claude Code 종료 |
| `/model` | Claude 모델 전환 |
| `/theme` | 다크/라이트 테마 전환 |
| `/vim` | Vim 모드 전환 |
| `/commit` | git 커밋 메시지 생성 |
| `/review` | 최근 변경 사항 검토 |
| `/diff` | 현재 diff 표시 |
| `/mcp` | MCP 서버 관리 |
| `/memory` | 로드된 CLAUDE.md 파일 표시 |
| `/session` | 세션 정보 표시 |
| `/status` | API/연결 상태 표시 |
| `/cost` | 토큰 사용량 및 예상 비용 표시 |

## 개발

### 전제 조건

- Go 1.21+
- `golangci-lint`(선택사항, 린팅용)

### 빌드 및 테스트

```bash
# 빌드
make build

# 모든 테스트 실행
make test

# 커버리지 리포트와 함께 테스트 실행
make test-cover

# Vet
make vet

# Lint(golangci-lint 필요)
make lint

# 빌드 + 테스트 + vet
make all
```

## 기여

기여를 환영합니다! Pull Request를 제출하기 전에 [CONTRIBUTING.md](CONTRIBUTING.md)를 읽어주세요.

빠른 체크리스트:
- 저장소를 포크하고 기능 브랜치 생성
- `make test`와 `make vet`가 통과하는지 확인
- 새 기능에 대한 테스트 작성
- 기존 코드 스타일 준수(`gofmt ./...` 실행)
- 제공된 템플릿을 사용하여 Pull Request 열기

## 보안

보안 취약점을 보고하려면 [SECURITY.md](SECURITY.md)를 참조하세요. 보안 버그에 대해 공개 GitHub Issue를 **열지 마세요**.

## 라이선스

이 프로젝트는 MIT 라이선스에 따라 라이선스됩니다 — 자세한 내용은 [LICENSE](LICENSE)를 참조하세요.

## 관련 프로젝트

- [claude-code](https://github.com/anthropics/claude-code) — 원본 TypeScript CLI
- [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go) — Anthropic API 공식 Go SDK
- [Model Context Protocol](https://modelcontextprotocol.io) — AI를 도구에 연결하기 위한 오픈 스탠다드
