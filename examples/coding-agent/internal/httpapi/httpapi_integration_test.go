package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/httpapi"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/runtimewire"
)

type runStateResponse struct {
	RunID              string `json:"run_id"`
	Status             string `json:"status"`
	Step               int    `json:"step"`
	Version            int64  `json:"version"`
	Output             string `json:"output"`
	Error              string `json:"error"`
	PendingRequirement *struct {
		ID     string `json:"id"`
		Kind   string `json:"kind"`
		Prompt string `json:"prompt"`
	} `json:"pending_requirement,omitempty"`
}

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func TestRunStartAndQueryKnownUnknown(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	defer server.Close()

	var started runStateResponse
	status := performJSON(t, server.Client(), http.MethodPost, server.URL+"/v1/runs/start", map[string]any{
		"user_prompt": "hello from test",
		"max_steps":   2,
	}, &started)
	if status != http.StatusOK {
		t.Fatalf("start status mismatch: got=%d want=%d", status, http.StatusOK)
	}
	if started.RunID == "" {
		t.Fatalf("expected run_id in start response")
	}
	if started.Status != string(agent.RunStatusCompleted) {
		t.Fatalf("start status mismatch: got=%s want=%s", started.Status, agent.RunStatusCompleted)
	}

	var queried runStateResponse
	status = performJSON(t, server.Client(), http.MethodGet, server.URL+"/v1/runs/"+started.RunID, nil, &queried)
	if status != http.StatusOK {
		t.Fatalf("query status mismatch: got=%d want=%d", status, http.StatusOK)
	}
	if queried.RunID != started.RunID {
		t.Fatalf("query run_id mismatch: got=%q want=%q", queried.RunID, started.RunID)
	}
	if queried.Status != started.Status {
		t.Fatalf("query status mismatch: got=%s want=%s", queried.Status, started.Status)
	}

	var unknown errorResponse
	status = performJSON(t, server.Client(), http.MethodGet, server.URL+"/v1/runs/does-not-exist", nil, &unknown)
	if status != http.StatusNotFound {
		t.Fatalf("unknown query status mismatch: got=%d want=%d", status, http.StatusNotFound)
	}
	if unknown.Error.Code != "not_found" {
		t.Fatalf("unknown query error code mismatch: got=%q want=%q", unknown.Error.Code, "not_found")
	}
}

func TestRunStartInvalidRequest(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	defer server.Close()

	var failed errorResponse
	status := performJSON(t, server.Client(), http.MethodPost, server.URL+"/v1/runs/start", map[string]any{
		"system_prompt": "missing user prompt",
	}, &failed)
	if status != http.StatusBadRequest {
		t.Fatalf("start invalid status mismatch: got=%d want=%d", status, http.StatusBadRequest)
	}
	if failed.Error.Code != "invalid_request" {
		t.Fatalf("start invalid error code mismatch: got=%q want=%q", failed.Error.Code, "invalid_request")
	}
}

func TestRunContinueResolutionGating(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	defer server.Close()

	var started runStateResponse
	status := performJSON(t, server.Client(), http.MethodPost, server.URL+"/v1/runs/start", map[string]any{
		"user_prompt": "[suspend] approval gate",
		"max_steps":   2,
	}, &started)
	if status != http.StatusOK {
		t.Fatalf("start status mismatch: got=%d want=%d", status, http.StatusOK)
	}
	if started.Status != string(agent.RunStatusSuspended) {
		t.Fatalf("expected suspended start status, got=%s", started.Status)
	}
	if started.PendingRequirement == nil {
		t.Fatalf("expected pending requirement for suspended run")
	}

	continueURL := server.URL + "/v1/runs/" + started.RunID + "/continue"

	var missingResolution errorResponse
	status = performJSON(t, server.Client(), http.MethodPost, continueURL, map[string]any{
		"max_steps": 2,
	}, &missingResolution)
	if status != http.StatusForbidden {
		t.Fatalf("continue missing resolution status mismatch: got=%d want=%d", status, http.StatusForbidden)
	}
	if missingResolution.Error.Code != "forbidden" {
		t.Fatalf("continue missing resolution code mismatch: got=%q want=%q", missingResolution.Error.Code, "forbidden")
	}

	var invalidResolution errorResponse
	status = performJSON(t, server.Client(), http.MethodPost, continueURL, map[string]any{
		"max_steps": 2,
		"resolution": map[string]any{
			"requirement_id": "req-wrong",
			"kind":           "approval",
			"outcome":        "approved",
		},
	}, &invalidResolution)
	if status != http.StatusBadRequest {
		t.Fatalf("continue invalid resolution status mismatch: got=%d want=%d", status, http.StatusBadRequest)
	}
	if invalidResolution.Error.Code != "invalid_request" {
		t.Fatalf("continue invalid resolution code mismatch: got=%q want=%q", invalidResolution.Error.Code, "invalid_request")
	}

	var continued runStateResponse
	status = performJSON(t, server.Client(), http.MethodPost, continueURL, map[string]any{
		"max_steps": 2,
		"resolution": map[string]any{
			"requirement_id": started.PendingRequirement.ID,
			"kind":           started.PendingRequirement.Kind,
			"outcome":        "approved",
		},
	}, &continued)
	if status != http.StatusOK {
		t.Fatalf("continue valid status mismatch: got=%d want=%d", status, http.StatusOK)
	}
	if continued.Status != string(agent.RunStatusCompleted) {
		t.Fatalf("continue valid status mismatch: got=%s want=%s", continued.Status, agent.RunStatusCompleted)
	}
}

