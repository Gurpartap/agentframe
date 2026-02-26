package httpapi

import (
	"net/http"

	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/runtimewire"
)

type handlers struct {
	runtime *runtimewire.Runtime
}

func NewRouter(runtime *runtimewire.Runtime) http.Handler {
	h := &handlers{runtime: runtime}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/runs/start", h.handleRunStart)
	mux.HandleFunc("POST /v1/runs/{run_id}/continue", h.handleRunContinue)
	mux.HandleFunc("POST /v1/runs/{run_id}/cancel", h.handleRunCancel)
	mux.HandleFunc("POST /v1/runs/{run_id}/steer", h.handleRunSteer)
	mux.HandleFunc("POST /v1/runs/{run_id}/follow-up", h.handleRunFollowUp)
	mux.HandleFunc("GET /v1/runs/{run_id}", h.handleRunQuery)
	return mux
}
