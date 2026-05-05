package config

import "os"

const defaultAPIHost = "https://api.airbyte.ai"

type Config struct {
	APIHost string
}

func Load() *Config {
	host := os.Getenv("AIRBYTE_API_HOST")
	if host == "" {
		host = defaultAPIHost
	}

	return &Config{
		APIHost: host,
	}
}
