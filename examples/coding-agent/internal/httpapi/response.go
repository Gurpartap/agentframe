package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/runstream"
)

const maxRequestBodyBytes = 1 << 20

const (
	errorCodeInvalidRequest = "invalid_request"
	errorCodeNotFound       = "not_found"
	errorCodeConflict       = "conflict"
	errorCodeForbidden      = "forbidden"
	errorCodeRuntime        = "runtime_error"
)

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type apiErrorResponse struct {
	Error apiError `json:"error"`
}

type runStateResponse struct {
	RunID              string                      `json:"run_id"`
	Status             agent.RunStatus             `json:"status"`
	Step               int                         `json:"step"`
	Version            int64                       `json:"version"`
	Output             string                      `json:"output,omitempty"`
	Error              string                      `json:"error,omitempty"`
	PendingRequirement *pendingRequirementResponse `json:"pending_requirement,omitempty"`
}

type pendingRequirementResponse struct {
	ID     string                `json:"id"`
	Kind   agent.RequirementKind `json:"kind"`
	Prompt string                `json:"prompt,omitempty"`
}

func writeRunState(w http.ResponseWriter, status int, state agent.RunState) {
	response := runStateResponse{
		RunID:   string(state.ID),
		Status:  state.Status,
		Step:    state.Step,
		Version: state.Version,
		Output:  state.Output,
		Error:   state.Error,
	}
	if state.PendingRequirement != nil {
		response.PendingRequirement = &pendingRequirementResponse{
			ID:     state.PendingRequirement.ID,
			Kind:   state.PendingRequirement.Kind,
			Prompt: state.PendingRequirement.Prompt,
		}
	}
	writeJSON(w, status, response)
}

func writeMappedError(w http.ResponseWriter, err error) {
	status, code := mapRuntimeError(err)
	writeError(w, status, code, err.Error())
}

func writeInvalidRequest(w http.ResponseWriter, message string) {
	writeError(w, http.StatusBadRequest, errorCodeInvalidRequest, message)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, apiErrorResponse{
		Error: apiError{
			Code:    code,
			Message: message,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func decodeJSONBody(r *http.Request, dst any) error {
	if r.Body == nil {
		return errors.New("request body is required")
	}

	decoder := json.NewDecoder(io.LimitReader(r.Body, maxRequestBodyBytes))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("request body is required")
		}
		return fmt.Errorf("invalid JSON body: %w", err)
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain exactly one JSON object")
	}

	return nil
}

func mapRuntimeError(err error) (int, string) {
	switch {
	case errors.Is(err, agent.ErrRunNotFound):
		return http.StatusNotFound, errorCodeNotFound
	case errors.Is(err, agent.ErrCommandConflict), errors.Is(err, agent.ErrRunVersionConflict):
		return http.StatusConflict, errorCodeConflict
	case errors.Is(err, runstream.ErrCursorInvalid), errors.Is(err, runstream.ErrCursorExpired):
		return http.StatusConflict, errorCodeConflict
	case errors.Is(err, agent.ErrRunNotContinuable),
		errors.Is(err, agent.ErrRunNotCancellable),
		errors.Is(err, agent.ErrResolutionRequired):
		return http.StatusForbidden, errorCodeForbidden
	case errors.Is(err, agent.ErrInvalidRunID),
		errors.Is(err, agent.ErrResolutionInvalid),
		errors.Is(err, agent.ErrResolutionUnexpected),
		errors.Is(err, agent.ErrCommandInvalid),
		errors.Is(err, agent.ErrCommandNil),
		errors.Is(err, agent.ErrCommandUnsupported),
		errors.Is(err, agent.ErrRunStateInvalid),
		errors.Is(err, agent.ErrToolDefinitionsInvalid),
		errors.Is(err, agent.ErrContextNil):
		return http.StatusBadRequest, errorCodeInvalidRequest
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return http.StatusInternalServerError, errorCodeRuntime
	default:
		return http.StatusInternalServerError, errorCodeRuntime
	}
}

func isAcceptedRunError(err error) bool {
	return errors.Is(err, agent.ErrMaxStepsExceeded) && !errors.Is(err, agent.ErrEventPublish)
}
