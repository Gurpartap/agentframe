package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

const (
	defaultHTTPAddr        = "127.0.0.1:8080"
	defaultShutdownTimeout = 5 * time.Second
	defaultLogFormat       = LogFormatText
	defaultModelMode       = ModelModeMock
	defaultProviderBaseURL = "https://api.openai.com/v1"
	defaultProviderModel   = "gpt-4.1-mini"
	defaultProviderTimeout = 30 * time.Second
	defaultToolMode        = ToolModeReal
	defaultBashTimeout     = 3 * time.Second
	defaultLogLevel        = slog.LevelInfo
)

type ModelMode string

const (
	ModelModeMock     ModelMode = "mock"
	ModelModeProvider ModelMode = "provider"
)

type ToolMode string

const (
	ToolModeMock ToolMode = "mock"
	ToolModeReal ToolMode = "real"
)

type LogFormat string

const (
	LogFormatText LogFormat = "text"
	LogFormatJSON LogFormat = "json"
)

// Config controls HTTP boot and shutdown behavior.
type Config struct {
	HTTPAddr        string
	ShutdownTimeout time.Duration
	LogFormat       LogFormat
	LogLevel        slog.Level
	ModelMode       ModelMode
	ProviderAPIKey  string
	ProviderModel   string
	ProviderBaseURL string
	ProviderTimeout time.Duration
	ToolMode        ToolMode
	WorkspaceRoot   string
	BashTimeout     time.Duration
}

