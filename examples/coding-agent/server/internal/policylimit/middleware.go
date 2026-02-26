package policylimit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	DefaultMaxRequestBodyBytes = 1 << 20
	DefaultRequestTimeout      = 10 * time.Second
	DefaultMaxCommandSteps     = 8
)

var (
	ErrRequestTooLarge       = errors.New("policy request body too large")
	ErrRequestTimedOut       = errors.New("policy request timeout exceeded")
	ErrCommandBudgetExceeded = errors.New("policy command budget exceeded")
)

type Config struct {
	MaxRequestBodyBytes int64
	RequestTimeout      time.Duration
	MaxCommandSteps     int
}

type RejectFunc func(http.ResponseWriter, *http.Request, error)

func NormalizeConfig(cfg Config) Config {
	if cfg.MaxRequestBodyBytes <= 0 {
		cfg.MaxRequestBodyBytes = DefaultMaxRequestBodyBytes
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = DefaultRequestTimeout
	}
	if cfg.MaxCommandSteps <= 0 {
		cfg.MaxCommandSteps = DefaultMaxCommandSteps
	}
	return cfg
}

func Middleware(cfg Config, _ RejectFunc) func(http.Handler) http.Handler {
	cfg = NormalizeConfig(cfg)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, cfg.MaxRequestBodyBytes)
			}

			ctx, cancel := context.WithTimeoutCause(r.Context(), cfg.RequestTimeout, ErrRequestTimedOut)
			defer cancel()

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func NormalizeCommandMaxSteps(input *int, maxAllowed int) (int, error) {
	if maxAllowed <= 0 {
		maxAllowed = DefaultMaxCommandSteps
	}

	if input == nil {
		if DefaultMaxCommandSteps > maxAllowed {
			return 0, fmt.Errorf(
				"%w: implicit max_steps=%d exceeds policy max_steps=%d",
				ErrCommandBudgetExceeded,
				DefaultMaxCommandSteps,
				maxAllowed,
			)
		}
		return 0, nil
	}

	if *input <= 0 {
		return 0, errors.New("max_steps must be greater than 0 when provided")
	}
	if *input > maxAllowed {
		return 0, fmt.Errorf(
			"%w: requested max_steps=%d policy max_steps=%d",
			ErrCommandBudgetExceeded,
			*input,
			maxAllowed,
		)
	}
	return *input, nil
}
