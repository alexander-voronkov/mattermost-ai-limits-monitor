package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ===== Augment Code =====

type AugmentCreditInfo struct {
	PlanName       string  `json:"planName"`
	UsageRemaining float64 `json:"usageRemaining"`
	UsageTotal     float64 `json:"usageTotal"`
	UsageUsed      float64 `json:"usageUsed"`
	CycleEnd       string  `json:"cycleEnd"`
	IsLow          bool    `json:"isLow"`
}

func (p *Plugin) getAugmentStatus(config *Configuration) ServiceStatus {
	if config.AugmentAccessToken == "" {
		return ServiceStatus{ID: "augment", Name: "Augment Code", Enabled: true, Status: "error", Error: "Access token not configured"}
	}

	if cached, ok := p.getCached("augment"); ok {
		return cached.(ServiceStatus)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("POST", "https://d2.api.augmentcode.com/get-credit-info", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+config.AugmentAccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "MattermostPlugin/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return ServiceStatus{ID: "augment", Name: "Augment Code", Enabled: true, Status: "error", Error: fmt.Sprintf("API error: %v", err)}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return ServiceStatus{ID: "augment", Name: "Augment Code", Enabled: true, Status: "error", Error: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))}
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return ServiceStatus{ID: "augment", Name: "Augment Code", Enabled: true, Status: "error", Error: fmt.Sprintf("Parse error: %v (body: %s)", err, string(body[:min(len(body), 200)]))}
	}

	info := AugmentCreditInfo{
		UsageRemaining: getFloat(raw, "usage_units_remaining"),
		UsageTotal:     getFloat(raw, "usage_units_total"),
		CycleEnd:       getString(raw, "current_billing_cycle_end_date_iso"),
	}
	info.UsageUsed = info.UsageTotal - info.UsageRemaining

	if display, ok := raw["display_info"].(map[string]interface{}); ok {
		info.PlanName = getString(display, "plan_display_name")
	}
	if isLow, ok := raw["is_credit_balance_low"].(bool); ok {
		info.IsLow = isLow
	}

	// Determine included per cycle
	included := getFloat(raw, "included_usage_units_per_billing_cycle")
	if included > 0 {
		info.UsageTotal = included
		info.UsageUsed = included - info.UsageRemaining
	}

	status := "ok"
	if info.IsLow || (included > 0 && info.UsageRemaining/included < 0.1) {
		status = "warning"
	}

	result := ServiceStatus{
		ID:       "augment",
		Name:     "Augment Code",
		Enabled:  true,
		Status:   status,
		Data:     info,
		CachedAt: time.Now().Unix(),
	}
	p.setCache("augment", result)
	return result
}

// ===== Z.AI =====

type ZaiQuotaInfo struct {
	PlanName      string         `json:"planName"`
	PlanStatus    string         `json:"planStatus"`
	TokensUsed    float64        `json:"tokensUsed"`
	TokensTotal   float64        `json:"tokensTotal"`
	TokensRemain  float64        `json:"tokensRemaining"`
	NextReset     int64          `json:"nextReset"`
	McpUsed       float64        `json:"mcpUsed"`
	McpTotal      float64        `json:"mcpTotal"`
	McpRemain     float64        `json:"mcpRemaining"`
}

func (p *Plugin) getZaiStatus(config *Configuration) ServiceStatus {
	if config.ZaiApiKey == "" {
		return ServiceStatus{ID: "zai", Name: "Z.AI", Enabled: true, Status: "error", Error: "API key not configured"}
	}

	if cached, ok := p.getCached("zai"); ok {
		return cached.(ServiceStatus)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	info := ZaiQuotaInfo{}

	// Fetch subscription
	req, _ := http.NewRequest("GET", "https://api.z.ai/api/biz/subscription/list", nil)
	req.Header.Set("Authorization", "Bearer "+config.ZaiApiKey)
	if resp, err := client.Do(req); err == nil {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var raw map[string]interface{}
		if json.Unmarshal(body, &raw) == nil {
			if data, ok := raw["data"].([]interface{}); ok && len(data) > 0 {
				if sub, ok := data[0].(map[string]interface{}); ok {
					info.PlanName = getString(sub, "productName")
					info.PlanStatus = getString(sub, "status")
				}
			}
		}
	}

	// Fetch quota
	req2, _ := http.NewRequest("GET", "https://api.z.ai/api/monitor/usage/quota/limit", nil)
	req2.Header.Set("Authorization", "Bearer "+config.ZaiApiKey)
	if resp2, err := client.Do(req2); err == nil {
		defer resp2.Body.Close()
		body, _ := io.ReadAll(resp2.Body)
		var raw map[string]interface{}
		if json.Unmarshal(body, &raw) == nil {
			if data, ok := raw["data"].(map[string]interface{}); ok {
				if limits, ok := data["limits"].([]interface{}); ok {
					for _, l := range limits {
						lm, ok := l.(map[string]interface{})
						if !ok {
							continue
						}
						switch getString(lm, "type") {
						case "TOKENS_LIMIT":
							info.TokensUsed = getFloat(lm, "currentValue")
							info.TokensTotal = getFloat(lm, "usage")
							info.TokensRemain = getFloat(lm, "remaining")
							info.NextReset = int64(getFloat(lm, "nextResetTime"))
						case "TIME_LIMIT":
							info.McpUsed = getFloat(lm, "currentValue")
							info.McpTotal = getFloat(lm, "usage")
							info.McpRemain = getFloat(lm, "remaining")
						}
					}
				}
			}
		}
	}

	status := "ok"
	if info.TokensTotal > 0 && info.TokensRemain/info.TokensTotal < 0.1 {
		status = "warning"
	}

	result := ServiceStatus{
		ID:       "zai",
		Name:     "Z.AI",
		Enabled:  true,
		Status:   status,
		Data:     info,
		CachedAt: time.Now().Unix(),
	}
	p.setCache("zai", result)
	return result
}

// ===== OpenAI =====

type OpenAIUsageInfo struct {
	TotalCost   float64 `json:"totalCost"`
	Period      string  `json:"period"`
	BucketCount int     `json:"bucketCount"`
}

func (p *Plugin) getOpenAIStatus(config *Configuration) ServiceStatus {
	if config.OpenaiApiKey == "" {
		return ServiceStatus{ID: "openai", Name: "OpenAI", Enabled: true, Status: "error", Error: "API key not configured"}
	}

	if cached, ok := p.getCached("openai"); ok {
		return cached.(ServiceStatus)
	}

	client := &http.Client{Timeout: 15 * time.Second}

	// start_time must be Unix seconds (integer), not a date string
	startTime := time.Now().AddDate(0, -1, 0).Unix()
	url := fmt.Sprintf("https://api.openai.com/v1/organization/costs?start_time=%d&bucket_width=1d&limit=31", startTime)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+config.OpenaiApiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return ServiceStatus{ID: "openai", Name: "OpenAI", Enabled: true, Status: "error", Error: fmt.Sprintf("API error: %v", err)}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		// Try to extract error message
		var errResp map[string]interface{}
		if json.Unmarshal(body, &errResp) == nil {
			if errObj, ok := errResp["error"].(map[string]interface{}); ok {
				return ServiceStatus{ID: "openai", Name: "OpenAI", Enabled: true, Status: "error",
					Error: fmt.Sprintf("%s", getString(errObj, "message"))}
			}
		}
		return ServiceStatus{ID: "openai", Name: "OpenAI", Enabled: true, Status: "error",
			Error: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))}
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return ServiceStatus{ID: "openai", Name: "OpenAI", Enabled: true, Status: "error", Error: "Invalid JSON response"}
	}

	startDate := time.Unix(startTime, 0).Format("2006-01-02")
	info := OpenAIUsageInfo{
		Period: startDate + " to now",
	}

	// Parse cost buckets: data[].results[].amount (in cents)
	if data, ok := raw["data"].([]interface{}); ok {
		info.BucketCount = len(data)
		for _, d := range data {
			if dm, ok := d.(map[string]interface{}); ok {
				if results, ok := dm["results"].([]interface{}); ok {
					for _, r := range results {
						if rm, ok := r.(map[string]interface{}); ok {
							info.TotalCost += getFloat(rm, "amount") / 100.0 // cents to dollars
						}
					}
				}
			}
		}
	}

	result := ServiceStatus{
		ID:       "openai",
		Name:     "OpenAI",
		Enabled:  true,
		Status:   "ok",
		Data:     info,
		CachedAt: time.Now().Unix(),
	}
	p.setCache("openai", result)
	return result
}

