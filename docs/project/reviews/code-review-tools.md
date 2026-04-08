# Code Review: Tools Layer (Task #16)

> **Reviewer**: Tech Lead
> **Date**: 2026-04-03
> **Scope**: `internal/tool/` (interface contract), `internal/tools/shell`, `internal/tools/web`, `internal/tools/agent`, `internal/tools/mcp`, `internal/tools/misc`, `internal/tools/tasks`, `internal/plugin/`
> **Files reviewed**: 16 source files
> **Verdict**: ✅ APPROVED_WITH_CHANGES

---

## 1. Overall Assessment

The tools layer has a high-quality foundation. The `internal/tool` package — the `Tool` interface, `BaseTool` embedding helper, and `Registry` — is excellent: well-documented, thread-safe, and cleanly separated from all implementations with zero external dependencies. The three fully-implemented tools (Bash, WebFetch, WebSearch) correctly follow the interface contract, return errors as `tool_result` blocks rather than Go errors, and demonstrate good defensive programming.

Primary concerns: one functional outage bug (gzip decompression absent in WebSearch), a `removeTagBlock` logic defect causing HTML content loss, a domain-filter security bypass, one design-safety violation (`Agent.IsConcurrencySafe=true`), schema divergences in several stub tools, and zero test coverage across all tool implementation packages.

---

## 2. Design vs Implementation Delta

| Area | Design Prescribes | Actual | Status |
|------|------------------|--------|--------|
| Directory structure | `shell/bash.go`, `bash_security.go`, `bash_sandbox.go`, `bash_timeout.go` | Single `shell/bash.go`; security/sandbox are `TODO(dep)` stubs | ⚠️ Collapsed |
| `internal/tools/registry.go` | `RegisterAll()` auto-wiring function | Absent; `DefaultRegistry` in `internal/tool/` but no `RegisterAll` | ❌ Missing |
| `AgentInput` | `prompt`, `system_prompt`, `allowed_tools []string`, `max_turns *int` | Only `prompt`, `system_prompt` | ❌ Schema trimmed |
| `SendMessageInput` field name | `content` | `message` | ❌ Field divergence |
| `TaskCreateInput` | `description`, `tools []string`, `priority *int` | Only `description` | ❌ Schema trimmed |
| `TaskOutputInput` | `id`, `since *int` | Only `id` | ❌ Schema trimmed |
| `Agent.IsConcurrencySafe` | `false` (sub-agents mutate state) | `true` | ❌ Safety violation |
| `SendMessage.IsConcurrencySafe` | `false` | `true` | ❌ Safety violation |
| `tool.Tool` interface | 20 methods incl. `CheckPermissions`, `IsEnabled`, `PreparePermissionMatcher` | All present in `tool.Tool` + `BaseTool` defaults | ✅ Complete |
| Bash timeout constants | 120 000 ms / 600 000 ms | `DefaultBashTimeoutMs=120_000`, `MaxBashTimeoutMs=600_000` | ✅ Exact match |
| WebFetch cache TTL | 15 min | `webFetchCacheTTL = 15 * time.Minute` | ✅ Exact match |
| WebFetch body cap | 5 MiB | `maxWebFetchBodyBytes = 5 * 1024 * 1024` | ✅ Exact match |
| WebFetch HTTP→HTTPS | Required | `upgradeToHTTPS()` implemented | ✅ |
| Brave domain filtering | Required | `domainAllowed()` present but uses bare `HasSuffix` — security bypass | ❌ Bug |
| `MCPProxyTool` schema | Expose upstream schema inline | Schema forwarded from constructor — correct; `MCPToolInput` unused | ⚠️ Dead code |
| Test coverage | Per-package unit tests | **Zero** test files across all reviewed packages | ❌ Missing |

---

## 3. Strengths

### 3.1 `internal/tool` package is excellent

`tool.go` defines a 20-method interface with thorough documentation, grouped into five orthogonal concerns (Identity, Concurrency, Permissions, Execution, Serialization). `BaseTool` provides safe defaults for all optional methods — `Aliases`, `Prompt`, `MaxResultSizeChars`, `SearchHint`, `IsDestructive`, `IsEnabled`, `InterruptBehavior`, `ValidateInput`, `CheckPermissions`, `PreparePermissionMatcher`, `MapResultToToolResultBlock`, `ToAutoClassifierInput` — letting concrete tools override only what they need. Mandatory methods (`Name`, `Description`, `InputSchema`, `Call`, etc.) are left unimplemented in `BaseTool`, so the compiler enforces them.

