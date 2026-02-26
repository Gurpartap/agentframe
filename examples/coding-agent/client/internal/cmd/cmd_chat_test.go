package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestExecuteChatSlashAndFreeTextFlow(t *testing.T) {
	t.Parallel()

	type runStateResponse struct {
		RunID   string `json:"run_id"`
		Status  string `json:"status"`
		Step    int    `json:"step"`
		Version int64  `json:"version"`
		Output  string `json:"output,omitempty"`
	}

	var (
		mu                   sync.Mutex
		startCalls           int
		statusCalls          int
		steerCalls           int
		freeTextFollowUpSeen string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeRunState := func(payload runStateResponse) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
		}

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs/start":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode start request: %v", err)
			}
			if request["user_prompt"] != "bootstrap run" {
				t.Fatalf("start prompt mismatch: %#v", request["user_prompt"])
			}
			mu.Lock()
			startCalls++
			mu.Unlock()
			writeRunState(runStateResponse{RunID: "run-1", Status: "max_steps_exceeded", Step: 1, Version: 2})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/run-1/events":
			cursor := r.URL.Query().Get("cursor")
			w.Header().Set("Content-Type", "application/x-ndjson")
			if cursor == "0" {
				_, _ = io.WriteString(w, `{"id":1,"event":{"run_id":"run-1","step":0,"type":"run_started","description":"run started"}}`+"\n")
				_, _ = io.WriteString(w, `{"id":2,"event":{"run_id":"run-1","step":1,"type":"run_checkpoint","description":"checkpoint"}}`+"\n")
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
				return
			}
			<-r.Context().Done()
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs/run-1/steer":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode steer request: %v", err)
			}
			if request["instruction"] != "shift plan" {
				t.Fatalf("steer instruction mismatch: %#v", request["instruction"])
			}
			mu.Lock()
			steerCalls++
			mu.Unlock()
			writeRunState(runStateResponse{RunID: "run-1", Status: "running", Step: 2, Version: 3})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs/run-1/follow-up":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode follow-up request: %v", err)
			}
			mu.Lock()
			freeTextFollowUpSeen = request["prompt"].(string)
			mu.Unlock()
			writeRunState(runStateResponse{RunID: "run-1", Status: "completed", Step: 3, Version: 4, Output: "done"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/run-1":
			mu.Lock()
			statusCalls++
			mu.Unlock()
			writeRunState(runStateResponse{RunID: "run-1", Status: "completed", Step: 3, Version: 4, Output: "done"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	inputReader := delayedInput(
		[]string{
			"/start bootstrap run\n",
			"/steer shift plan\n",
			"free text followup\n",
			"/status\n",
			"/quit\n",
		},
		40*time.Millisecond,
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := executeWithInput(
		ctx,
		[]string{"--base-url", server.URL, "--token", "chat-token", "chat"},
		inputReader,
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("execute chat: %v stderr=%s", err, stderr.String())
	}

	mu.Lock()
	defer mu.Unlock()
	if startCalls != 1 {
		t.Fatalf("start call mismatch: got=%d want=%d", startCalls, 1)
	}
	if steerCalls != 1 {
		t.Fatalf("steer call mismatch: got=%d want=%d", steerCalls, 1)
	}
	if statusCalls != 1 {
		t.Fatalf("status call mismatch: got=%d want=%d", statusCalls, 1)
	}
	if freeTextFollowUpSeen != "free text followup" {
		t.Fatalf("free text follow-up prompt mismatch: got=%q want=%q", freeTextFollowUpSeen, "free text followup")
	}

	output := stdout.String()
	if !strings.Contains(output, "event id=1 run_id=run-1") {
		t.Fatalf("missing stream event output: %q", output)
	}
	if !strings.Contains(output, "run_id: run-1") {
		t.Fatalf("missing run state output: %q", output)
	}
	if !strings.Contains(output, "\r\033[2K") {
		t.Fatalf("missing prompt clear/redraw sequence: %q", output)
	}
}

func TestExecuteChatSuspendedResolutionFlow(t *testing.T) {
	t.Parallel()

	type runStateResponse struct {
		RunID              string `json:"run_id"`
		Status             string `json:"status"`
		Step               int    `json:"step"`
		Version            int64  `json:"version"`
		Output             string `json:"output,omitempty"`
		PendingRequirement *struct {
			ID     string `json:"id"`
			Kind   string `json:"kind"`
			Prompt string `json:"prompt"`
		} `json:"pending_requirement,omitempty"`
	}

	var (
		mu                  sync.Mutex
		continueCallCount   int
		capturedRequirement map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeRunState := func(payload runStateResponse) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
		}

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs/start":
			writeRunState(runStateResponse{
				RunID:   "run-suspended",
				Status:  "suspended",
				Step:    1,
				Version: 2,
				PendingRequirement: &struct {
					ID     string `json:"id"`
					Kind   string `json:"kind"`
					Prompt string `json:"prompt"`
				}{
					ID:     "req-approval",
					Kind:   "approval",
					Prompt: "approve deterministic continuation",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/run-suspended":
			writeRunState(runStateResponse{
				RunID:   "run-suspended",
				Status:  "suspended",
				Step:    1,
				Version: 2,
				PendingRequirement: &struct {
					ID     string `json:"id"`
					Kind   string `json:"kind"`
					Prompt string `json:"prompt"`
				}{
					ID:     "req-approval",
					Kind:   "approval",
					Prompt: "approve deterministic continuation",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/run-suspended/events":
			w.Header().Set("Content-Type", "application/x-ndjson")
			<-r.Context().Done()
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs/run-suspended/continue":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode continue request: %v", err)
			}
			mu.Lock()
			continueCallCount++
			if resolution, ok := request["resolution"].(map[string]any); ok {
				capturedRequirement = resolution
			}
			mu.Unlock()
			writeRunState(runStateResponse{
				RunID:   "run-suspended",
				Status:  "completed",
				Step:    2,
				Version: 3,
				Output:  "done",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	inputReader := delayedInput(
		[]string{
			"/start [suspend] approval gate\n",
			"/continue\n",
			"\n",         // requirement_id default req-approval
			"\n",         // kind default approval
			"approved\n", // outcome
			"\n",         // optional value
			"/quit\n",
		},
		35*time.Millisecond,
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := executeWithInput(
		ctx,
		[]string{"--base-url", server.URL, "--token", "chat-token", "chat"},
		inputReader,
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("execute chat suspended flow: %v stderr=%s", err, stderr.String())
	}

	mu.Lock()
	defer mu.Unlock()
	if continueCallCount != 1 {
		t.Fatalf("continue call count mismatch: got=%d want=%d", continueCallCount, 1)
	}
	if capturedRequirement == nil {
		t.Fatalf("expected continue resolution payload")
	}
	if capturedRequirement["requirement_id"] != "req-approval" {
		t.Fatalf("requirement_id mismatch: %#v", capturedRequirement["requirement_id"])
	}
	if capturedRequirement["kind"] != "approval" {
		t.Fatalf("kind mismatch: %#v", capturedRequirement["kind"])
	}
	if capturedRequirement["outcome"] != "approved" {
		t.Fatalf("outcome mismatch: %#v", capturedRequirement["outcome"])
	}

	output := stdout.String()
	if !strings.Contains(output, "run is suspended and requires a resolution payload") {
		t.Fatalf("missing suspension guidance output: %q", output)
	}
	if !strings.Contains(output, "requirement_id [req-approval]:") {
		t.Fatalf("missing requirement prompt output: %q", output)
	}
}

func delayedInput(lines []string, delay time.Duration) io.Reader {
	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		for _, line := range lines {
			_, _ = writer.Write([]byte(line))
			time.Sleep(delay)
		}
	}()
	return reader
}
