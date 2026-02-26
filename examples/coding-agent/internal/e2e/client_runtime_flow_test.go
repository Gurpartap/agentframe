package e2e_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/clientapi"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/clientevents"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/config"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/httpapi"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/runtimewire"
)

const testClientAuthToken = "client-e2e-token"

func TestClientStartEventsFollowUpFlow(t *testing.T) {
	t.Parallel()

	server, apiClient := newClientE2EServer(t)
	defer server.Close()

	maxSteps := 1
	started, _, err := apiClient.Start(context.Background(), clientapi.StartRequest{
		UserPrompt: "[loop] e2e start events follow-up",
		MaxSteps:   &maxSteps,
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if started.Status != string(agent.RunStatusMaxStepsExceeded) {
		t.Fatalf("start status mismatch: got=%s want=%s", started.Status, agent.RunStatusMaxStepsExceeded)
	}

	initialFrames := readStreamFrames(t, server.Client(), server.URL, started.RunID, 0, 6, 2*time.Second)
	if len(initialFrames) != 6 {
		t.Fatalf("initial frame count mismatch: got=%d want=%d", len(initialFrames), 6)
	}
	for i := 0; i < len(initialFrames); i++ {
		if initialFrames[i].ID != int64(i+1) {
			t.Fatalf("initial frame id mismatch at index %d: got=%d want=%d", i, initialFrames[i].ID, i+1)
		}
	}

	followed, _, err := apiClient.FollowUp(context.Background(), started.RunID, clientapi.FollowUpRequest{
		Prompt:   "finish flow",
		MaxSteps: intPtr(2),
	})
	if err != nil {
		t.Fatalf("follow-up run: %v", err)
	}
	if followed.Status != string(agent.RunStatusCompleted) {
		t.Fatalf("follow-up status mismatch: got=%s want=%s", followed.Status, agent.RunStatusCompleted)
	}

	lastID := initialFrames[len(initialFrames)-1].ID
	reconnectedFrames := readStreamFrames(
		t,
		server.Client(),
		server.URL,
		started.RunID,
		lastID,
		4,
		2*time.Second,
	)
	if len(reconnectedFrames) != 4 {
		t.Fatalf("reconnected frame count mismatch: got=%d want=%d", len(reconnectedFrames), 4)
	}
	for i := 0; i < len(reconnectedFrames); i++ {
		expectedID := lastID + int64(i+1)
		if reconnectedFrames[i].ID != expectedID {
			t.Fatalf("reconnected frame id mismatch at index %d: got=%d want=%d", i, reconnectedFrames[i].ID, expectedID)
		}
	}
}

func TestClientSuspendedResolutionContinueFlow(t *testing.T) {
	t.Parallel()

	server, apiClient := newClientE2EServer(t)
	defer server.Close()

	started, _, err := apiClient.Start(context.Background(), clientapi.StartRequest{
		UserPrompt: "[suspend] e2e resolution path",
		MaxSteps:   intPtr(2),
	})
	if err != nil {
		t.Fatalf("start suspended run: %v", err)
	}
	if started.Status != string(agent.RunStatusSuspended) {
		t.Fatalf("suspended start status mismatch: got=%s want=%s", started.Status, agent.RunStatusSuspended)
	}
	if started.PendingRequirement == nil {
		t.Fatalf("expected pending requirement in suspended run")
	}

	_, _, err = apiClient.Continue(context.Background(), started.RunID, clientapi.ContinueRequest{
		MaxSteps: intPtr(2),
	})
	if err == nil {
		t.Fatalf("expected continue without resolution to fail")
	}
	var requestErr *clientapi.RequestError
	if !errors.As(err, &requestErr) {
		t.Fatalf("expected RequestError, got %T (%v)", err, err)
	}
	if requestErr.StatusCode != http.StatusForbidden {
		t.Fatalf("continue missing resolution status mismatch: got=%d want=%d", requestErr.StatusCode, http.StatusForbidden)
	}
	if requestErr.Code != "forbidden" {
		t.Fatalf("continue missing resolution code mismatch: got=%q want=%q", requestErr.Code, "forbidden")
	}

	continued, _, err := apiClient.Continue(context.Background(), started.RunID, clientapi.ContinueRequest{
		MaxSteps: intPtr(2),
		Resolution: &clientapi.Resolution{
			RequirementID: started.PendingRequirement.ID,
			Kind:          started.PendingRequirement.Kind,
			Outcome:       "approved",
		},
	})
	if err != nil {
		t.Fatalf("continue with resolution: %v", err)
	}
	if continued.Status != string(agent.RunStatusCompleted) {
		t.Fatalf("continue with resolution status mismatch: got=%s want=%s", continued.Status, agent.RunStatusCompleted)
	}
}

func TestClientCancelFlow(t *testing.T) {
	t.Parallel()

	server, apiClient := newClientE2EServer(t)
	defer server.Close()

	started, _, err := apiClient.Start(context.Background(), clientapi.StartRequest{
		UserPrompt: "[loop] e2e cancel path",
		MaxSteps:   intPtr(1),
	})
	if err != nil {
		t.Fatalf("start cancel flow run: %v", err)
	}
	if started.Status != string(agent.RunStatusMaxStepsExceeded) {
		t.Fatalf("cancel flow start status mismatch: got=%s want=%s", started.Status, agent.RunStatusMaxStepsExceeded)
	}

	cancelled, _, err := apiClient.Cancel(context.Background(), started.RunID)
	if err != nil {
		t.Fatalf("cancel run: %v", err)
	}
	if cancelled.Status != string(agent.RunStatusCancelled) {
		t.Fatalf("cancel status mismatch: got=%s want=%s", cancelled.Status, agent.RunStatusCancelled)
	}
}

func newClientE2EServer(t *testing.T) (*httptest.Server, *clientapi.Client) {
	t.Helper()

	cfg := config.Default()
	cfg.ModelMode = config.ModelModeMock
	cfg.ToolMode = config.ToolModeMock

	runtime, err := runtimewire.New(cfg)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	policy := httpapi.DefaultPolicyConfig()
	policy.AuthToken = testClientAuthToken

	server := httptest.NewServer(httpapi.NewRouter(runtime, policy))

	apiClient, err := clientapi.New(server.URL, testClientAuthToken, server.Client())
	if err != nil {
		server.Close()
		t.Fatalf("new client api: %v", err)
	}

	return server, apiClient
}

func readStreamFrames(
	t *testing.T,
	httpClient *http.Client,
	baseURL string,
	runID string,
	cursor int64,
	wantFrames int,
	timeout time.Duration,
) []clientevents.StreamEvent {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		baseURL+"/v1/runs/"+runID+"/events?cursor="+strconv.FormatInt(cursor, 10),
		nil,
	)
	if err != nil {
		t.Fatalf("new stream request: %v", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("do stream request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("stream status mismatch: got=%d want=%d body=%s", resp.StatusCode, http.StatusOK, string(body))
	}

	reader := clientevents.NewReader(resp.Body)
	frames := make([]clientevents.StreamEvent, 0, wantFrames)
	for len(frames) < wantFrames {
		frame, _, err := reader.Next()
		if err != nil {
			t.Fatalf("read stream frame: %v", err)
		}
		if frame.ID <= 0 {
			t.Fatalf("stream frame id must be > 0, got=%d", frame.ID)
		}
		if frame.Event.RunID != agent.RunID(runID) {
			t.Fatalf("stream frame run_id mismatch: got=%q want=%q", frame.Event.RunID, runID)
		}
		frames = append(frames, frame)
	}
	return frames
}

func intPtr(value int) *int {
	return &value
}