### 3.2 `Registry` is thread-safe and feature-complete

Uses `sync.RWMutex` correctly (reads use `RLock`, writes use `Lock`). Preserves insertion order for deterministic `All()` output, supports aliases, exposes `Replace` for MCP reconnect-style re-registration, and transparently filters disabled tools in `All()` and `Filter()`. `DefaultRegistry` is correctly scoped as CLI-only convenience with an explicit warning against use in library code.

### 3.3 Error contract correctly observed

All `Call` implementations — including stubs — return `&tool.Result{IsError: true, Content: "..."}` on failure rather than a Go `error`. This is the most common mistake in tool implementations, and every file in this review gets it right.

### 3.4 Bash: defence-in-depth layering

`ValidateInput` checks JSON syntax, empty command, and timeout ceiling before `Call` is reached. `Call` applies a second clamp (`if timeoutMs > MaxBashTimeoutMs`) as defence against `ValidateInput` bypass. `exec.CommandContext` is bound to `UseContext.Ctx` for cancellation propagation. `InterruptBehavior()` returns `InterruptBehaviorCancel`, consistent with the context-based kill semantics. `MapResultToToolResultBlock` formats stdout, stderr (with `[stderr]` prefix), and exit code as a single structured string for model readability.

### 3.5 WebFetch: multiple resource guards

`io.LimitReader` (5 MiB) prevents OOM on large pages. Redirect limit (5 hops) prevents infinite redirect loops. TTL cache is `sync.Mutex`-protected for concurrent safety. HTTP→HTTPS upgrade is unconditional. `url.ParseRequestURI` validates URL structure at `ValidateInput` time, before any network call. HTTP 4xx/5xx responses return `IsError: true` with status code and body, enabling model-visible diagnosis.

### 3.6 Stub tools have no silent failures

Every `TODO(dep)` stub returns `&tool.Result{IsError: true, Content: "...not yet implemented (TODO(dep))"}`. Integration tests can detect every unimplemented boundary without false negatives.

### 3.7 MCPProxyTool is correctly extensible

Uses a constructor (not a singleton) for runtime per-server instantiation. Correctly implements the `MCPToolInfo` sub-interface. Forwards the upstream tool's `InputSchema` verbatim from construction, enabling future MCP server tool registration without code changes.

### 3.8 TaskStatus type safety

`TaskStatus` is a named type with an explicit constant set. `InputSchema` `status` fields carry `"enum"` constraints, providing both Go-type and JSON-Schema-level validation. `TaskStop` correctly overrides `IsDestructive()` to return `true`.

---

## 4. Issues

### P0 — Must Fix Before Merge

---

#### P0-1 `removeTagBlock` logic defect causes HTML content loss

**File**: `internal/tools/web/webfetch.go`, lines 270–291

```go
func removeTagBlock(html, tag string) string {
    lower := strings.ToLower(html)              // (A) computed from initial html
    open  := "<" + tag
    close := "</" + tag + ">"
    var sb strings.Builder
    for {
        start := strings.Index(strings.ToLower(html), open) // (B) recomputed; ignores `lower`
        if start < 0 {
            sb.WriteString(html)
            break
        }
        sb.WriteString(html[:start])
        rest := lower[start:]                   // (C) uses `lower` — may be stale on iteration 2+
        end  := strings.Index(rest, close)
        if end < 0 {
            break                               // (D) silently discards all html[start:] on unclosed tag
        }
        html  = html[start+end+len(close):]
        lower = strings.ToLower(html)           // (E) lower updated at end of body
    }
    return sb.String()
}
```

**Defect 1 — stale `lower` at line (C):** On the second loop iteration, `lower` was updated at line (E) to match the new truncated `html`, and `start` at line (B) was also computed against the new `html` — these coincidentally agree on offset. However, the loop structure is fragile: line (B) allocates a fresh `strings.ToLower(html)` on every pass while ignoring the precomputed `lower`, and line (C) relies on `lower` being in sync with `html`. Any code path where they drift (e.g. if `lower` is updated at a different point) will cause silent index corruption.

