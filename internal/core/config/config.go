package config

import (
	"os"
)

type Config struct {
	HTTPAddr string
	DBPath   string
	BaseURL  string
}

func Load() *Config {
	return &Config{
		HTTPAddr: getEnv("BIGLYBIGLY_HTTP_ADDR", ":8082"),
		DBPath:   getEnv("BIGLYBIGLY_DB_PATH", "./biglybigly.db"),
		BaseURL:  getEnv("BIGLYBIGLY_BASE_URL", "http://localhost:8082"),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
