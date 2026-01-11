package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const configFileName = ".contexTUI.json"

// loadConfig loads project-specific configuration
func loadConfig(rootPath string) Config {
	configPath := filepath.Join(rootPath, configFileName)

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

// saveConfig saves project-specific configuration
func saveConfig(rootPath string, cfg Config) {
	configPath := filepath.Join(rootPath, configFileName)

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return // Silently fail
	}

	os.WriteFile(configPath, data, 0644)
}
