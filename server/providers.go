package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
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
		ID: "augment", Name: "Augment Code", Enabled: true, Status: status,
		Data: info, CachedAt: time.Now().Unix(),
	}
	p.setCache("augment", result)
	return result
}

// ===== Z.AI =====

type ZaiQuotaInfo struct {
	PlanName     string  `json:"planName"`
	PlanStatus   string  `json:"planStatus"`
	TokensUsed   float64 `json:"tokensUsed"`
	TokensTotal  float64 `json:"tokensTotal"`
	TokensRemain float64 `json:"tokensRemaining"`
	NextReset    int64   `json:"nextReset"`
	McpUsed      float64 `json:"mcpUsed"`
	McpTotal     float64 `json:"mcpTotal"`
	McpRemain    float64 `json:"mcpRemaining"`
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
		ID: "zai", Name: "Z.AI", Enabled: true, Status: status,
		Data: info, CachedAt: time.Now().Unix(),
	}
	p.setCache("zai", result)
	return result
}

// ===== OpenAI =====

type OpenAIUsageInfo struct {
	TotalCost     float64 `json:"totalCost"`
	Budget        float64 `json:"budget,omitempty"`
	CreditBalance float64 `json:"creditBalance,omitempty"`
	Period        string  `json:"period"`
	DaysUntilReset int    `json:"daysUntilReset"`
	BucketCount   int     `json:"bucketCount"`
}

func (p *Plugin) getOpenAIStatus(config *Configuration) ServiceStatus {
	if config.OpenaiApiKey == "" {
		return ServiceStatus{ID: "openai", Name: "OpenAI", Enabled: true, Status: "error", Error: "API key not configured"}
	}

	if cached, ok := p.getCached("openai"); ok {
		return cached.(ServiceStatus)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	// Start of current month
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	startTime := monthStart.Unix()
	url := fmt.Sprintf("https://api.openai.com/v1/organization/costs?start_time=%d&end_time=%d&bucket_width=1d&limit=31", startTime, now.Unix())

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
		var errResp map[string]interface{}
		if json.Unmarshal(body, &errResp) == nil {
			if errObj, ok := errResp["error"].(map[string]interface{}); ok {
				return ServiceStatus{ID: "openai", Name: "OpenAI", Enabled: true, Status: "error",
					Error: getString(errObj, "message")}
			}
		}
		return ServiceStatus{ID: "openai", Name: "OpenAI", Enabled: true, Status: "error",
			Error: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))}
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return ServiceStatus{ID: "openai", Name: "OpenAI", Enabled: true, Status: "error", Error: "Invalid JSON response"}
	}

	info := OpenAIUsageInfo{Period: monthStart.Format("Jan 2006")}

	if data, ok := raw["data"].([]interface{}); ok {
		info.BucketCount = len(data)
		for _, d := range data {
			if dm, ok := d.(map[string]interface{}); ok {
				if results, ok := dm["results"].([]interface{}); ok {
					for _, r := range results {
						if rm, ok := r.(map[string]interface{}); ok {
							if amountObj, ok := rm["amount"].(map[string]interface{}); ok {
								valStr := getString(amountObj, "value")
								if valStr != "" {
									val, _ := strconv.ParseFloat(valStr, 64)
									info.TotalCost += val
								}
							}
						}
					}
				}
			}
		}
	}

	// Add budget and credit balance from config
	if config.OpenaiMonthlyBudget != "" {
		info.Budget, _ = strconv.ParseFloat(config.OpenaiMonthlyBudget, 64)
	}
	if config.OpenaiCreditBalance != "" {
		info.CreditBalance, _ = strconv.ParseFloat(config.OpenaiCreditBalance, 64)
	}

	// Days until month reset
	nextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	info.DaysUntilReset = int(nextMonth.Sub(now).Hours() / 24)

	status := "ok"
	if info.Budget > 0 && info.TotalCost/info.Budget > 0.8 {
		status = "warning"
	}
	if info.Budget > 0 && info.TotalCost >= info.Budget {
		status = "error"
	}

	result := ServiceStatus{
		ID: "openai", Name: "OpenAI", Enabled: true, Status: status,
		Data: info, CachedAt: time.Now().Unix(),
	}
	p.setCache("openai", result)
	return result
}

// ===== Claude (claude.ai usage via OAuth) =====

type ClaudeUsageInfo struct {
	Utilization5h float64 `json:"utilization5h"`
	Reset5h       string  `json:"reset5h,omitempty"`
	Utilization7d float64 `json:"utilization7d"`
	Reset7d       string  `json:"reset7d,omitempty"`
	SonnetUtil    float64 `json:"sonnetUtil,omitempty"`
	OpusUtil      float64 `json:"opusUtil,omitempty"`
	HasData       bool    `json:"hasData"`
}

