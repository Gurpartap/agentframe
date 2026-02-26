package httpapi_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/policyauth"
)

type streamLine struct {
	ID    int64       `json:"id"`
	Event agent.Event `json:"event"`
}

func TestRunEventsOrdering(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	defer server.Close()

	var started runStateResponse
	status := performJSON(t, server.Client(), http.MethodPost, server.URL+"/v1/runs/start", map[string]any{
		"user_prompt": "[loop] stream order",
		"max_steps":   1,
	}, &started)
	if status != http.StatusOK {
		t.Fatalf("start status mismatch: got=%d want=%d", status, http.StatusOK)
	}
	if started.Status != string(agent.RunStatusMaxStepsExceeded) {
		t.Fatalf("start status mismatch: got=%s want=%s", started.Status, agent.RunStatusMaxStepsExceeded)
	}

	frames := readNDJSONFrames(
		t,
		server.Client(),
		server.URL+"/v1/runs/"+started.RunID+"/events?cursor=0",
		6,
		2*time.Second,
	)

	expectedTypes := []agent.EventType{
		agent.EventTypeRunStarted,
		agent.EventTypeAssistantMessage,
		agent.EventTypeToolResult,
		agent.EventTypeRunFailed,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	}
	for i := range frames {
		if frames[i].ID != int64(i+1) {
			t.Fatalf("event id mismatch at index %d: got=%d want=%d", i, frames[i].ID, i+1)
		}
		if string(frames[i].Event.RunID) != started.RunID {
			t.Fatalf("event run_id mismatch at index %d: got=%q want=%q", i, frames[i].Event.RunID, started.RunID)
		}
		if frames[i].Event.Type != expectedTypes[i] {
			t.Fatalf("event type mismatch at index %d: got=%s want=%s", i, frames[i].Event.Type, expectedTypes[i])
		}
	}
}

