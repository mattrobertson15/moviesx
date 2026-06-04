package config

import "os"

type Config struct {
	Port     string
	LogLevel string
	DataDir  string
}

// Flags holds parsed CLI flag values. Empty string means "not set by flag".
type Flags struct {
	Port     string
	LogLevel string
	DataDir  string
}

// Load builds a Config using precedence: defaults < env vars < CLI flags.
func Load(flags Flags) Config {
	cfg := Config{
		Port:     getEnv("MOVIES_PORT", "8080"),
		LogLevel: getEnv("MOVIES_LOG_LEVEL", "info"),
		DataDir:  getEnv("MOVIES_DATA_DIR", "/data"),
	}
	if flags.Port != "" {
		cfg.Port = flags.Port
	}
	if flags.LogLevel != "" {
		cfg.LogLevel = flags.LogLevel
	}
	if flags.DataDir != "" {
		cfg.DataDir = flags.DataDir
	}
	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
