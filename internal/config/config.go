// Package config manages application configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds all application configuration.
type Config struct {
	CasesPath   string `yaml:"cases_path" mapstructure:"cases_path"`
	ToolsPath   string `yaml:"tools_path" mapstructure:"tools_path"`
	LogPath     string `yaml:"log_path" mapstructure:"log_path"`
	HistoryPath string `yaml:"history_path" mapstructure:"history_path"`
	LogLevel    string `yaml:"log_level" mapstructure:"log_level"`
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
		HistoryPath: filepath.Join(home, ".mimir_history"),
		LogLevel:    "INFO",
	}
}

// Load loads configuration from defaults, config file, and env vars.
func Load() *Config {
	cfg := DefaultConfig()

	// Override from config file if it exists
	paths := []string{
		"mimir.yaml",
		"config/mimir.yaml",
		filepath.Join(os.Getenv("HOME"), ".mimir", "config", "mimir.yaml"),
		filepath.Join(os.Getenv("HOME"), ".mimir.yaml"),
	}
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil {
			var fileCfg Config
			if err := yaml.Unmarshal(data, &fileCfg); err == nil {
				cfg.merge(&fileCfg)
			}
			break
		}
	}

	// Override from env vars
	if v := os.Getenv("MIMIR_CASES_PATH"); v != "" {
		cfg.CasesPath = v
	}
	if v := os.Getenv("MIMIR_TOOLS_PATH"); v != "" {
		cfg.ToolsPath = v
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

// merge applies non-zero values from other into cfg.
func (c *Config) merge(other *Config) {
	if other.CasesPath != "" {
		c.CasesPath = other.CasesPath
	}
	if other.ToolsPath != "" {
		c.ToolsPath = other.ToolsPath
	}
	if other.LogPath != "" {
		c.LogPath = other.LogPath
	}
	if other.HistoryPath != "" {
		c.HistoryPath = other.HistoryPath
	}
	if other.LogLevel != "" {
		c.LogLevel = other.LogLevel
	}
}

// WriteDefault writes a default config file to the given path.
func WriteDefault(path string) error {
	data, err := yaml.Marshal(DefaultConfig())
	if err != nil {
		return err
	}
	header := "# Mimir Configuration\n# See https://github.com/yusif-v/mimir for documentation\n\n"
	return os.WriteFile(path, append([]byte(header), data...), 0644)
}