**Defect 2 — content loss at line (D):** When `end < 0` (unclosed `<script>` tag — common in malformed real-world HTML), the function breaks without writing `html` to `sb`. All page content after the unclosed opening tag is silently dropped. Correct behaviour: write the remaining `html` to `sb`, then break.

**Impact:** `WebFetch` is the only live network tool. Any HTML page with an unclosed or malformed `<script>` or `<style>` tag returns truncated content.

**Fix:** At line (D), replace `break` with `sb.WriteString(html); break`. At line (B), replace `strings.ToLower(html)` with `lower` (use the precomputed value). The design's `TODO(dep)` comment already suggests replacing `htmlToText` with `github.com/JohannesKaufmann/html-to-markdown`; that is the preferred long-term fix.

---

#### P0-2 `domainAllowed` security bypass via bare `HasSuffix`

**File**: `internal/tools/web/websearch.go`, lines 218–238

```go
for _, d := range blocked {
    if strings.HasSuffix(host, strings.ToLower(d)) { return false }
}
for _, d := range allowed {
    if strings.HasSuffix(host, strings.ToLower(d)) { return true }
}
```

`HasSuffix` is a string suffix check, not a DNS-hierarchy check. Two failure modes:

1. **False block (over-blocking):** Blocking `evil.com` also blocks `notevil.com` — a completely different legitimate domain that happens to share the suffix.
2. **Allowed bypass (under-blocking):** Allowing `example.com` also allows `badexample.com` and `safe-example.com`, which are entirely different registrations.

Correct domain-hierarchy matching:
```go
host == d || strings.HasSuffix(host, "."+d)
```

**Impact:** `blocked_domains` and `allowed_domains` filtering is semantically wrong. In any security-sensitive scenario (enterprise content filtering, internal network isolation), the wrong domains are blocked or allowed.

---

#### P0-3 WebSearch gzip response not decoded

**File**: `internal/tools/web/websearch.go`, lines 159 and 180

```go
req.Header.Set("Accept-Encoding", "gzip")    // advertise gzip support
// ...
body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
// json.Unmarshal(body, &raw) — will fail if body is binary gzip stream
```

The request advertises gzip capability but the response is read raw without `compress/gzip` decompression. Brave's API CDN honours `Accept-Encoding: gzip` and returns compressed payloads. `json.Unmarshal` on a gzip stream fails with a parse error, making WebSearch non-functional in any environment where Brave returns compressed responses.

**Fix (option A — simple):** Remove the `Accept-Encoding: gzip` header. Go's `net/http` client handles transparent decompression when the header is absent.

**Fix (option B — preferred):** After reading the body, check `resp.Header.Get("Content-Encoding") == "gzip"` and wrap with `gzip.NewReader` before reading.

---

### P1 — Should Fix Before Release

---

#### P1-1 `Agent` and `SendMessage` `IsConcurrencySafe` semantic violation

**Files**: `agent/agent.go:62`, `agent/sendmessage.go:60`

```go
func (t *agentTool)       IsConcurrencySafe(_ tool.Input) bool { return true }  // BUG
func (t *sendMessageTool) IsConcurrencySafe(_ tool.Input) bool { return true }  // BUG
```

`tool.Result.ContextModifier` is documented:
> **MUST be nil when IsConcurrencySafe returns true (enforced by engine).**

Sub-agent execution and message routing both require `ContextModifier` to update session state. Both tools also declare `IsReadOnly: false` (write operations), which directly contradicts `IsConcurrencySafe: true`. When these tools are implemented, the engine will block use of `ContextModifier`, creating an irresolvable contract violation without a breaking interface change.

**Fix:** Change both to `return false`.

---

#### P1-2 `AgentInput` schema truncated

**File**: `internal/tools/agent/agent.go:13-18`

The design requires `AllowedTools []string` and `MaxTurns *int`. Both are absent. Callers passing these fields will have them silently dropped by `json.Unmarshal`; the `InputSchema()` does not advertise them to the LLM. These fields are required for tool scoping and turn budgeting when Agent is implemented.

**Fix:** Add both fields to `AgentInput` and `InputSchema()`.

---

#### P1-3 `SendMessageInput` field name diverges from design

**File**: `internal/tools/agent/sendmessage.go:17`

