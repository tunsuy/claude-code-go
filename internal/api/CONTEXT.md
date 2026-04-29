---
package: api
import_path: internal/api
layer: services
generated_at: 2026-04-29T02:31:52Z
source_files: [accumulate.go, client.go, debug_logger.go, errors.go, factory.go, json.go, openai_client.go, openai_stream.go, openai_types.go, retry.go, stream.go, usage.go]
---

# internal/api

> Layer: **Services** · Files: 12 · Interfaces: 2 · Structs: 25 · Functions: 7

## Interfaces

### Client (2 methods)
> Client is the unified Anthropic API entry point.

```go
type Client interface {
    Stream(ctx context.Context, req *MessageRequest) (StreamReader, error)
    Complete(ctx context.Context, req *MessageRequest) (*MessageResponse, error)
}
```

### StreamReader (1 methods)
> StreamReader wraps SSE event stream reading, following io.Closer semantics.

```go
type StreamReader interface {
    Next() (*StreamEvent, error)
}
```

## Structs

- **APIError** — 4 fields: StatusCode, Message, Headers, Kind
- **APIErrorData** — 2 fields: Type, Message
- **Accumulator** — 3 fields
- **CacheCreationUsage** — 2 fields: Ephemeral1hInputTokens, Ephemeral5mInputTokens
- **CannotRetryError** — 2 fields: Cause, RetryContext
- **ClientConfig** — 13 fields: Provider, APIKey, BaseURL, MaxRetries, TimeoutSeconds, CustomHeaders, AWSRegion, GCPProject, ...
- **ContentBlock** — 10 fields: Type, Text, ID, Name, Input, ToolUseID, Content, IsError, ...
- **ContentBlockDeltaData** — 2 fields: Index, Delta
- **DebugLogger** — 4 fields
- **Delta** — 4 fields: Type, Text, PartialJSON, Thinking
- **ExtraUsage** — 4 fields: IsEnabled, MonthlyLimit, UsedCredits, Utilization
- **FallbackTriggeredError** — 2 fields: OriginalModel, FallbackModel
- **MessageDeltaData** — 2 fields: Delta, Usage
- **MessageParam** — 2 fields: Role, Content
- **MessageRequest** — 8 fields: Model, MaxTokens, Messages, System, Tools, Stream, ThinkingBudget, QuerySource
- **MessageResponse** — 7 fields: ID, Type, Role, Content, Model, StopReason, Usage
- **MessageStartData** — 1 fields: Message
- **RateLimit** — 2 fields: Utilization, ResetsAt
- **RetryContext** — 3 fields: Attempt, Model, QuerySource
- **RetryOptions** — 6 fields: MaxRetries, Model, FallbackModel, QuerySource, InitialConsecutive529, Signal
- **ServerToolUse** — 2 fields: WebSearchRequests, WebFetchRequests
- **StreamEvent** — 6 fields: Type, Data, MessageStart, ContentBlockDelta, MessageDelta, Error
- **ToolSchema** — 3 fields: Name, Description, InputSchema
- **Usage** — 9 fields: InputTokens, CacheCreationInputTokens, CacheReadInputTokens, OutputTokens, ServerToolUse, ServiceTier, CacheCreation, InferenceGeo, ...
- **Utilization** — 6 fields: FiveHour, SevenDay, SevenDayOAuthApps, SevenDayOpus, SevenDaySonnet, ExtraUsage

## Function Types

- `RetryableFunc` — `func(ctx context.Context, attempt int) (T, error)`

## Functions

- `Is529Error(err error) bool`
- `IsOAuthTokenRevokedError(err error) bool`
- `NewClient(cfg ClientConfig, httpClient *http.Client) (Client, error)`
- `NewDebugLogger(debug bool, debugFile string) (*DebugLogger, error)`
- `ParseContextOverflowError(err error) (inputTokens int, contextLimit int, ok bool)`
- `RetryDelay(attempt int, retryAfterHeader string, maxDelayMS int) time.Duration`
- `WithRetry(ctx context.Context, fn any, opts RetryOptions) (T, error)`

## Constants

- `BaseDelayMS`
- `DefaultMaxRetries`
- `ErrKindConnectionError`
- `ErrKindConnectionTimeout`
- `ErrKindContextWindow`
- `ErrKindForbidden`
- `ErrKindInvalidRequest`
- `ErrKindOverloaded`
- `ErrKindRateLimit`
- `ErrKindServerError`
- `ErrKindUnauthorized`
- `ErrKindUnknown`
- `EventContentBlockDelta`
- `EventContentBlockStart`
- `EventContentBlockStop`
- `EventError`
- `EventMessageDelta`
- `EventMessageStart`
- `EventMessageStop`
- `EventPing`
- `HeartbeatInterval`
- `Max529Retries`
- `MaxDelayMS`
- `PersistentMaxBackoff`
- `PersistentResetCap`
- `ProviderBedrock`
- `ProviderDirect`
- `ProviderFoundry`
- `ProviderOpenAI`
- `ProviderVertex`

## Change Impact

**Test Mocks (must add new methods when interfaces change):**
- `mockClient` in `internal/engine/engine_test.go`
- `mockClient` in `test/integration/engine_e2e_test.go`

**Exported type references (files that use types from this package):**
- `APIError` → `internal/engine/engine_test.go` (test), `internal/engine/query.go`
- `Accumulator` → `internal/engine/query.go`
- `Client` → `internal/bootstrap/wire.go`, `internal/compact/auto.go`, `internal/engine/engine.go`, `internal/engine/engine_test.go` (test)
- `ClientConfig` → `internal/bootstrap/wire.go`
- `ContentBlock` → `internal/engine/engine_test.go` (test), `internal/engine/query.go`
- `ContentBlockDeltaData` → `internal/engine/engine_test.go` (test)
- `Delta` → `internal/engine/engine_test.go` (test)
- `MessageDeltaData` → `internal/engine/engine_test.go` (test)
- `MessageParam` → `internal/compact/auto.go`, `internal/engine/query.go`
- `MessageRequest` → `internal/compact/auto.go`, `internal/engine/engine_test.go` (test), `internal/engine/forked_agent_test.go` (test), `internal/engine/query.go`, `internal/engine/stop_hooks_test.go` (test)
- `MessageResponse` → `internal/compact/auto.go`, `internal/engine/engine_test.go` (test), `internal/engine/query.go`
- `MessageStartData` → `internal/engine/engine_test.go` (test)
- `StreamEvent` → `internal/engine/engine_test.go` (test)
- `StreamReader` → `internal/engine/engine_test.go` (test), `internal/engine/forked_agent_test.go` (test), `internal/engine/stop_hooks_test.go` (test)
- `ToolSchema` → `internal/engine/query.go`
- `Usage` → `internal/engine/engine_test.go` (test), `internal/engine/query.go`

## Dependencies

**Imports:** *(none — zero-dependency)*

**Imported by:** `internal/bootstrap`, `internal/compact`, `internal/engine`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