func TestRunEventsReconnectFromCursor(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	defer server.Close()

	var started runStateResponse
	status := performJSON(t, server.Client(), http.MethodPost, server.URL+"/v1/runs/start", map[string]any{
		"user_prompt": "[loop] reconnect stream",
		"max_steps":   1,
	}, &started)
	if status != http.StatusOK {
		t.Fatalf("start status mismatch: got=%d want=%d", status, http.StatusOK)
	}
	if started.Status != string(agent.RunStatusMaxStepsExceeded) {
		t.Fatalf("start status mismatch: got=%s want=%s", started.Status, agent.RunStatusMaxStepsExceeded)
	}

	initialFrames := readNDJSONFrames(
		t,
		server.Client(),
		server.URL+"/v1/runs/"+started.RunID+"/events?cursor=0",
		6,
		2*time.Second,
	)
	lastID := initialFrames[len(initialFrames)-1].ID

	followUpDone := make(chan error, 1)
	go func() {
		time.Sleep(100 * time.Millisecond)
		payload, err := json.Marshal(map[string]any{
			"prompt":    "finish reconnect path",
			"max_steps": 2,
		})
		if err != nil {
			followUpDone <- fmt.Errorf("marshal follow-up payload: %w", err)
			return
		}
		req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/runs/"+started.RunID+"/follow-up", bytes.NewReader(payload))
		if err != nil {
			followUpDone <- fmt.Errorf("new follow-up request: %w", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(policyauth.HeaderAuthorization, policyauth.BearerPrefix+testAuthToken)

		resp, err := server.Client().Do(req)
		if err != nil {
			followUpDone <- fmt.Errorf("do follow-up request: %w", err)
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			followUpDone <- fmt.Errorf("read follow-up response: %w", err)
			return
		}
		if resp.StatusCode != http.StatusOK {
			followUpDone <- fmt.Errorf("follow-up status mismatch: got=%d want=%d body=%s", resp.StatusCode, http.StatusOK, string(body))
			return
		}

		var followed runStateResponse
		if err := json.Unmarshal(body, &followed); err != nil {
			followUpDone <- fmt.Errorf("decode follow-up response: %w", err)
			return
		}
		if followed.Status != string(agent.RunStatusCompleted) {
			followUpDone <- fmt.Errorf("follow-up status mismatch: got=%s want=%s", followed.Status, agent.RunStatusCompleted)
			return
		}
		followUpDone <- nil
	}()

	reconnectedFrames := readNDJSONFrames(
		t,
		server.Client(),
		server.URL+"/v1/runs/"+started.RunID+"/events?cursor="+strconv.FormatInt(lastID, 10),
		4,
		3*time.Second,
	)

	if err := <-followUpDone; err != nil {
		t.Fatal(err)
	}

	expectedTypes := []agent.EventType{
		agent.EventTypeAssistantMessage,
		agent.EventTypeRunCompleted,
		agent.EventTypeRunCheckpoint,
		agent.EventTypeCommandApplied,
	}
	for i := range reconnectedFrames {
		expectedID := lastID + int64(i+1)
		if reconnectedFrames[i].ID != expectedID {
			t.Fatalf("reconnect id mismatch at index %d: got=%d want=%d", i, reconnectedFrames[i].ID, expectedID)
		}
		if reconnectedFrames[i].ID <= lastID {
			t.Fatalf("reconnect replayed old id at index %d: got=%d last_seen=%d", i, reconnectedFrames[i].ID, lastID)
		}
		if reconnectedFrames[i].Event.Type != expectedTypes[i] {
			t.Fatalf("reconnect event type mismatch at index %d: got=%s want=%s", i, reconnectedFrames[i].Event.Type, expectedTypes[i])
		}
	}
}

func TestRunEventsInvalidAndExpiredCursor(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	defer server.Close()

	var started runStateResponse
	status := performJSON(t, server.Client(), http.MethodPost, server.URL+"/v1/runs/start", map[string]any{
		"user_prompt": "[loop] expire cursor",
		"max_steps":   1,
	}, &started)
	if status != http.StatusOK {
		t.Fatalf("start status mismatch: got=%d want=%d", status, http.StatusOK)
	}

	var invalidCursor errorResponse
	status = performJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/v1/runs/"+started.RunID+"/events?cursor=abc",
		nil,
		&invalidCursor,
	)
	if status != http.StatusConflict {
		t.Fatalf("invalid cursor status mismatch: got=%d want=%d", status, http.StatusConflict)
	}
	if invalidCursor.Error.Code != "conflict" {
		t.Fatalf("invalid cursor error code mismatch: got=%q want=%q", invalidCursor.Error.Code, "conflict")
	}

	for i := 0; i < 10; i++ {
		var continued runStateResponse
		status = performJSON(
			t,
			server.Client(),
			http.MethodPost,
			server.URL+"/v1/runs/"+started.RunID+"/continue",
			map[string]any{
				"max_steps": 1,
			},
			&continued,
		)
		if status != http.StatusOK {
			t.Fatalf("continue status mismatch at iteration %d: got=%d want=%d", i, status, http.StatusOK)
		}
		if continued.Status != string(agent.RunStatusMaxStepsExceeded) {
			t.Fatalf(
				"continue status mismatch at iteration %d: got=%s want=%s",
				i,
				continued.Status,
				agent.RunStatusMaxStepsExceeded,
			)
		}
	}

	var expiredCursor errorResponse
	status = performJSON(
		t,
		server.Client(),
		http.MethodGet,
		server.URL+"/v1/runs/"+started.RunID+"/events?cursor=1",
		nil,
		&expiredCursor,
	)
	if status != http.StatusConflict {
		t.Fatalf("expired cursor status mismatch: got=%d want=%d", status, http.StatusConflict)
	}
	if expiredCursor.Error.Code != "conflict" {
		t.Fatalf("expired cursor error code mismatch: got=%q want=%q", expiredCursor.Error.Code, "conflict")
	}
	if !strings.Contains(expiredCursor.Error.Message, "cursor expired") {
		t.Fatalf("expired cursor message mismatch: got=%q", expiredCursor.Error.Message)
	}
}

func readNDJSONFrames(
	t *testing.T,
	client *http.Client,
	url string,
	wantFrames int,
	timeout time.Duration,
) []streamLine {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new stream request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("stream request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			t.Fatalf("read non-200 stream response: %v", readErr)
		}
		t.Fatalf("stream status mismatch: got=%d body=%s", resp.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/x-ndjson") {
		t.Fatalf("stream content type mismatch: got=%q want contains %q", contentType, "application/x-ndjson")
	}

	frames := make([]streamLine, 0, wantFrames)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var frame streamLine
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatalf("decode stream payload: %v raw=%s", err, line)
		}
		if frame.ID <= 0 {
			t.Fatalf("stream frame missing valid id: got=%d", frame.ID)
		}

		frames = append(frames, frame)
		if len(frames) >= wantFrames {
			cancel()
			break
		}
	}

	if len(frames) < wantFrames {
		if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("stream scan error: %v", err)
		}
		t.Fatalf("insufficient stream frames: got=%d want=%d", len(frames), wantFrames)
	}

	return frames[:wantFrames]
}
