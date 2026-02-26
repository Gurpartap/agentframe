package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/policylimit"
)

type startRequest struct {
	RunID        string `json:"run_id"`
	SystemPrompt string `json:"system_prompt"`
	UserPrompt   string `json:"user_prompt"`
	MaxSteps     *int   `json:"max_steps"`
}

type continueRequest struct {
	MaxSteps   *int               `json:"max_steps"`
	Resolution *resolutionRequest `json:"resolution"`
}

type resolutionRequest struct {
	RequirementID string `json:"requirement_id"`
	Kind          string `json:"kind"`
	Outcome       string `json:"outcome"`
	Value         string `json:"value"`
}

type steerRequest struct {
	Instruction string `json:"instruction"`
}

type followUpRequest struct {
	Prompt   string `json:"prompt"`
	MaxSteps *int   `json:"max_steps"`
}

func (h *handlers) handleRunStart(w http.ResponseWriter, r *http.Request) {
	if !h.ensureRuntime(w) {
		return
	}

	var request startRequest
	if err := decodeJSONBody(r, &request); err != nil {
		writeMappedError(w, err)
		return
	}

	if strings.TrimSpace(request.UserPrompt) == "" {
		writeInvalidRequest(w, "user_prompt is required")
		return
	}
	if request.RunID != "" && strings.TrimSpace(request.RunID) == "" {
		writeInvalidRequest(w, "run_id must not be blank")
		return
	}

	maxSteps, err := validateMaxSteps(request.MaxSteps, h.policy.MaxCommandSteps)
	if err != nil {
		writeMappedError(w, err)
		return
	}

	result, err := h.runtime.Runner.Run(r.Context(), agent.RunInput{
		RunID:        agent.RunID(strings.TrimSpace(request.RunID)),
		SystemPrompt: request.SystemPrompt,
		UserPrompt:   request.UserPrompt,
		MaxSteps:     maxSteps,
		Tools:        h.runtime.ToolDefinitions,
	})
	if err != nil && !isAcceptedRunError(err) {
		writeMappedError(w, err)
		return
	}

	writeRunState(w, http.StatusOK, result.State)
}

func (h *handlers) handleRunContinue(w http.ResponseWriter, r *http.Request) {
	if !h.ensureRuntime(w) {
		return
	}

	runID, err := pathRunID(r)
	if err != nil {
		writeMappedError(w, err)
		return
	}

	var request continueRequest
	if err := decodeJSONBody(r, &request); err != nil {
		writeMappedError(w, err)
		return
	}

	maxSteps, err := validateMaxSteps(request.MaxSteps, h.policy.MaxCommandSteps)
	if err != nil {
		writeMappedError(w, err)
		return
	}

	result, err := h.runtime.Runner.Continue(
		r.Context(),
		runID,
		maxSteps,
		h.runtime.ToolDefinitions,
		toResolution(request.Resolution),
	)
	if err != nil && !isAcceptedRunError(err) {
		writeMappedError(w, err)
		return
	}

	writeRunState(w, http.StatusOK, result.State)
}

func (h *handlers) handleRunCancel(w http.ResponseWriter, r *http.Request) {
	if !h.ensureRuntime(w) {
		return
	}

	runID, err := pathRunID(r)
	if err != nil {
		writeMappedError(w, err)
		return
	}

	result, err := h.runtime.Runner.Cancel(r.Context(), runID)
	if err != nil {
		writeMappedError(w, err)
		return
	}

	writeRunState(w, http.StatusOK, result.State)
}

func (h *handlers) handleRunSteer(w http.ResponseWriter, r *http.Request) {
	if !h.ensureRuntime(w) {
		return
	}

	runID, err := pathRunID(r)
	if err != nil {
		writeMappedError(w, err)
		return
	}

	var request steerRequest
	if err := decodeJSONBody(r, &request); err != nil {
		writeMappedError(w, err)
		return
	}
	if strings.TrimSpace(request.Instruction) == "" {
		writeInvalidRequest(w, "instruction is required")
		return
	}

	result, err := h.runtime.Runner.Steer(r.Context(), runID, request.Instruction)
	if err != nil {
		writeMappedError(w, err)
		return
	}

	writeRunState(w, http.StatusOK, result.State)
}

func (h *handlers) handleRunFollowUp(w http.ResponseWriter, r *http.Request) {
	if !h.ensureRuntime(w) {
		return
	}

	runID, err := pathRunID(r)
	if err != nil {
		writeMappedError(w, err)
		return
	}

	var request followUpRequest
	if err := decodeJSONBody(r, &request); err != nil {
		writeMappedError(w, err)
		return
	}
	if strings.TrimSpace(request.Prompt) == "" {
		writeInvalidRequest(w, "prompt is required")
		return
	}

	maxSteps, err := validateMaxSteps(request.MaxSteps, h.policy.MaxCommandSteps)
	if err != nil {
		writeMappedError(w, err)
		return
	}

	result, err := h.runtime.Runner.FollowUp(r.Context(), runID, request.Prompt, maxSteps, h.runtime.ToolDefinitions)
	if err != nil && !isAcceptedRunError(err) {
		writeMappedError(w, err)
		return
	}

	writeRunState(w, http.StatusOK, result.State)
}

func (h *handlers) ensureRuntime(w http.ResponseWriter) bool {
	if h.runtime == nil || h.runtime.Runner == nil || h.runtime.RunStore == nil || h.runtime.StreamBroker == nil {
		writeError(w, http.StatusInternalServerError, errorCodeRuntime, "runtime dependencies are not initialized")
		return false
	}
	return true
}

func validateMaxSteps(input *int, maxAllowed int) (int, error) {
	maxSteps, err := policylimit.NormalizeCommandMaxSteps(input, maxAllowed)
	if err == nil {
		return maxSteps, nil
	}
	if errors.Is(err, policylimit.ErrCommandBudgetExceeded) {
		return 0, err
	}
	return 0, invalidRequestError(err.Error())
}

func toResolution(input *resolutionRequest) *agent.Resolution {
	if input == nil {
		return nil
	}
	return &agent.Resolution{
		RequirementID: strings.TrimSpace(input.RequirementID),
		Kind:          agent.RequirementKind(input.Kind),
		Outcome:       agent.ResolutionOutcome(input.Outcome),
		Value:         input.Value,
	}
}

func pathRunID(r *http.Request) (agent.RunID, error) {
	runID := strings.TrimSpace(r.PathValue("run_id"))
	if runID == "" {
		return "", agent.ErrInvalidRunID
	}
	return agent.RunID(runID), nil
}