func TestRunSteerFollowUpAndCancelBehavior(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	defer server.Close()

	var runOne runStateResponse
	status := performJSON(t, server.Client(), http.MethodPost, server.URL+"/v1/runs/start", map[string]any{
		"user_prompt": "[loop] keep running",
		"max_steps":   1,
	}, &runOne)
	if status != http.StatusOK {
		t.Fatalf("run one start status mismatch: got=%d want=%d", status, http.StatusOK)
	}
	if runOne.Status != string(agent.RunStatusMaxStepsExceeded) {
		t.Fatalf("run one expected max steps, got=%s", runOne.Status)
	}

	var steered runStateResponse
	status = performJSON(t, server.Client(), http.MethodPost, server.URL+"/v1/runs/"+runOne.RunID+"/steer", map[string]any{
		"instruction": "shift approach",
	}, &steered)
	if status != http.StatusOK {
		t.Fatalf("steer status mismatch: got=%d want=%d", status, http.StatusOK)
	}
	if steered.RunID != runOne.RunID {
		t.Fatalf("steer run_id mismatch: got=%q want=%q", steered.RunID, runOne.RunID)
	}

	var followed runStateResponse
	status = performJSON(t, server.Client(), http.MethodPost, server.URL+"/v1/runs/"+runOne.RunID+"/follow-up", map[string]any{
		"prompt":    "finish now",
		"max_steps": 2,
	}, &followed)
	if status != http.StatusOK {
		t.Fatalf("follow-up status mismatch: got=%d want=%d", status, http.StatusOK)
	}
	if followed.Status != string(agent.RunStatusCompleted) {
		t.Fatalf("follow-up expected completed, got=%s", followed.Status)
	}

	var runTwo runStateResponse
	status = performJSON(t, server.Client(), http.MethodPost, server.URL+"/v1/runs/start", map[string]any{
		"user_prompt": "[loop] cancel target",
		"max_steps":   1,
	}, &runTwo)
	if status != http.StatusOK {
		t.Fatalf("run two start status mismatch: got=%d want=%d", status, http.StatusOK)
	}

	var cancelled runStateResponse
	status = performJSON(t, server.Client(), http.MethodPost, server.URL+"/v1/runs/"+runTwo.RunID+"/cancel", map[string]any{}, &cancelled)
	if status != http.StatusOK {
		t.Fatalf("cancel status mismatch: got=%d want=%d", status, http.StatusOK)
	}
	if cancelled.Status != string(agent.RunStatusCancelled) {
		t.Fatalf("cancel expected cancelled, got=%s", cancelled.Status)
	}
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	runtime, err := runtimewire.New()
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	return httptest.NewServer(httpapi.NewRouter(runtime))
}

func performJSON(t *testing.T, client *http.Client, method, url string, payload any, out any) int {
	t.Helper()

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	if out != nil {
		if err := json.Unmarshal(responseBody, out); err != nil {
			t.Fatalf("decode response: %v body=%s", err, string(responseBody))
		}
	}

	return resp.StatusCode
}