// Load reads runtime configuration from environment variables.
func Load() (Config, error) {
	cfg := Default()

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
	if level := strings.TrimSpace(os.Getenv("CODING_AGENT_LOG_LEVEL")); level != "" {
		parsed, err := parseLogLevel(level)
		if err != nil {
			return Config{}, err
		}
		cfg.LogLevel = parsed
	}
	if format := strings.TrimSpace(os.Getenv("CODING_AGENT_LOG_FORMAT")); format != "" {
		parsed, err := parseLogFormat(format)
		if err != nil {
			return Config{}, err
		}
		cfg.LogFormat = parsed
	}

	if mode := strings.TrimSpace(os.Getenv("CODING_AGENT_MODEL_MODE")); mode != "" {
		cfg.ModelMode = ModelMode(mode)
	}
	if key := strings.TrimSpace(os.Getenv("CODING_AGENT_PROVIDER_API_KEY")); key != "" {
		cfg.ProviderAPIKey = key
	}
	if model := strings.TrimSpace(os.Getenv("CODING_AGENT_PROVIDER_MODEL")); model != "" {
		cfg.ProviderModel = model
	}
	if baseURL := strings.TrimSpace(os.Getenv("CODING_AGENT_PROVIDER_BASE_URL")); baseURL != "" {
		cfg.ProviderBaseURL = baseURL
	}
	if timeout := strings.TrimSpace(os.Getenv("CODING_AGENT_PROVIDER_TIMEOUT")); timeout != "" {
		parsed, err := time.ParseDuration(timeout)
		if err != nil {
			return Config{}, fmt.Errorf("parse CODING_AGENT_PROVIDER_TIMEOUT: %w", err)
		}
		if parsed <= 0 {
			return Config{}, fmt.Errorf("parse CODING_AGENT_PROVIDER_TIMEOUT: value must be > 0")
		}
		cfg.ProviderTimeout = parsed
	}
	if mode := strings.TrimSpace(os.Getenv("CODING_AGENT_TOOL_MODE")); mode != "" {
		cfg.ToolMode = ToolMode(mode)
	}
	if root := strings.TrimSpace(os.Getenv("CODING_AGENT_WORKSPACE_ROOT")); root != "" {
		cfg.WorkspaceRoot = root
	}
	if timeout := strings.TrimSpace(os.Getenv("CODING_AGENT_BASH_TIMEOUT")); timeout != "" {
		parsed, err := time.ParseDuration(timeout)
		if err != nil {
			return Config{}, fmt.Errorf("parse CODING_AGENT_BASH_TIMEOUT: %w", err)
		}
		if parsed <= 0 {
			return Config{}, fmt.Errorf("parse CODING_AGENT_BASH_TIMEOUT: value must be > 0")
		}
		cfg.BashTimeout = parsed
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func Default() Config {
	workspaceRoot, err := os.Getwd()
	if err != nil || strings.TrimSpace(workspaceRoot) == "" {
		workspaceRoot = "."
	}

	return Config{
		HTTPAddr:        defaultHTTPAddr,
		ShutdownTimeout: defaultShutdownTimeout,
		LogFormat:       defaultLogFormat,
		LogLevel:        defaultLogLevel,
		ModelMode:       defaultModelMode,
		ProviderModel:   defaultProviderModel,
		ProviderBaseURL: defaultProviderBaseURL,
		ProviderTimeout: defaultProviderTimeout,
		ToolMode:        defaultToolMode,
		WorkspaceRoot:   workspaceRoot,
		BashTimeout:     defaultBashTimeout,
	}
}

func (c Config) Validate() error {
	switch c.ModelMode {
	case ModelModeMock:
	case ModelModeProvider:
		if strings.TrimSpace(c.ProviderAPIKey) == "" {
			return errors.New("validate config: provider mode requires CODING_AGENT_PROVIDER_API_KEY")
		}
		if strings.TrimSpace(c.ProviderModel) == "" {
			return errors.New("validate config: provider mode requires CODING_AGENT_PROVIDER_MODEL")
		}
		if strings.TrimSpace(c.ProviderBaseURL) == "" {
			return errors.New("validate config: provider mode requires CODING_AGENT_PROVIDER_BASE_URL")
		}
		if c.ProviderTimeout <= 0 {
			return errors.New("validate config: provider mode requires CODING_AGENT_PROVIDER_TIMEOUT > 0")
		}
	default:
		return fmt.Errorf(
			"validate config: unsupported CODING_AGENT_MODEL_MODE %q (allowed: %q, %q)",
			c.ModelMode,
			ModelModeMock,
			ModelModeProvider,
		)
	}

	switch c.ToolMode {
	case ToolModeMock:
	case ToolModeReal:
		if strings.TrimSpace(c.WorkspaceRoot) == "" {
			return errors.New("validate config: real tool mode requires CODING_AGENT_WORKSPACE_ROOT")
		}
		if c.BashTimeout <= 0 {
			return errors.New("validate config: real tool mode requires CODING_AGENT_BASH_TIMEOUT > 0")
		}
	default:
		return fmt.Errorf(
			"validate config: unsupported CODING_AGENT_TOOL_MODE %q (allowed: %q, %q)",
			c.ToolMode,
			ToolModeMock,
			ToolModeReal,
		)
	}

	switch c.LogLevel {
	case slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError:
	default:
		return fmt.Errorf(
			"validate config: unsupported CODING_AGENT_LOG_LEVEL %q (allowed: %q, %q, %q, %q)",
			c.LogLevel.String(),
			slog.LevelDebug.String(),
			slog.LevelInfo.String(),
			slog.LevelWarn.String(),
			slog.LevelError.String(),
		)
	}

	switch c.LogFormat {
	case LogFormatText, LogFormatJSON:
	default:
		return fmt.Errorf(
			"validate config: unsupported CODING_AGENT_LOG_FORMAT %q (allowed: %q, %q)",
			c.LogFormat,
			LogFormatText,
			LogFormatJSON,
		)
	}

	return nil
}

func parseLogLevel(input string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf(
			"parse CODING_AGENT_LOG_LEVEL: unsupported value %q (allowed: %q, %q, %q, %q)",
			input,
			slog.LevelDebug.String(),
			slog.LevelInfo.String(),
			slog.LevelWarn.String(),
			slog.LevelError.String(),
		)
	}
}

func parseLogFormat(input string) (LogFormat, error) {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case string(LogFormatText):
		return LogFormatText, nil
	case string(LogFormatJSON):
		return LogFormatJSON, nil
	default:
		return "", fmt.Errorf(
			"parse CODING_AGENT_LOG_FORMAT: unsupported value %q (allowed: %q, %q)",
			input,
			LogFormatText,
			LogFormatJSON,
		)
	}
}