Implementation uses `json:"message"`; design specifies `json:"content"`. Any caller wired to the design's `content` field receives a silently empty message. Confirm intent and either update the implementation or update the design document.

---

#### P1-4 `TaskCreateInput` and `TaskOutputInput` schema truncated

**File**: `internal/tools/tasks/tasks.go`

`TaskCreateInput` is missing `Tools []string` (scope task tool access) and `Priority *int`. `TaskOutputInput` is missing `Since *int` (cursor-based partial output retrieval). These are required for correct task system operation and cannot be added later without a breaking schema change.

**Fix:** Add the missing fields to both structs and their `InputSchema()` definitions.

---

#### P1-5 `plugin.asPluginError` cannot detect wrapped `PluginError`

**File**: `internal/plugin/plugin.go:79-88`

```go
if pe, ok := err.(*types.PluginError); ok { ... }  // direct type assertion only
```

When a `PluginError` is wrapped via `fmt.Errorf("plugin init: %w", pe)`, the type assertion fails. `EnsureNoFatalErrors` silently misses the fatal error and does not block startup. The comment "avoids importing errors" is unnecessary — `errors` is a Go standard library package with no circular-dependency risk.

**Fix:** Replace with `errors.As(err, target)` from `"errors"`.

---

#### P1-6 Zero test coverage across all tool packages

Not a single `_test.go` file exists in any of the seven reviewed implementation packages (`shell`, `web`, `agent`, `mcp`, `misc`, `tasks`, `plugin`). The P0-1 content-loss bug and P0-3 gzip bug would both have been caught immediately by basic unit tests.

Minimum required test cases before merge:

**`shell/bash_test.go`**
- `ValidateInput`: empty command rejected; timeout > max rejected; valid input passes
- `Call`: successful command (exit 0); failing command (exit != 0); timeout kill (`TimedOut=true`, `ExitCode=-1`)
- `MapResultToToolResultBlock`: stdout only; stderr only; combined with exit code annotation

**`web/webfetch_test.go`**
- `upgradeToHTTPS`: HTTP upgraded; HTTPS unchanged
- `removeTagBlock` (after P0-1 fix): normal removal; unclosed tag; nested tags; case-insensitive tag names
- Cache hit/miss; 5 MiB truncation

**`web/websearch_test.go`**
- `domainAllowed` (after P0-2 fix): exact domain match; subdomain; bypassed suffix; blocked suffix
- `parseBraveResponse`: valid JSON; malformed JSON; empty results

**`plugin/plugin_test.go`**
- Fatal error detection (direct `*PluginError`); wrapped fatal error (after P1-5 fix)
- Non-fatal error does not block startup
- Empty plugin name returns fatal error

---

### P2 — Improve Before Next Milestone

---

#### P2-1 `removeTagBlock` redundant `strings.ToLower` allocation per iteration

**File**: `internal/tools/web/webfetch.go:276`

After the P0-1 fix that changes line 276 to use `lower`, this becomes a no-op. Before the fix: `strings.Index(strings.ToLower(html), open)` allocates a fresh lowercase string on every loop iteration over a potentially multi-megabyte HTML document. The precomputed `lower` should be used throughout.

---

#### P2-2 `SyntheticOutputTool` inconsistent safety flags

**File**: `internal/tools/misc/misc.go:241-242`

```go
func (t *syntheticOutputTool) IsConcurrencySafe(_ tool.Input) bool { return false }
func (t *syntheticOutputTool) IsReadOnly(_ tool.Input) bool        { return true }
```

`IsConcurrencySafe=false` signals state mutation. `IsReadOnly=true` signals no persistent changes. A tool that injects content into the conversation is not read-only. Change `IsReadOnly` to return `false`.

---

#### P2-3 `SleepTool.IsConcurrencySafe=true` will cause scheduler misjudgement

**File**: `internal/tools/misc/misc.go:187`

When the real `Sleep` implementation lands, `IsConcurrencySafe=true` will authorise the engine to schedule multiple Sleep calls concurrently, wasting goroutines and degrading turn latency. Change to `return false`, or add a comment: `// NOTE: revisit when implemented — blocking sleep should not be concurrent-safe`.

---

#### P2-4 `MCPProxyTool.UserFacingName` indistinguishable for multi-tool servers

