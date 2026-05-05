package config

import "testing"

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("AIRBYTE_API_HOST", "")

	cfg := Load()
	if cfg.APIHost != "https://api.airbyte.ai" {
		t.Errorf("APIHost = %q, want %q", cfg.APIHost, "https://api.airbyte.ai")
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("AIRBYTE_API_HOST", "https://custom.example.com")

	cfg := Load()
	if cfg.APIHost != "https://custom.example.com" {
		t.Errorf("APIHost = %q, want %q", cfg.APIHost, "https://custom.example.com")
	}
}
