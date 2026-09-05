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
  <img src="assets/image.png" alt="Claude Code Go - 대화형 세션" width="800">
  <br>
  <em>파일 읽기 및 실시간 사고 표시를 지원하는 대화형 TUI</em>
</p>

<p align="center">
  <img src="assets/image1.png" alt="Claude Code Go - 프로젝트 분석" width="800">
  <br>
  <em>아키텍처 세부 정보를 포함한 종합적인 프로젝트 분석</em>
</p>

---

## 소개

이 프로젝트는 [Claude Code](https://claude.ai/code)(Anthropic의 공식 TypeScript CLI)를 **Go 언어로 완전히 재구현**한 것입니다. TUI 인터페이스, 도구 호출, 권한 시스템, 멀티 에이전트 조정, MCP 프로토콜, 세션 관리 등 모든 핵심 기능을 포함합니다.

> **🤖 AI 에이전트로 완전히 구축 — 인간이 작성한 프로덕션 코드가 단 한 줄도 없습니다.** 초기 빌드(2026년 4월)는 9개의 Claude AI 에이전트가 9일간 협력하여 완성했으며, 2026년 8월부터는 한 개의 메인 Claude 세션이 행동 동등성 갭 원장에 따라 프로젝트를 유지 보수하고, 한 명의 인간 담당자가 리뷰하고 병합합니다. [전체 이야기 ↓](#ai-에이전트로-완전히-구축)

## ❤️ 스폰서

> [여기에 게시되길 원하시나요?](https://github.com/tunsuy)

<table>
  <tr>
    <td width="180"><a href="https://www.orcarouter.ai/ref/ref_49eee7ba2e9927450075"><img src="assets/sponsors/orcarouter.png" alt="OrcaRouter" width="150"></a></td>
    <td><strong><a href="https://www.orcarouter.ai/ref/ref_49eee7ba2e9927450075">OrcaRouter</a></strong> — 하나의 게이트웨이로 모든 모델을. OrcaRouter는 각 요청을 200개 이상의 모델(프론티어 및 오픈소스) 중 가장 적합한 모델로 라우팅하며, 요금은 프로바이더 가격 그대로이고 페일오버와 요청 로그도 제공합니다. OpenAI 호환이므로 Claude Code Go에 바로 연결됩니다 — 구체적인 설정은 <a href="#api-프로바이더">API 프로바이더</a> 섹션을 참고하세요.</td>
  </tr>
</table>

*이 섹션의 링크는 추천인(referral) 링크일 수 있습니다.*

## 기능

- **대화형 TUI** — [Bubble Tea](https://github.com/charmbracelet/bubbletea)로 구축된 완전한 터미널 UI, 다크/라이트 테마 지원
- **에이전트 도구 사용** — 34개 내장 도구(파일, 셸, 검색, 웹, 작업, 서브 에이전트 등), 모두 9계층 권한 파이프라인을 통해 중재
- **CLI 하위 명령** — `claude mcp …`, `claude plugin …`, `claude auth …`, `claude doctor`, `claude update`; `mcp` 및 `plugin` 그룹은 TypeScript CLI와 바이트 단위까지 동일한 출력 생성
- **플러그인 관리** — 로컬 마켓플레이스에서 플러그인 설치/활성화/비활성화, 매니페스트 완전 검증 지원(`claude plugin validate`)
- **멀티 에이전트 조정** — 병렬 작업을 위한 백그라운드 서브 에이전트 생성
- **MCP 지원** — [Model Context Protocol](https://modelcontextprotocol.io)을 통한 외부 도구 연결(stdio + HTTP 전송), Claude Desktop에서 가져오기 가능
- **CLAUDE.md 메모리** — 디렉터리 트리의 `CLAUDE.md` 파일에서 프로젝트 컨텍스트 자동 로드
- **세션 관리** — 이전 대화 재개; 긴 기록 자동 압축
- **Vim 모드** — 입력 영역에서 선택적 Vim 키 바인딩
- **OAuth + API 키 인증** — Anthropic OAuth로 로그인(`claude auth login`)하거나 `ANTHROPIC_API_KEY` 제공
- **내장 슬래시 명령** — `/compact`, `/commit`, `/review`, `/model`, `/theme` 등(아래 참조)
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
# 또는 영구적으로 저장:
claude auth login --api-key sk-ant-...
```

**OAuth(대화형 사용에 권장):**

```bash
claude auth login    # 브라우저에서 OAuth 플로우 열기
claude auth status   # 로그인된 계정 확인
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

**OrcaRouter (AI 게이트웨이):**
[OrcaRouter](https://www.orcarouter.ai/ref/ref_49eee7ba2e9927450075)는 AI 게이트웨이입니다. 각 요청을 200개 이상의 모델(프론티어 및 오픈소스) 중 가장 적합한 모델로 라우팅하며, 요금은 프로바이더 가격 그대로이고 페일오버와 요청 로그도 제공합니다. OpenAI 호환이므로 `openai` 프로바이더를 통해 바로 사용할 수 있습니다:

```bash
export CLAUDE_PROVIDER=openai
export OPENAI_API_KEY=<your-orcarouter-key>
export OPENAI_BASE_URL=https://api.orcarouter.ai/v1
export OPENAI_MODEL=orcarouter/auto   # OrcaRouter가 요청마다 모델을 선택
```

*(위 링크는 추천인(referral) 링크입니다.)*

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

### CLI 하위 명령

비대화형 관리 명령입니다. `mcp` 및 `plugin` 그룹은 TypeScript CLI와 출력이 바이트 단위까지 호환됩니다:

| 명령 | 설명 |
|------|------|
| `claude mcp add <name> -- <command> [args…]` | stdio MCP 서버 등록(원격은 `--transport http <name> <url>`, 환경 변수는 `-e KEY=VAL`) |
| `claude mcp list` / `claude mcp get <name>` | 설정된 서버 목록 조회 / 개별 서버 헬스 체크 |
| `claude mcp add-json <name> <json>` | raw JSON으로 서버 등록 |
| `claude mcp add-from-claude-desktop` | Claude Desktop 설정에서 서버 가져오기 |
| `claude mcp remove <name>` / `reset-project-choices` | 서버 제거 / 프로젝트 로컬 선택 초기화 |
| `claude plugin install <plugin>` | 구성된 마켓플레이스에서 플러그인 설치 |
| `claude plugin list` / `enable` / `disable` / `uninstall` / `update` | 설치된 플러그인 관리 |
| `claude plugin validate <path>` | 플러그인 또는 마켓플레이스 매니페스트 검증(`--strict`, `--json`) |
| `claude plugin marketplace add/list/remove/update` | 플러그인 마켓플레이스 관리(로컬 경로) |
| `claude auth login` / `logout` / `status` | OAuth 또는 API 키 인증 |
| `claude doctor` | 환경 상태 점검 |

### 슬래시 명령

입력창에서 `/`를 입력하면 사용 가능한 모든 명령을 볼 수 있습니다:

| 명령 | 설명 |
|------|------|
| `/help` | 모든 명령 표시 |
| `/clear` | 대화 기록 지우기 |
| `/compact` | 기록을 요약하여 컨텍스트 사용량 감소 |
| `/exit` | Claude Code 종료 |
| `/model` | 활성 모델 조회 또는 설정 |
| `/theme` | 색상 테마 전환 |
| `/vim` | Vim 키 바인딩 전환 |
| `/effort` | effort 레벨 설정(low / medium / high) |
| `/status` | 세션 상태 표시 |
| `/cost` | 이 세션의 토큰 사용량 표시 |
| `/session` | 현재 세션 ID 표시 |
| `/memory` | 로드된 CLAUDE.md 메모리 파일 표시 |
| `/dream` | 메모리 통합 수동 트리거 |
| `/review` | Claude로 현재 git diff 검토 |
| `/commit` | Claude에게 git 커밋 작성 및 생성 요청 |
| `/diff` | 현재 git diff 표시 |
| `/init` | 이 프로젝트용 CLAUDE.md 생성 |

`/config`, `/mcp`, `/resume`, `/terminal-setup`는 등록은 되어 있지만 아직 구현되지 않았습니다 — 그동안 MCP 관리는 위의 CLI 하위 명령을 사용하세요.

## AI 에이전트로 완전히 구축

> **이 저장소에는 인간이 작성한 프로덕션 코드가 단 한 줄도 없습니다.**

전체 프로젝트(아키텍처 설계, 상세 설계 문서, 병렬 구현, 코드 리뷰, QA, 통합 테스트)는 구조화된 멀티 에이전트 워크플로우에서 협력하는 **9개의 Claude AI 에이전트**에 의해 생성되었습니다:

```
PM Agent          →  프로젝트 계획, 마일스톤, 작업 스케줄링
Tech Lead Agent   →  아키텍처 설계, 설계 문서 리뷰, 코드 리뷰
Agent-Infra       →  인프라 계층(타입, 설정, 상태, 세션)
Agent-Services    →  서비스 계층(API 클라이언트, OAuth, MCP, 압축)
Agent-Core        →  코어 엔진(LLM 루프, 도구 디스패치, 코디네이터)
Agent-Tools       →  도구 계층(파일, 셸, 검색, 웹 — 빌드 시점 기준 18개 도구)
Agent-TUI         →  UI 계층(Bubble Tea MVU, 테마, Vim 모드)
Agent-CLI         →  진입점 계층(Cobra CLI, DI, 부트스트랩 단계)
QA Agent          →  테스트 전략, 계층별 승인, 통합 테스트
```

결과: **9일 만에 약 7,000줄의 프로덕션 코드 + 전체 테스트 스위트** 완성, `go test -race ./...` 통과.

이는 규모가 있는 다계층 Go 애플리케이션이 AI 에이전트들의 비동기 협업만으로 설계, 구현, 리뷰, 출시까지 완전히 가능하다는 실제 사례입니다. 2026년 8월부터는 프로젝트가 같은 정신으로 유지 보수되고 있습니다 — 원본 TypeScript CLI와의 행동 동등성 갭 원장(behavior-parity gap ledger)을 한 개의 메인 Claude 세션이 작업하고, 한 명의 인간 담당자가 리뷰하고 병합합니다. 여전히 인간이 작성한 프로덕션 코드는 단 한 줄도 없습니다(현재 약 33,000줄). 전체 의사결정 기록은 [`docs/project/`](docs/project/)에서 확인할 수 있습니다.

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

## 로드맵

초기 빌드(2026년 4월)는 9일 만에 완전한 6계층 아키텍처를 완성했습니다. 2026년 8월부터 이 프로젝트는 **유지 보수 모드**에 있습니다: 개발은 이 CLI를 원본 TypeScript 버전과 비교하는 행동 동등성 갭 원장([`test/parity/cases.md`](test/parity/cases.md))이 주도하며 — 닫힌 갭은 모두 테스트로 바이트 단위까지 고정되고, 의도적인 차이는 그 이유와 함께 `waived`로 기록됩니다.

최근 마일스톤:

- **2026-09-05** — `mcp`(7개 명령) 및 `plugin`(8개 명령 + `marketplace` 하위 트리) CLI 그룹 추가, 출력이 TS CLI와 바이트 단위까지 동일; 사용자 노출 스텁 42 → 27로 감소
- **2026-08-30** — `--version` 형식 동등화; TS CLI 대비 전체 하위 명령 인벤토리 작성(§B11)
- **2026-08-29** — 유지 보수 모드 프로세스 착수: CI 강제 부채 레드라인, 패리티 테스트 스켈레톤

원래의 단계별 계획(v0.2.0 → v1.0.0)은 단계별 완료 상태와 함께 [`docs/ROADMAP.md`](docs/ROADMAP.md)에 보존되어 있습니다.

### 현재 상태

```
✅ 완료:  코어 엔진 + 스트리밍, TUI, 34개 도구, 17개 슬래시 명령(4개 스텁),
          mcp/plugin/auth/doctor CLI 하위 명령, 9계층 권한 파이프라인,
          컨텍스트 압축(snip/micro/auto), OAuth, 세션 지속성
🔧 모드:  유지 보수 — TypeScript CLI 대비 갭 원장 기반 동등성 작업
📉 부채:  56개 TODO(dep) / 27개 사용자 노출 스텁, CI 강제 감소 전용 레드라인
🚀 릴리스: v0.8.0, 5개 플랫폼 바이너리
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
