package model

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
)

// ProviderSettings holds per-provider config stored in-app (not env).
type ProviderSettings struct {
	APIKey      string  `json:"api_key"`
	APIURL      string  `json:"api_url,omitempty"`
	Model       string  `json:"model"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
	TopP        float64 `json:"top_p"`
	Stream      bool    `json:"stream"`
	// NVIDIA-specific
	EnableThinking bool `json:"enable_thinking,omitempty"`
	// Groq-specific
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

// AppSettings is the top-level persisted settings file.
type AppSettings struct {
	Provider string                      `json:"provider"` // zai|nvidia|openrouter|groq
	Providers map[string]ProviderSettings `json:"providers"`
}

var (
	settingsMu   sync.RWMutex
	appSettings  *AppSettings
	settingsFile = "settings.json"
)

var providerDefaults = map[string]ProviderSettings{
	"zai": {
		Model:       "GLM-4.7",
		MaxTokens:   8192,
		Temperature: 0.7,
		TopP:        0.95,
		Stream:      true,
	},
	"nvidia": {
		APIURL:         "https://integrate.api.nvidia.com/v1/chat/completions",
		Model:          "qwen/qwq-32b",
		MaxTokens:      16384,
		Temperature:    0.6,
		TopP:           0.95,
		Stream:         true,
		EnableThinking: true,
	},
	"openrouter": {
		Model:       "qwen/qwen3-14b:free",
		MaxTokens:   8192,
		Temperature: 0.7,
		TopP:        0.95,
		Stream:      true,
	},
	"groq": {
		APIURL:          "https://api.groq.com/openai/v1/chat/completions",
		Model:           "openai/gpt-oss-120b",
		MaxTokens:       8192,
		Temperature:     1.0,
		TopP:            1.0,
		Stream:          true,
		ReasoningEffort: "medium",
	},
}

// KnownModels lists selectable models per provider.
var KnownModels = map[string][]string{
	"zai": {
		"GLM-4.7", "GLM-4.6", "GLM-4.5", "GLM-4.5-Air",
		"GLM-4.7-thinking", "GLM-4.7-search", "GLM-4.6-thinking",
	},
	"nvidia": {
		"qwen/qwq-32b", "nvidia/llama-3.1-nemotron-ultra-253b-v1",
		"meta/llama-4-maverick-17b-128e-instruct", "meta/llama-4-scout-17b-16e-instruct",
		"deepseek-ai/deepseek-r1", "qwen/qwen2.5-72b-instruct",
	},
	"openrouter": {
		"qwen/qwen3-14b:free", "qwen/qwen3-235b-a22b:free",
		"google/gemini-2.5-pro-exp-03-25:free", "deepseek/deepseek-r1:free",
		"meta-llama/llama-4-scout:free", "microsoft/phi-4-reasoning:free",
	},
	"groq": {
		"openai/gpt-oss-120b", "openai/gpt-oss-20b",
		"meta-llama/llama-4-scout-17b-16e-instruct",
		"meta-llama/llama-4-maverick-17b-128e-instruct",
		"llama-3.3-70b-versatile", "mistral-saba-24b",
		"qwen-qwq-32b", "deepseek-r1-distill-llama-70b",
	},
}

var KnownProviders = []string{"zai", "nvidia", "openrouter", "groq"}

func loadSettings() *AppSettings {
	s := &AppSettings{
		Provider:  "zai",
		Providers: make(map[string]ProviderSettings),
	}
	// Seed defaults
	for p, d := range providerDefaults {
		s.Providers[p] = d
	}
	// Overlay from env for API keys (keep them in env, not persisted in plaintext settings)
	s.Providers["nvidia"] = overlayEnvAPIKey(s.Providers["nvidia"], "NVIDIA_API_KEY")
	s.Providers["openrouter"] = overlayEnvAPIKey(s.Providers["openrouter"], "OPENROUTER_API_KEY")
	s.Providers["groq"] = overlayEnvAPIKey(s.Providers["groq"], "GROQ_API_KEY")

	// Load persisted provider from env (backwards compat)
	if p := strings.ToLower(strings.TrimSpace(os.Getenv("AI_PROVIDER"))); p != "" {
		s.Provider = p
	}

	data, err := os.ReadFile(settingsFile)
	if err != nil {
		return s
	}
	var saved AppSettings
	if err := json.Unmarshal(data, &saved); err != nil {
		return s
	}
	if saved.Provider != "" {
		s.Provider = saved.Provider
	}
	for p, ps := range saved.Providers {
		merged := s.Providers[p]
		// Overlay saved non-zero fields
		if ps.Model != "" {
			merged.Model = ps.Model
		}
		if ps.MaxTokens > 0 {
			merged.MaxTokens = ps.MaxTokens
		}
		if ps.Temperature != 0 {
			merged.Temperature = ps.Temperature
		}
		if ps.TopP != 0 {
			merged.TopP = ps.TopP
		}
		if ps.APIURL != "" {
			merged.APIURL = ps.APIURL
		}
		if ps.ReasoningEffort != "" {
			merged.ReasoningEffort = ps.ReasoningEffort
		}
		merged.Stream = ps.Stream
		merged.EnableThinking = ps.EnableThinking
		// Do NOT load API key from file — use env only
		s.Providers[p] = merged
	}
	return s
}

func overlayEnvAPIKey(ps ProviderSettings, envKey string) ProviderSettings {
	if k := strings.TrimSpace(os.Getenv(envKey)); k != "" {
		ps.APIKey = k
	}
	return ps
}

func getSettings() *AppSettings {
	settingsMu.RLock()
	s := appSettings
	settingsMu.RUnlock()
	if s != nil {
		return s
	}
	settingsMu.Lock()
	defer settingsMu.Unlock()
	if appSettings == nil {
		appSettings = loadSettings()
	}
	return appSettings
}

func saveSettings() {
	settingsMu.RLock()
	s := appSettings
	settingsMu.RUnlock()
	if s == nil {
		return
	}
	// Strip API keys before saving
	toSave := &AppSettings{
		Provider:  s.Provider,
		Providers: make(map[string]ProviderSettings),
	}
	for p, ps := range s.Providers {
		ps.APIKey = "" // never persist API keys
		toSave.Providers[p] = ps
	}
	data, _ := json.MarshalIndent(toSave, "", "  ")
	os.WriteFile(settingsFile, data, 0600)
}

// GetActiveProvider returns the currently selected provider.
func GetActiveProvider() string {
	return getSettings().Provider
}

// GetProviderSettings returns settings for the active (or given) provider.
func GetProviderSettings(provider string) ProviderSettings {
	s := getSettings()
	ps, ok := s.Providers[provider]
	if !ok {
		if d, ok := providerDefaults[provider]; ok {
			return d
		}
	}
	// Re-overlay API key from env at read time
	switch provider {
	case "nvidia":
		ps = overlayEnvAPIKey(ps, "NVIDIA_API_KEY")
	case "openrouter":
		ps = overlayEnvAPIKey(ps, "OPENROUTER_API_KEY")
	case "groq":
		ps = overlayEnvAPIKey(ps, "GROQ_API_KEY")
	}
	return ps
}

// SetProvider changes the active provider and persists.
func SetProvider(provider string) {
	settingsMu.Lock()
	s := getSettings()
	s.Provider = provider
	settingsMu.Unlock()
	saveSettings()
}

// SetProviderModel changes the model for the given provider and persists.
func SetProviderModel(provider, model string) {
	settingsMu.Lock()
	s := getSettings()
	ps := s.Providers[provider]
	ps.Model = model
	s.Providers[provider] = ps
	settingsMu.Unlock()
	saveSettings()
}

// UpdateProviderSettings applies a partial update and persists.
func UpdateProviderSettings(provider string, fn func(*ProviderSettings)) {
	settingsMu.Lock()
	s := getSettings()
	ps := s.Providers[provider]
	fn(&ps)
	s.Providers[provider] = ps
	settingsMu.Unlock()
	saveSettings()
}

// GetActiveModel returns the model for the active provider.
func GetActiveModel(fallback string) string {
	s := getSettings()
	ps := GetProviderSettings(s.Provider)
	if ps.Model != "" {
		return ps.Model
	}
	return fallback
}
