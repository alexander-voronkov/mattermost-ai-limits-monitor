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

// ===== Claude (Anthropic) via webhook push =====

// ClaudeRateLimitInfo holds unified rate limit data pushed by external script.
type ClaudeRateLimitInfo struct {
	AuthMethod       string            `json:"authMethod"`
	Utilization5h    string            `json:"utilization5h"`
	Reset5h          string            `json:"reset5h"`
	Status5h         string            `json:"status5h"`
	Utilization7d    string            `json:"utilization7d"`
	Reset7d          string            `json:"reset7d"`
	Status7d         string            `json:"status7d"`
	OverageStatus    string            `json:"overageStatus"`
	RepresentClaim   string            `json:"representativeClaim"`
	AllHeaders       map[string]string `json:"allHeaders,omitempty"`
	CheckTimestamp   int64             `json:"checkTimestamp,omitempty"`
}

// claudeWebhookPayload is the JSON payload pushed by the external check script.
type claudeWebhookPayload struct {
	Timestamp  int64             `json:"timestamp"`
	RateLimits map[string]string `json:"rateLimits"`
	Error      string            `json:"error,omitempty"`
}

func (p *Plugin) getClaudeStatus(config *Configuration) ServiceStatus {
	if !config.ClaudeEnabled {
		return ServiceStatus{ID: "claude", Name: "Claude (Anthropic)", Enabled: false, Status: "disabled"}
	}

	// Check if we have webhook data
	if cached, ok := p.getCached("claude"); ok {
		return cached.(ServiceStatus)
	}

	// No data yet — return waiting status
	return ServiceStatus{
		ID: "claude", Name: "Claude (Anthropic)", Enabled: true, Status: "warning",
		Error: "Waiting for data. Push rate limits via POST /api/v1/claude-push (use claude-check.sh cron job).",
	}
}

// handleClaudePush accepts rate limit data from external script.
func (p *Plugin) handleClaudePush(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var payload claudeWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if payload.Error != "" {
		result := ServiceStatus{
			ID: "claude", Name: "Claude (Anthropic)", Enabled: true, Status: "error",
			Error:    payload.Error,
			CachedAt: time.Now().Unix(),
		}
		p.setCache("claude", result)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "error_stored"})
		return
	}

	rl := payload.RateLimits
	info := ClaudeRateLimitInfo{
		AuthMethod:     "oauth (claude.ai)",
		Utilization5h:  rl["anthropic-ratelimit-unified-5h-utilization"],
		Reset5h:        rl["anthropic-ratelimit-unified-5h-reset"],
		Status5h:       rl["anthropic-ratelimit-unified-5h-status"],
		Utilization7d:  rl["anthropic-ratelimit-unified-7d-utilization"],
		Reset7d:        rl["anthropic-ratelimit-unified-7d-reset"],
		Status7d:       rl["anthropic-ratelimit-unified-7d-status"],
		OverageStatus:  rl["anthropic-ratelimit-unified-overage-status"],
		RepresentClaim: rl["anthropic-ratelimit-unified-representative-claim"],
		AllHeaders:     rl,
		CheckTimestamp: payload.Timestamp,
	}

	status := "ok"
	if u5h := parseFloat(info.Utilization5h); u5h > 0.8 {
		status = "warning"
	}
	if info.Status5h == "rejected" || info.Status7d == "rejected" {
		status = "error"
	}

	result := ServiceStatus{
		ID:       "claude",
		Name:     "Claude (Anthropic)",
		Enabled:  true,
		Status:   status,
		Data:     info,
		CachedAt: time.Now().Unix(),
	}
	// Cache for 1 hour (data is pushed periodically by cron)
	p.cacheLock.Lock()
	p.cache["claude"] = &CacheEntry{Data: result, FetchedAt: time.Now().Add(55 * time.Minute)}
	p.cacheLock.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func parseFloat(s string) float64 {
	if s == "" {
		return 0
	}
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
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
