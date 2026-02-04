package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/mattermost/mattermost/server/public/plugin"
)

// Plugin implements the Mattermost plugin interface.
type Plugin struct {
	plugin.MattermostPlugin

	configurationLock sync.RWMutex
	configuration     *Configuration

	// Cache
	cacheLock    sync.RWMutex
	cache        map[string]*CacheEntry
}

// Configuration holds the plugin settings from System Console.
type Configuration struct {
	AugmentEnabled     bool   `json:"augmentenabled"`
	AugmentAccessToken string `json:"augmentaccesstoken"`
	ZaiEnabled         bool   `json:"zaienabled"`
	ZaiApiKey          string `json:"zaiapikey"`
	OpenaiEnabled      bool   `json:"openaienabled"`
	OpenaiApiKey       string `json:"openaiapikey"`
	ClaudeEnabled      bool   `json:"claudeenabled"`
	ClaudeAdminApiKey  string `json:"claudeadminapikey"`
	CacheTTLSeconds    string `json:"cachettlseconds"`
}

// CacheEntry stores cached API response.
type CacheEntry struct {
	Data      interface{}
	FetchedAt time.Time
}

// ServiceStatus represents the status of one AI service.
type ServiceStatus struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Enabled     bool        `json:"enabled"`
	Status      string      `json:"status"` // "ok", "warning", "error", "disabled"
	Data        interface{} `json:"data,omitempty"`
	Error       string      `json:"error,omitempty"`
	CachedAt    int64       `json:"cachedAt,omitempty"`
}

// AllServicesResponse is the response for GET /api/v1/status.
type AllServicesResponse struct {
	Services []ServiceStatus `json:"services"`
}

func (p *Plugin) OnActivate() error {
	p.cache = make(map[string]*CacheEntry)
	return nil
}

func (p *Plugin) getConfiguration() *Configuration {
	p.configurationLock.RLock()
	defer p.configurationLock.RUnlock()

	if p.configuration == nil {
		return &Configuration{}
	}
	return p.configuration
}

func (p *Plugin) OnConfigurationChange() error {
	var configuration Configuration
	if err := p.API.LoadPluginConfiguration(&configuration); err != nil {
		return err
	}
	p.configurationLock.Lock()
	p.configuration = &configuration
	p.configurationLock.Unlock()

	// Clear cache on config change
	p.cacheLock.Lock()
	p.cache = make(map[string]*CacheEntry)
	p.cacheLock.Unlock()

	return nil
}

func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	// Check user is logged in
	userID := r.Header.Get("Mattermost-User-Id")
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Only system admins can view AI service usage
	user, appErr := p.API.GetUser(userID)
	if appErr != nil {
		http.Error(w, "Failed to get user", http.StatusInternalServerError)
		return
	}
	if !user.IsSystemAdmin() {
		// Allow all users to see status, but could restrict later
		_ = user
	}

	switch {
	case r.URL.Path == "/api/v1/status" && r.Method == http.MethodGet:
		p.handleGetStatus(w, r)
	case r.URL.Path == "/api/v1/refresh" && r.Method == http.MethodPost:
		p.handleRefresh(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (p *Plugin) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	config := p.getConfiguration()
	services := []ServiceStatus{}

	if config.AugmentEnabled {
		services = append(services, p.getAugmentStatus(config))
	}
	if config.ZaiEnabled {
		services = append(services, p.getZaiStatus(config))
	}
	if config.OpenaiEnabled {
		services = append(services, p.getOpenAIStatus(config))
	}
	if config.ClaudeEnabled {
		services = append(services, p.getClaudeStatus(config))
	}

	// If nothing enabled, show placeholder
	if len(services) == 0 {
		services = append(services, ServiceStatus{
			ID:     "none",
			Name:   "No Services Configured",
			Status: "disabled",
			Error:  "Enable AI services in System Console → Plugins → AI Limits Monitor",
		})
	}

	resp := AllServicesResponse{Services: services}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (p *Plugin) handleRefresh(w http.ResponseWriter, r *http.Request) {
	// Clear cache
	p.cacheLock.Lock()
	p.cache = make(map[string]*CacheEntry)
	p.cacheLock.Unlock()

	// Return fresh data
	p.handleGetStatus(w, r)
}

func (p *Plugin) getCacheTTL() time.Duration {
	config := p.getConfiguration()
	ttl := 300 // default 5 min
	if config.CacheTTLSeconds != "" {
		parsed := 0
		for _, c := range config.CacheTTLSeconds {
			if c >= '0' && c <= '9' {
				parsed = parsed*10 + int(c-'0')
			}
		}
		if parsed > 0 {
			ttl = parsed
		}
	}
	return time.Duration(ttl) * time.Second
}

func (p *Plugin) getCached(key string) (interface{}, bool) {
	p.cacheLock.RLock()
	defer p.cacheLock.RUnlock()

	entry, ok := p.cache[key]
	if !ok {
		return nil, false
	}
	if time.Since(entry.FetchedAt) > p.getCacheTTL() {
		return nil, false
	}
	return entry.Data, true
}

func (p *Plugin) setCache(key string, data interface{}) {
	p.cacheLock.Lock()
	defer p.cacheLock.Unlock()

	p.cache[key] = &CacheEntry{
		Data:      data,
		FetchedAt: time.Now(),
	}
}

func main() {
	plugin.ClientMain(&Plugin{})
}
