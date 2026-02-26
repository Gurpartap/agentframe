package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL   = "http://127.0.0.1:8080"
	defaultAuthToken = "coding-agent-dev-token"
	defaultTimeout   = 15 * time.Second
)

const (
	envBaseURL   = "CODING_AGENT_BASE_URL"
	envAuthToken = "CODING_AGENT_AUTH_TOKEN"
	envJSON      = "CODING_AGENT_CLIENT_JSON"
	envTimeout   = "CODING_AGENT_CLIENT_TIMEOUT"
)

type Config struct {
	BaseURL   string
	AuthToken string
	JSON      bool
	Timeout   time.Duration
}

func Default() Config {
	return Config{
		BaseURL:   defaultBaseURL,
		AuthToken: defaultAuthToken,
		JSON:      false,
		Timeout:   defaultTimeout,
	}
}

func Load() (Config, error) {
	cfg := Default()

	if baseURL := strings.TrimSpace(os.Getenv(envBaseURL)); baseURL != "" {
		cfg.BaseURL = baseURL
	}
	if token := strings.TrimSpace(os.Getenv(envAuthToken)); token != "" {
		cfg.AuthToken = token
	}
	if rawJSON := strings.TrimSpace(os.Getenv(envJSON)); rawJSON != "" {
		parsed, err := strconv.ParseBool(rawJSON)
		if err != nil {
			return Config{}, fmt.Errorf("parse %s: %w", envJSON, err)
		}
		cfg.JSON = parsed
	}
	if rawTimeout := strings.TrimSpace(os.Getenv(envTimeout)); rawTimeout != "" {
		parsed, err := time.ParseDuration(rawTimeout)
		if err != nil {
			return Config{}, fmt.Errorf("parse %s: %w", envTimeout, err)
		}
		cfg.Timeout = parsed
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Validate() error {
	trimmedBaseURL := strings.TrimSpace(c.BaseURL)
	if trimmedBaseURL == "" {
		return fmt.Errorf("validate client config: %s is required", envBaseURL)
	}
	parsed, err := url.Parse(trimmedBaseURL)
	if err != nil {
		return fmt.Errorf("validate client config: parse %s: %w", envBaseURL, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("validate client config: %s must include scheme and host", envBaseURL)
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("validate client config: %s must be > 0", envTimeout)
	}
	return nil
}
