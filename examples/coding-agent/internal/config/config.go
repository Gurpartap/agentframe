package config

import (
	"fmt"
	"os"
	"time"
)

const (
	defaultHTTPAddr        = "127.0.0.1:8080"
	defaultShutdownTimeout = 5 * time.Second
)

// Config controls HTTP boot and shutdown behavior.
type Config struct {
	HTTPAddr        string
	ShutdownTimeout time.Duration
}

// Load reads runtime configuration from environment variables.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:        defaultHTTPAddr,
		ShutdownTimeout: defaultShutdownTimeout,
	}

	if addr := os.Getenv("CODING_AGENT_HTTP_ADDR"); addr != "" {
		cfg.HTTPAddr = addr
	}

	if timeout := os.Getenv("CODING_AGENT_SHUTDOWN_TIMEOUT"); timeout != "" {
		parsed, err := time.ParseDuration(timeout)
		if err != nil {
			return Config{}, fmt.Errorf("parse CODING_AGENT_SHUTDOWN_TIMEOUT: %w", err)
		}
		if parsed <= 0 {
			return Config{}, fmt.Errorf("parse CODING_AGENT_SHUTDOWN_TIMEOUT: value must be > 0")
		}
		cfg.ShutdownTimeout = parsed
	}

	return cfg, nil
}