// ===== Claude (Anthropic) =====

type ClaudeRateLimitInfo struct {
	Model             string `json:"model"`
	RequestLimit      string `json:"requestLimit"`
	RequestsRemaining string `json:"requestsRemaining"`
	RequestsReset     string `json:"requestsReset"`
	TokenLimit        string `json:"tokenLimit"`
	TokensRemaining   string `json:"tokensRemaining"`
	TokensReset       string `json:"tokensReset"`
	InputTokensUsed   int64  `json:"inputTokensUsed"`
	OutputTokensUsed  int64  `json:"outputTokensUsed"`
}

func (p *Plugin) getClaudeStatus(config *Configuration) ServiceStatus {
	if config.ClaudeApiKey == "" {
		return ServiceStatus{
			ID: "claude", Name: "Claude (Anthropic)", Enabled: true, Status: "error",
			Error: "Anthropic API key not configured",
		}
	}

	if cached, ok := p.getCached("claude"); ok {
		return cached.(ServiceStatus)
	}

	client := &http.Client{Timeout: 15 * time.Second}

	// Send a minimal request to get rate limit headers
	// Cost: ~1 input token + 1 output token ≈ $0.00001
	model := config.ClaudeModel
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	payload := fmt.Sprintf(`{"model":"%s","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`, model)

	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", strings.NewReader(payload))
	req.Header.Set("x-api-key", config.ClaudeApiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return ServiceStatus{ID: "claude", Name: "Claude (Anthropic)", Enabled: true, Status: "error",
			Error: fmt.Sprintf("API error: %v", err)}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		var errResp map[string]interface{}
		if json.Unmarshal(body, &errResp) == nil {
			if errObj, ok := errResp["error"].(map[string]interface{}); ok {
				return ServiceStatus{ID: "claude", Name: "Claude (Anthropic)", Enabled: true, Status: "error",
					Error: getString(errObj, "message")}
			}
		}
		return ServiceStatus{ID: "claude", Name: "Claude (Anthropic)", Enabled: true, Status: "error",
			Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}

	info := ClaudeRateLimitInfo{
		Model: model,
		// Rate limit headers from response
		RequestLimit:      resp.Header.Get("anthropic-ratelimit-requests-limit"),
		RequestsRemaining: resp.Header.Get("anthropic-ratelimit-requests-remaining"),
		RequestsReset:     resp.Header.Get("anthropic-ratelimit-requests-reset"),
		TokenLimit:        resp.Header.Get("anthropic-ratelimit-tokens-limit"),
		TokensRemaining:   resp.Header.Get("anthropic-ratelimit-tokens-remaining"),
		TokensReset:       resp.Header.Get("anthropic-ratelimit-tokens-reset"),
	}

	// Parse usage from response body
	var respData map[string]interface{}
	if json.Unmarshal(body, &respData) == nil {
		if usage, ok := respData["usage"].(map[string]interface{}); ok {
			info.InputTokensUsed = int64(getFloat(usage, "input_tokens"))
			info.OutputTokensUsed = int64(getFloat(usage, "output_tokens"))
		}
	}

	status := "ok"
	// Could add warning logic based on remaining tokens ratio

	result := ServiceStatus{
		ID:       "claude",
		Name:     "Claude (Anthropic)",
		Enabled:  true,
		Status:   status,
		Data:     info,
		CachedAt: time.Now().Unix(),
	}
	p.setCache("claude", result)
	return result
}

// ===== Helpers =====

func getFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case json.Number:
			f, _ := n.Float64()
			return f
		}
	}
	return 0
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
