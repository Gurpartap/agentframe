package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/config"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/httpapi"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/policyauth"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/policylimit"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/runtimewire"
)

const testAuthToken = "integration-test-token"

type runStateResponse struct {
	RunID              string `json:"run_id"`
	Status             string `json:"status"`
	Step               int    `json:"step"`
	Version            int64  `json:"version"`
	Output             string `json:"output"`
	Error              string `json:"error"`
	PendingRequirement *struct {
		ID         string `json:"id"`
		Kind       string `json:"kind"`
		Origin     string `json:"origin"`
		ToolCallID string `json:"tool_call_id,omitempty"`
		Prompt     string `json:"prompt"`
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
	if started.PendingRequirement.Origin != string(agent.RequirementOriginModel) {
		t.Fatalf(
			"pending requirement origin mismatch: got=%q want=%q",
			started.PendingRequirement.Origin,
			agent.RequirementOriginModel,
		)
	}
	if started.PendingRequirement.ToolCallID != "" {
		t.Fatalf("expected empty tool_call_id for model-origin requirement, got=%q", started.PendingRequirement.ToolCallID)
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

func TestRunStartBashPolicyDeniedSuspendsWithToolOriginRequirement(t *testing.T) {
	t.Parallel()

	server := newTestServerWithRuntimeConfig(
		t,
		httpapi.PolicyConfig{
			AuthToken:           testAuthToken,
			MaxRequestBodyBytes: 4 << 10,
			RequestTimeout:      2 * time.Second,
			MaxCommandSteps:     policylimit.DefaultMaxCommandSteps,
		},
		func(cfg *config.Config) {
			cfg.ModelMode = config.ModelModeMock
			cfg.ToolMode = config.ToolModeReal
			cfg.WorkspaceRoot = t.TempDir()
		},
	)
	defer server.Close()

	var started runStateResponse
	status := performJSON(t, server.Client(), http.MethodPost, server.URL+"/v1/runs/start", map[string]any{
		"user_prompt": "[e2e-bash-policy-denied]",
		"max_steps":   4,
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
	if started.PendingRequirement.Kind != string(agent.RequirementKindApproval) {
		t.Fatalf("pending requirement kind mismatch: got=%q want=%q", started.PendingRequirement.Kind, agent.RequirementKindApproval)
	}
	if started.PendingRequirement.Origin != string(agent.RequirementOriginTool) {
		t.Fatalf(
			"pending requirement origin mismatch: got=%q want=%q",
			started.PendingRequirement.Origin,
			agent.RequirementOriginTool,
		)
	}
	if started.PendingRequirement.ToolCallID != "call-bash-denied-1" {
		t.Fatalf(
			"pending requirement tool_call_id mismatch: got=%q want=%q",
			started.PendingRequirement.ToolCallID,
			"call-bash-denied-1",
		)
	}
	if started.PendingRequirement.ID != "req-bash-policy-call-bash-denied-1" {
		t.Fatalf(
			"pending requirement id mismatch: got=%q want=%q",
			started.PendingRequirement.ID,
			"req-bash-policy-call-bash-denied-1",
		)
	}
	if started.PendingRequirement.Prompt == "" {
		t.Fatalf("expected pending requirement prompt")
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

func TestMutatingRoutesRequireAuth(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	defer server.Close()

	body, err := json.Marshal(map[string]any{
		"user_prompt": "auth rejection probe",
		"max_steps":   1,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/runs/start", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("do unauthorized request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized status mismatch: got=%d want=%d", resp.StatusCode, http.StatusUnauthorized)
	}

	var rejected errorResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read unauthorized response: %v", err)
	}
	if err := json.Unmarshal(responseBody, &rejected); err != nil {
		t.Fatalf("decode unauthorized response: %v body=%s", err, string(responseBody))
	}
	if rejected.Error.Code != "unauthorized" {
		t.Fatalf("unauthorized code mismatch: got=%q want=%q", rejected.Error.Code, "unauthorized")
	}

	var accepted runStateResponse
	status := performJSON(t, server.Client(), http.MethodPost, server.URL+"/v1/runs/start", map[string]any{
		"user_prompt": "auth acceptance probe",
		"max_steps":   1,
	}, &accepted)
	if status != http.StatusOK {
		t.Fatalf("authorized start status mismatch: got=%d want=%d", status, http.StatusOK)
	}
}

func TestPolicyLimitRejections(t *testing.T) {
	t.Parallel()

	server := newTestServerWithPolicy(t, httpapi.PolicyConfig{
		AuthToken:           testAuthToken,
		MaxRequestBodyBytes: 128,
		RequestTimeout:      30 * time.Millisecond,
		MaxCommandSteps:     2,
	})
	defer server.Close()

	var oversized errorResponse
	status := performJSON(t, server.Client(), http.MethodPost, server.URL+"/v1/runs/start", map[string]any{
		"user_prompt": strings.Repeat("x", 1024),
		"max_steps":   1,
	}, &oversized)
	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status mismatch: got=%d want=%d", status, http.StatusRequestEntityTooLarge)
	}
	if oversized.Error.Code != "policy_rejected" {
		t.Fatalf("oversized error code mismatch: got=%q want=%q", oversized.Error.Code, "policy_rejected")
	}

	var budget errorResponse
	status = performJSON(t, server.Client(), http.MethodPost, server.URL+"/v1/runs/start", map[string]any{
		"user_prompt": "budget cap probe",
		"max_steps":   3,
	}, &budget)
	if status != http.StatusTooManyRequests {
		t.Fatalf("budget status mismatch: got=%d want=%d", status, http.StatusTooManyRequests)
	}
	if budget.Error.Code != "policy_rejected" {
		t.Fatalf("budget error code mismatch: got=%q want=%q", budget.Error.Code, "policy_rejected")
	}

	var timedOut errorResponse
	status = performJSON(t, server.Client(), http.MethodPost, server.URL+"/v1/runs/start", map[string]any{
		"user_prompt": "[sleep] timeout probe",
		"max_steps":   1,
	}, &timedOut)
	if status != http.StatusRequestTimeout {
		t.Fatalf("timeout status mismatch: got=%d want=%d", status, http.StatusRequestTimeout)
	}
	if timedOut.Error.Code != "policy_rejected" {
		t.Fatalf("timeout error code mismatch: got=%q want=%q", timedOut.Error.Code, "policy_rejected")
	}
}

func TestCancellationAndConflictDeterminism(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	defer server.Close()

	const runID = "deterministic-run"

	var initial runStateResponse
	status := performJSON(t, server.Client(), http.MethodPost, server.URL+"/v1/runs/start", map[string]any{
		"run_id":      runID,
		"user_prompt": "[loop] deterministic conflict",
		"max_steps":   1,
	}, &initial)
	if status != http.StatusOK {
		t.Fatalf("initial start status mismatch: got=%d want=%d", status, http.StatusOK)
	}
	if initial.Status != string(agent.RunStatusMaxStepsExceeded) {
		t.Fatalf("initial status mismatch: got=%s want=%s", initial.Status, agent.RunStatusMaxStepsExceeded)
	}

	var conflict errorResponse
	status = performJSON(t, server.Client(), http.MethodPost, server.URL+"/v1/runs/start", map[string]any{
		"run_id":      runID,
		"user_prompt": "second start same id",
		"max_steps":   1,
	}, &conflict)
	if status != http.StatusConflict {
		t.Fatalf("conflict status mismatch: got=%d want=%d", status, http.StatusConflict)
	}
	if conflict.Error.Code != "conflict" {
		t.Fatalf("conflict code mismatch: got=%q want=%q", conflict.Error.Code, "conflict")
	}

	var cancelled runStateResponse
	status = performJSON(t, server.Client(), http.MethodPost, server.URL+"/v1/runs/"+runID+"/cancel", map[string]any{}, &cancelled)
	if status != http.StatusOK {
		t.Fatalf("cancel status mismatch: got=%d want=%d", status, http.StatusOK)
	}
	if cancelled.Status != string(agent.RunStatusCancelled) {
		t.Fatalf("cancel status mismatch: got=%s want=%s", cancelled.Status, agent.RunStatusCancelled)
	}

	var repeatCancel errorResponse
	status = performJSON(t, server.Client(), http.MethodPost, server.URL+"/v1/runs/"+runID+"/cancel", map[string]any{}, &repeatCancel)
	if status != http.StatusForbidden {
		t.Fatalf("repeat cancel status mismatch: got=%d want=%d", status, http.StatusForbidden)
	}
	if repeatCancel.Error.Code != "forbidden" {
		t.Fatalf("repeat cancel code mismatch: got=%q want=%q", repeatCancel.Error.Code, "forbidden")
	}
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return newTestServerWithRuntimeConfig(t, httpapi.PolicyConfig{
		AuthToken:           testAuthToken,
		MaxRequestBodyBytes: 4 << 10,
		RequestTimeout:      2 * time.Second,
		MaxCommandSteps:     policylimit.DefaultMaxCommandSteps,
	}, nil)
}

func newTestServerWithPolicy(t *testing.T, policy httpapi.PolicyConfig) *httptest.Server {
	t.Helper()
	return newTestServerWithRuntimeConfig(t, policy, nil)
}

func newTestServerWithRuntimeConfig(
	t *testing.T,
	policy httpapi.PolicyConfig,
	configure func(*config.Config),
) *httptest.Server {
	t.Helper()

	cfg := config.Default()
	cfg.ModelMode = config.ModelModeMock
	cfg.ToolMode = config.ToolModeMock
	if configure != nil {
		configure(&cfg)
	}

	runtime, err := runtimewire.New(cfg)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	return httptest.NewServer(httpapi.NewRouter(runtime, policy))
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
	if method == http.MethodPost {
		req.Header.Set(policyauth.HeaderAuthorization, policyauth.BearerPrefix+testAuthToken)
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