func (p *Plugin) getClaudeStatus(config *Configuration) ServiceStatus {
	if !config.ClaudeEnabled {
		return ServiceStatus{ID: "claude", Name: "claude.ai", Enabled: false, Status: "disabled"}
	}

	if config.ClaudeAccessToken == "" {
		return ServiceStatus{
			ID: "claude", Name: "claude.ai", Enabled: true, Status: "error",
			Error: "Access token not configured. Run 'claude' CLI on server, authorize, then copy tokens from ~/.claude/.credentials.json",
		}
	}

	if cached, ok := p.getCached("claude"); ok {
		return cached.(ServiceStatus)
	}

	client := &http.Client{Timeout: 15 * time.Second}

	req, _ := http.NewRequest("GET", "https://api.anthropic.com/api/oauth/usage", nil)
	req.Header.Set("Authorization", "Bearer "+config.ClaudeAccessToken)
	req.Header.Set("User-Agent", "MattermostPlugin/1.0")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")

	resp, err := client.Do(req)
	if err != nil {
		return ServiceStatus{ID: "claude", Name: "claude.ai", Enabled: true, Status: "error",
			Error: fmt.Sprintf("API error: %v", err)}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// If auth error, try to refresh token
	if (resp.StatusCode == 401 || resp.StatusCode == 403) && config.ClaudeRefreshToken != "" {
		newToken, refreshErr := p.refreshClaudeToken(config)
		if refreshErr == nil && newToken != "" {
			// Retry with new token
			req2, _ := http.NewRequest("GET", "https://api.anthropic.com/api/oauth/usage", nil)
			req2.Header.Set("Authorization", "Bearer "+newToken)
			req2.Header.Set("User-Agent", "MattermostPlugin/1.0")
			req2.Header.Set("Accept", "application/json")
			req2.Header.Set("anthropic-version", "2023-06-01")
			req2.Header.Set("anthropic-beta", "oauth-2025-04-20")
			resp2, err2 := client.Do(req2)
			if err2 == nil {
				defer resp2.Body.Close()
				body, _ = io.ReadAll(resp2.Body)
				resp = resp2
			}
		}
	}

	if resp.StatusCode != 200 {
		return ServiceStatus{ID: "claude", Name: "claude.ai", Enabled: true, Status: "error",
			Error: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))}
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return ServiceStatus{ID: "claude", Name: "claude.ai", Enabled: true, Status: "error",
			Error: "Invalid JSON from usage API"}
	}

	info := ClaudeUsageInfo{HasData: false}

	if fiveHour, ok := raw["five_hour"].(map[string]interface{}); ok {
		if util, exists := fiveHour["utilization"]; exists {
			info.Utilization5h = toFloat(util)
			info.HasData = true
		}
		if resetAt, ok := fiveHour["resets_at"].(string); ok {
			info.Reset5h = resetAt
		}
	}

	if sevenDay, ok := raw["seven_day"].(map[string]interface{}); ok {
		if util, exists := sevenDay["utilization"]; exists {
			info.Utilization7d = toFloat(util)
			info.HasData = true
		}
		if resetAt, ok := sevenDay["resets_at"].(string); ok {
			info.Reset7d = resetAt
		}
	}

	if sonnet, ok := raw["seven_day_sonnet"].(map[string]interface{}); ok {
		if util, exists := sonnet["utilization"]; exists {
			info.SonnetUtil = toFloat(util)
		}
	}
	if opus, ok := raw["seven_day_opus"].(map[string]interface{}); ok {
		if util, exists := opus["utilization"]; exists {
			info.OpusUtil = toFloat(util)
		}
	}

	status := "ok"
	if info.Utilization5h > 80 || info.Utilization7d > 80 {
		status = "warning"
	}
	if info.Utilization5h >= 100 || info.Utilization7d >= 100 {
		status = "error"
	}

	result := ServiceStatus{
		ID: "claude", Name: "claude.ai", Enabled: true, Status: status,
		Data: info, CachedAt: time.Now().Unix(),
	}
	p.setCache("claude", result)
	return result
}

// refreshClaudeToken uses refresh_token to get new access_token and saves it to config.
func (p *Plugin) refreshClaudeToken(config *Configuration) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	formData := "grant_type=refresh_token&client_id=9d1c250a-e61b-44d9-88ed-5944d1962f5e&refresh_token=" + config.ClaudeRefreshToken

	req, _ := http.NewRequest("POST", "https://platform.claude.com/v1/oauth/token", strings.NewReader(formData))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "MattermostPlugin/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("refresh HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}

	var tokenResp map[string]interface{}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", err
	}

	newToken := getString(tokenResp, "access_token")
	if newToken == "" {
		return "", fmt.Errorf("empty access_token")
	}

	// Update config with new token
	config.ClaudeAccessToken = newToken
	if rt := getString(tokenResp, "refresh_token"); rt != "" {
		config.ClaudeRefreshToken = rt
	}

	// Save updated tokens to plugin config
	cfgMap := map[string]interface{}{}
	cfgBytes, _ := json.Marshal(config)
	json.Unmarshal(cfgBytes, &cfgMap)
	p.API.SavePluginConfig(cfgMap)

	return newToken, nil
}

func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case json.Number:
		f, _ := n.Float64()
		return f
	case int:
		return float64(n)
	}
	return 0
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
