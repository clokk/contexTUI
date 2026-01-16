package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const FileName = ".contexTUI.json"

// Config represents user preferences saved per-project
type Config struct {
	SplitRatio   float64 `json:"splitRatio,omitempty"`
	ShowDotfiles bool    `json:"showDotfiles,omitempty"`
}

// Load loads project-specific configuration
func Load(rootPath string) Config {
	configPath := filepath.Join(rootPath, FileName)

	data, err := os.ReadFile(configPath)
	if err != nil {
		return Config{} // Return empty config (use defaults)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{} // Malformed config, use defaults
	}

	return cfg
}

// Save saves project-specific configuration
func Save(rootPath string, cfg Config) {
	configPath := filepath.Join(rootPath, FileName)

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return // Silently fail
	}

	os.WriteFile(configPath, data, 0644)
}
