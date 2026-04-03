package api

// Usage corresponds to the Anthropic Beta Messages API usage fields.
type Usage struct {
	InputTokens              int               `json:"input_tokens"`
	CacheCreationInputTokens int               `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int               `json:"cache_read_input_tokens"`
	OutputTokens             int               `json:"output_tokens"`
	ServerToolUse            ServerToolUse     `json:"server_tool_use"`
	ServiceTier              string            `json:"service_tier"`  // "standard" | "priority" | "batch"
	CacheCreation            CacheCreationUsage `json:"cache_creation"`
	InferenceGeo             string            `json:"inference_geo"`
	Speed                    string            `json:"speed"` // "standard" | "turbo"
}

// ServerToolUse holds server-side tool usage counts.
type ServerToolUse struct {
	WebSearchRequests int `json:"web_search_requests"`
	WebFetchRequests  int `json:"web_fetch_requests"`
}

// CacheCreationUsage holds cache creation token breakdowns.
type CacheCreationUsage struct {
	Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
	Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens"`
}

// EmptyUsage is the zero-value Usage constant (mirrors TS EMPTY_USAGE).
var EmptyUsage = Usage{ServiceTier: "standard", Speed: "standard"}

// Utilization corresponds to the /api/oauth/usage response.
type Utilization struct {
	FiveHour          *RateLimit  `json:"five_hour,omitempty"`
	SevenDay          *RateLimit  `json:"seven_day,omitempty"`
	SevenDayOAuthApps *RateLimit  `json:"seven_day_oauth_apps,omitempty"`
	SevenDayOpus      *RateLimit  `json:"seven_day_opus,omitempty"`
	SevenDaySonnet    *RateLimit  `json:"seven_day_sonnet,omitempty"`
	ExtraUsage        *ExtraUsage `json:"extra_usage,omitempty"`
}

// RateLimit holds rate-limit utilization data.
type RateLimit struct {
	Utilization *float64 `json:"utilization"` // 0-100
	ResetsAt    *string  `json:"resets_at"`   // ISO 8601
}

// ExtraUsage holds pay-per-use extra usage data.
type ExtraUsage struct {
	IsEnabled    bool     `json:"is_enabled"`
	MonthlyLimit *int64   `json:"monthly_limit"`
	UsedCredits  *int64   `json:"used_credits"`
	Utilization  *float64 `json:"utilization"`
}