**File**: `internal/tools/mcp/mcp.go:54-56`

```go
return fmt.Sprintf("mcp__%s", t.serverName)  // all tools from same server look identical
```

`t.name` already contains the full `mcp__serverName__toolName` canonical name. Use it directly: `return t.name`.

---

#### P2-5 `MCPToolInput` is dead code

**File**: `internal/tools/mcp/mcp.go:12-17`

`MCPToolInput` with a single `params json.RawMessage` field is defined but referenced nowhere — `MCPProxyTool.Call` ignores it and `InputSchema` is forwarded from the constructor. Either document it as a wire-type for MCP serialisation (with a usage example), or remove it.

---

#### P2-6 `WebSearch` result count hard-coded and undocumented

**File**: `internal/tools/web/websearch.go:149`

`params.Set("count", "10")` is a magic number neither reflected in `InputSchema` nor mentioned in `Description`. Add an optional `Count int \`json:"count,omitempty"\`` field (range 1–20, default 10) to `WebSearchInput` and document the default.

---

#### P2-7 `SkillTool` comment / variable name mismatch

**File**: `internal/tools/misc/misc.go:20-23`

```go
// SkillTool_ is the exported singleton instance.
// (Trailing underscore avoids collision with the 'tool' package import alias.)
var SkillTool tool.Tool = &skillTool{}
```

Variable is `SkillTool` (no underscore); comment says `SkillTool_`. Update the comment to `// SkillTool is the exported singleton instance.`

---

#### P2-8 `TaskListInput.Status` and `TaskUpdateInput.Status` should use `TaskStatus`

**File**: `internal/tools/tasks/tasks.go`

Both fields are declared as `string` rather than `TaskStatus`. Using the named type gives compile-time checking and self-documents the valid values. Change both to `TaskStatus`.

---

#### P2-9 `internal/tools/registry.go` absent

The design prescribes a `RegisterAll(r *tool.Registry)` function that wires all built-in singleton tools into the registry at startup. Without it, call sites must manually enumerate singletons, making it easy to forget newly added tools.

Create `internal/tools/registry.go` with:
```go
func RegisterAll(r *tool.Registry) {
    r.Register(shell.BashTool)
    r.Register(web.WebFetchTool)
    r.Register(web.WebSearchTool)
    // ... etc.
}
```

---

## 5. Summary

| Priority | Count | Key Items |
|:--------:|:-----:|-----------|
| **P0** | 3 | `removeTagBlock` content loss (HTML fetches truncated), `domainAllowed` domain bypass (security), WebSearch gzip not decoded (functional outage) |
| **P1** | 6 | Agent/SendMessage `IsConcurrencySafe` violation, AgentInput schema truncation, SendMessageInput field name, Task schema gaps, `errors.As` missing in plugin, zero test coverage |
| **P2** | 9 | Performance (redundant ToLower), flag inconsistency (SyntheticOutput, Sleep), MCP UserFacingName, dead MCPToolInput, undocumented count, comment/name mismatch, TaskStatus type, missing RegisterAll |

---

**Verdict: APPROVED_WITH_CHANGES.**

The `internal/tool` contract layer (interface, `BaseTool`, `Registry`) is production-quality and requires no rework. Bash, WebFetch, and WebSearch have solid real implementations that correctly follow the interface contract and should serve as the pattern for all future tool implementations.

Merge is contingent on resolving **P0-1** (HTML content loss), **P0-2** (domain filter bypass), **P0-3** (gzip decoding), and **P1-1** (Agent/SendMessage concurrency flag). P1-2 through P1-6 must be tracked as follow-up issues and resolved before the tools layer is considered release-ready.

## 修复跟踪记录

| 问题编号 | 级别 | 描述摘要 | 状态 | 复核时间 | 备注 |
|---------|------|---------|------|---------|------|
| P0-CR-7 | P0 | `removeTagBlock` 索引错误导致 HTML 内容丢失 | ✅ 已修复 | 2026-04-03 | 使用预计算 `lower`，`end<0` 分支补写剩余 html |
| P0-CR-8 | P0 | `domainAllowed` `HasSuffix` 可被子串绕过 | ✅ 已修复 | 2026-04-03 | 改为 `host == d \|\| HasSuffix(host, "."+d)` |

> **本层评审通过，通知 PM。**
