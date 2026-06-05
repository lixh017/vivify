package config

import (
	"os"
	"strings"
)

// Config holds all process-wide configuration read at startup. Fields
// are populated from environment variables (see .env.example) with
// reasonable dev defaults. Missing AI provider keys are allowed —
// the agents.Router treats them as "demo mode" so the server still
// boots and the UI still works without any external dependencies.
type Config struct {
	Port            string
	DBPath          string
	AnthropicAPIKey string
	DeepSeekAPIKey  string
	GeminiAPIKey    string

	// ModelOverrides is the parsed form of the AI_MODEL_OVERRIDES
	// env var: a comma-separated list of "task=model" pairs that
	// pin a single OPC task to a non-default model without
	// recompiling. See agents.ParseOverrides for the grammar.
	//
	// We store the raw string here (not the parsed map) because
	// the agents package owns the TaskName / RouteConfig types and
	// we don't want to pull them into config. The wiring code in
	// cmd/server/main.go calls ParseOverrides on this value when
	// constructing the router.
	ModelOverrides string
}

// Load reads configuration from the process environment. Unset
// variables fall through to dev-friendly defaults; missing AI keys
// are explicitly OK (demo mode).
func Load() *Config {
	return &Config{
		Port:            getEnv("PORT", "8080"),
		DBPath:          getEnv("DB_PATH", "/data/opc.db"),
		AnthropicAPIKey: getEnv("ANTHROPIC_API_KEY", ""),
		DeepSeekAPIKey:  getEnv("DEEPSEEK_API_KEY", ""),
		GeminiAPIKey:    getEnv("GEMINI_API_KEY", ""),
		ModelOverrides:  strings.TrimSpace(getEnv("AI_MODEL_OVERRIDES", "")),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
