package utils

import (
	"github.com/spf13/viper"
)

type Config struct {
	DaemonPort             int    `mapstructure:"daemon_port"`
	DashboardPort          int    `mapstructure:"dashboard_port"`
	CacheDir               string `mapstructure:"cache_dir"`
	CacheMaxSize           int64  `mapstructure:"cache_max_size"`
	NodeID                 string `mapstructure:"node_id"`
	TrackerURL             string `mapstructure:"tracker_url"`
	LogLevel               string `mapstructure:"log_level"`
	ContentBlindingEnabled bool   `mapstructure:"content_blinding_enabled"`
        IncognitoMode bool `mapstructure:"incognito_mode"`
}

func LoadConfig(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigName("flow")
	v.SetConfigType("yaml")
	v.AddConfigPath(path)
	v.AddConfigPath(".")

	v.SetDefault("daemon_port", 7676)
	v.SetDefault("dashboard_port", 7677)
	v.SetDefault("cache_dir", "./flow-cache")
	v.SetDefault("cache_max_size", int64(10*1024*1024*1024))
	v.SetDefault("log_level", "info")
	v.SetDefault("content_blinding_enabled", true)
        v.SetDefault("incognito_mode", false)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, WrapError("CONFIG_LOAD", "failed to read config file", err)
		}
		LogWarn("no config file found, using defaults")
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, WrapError("CONFIG_PARSE", "failed to parse config", err)
	}
	return &cfg, nil
}

func (c *Config) Save(path string) error {
	v := viper.New()
	v.SetConfigFile(path)
	v.Set("daemon_port", c.DaemonPort)
	v.Set("dashboard_port", c.DashboardPort)
	v.Set("cache_dir", c.CacheDir)
	v.Set("cache_max_size", c.CacheMaxSize)
	v.Set("node_id", c.NodeID)
	v.Set("tracker_url", c.TrackerURL)
	v.Set("log_level", c.LogLevel)
	v.Set("content_blinding_enabled", c.ContentBlindingEnabled)
	return v.WriteConfig()
}

// Reloadable fields yang aman diubah tanpa restart: cache size limit dan
// content blinding toggle. Port/cache_dir/tracker_url butuh restart
// listener/koneksi, jadi sengaja gak termasuk di sini.
func (c *Config) ApplyHotReload(newCfg *Config, onCacheSizeChange func(int64), onBlindingChange func(bool)) {
	if newCfg.CacheMaxSize != c.CacheMaxSize {
		LogInfo("hot reload: cache_max_size changed %d -> %d", c.CacheMaxSize, newCfg.CacheMaxSize)
		c.CacheMaxSize = newCfg.CacheMaxSize
		if onCacheSizeChange != nil {
			onCacheSizeChange(newCfg.CacheMaxSize)
		}
	}
	if newCfg.ContentBlindingEnabled != c.ContentBlindingEnabled {
		LogInfo("hot reload: content_blinding_enabled changed %v -> %v", c.ContentBlindingEnabled, newCfg.ContentBlindingEnabled)
		c.ContentBlindingEnabled = newCfg.ContentBlindingEnabled
		if onBlindingChange != nil {
			onBlindingChange(newCfg.ContentBlindingEnabled)
		}
	}
}
