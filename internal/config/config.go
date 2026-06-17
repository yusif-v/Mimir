// Package config manages application configuration.
// Follows FHS-like layout under ~/.mimir/:
//
//	~/.mimir/config.yaml       # Static config (like /etc/mimir/)
//	~/.mimir/investigations/   # Cases (like /var/lib/mimir/)
//	~/.mimir/tools/            # Tool templates (like /opt/mimir/)
//	~/.mimir/plugins/          # Plugins (like /opt/mimir/plugins/)
//	~/.mimir/cache/            # Cache (like /var/lib/mimir/cache/)
//	~/.mimir/logs/             # Logs (like /var/log/mimir/)
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds all application configuration.
type Config struct {
	HomeDir     string `yaml:"home_dir" mapstructure:"home_dir"`
	CasesPath   string `yaml:"cases_path" mapstructure:"cases_path"`
	ToolsPath   string `yaml:"tools_path" mapstructure:"tools_path"`
	PluginsPath string `yaml:"plugins_path" mapstructure:"plugins_path"`
	CachePath   string `yaml:"cache_path" mapstructure:"cache_path"`
	LogPath     string `yaml:"log_path" mapstructure:"log_path"`
	LogFile     string `yaml:"log_file" mapstructure:"log_file"`
	HistoryPath string `yaml:"history_path" mapstructure:"history_path"`
	LogLevel    string `yaml:"log_level" mapstructure:"log_level"`
}

// DefaultConfig returns sensible defaults under ~/.mimir/.
func DefaultConfig() *Config {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	mimirDir := filepath.Join(home, ".mimir")

	return &Config{
		HomeDir:     mimirDir,
		CasesPath:   filepath.Join(mimirDir, "investigations"),
		ToolsPath:   filepath.Join(mimirDir, "tools"),
		PluginsPath: filepath.Join(mimirDir, "plugins"),
		CachePath:   filepath.Join(mimirDir, "cache"),
		LogPath:     filepath.Join(mimirDir, "logs"),
		LogFile:     filepath.Join(mimirDir, "logs", "mimir.log"),
		HistoryPath: filepath.Join(mimirDir, ".history"),
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
		filepath.Join(cfg.HomeDir, "config.yaml"),
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
	if v := os.Getenv("MIMIR_HOME"); v != "" {
		cfg.HomeDir = v
		// Re-derive all paths from new home
		cfg.CasesPath = filepath.Join(v, "investigations")
		cfg.ToolsPath = filepath.Join(v, "tools")
		cfg.PluginsPath = filepath.Join(v, "plugins")
		cfg.CachePath = filepath.Join(v, "cache")
		cfg.LogPath = filepath.Join(v, "logs")
		cfg.LogFile = filepath.Join(v, "logs", "mimir.log")
		cfg.HistoryPath = filepath.Join(v, ".history")
	}
	if v := os.Getenv("MIMIR_CASES_PATH"); v != "" {
		cfg.CasesPath = v
	}
	if v := os.Getenv("MIMIR_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	// Ensure all directories exist
	for _, dir := range []string{
		cfg.CasesPath, cfg.ToolsPath, cfg.PluginsPath,
		cfg.CachePath, cfg.LogPath,
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not create %s: %v\n", dir, err)
		}
	}

	return cfg
}

// ConfigPath returns the expected path for the config file.
func (c *Config) ConfigPath() string {
	return filepath.Join(c.HomeDir, "config.yaml")
}

// WriteDefault writes a default config file to the given path.
func WriteDefault(path string) error {
	data, err := yaml.Marshal(DefaultConfig())
	if err != nil {
		return err
	}
	header := `# Mimir Configuration
# Home: ` + DefaultConfig().HomeDir + `
# Docs: https://github.com/yusif-v/mimir

`
	return os.WriteFile(path, append([]byte(header), data...), 0644)
}

// merge applies non-zero values from other into cfg.
func (c *Config) merge(other *Config) {
	if other.HomeDir != "" {
		c.HomeDir = other.HomeDir
	}
	if other.CasesPath != "" {
		c.CasesPath = other.CasesPath
	}
	if other.ToolsPath != "" {
		c.ToolsPath = other.ToolsPath
	}
	if other.PluginsPath != "" {
		c.PluginsPath = other.PluginsPath
	}
	if other.CachePath != "" {
		c.CachePath = other.CachePath
	}
	if other.LogPath != "" {
		c.LogPath = other.LogPath
	}
	if other.LogFile != "" {
		c.LogFile = other.LogFile
	}
	if other.HistoryPath != "" {
		c.HistoryPath = other.HistoryPath
	}
	if other.LogLevel != "" {
		c.LogLevel = other.LogLevel
	}
}
