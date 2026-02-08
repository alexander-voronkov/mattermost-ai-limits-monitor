package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	cacheLock sync.RWMutex
	cache     map[string]*CacheEntry

}

// Configuration holds the plugin settings from System Console.
type Configuration struct {
	AllowedUserIds     string `json:"alloweduserids"`
	AllowedTeamIds     string `json:"allowedteamids"`
	AugmentEnabled     bool   `json:"augmentenabled"`
	AugmentAccessToken string `json:"augmentaccesstoken"`
	ZaiEnabled         bool   `json:"zaienabled"`
	ZaiApiKey          string `json:"zaiapikey"`
	OpenaiEnabled        bool   `json:"openaienabled"`
	OpenaiApiKey         string `json:"openaiapikey"`
	OpenaiMonthlyBudget  string `json:"openaimonthlybudget"`
	OpenaiCreditBalance  string `json:"openaicreditbalance"`
	ClaudeEnabled      bool   `json:"claudeenabled"`
	ClaudeAccessToken  string `json:"claudeaccesstoken"`
	ClaudeRefreshToken string `json:"clauderefreshtoken"`
}

// CacheEntry stores cached API response.
type CacheEntry struct {
	Data      interface{}
	FetchedAt time.Time
}

// ServiceStatus represents the status of one AI service.
type ServiceStatus struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Enabled  bool        `json:"enabled"`
	Status   string      `json:"status"` // "ok", "warning", "error", "disabled"
	Data     interface{} `json:"data,omitempty"`
	Error    string      `json:"error,omitempty"`
	CachedAt int64       `json:"cachedAt,omitempty"`
}

// AllServicesResponse is the response for GET /api/v1/status.
type AllServicesResponse struct {
	Services []ServiceStatus `json:"services"`
}

// (no KV store needed — session key is in plugin config)

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

// checkAccess returns true if user is allowed to access this plugin.
func (p *Plugin) checkAccess(userID string) bool {
	config := p.getConfiguration()
	
	// If no restrictions set, allow everyone
	if config.AllowedUserIds == "" && config.AllowedTeamIds == "" {
		return true
	}
	
	// Check allowed users
	if config.AllowedUserIds != "" {
		allowedUsers := strings.Split(config.AllowedUserIds, ",")
		for _, id := range allowedUsers {
			if strings.TrimSpace(id) == userID {
				return true
			}
		}
	}
	
	// Check allowed teams
	if config.AllowedTeamIds != "" {
		allowedTeams := strings.Split(config.AllowedTeamIds, ",")
		for _, teamID := range allowedTeams {
			teamID = strings.TrimSpace(teamID)
			if teamID == "" {
				continue
			}
			// Check if user is member of this team
			_, err := p.API.GetTeamMember(teamID, userID)
			if err == nil {
				return true
			}
		}
	}
	
	return false
}

func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	// Serve static assets from webapp/dist/
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		p.serveStaticFile(w, r)
		return
	}

	// Check user is logged in for API routes
	userID := r.Header.Get("Mattermost-User-Id")
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	
	// Check access permissions
	if !p.checkAccess(userID) {
		http.Error(w, `{"error": "access_denied", "message": "You don't have permission to access this plugin"}`, http.StatusForbidden)
		return
	}

	switch {
	case r.URL.Path == "/api/v1/access" && r.Method == http.MethodGet:
		// Always returns OK if we got here (access already checked above)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"allowed": true}`))
	case r.URL.Path == "/api/v1/status" && r.Method == http.MethodGet:
		p.handleGetStatus(w, r)
	case r.URL.Path == "/api/v1/refresh" && r.Method == http.MethodPost:
		p.handleRefresh(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (p *Plugin) serveStaticFile(w http.ResponseWriter, r *http.Request) {
	bundlePath, err := p.API.GetBundlePath()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	cleanPath := filepath.Clean(r.URL.Path)
	if strings.Contains(cleanPath, "..") {
		http.NotFound(w, r)
		return
	}
	filePath := filepath.Join(bundlePath, "webapp", "dist", cleanPath)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// Also check assets/
		filePath = filepath.Join(bundlePath, "assets", cleanPath)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
	}
	http.ServeFile(w, r, filePath)
}

func (p *Plugin) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	config := p.getConfiguration()
	services := []ServiceStatus{}

	if config.AugmentEnabled {
		services = append(services, p.getAugmentStatus(config))
	} else {
		services = append(services, ServiceStatus{ID: "augment", Name: "Augment Code", Enabled: false, Status: "disabled", Error: "Not configured. Enable in System Console → Plugins → AI Limits Monitor."})
	}

	if config.ZaiEnabled {
		services = append(services, p.getZaiStatus(config))
	} else {
		services = append(services, ServiceStatus{ID: "zai", Name: "Z.AI", Enabled: false, Status: "disabled", Error: "Not configured. Enable in System Console → Plugins → AI Limits Monitor."})
	}

	if config.OpenaiEnabled {
		services = append(services, p.getOpenAIStatus(config))
	} else {
		services = append(services, ServiceStatus{ID: "openai", Name: "OpenAI", Enabled: false, Status: "disabled", Error: "Not configured. Enable in System Console → Plugins → AI Limits Monitor."})
	}

	if config.ClaudeEnabled {
		services = append(services, p.getClaudeStatus(config))
	} else {
		services = append(services, ServiceStatus{ID: "claude", Name: "Claude (Anthropic)", Enabled: false, Status: "disabled", Error: "Not configured. Enable in System Console → Plugins → AI Limits Monitor."})
	}

	resp := AllServicesResponse{Services: services}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (p *Plugin) handleRefresh(w http.ResponseWriter, r *http.Request) {
	p.cacheLock.Lock()
	p.cache = make(map[string]*CacheEntry)
	p.cacheLock.Unlock()

	p.handleGetStatus(w, r)
}

func (p *Plugin) getCacheTTL() time.Duration {
	return 5 * time.Minute
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
