package httpapi

import (
	"net/http"
	"time"

	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/policyauth"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/policylimit"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/runtimewire"
)

type PolicyConfig struct {
	AuthToken           string
	MaxRequestBodyBytes int64
	RequestTimeout      time.Duration
	MaxCommandSteps     int
}

func DefaultPolicyConfig() PolicyConfig {
	return PolicyConfig{
		AuthToken:           policyauth.DefaultToken,
		MaxRequestBodyBytes: policylimit.DefaultMaxRequestBodyBytes,
		RequestTimeout:      policylimit.DefaultRequestTimeout,
		MaxCommandSteps:     policylimit.DefaultMaxCommandSteps,
	}
}

func normalizePolicyConfig(input PolicyConfig) PolicyConfig {
	defaults := DefaultPolicyConfig()
	if input.AuthToken == "" {
		input.AuthToken = defaults.AuthToken
	}
	if input.MaxRequestBodyBytes <= 0 {
		input.MaxRequestBodyBytes = defaults.MaxRequestBodyBytes
	}
	if input.RequestTimeout <= 0 {
		input.RequestTimeout = defaults.RequestTimeout
	}
	if input.MaxCommandSteps <= 0 {
		input.MaxCommandSteps = defaults.MaxCommandSteps
	}
	return input
}

type handlers struct {
	runtime *runtimewire.Runtime
	policy  PolicyConfig
}

func NewRouter(runtime *runtimewire.Runtime, policy ...PolicyConfig) http.Handler {
	normalized := DefaultPolicyConfig()
	if len(policy) > 0 {
		normalized = normalizePolicyConfig(policy[0])
	}

	h := &handlers{
		runtime: runtime,
		policy:  normalized,
	}

	reject := func(w http.ResponseWriter, _ *http.Request, err error) {
		writeMappedError(w, err)
	}

	applyMutatingPolicies := chain(
		policyauth.Middleware(normalized.AuthToken, reject),
		policylimit.Middleware(policylimit.Config{
			MaxRequestBodyBytes: normalized.MaxRequestBodyBytes,
			RequestTimeout:      normalized.RequestTimeout,
			MaxCommandSteps:     normalized.MaxCommandSteps,
		}, reject),
	)

	mux := http.NewServeMux()
	mux.Handle("POST /v1/runs/start", applyMutatingPolicies(http.HandlerFunc(h.handleRunStart)))
	mux.Handle("POST /v1/runs/{run_id}/continue", applyMutatingPolicies(http.HandlerFunc(h.handleRunContinue)))
	mux.Handle("POST /v1/runs/{run_id}/cancel", applyMutatingPolicies(http.HandlerFunc(h.handleRunCancel)))
	mux.Handle("POST /v1/runs/{run_id}/steer", applyMutatingPolicies(http.HandlerFunc(h.handleRunSteer)))
	mux.Handle("POST /v1/runs/{run_id}/follow-up", applyMutatingPolicies(http.HandlerFunc(h.handleRunFollowUp)))
	mux.HandleFunc("GET /v1/runs/{run_id}", h.handleRunQuery)
	mux.HandleFunc("GET /v1/runs/{run_id}/events", h.handleRunEvents)
	return mux
}

type middleware func(http.Handler) http.Handler

func chain(middlewares ...middleware) middleware {
	return func(next http.Handler) http.Handler {
		wrapped := next
		for i := len(middlewares) - 1; i >= 0; i-- {
			wrapped = middlewares[i](wrapped)
		}
		return wrapped
	}
}
