package config

import "os"

type Config struct {
	Port     string
	LogLevel string
	DataDir  string
}

func Load() Config {
	return Config{
		Port:     getEnv("MOVIES_PORT", "8080"),
		LogLevel: getEnv("MOVIES_LOG_LEVEL", "info"),
		DataDir:  getEnv("MOVIES_DATA_DIR", "/data"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
