// Package config manages application configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config holds all application configuration.
type Config struct {
	CasesPath  string `mapstructure:"cases_path"`
	ToolsPath  string `mapstructure:"tools_path"`
	LogPath    string `mapstructure:"log_path"`
	HistoryPath string `mapstructure:"history_path"`
	LogLevel   string `mapstructure:"log_level"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	mimirDir := filepath.Join(home, "Mimir")

	return &Config{
		CasesPath:   filepath.Join(mimirDir, "Investigations"),
		ToolsPath:   filepath.Join(mimirDir, "tools"),
		LogPath:     filepath.Join(mimirDir, "logs"),
		HistoryPath: filepath.Join(mimirDir, ".mhistory"),
		LogLevel:    "INFO",
	}
}

// Load loads configuration from defaults, config file, and env vars.
func Load() (*Config) {
	cfg := DefaultConfig()

	// Override from config file if it exists
	paths := []string{
		"mimir.yaml",
		"config/mimir.yaml",
		filepath.Join(os.Getenv("HOME"), "Mimir", "config", "mimir.yaml"),
		filepath.Join(os.Getenv("HOME"), ".mimir.yaml"),
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			// TODO: parse YAML when we add viper
			_ = p
			break
		}
	}

	// Override from env
	if v := os.Getenv("MIMIR_CASES_PATH"); v != "" {
		cfg.CasesPath = v
	}
	if v := os.Getenv("MIMIR_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	// Ensure directories exist
	for _, dir := range []string{cfg.CasesPath, cfg.ToolsPath, cfg.LogPath} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not create %s: %v\n", dir, err)
		}
	}

	return cfg
}
