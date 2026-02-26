package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Gurpartap/agentframe/examples/coding-agent/client/internal/api"
)

func TestExecuteCommandCoverage(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name        string
		commandArgs []string
		method      string
		path        string
		wantAuth    bool
		validate    func(t *testing.T, body []byte)
	}

	runStateJSON := `{"run_id":"run-000001","status":"completed","step":2,"version":3}` + "\n"

	tests := []testCase{
		{
			name:        "health",
			commandArgs: []string{"health"},
			method:      http.MethodGet,
			path:        "/healthz",
			wantAuth:    false,
			validate: func(t *testing.T, body []byte) {
				t.Helper()
				if len(body) != 0 {
					t.Fatalf("health body should be empty, got %q", string(body))
				}
			},
		},
		{
			name:        "start",
			commandArgs: []string{"start", "--user-prompt", "hello", "--max-steps", "2"},
			method:      http.MethodPost,
			path:        "/v1/runs/start",
			wantAuth:    true,
			validate: func(t *testing.T, body []byte) {
				t.Helper()
				var decoded map[string]any
				if err := json.Unmarshal(body, &decoded); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				if decoded["user_prompt"] != "hello" {
					t.Fatalf("start user_prompt mismatch: %#v", decoded["user_prompt"])
				}
				if decoded["max_steps"] != float64(2) {
					t.Fatalf("start max_steps mismatch: %#v", decoded["max_steps"])
				}
			},
		},
		{
			name:        "get",
			commandArgs: []string{"get", "run-000001"},
			method:      http.MethodGet,
			path:        "/v1/runs/run-000001",
			wantAuth:    false,
			validate: func(t *testing.T, body []byte) {
				t.Helper()
				if len(body) != 0 {
					t.Fatalf("get body should be empty, got %q", string(body))
				}
			},
		},
		{
			name:        "continue",
			commandArgs: []string{"continue", "run-000001", "--max-steps", "3", "--requirement-id", "req-1", "--kind", "approval", "--outcome", "approved", "--value", "ok"},
			method:      http.MethodPost,
			path:        "/v1/runs/run-000001/continue",
			wantAuth:    true,
			validate: func(t *testing.T, body []byte) {
				t.Helper()
				var decoded map[string]any
				if err := json.Unmarshal(body, &decoded); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				if decoded["max_steps"] != float64(3) {
					t.Fatalf("continue max_steps mismatch: %#v", decoded["max_steps"])
				}
				resolution, ok := decoded["resolution"].(map[string]any)
				if !ok {
					t.Fatalf("continue resolution missing: %#v", decoded["resolution"])
				}
				if resolution["requirement_id"] != "req-1" {
					t.Fatalf("resolution requirement_id mismatch: %#v", resolution["requirement_id"])
				}
			},
		},
		{
			name:        "steer",
			commandArgs: []string{"steer", "run-000001", "--instruction", "shift strategy"},
			method:      http.MethodPost,
			path:        "/v1/runs/run-000001/steer",
			wantAuth:    true,
			validate: func(t *testing.T, body []byte) {
				t.Helper()
				var decoded map[string]any
				if err := json.Unmarshal(body, &decoded); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				if decoded["instruction"] != "shift strategy" {
					t.Fatalf("steer instruction mismatch: %#v", decoded["instruction"])
				}
			},
		},
		{
			name:        "follow-up",
			commandArgs: []string{"follow-up", "run-000001", "--prompt", "finish", "--max-steps", "4"},
			method:      http.MethodPost,
			path:        "/v1/runs/run-000001/follow-up",
			wantAuth:    true,
			validate: func(t *testing.T, body []byte) {
				t.Helper()
				var decoded map[string]any
				if err := json.Unmarshal(body, &decoded); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				if decoded["prompt"] != "finish" {
					t.Fatalf("follow-up prompt mismatch: %#v", decoded["prompt"])
				}
			},
		},
		{
			name:        "cancel",
			commandArgs: []string{"cancel", "run-000001"},
			method:      http.MethodPost,
			path:        "/v1/runs/run-000001/cancel",
			wantAuth:    true,
			validate: func(t *testing.T, body []byte) {
				t.Helper()
				if len(body) != 0 {
					t.Fatalf("cancel body should be empty, got %q", string(body))
				}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tc.method {
					t.Fatalf("method mismatch: got=%s want=%s", r.Method, tc.method)
				}
				if r.URL.Path != tc.path {
					t.Fatalf("path mismatch: got=%s want=%s", r.URL.Path, tc.path)
				}

				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("read body: %v", err)
				}
				tc.validate(t, body)

				gotAuth := r.Header.Get("Authorization")
				if tc.wantAuth {
					if gotAuth != "Bearer test-token" {
						t.Fatalf("auth mismatch: got=%q want=%q", gotAuth, "Bearer test-token")
					}
				} else if gotAuth != "" {
					t.Fatalf("expected no auth header, got %q", gotAuth)
				}

				if tc.name == "health" {
					_, _ = io.WriteString(w, "ok")
					return
				}

				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, runStateJSON)
			}))
			defer server.Close()

			args := append([]string{"--base-url", server.URL, "--token", "test-token"}, tc.commandArgs...)

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := Execute(context.Background(), args, &stdout, &stderr)
			if err != nil {
				t.Fatalf("execute: %v stderr=%s", err, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("unexpected stderr output: %s", stderr.String())
			}

			if tc.name == "health" {
				if stdout.String() != "ok\n" {
					t.Fatalf("health stdout mismatch: got=%q want=%q", stdout.String(), "ok\n")
				}
				return
			}

			output := stdout.String()
			if !strings.Contains(output, "run_id: run-000001") {
				t.Fatalf("missing run_id in stdout: %q", output)
			}
			if !strings.Contains(output, "status: completed") {
				t.Fatalf("missing status in stdout: %q", output)
			}
		})
	}
}

func TestExecuteJSONModeWritesRawPayload(t *testing.T) {
	t.Parallel()

	rawPayload := []byte("{\"run_id\":\"run-raw\",\"status\":\"completed\",\"step\":1,\"version\":2}\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/runs/run-raw" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(rawPayload)
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Execute(
		context.Background(),
		[]string{"--base-url", server.URL, "--json", "get", "run-raw"},
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("execute: %v stderr=%s", err, stderr.String())
	}
	if stdout.String() != string(rawPayload) {
		t.Fatalf("raw stdout mismatch: got=%q want=%q", stdout.String(), string(rawPayload))
	}
}

func TestExecuteReturnsAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/runs/start" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"code":"unauthorized","message":"invalid bearer token"}}`)
	}))
	defer server.Close()

	err := Execute(
		context.Background(),
		[]string{"--base-url", server.URL, "start", "--user-prompt", "hello"},
		io.Discard,
		io.Discard,
	)
	if err == nil {
		t.Fatalf("expected error")
	}

	var requestError *api.RequestError
	if !errors.As(err, &requestError) {
		t.Fatalf("expected RequestError, got %T (%v)", err, err)
	}
	if requestError.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status mismatch: got=%d want=%d", requestError.StatusCode, http.StatusUnauthorized)
	}
	if requestError.Code != "unauthorized" {
		t.Fatalf("code mismatch: got=%q want=%q", requestError.Code, "unauthorized")
	}
}

func TestExecuteContinueRejectsUnsupportedResolutionKind(t *testing.T) {
	t.Parallel()

	err := Execute(
		context.Background(),
		[]string{
			"continue",
			"run-000001",
			"--requirement-id", "req-1",
			"--kind", "unknown_kind",
			"--outcome", "approved",
		},
		io.Discard,
		io.Discard,
	)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !strings.Contains(err.Error(), "continue resolution kind") {
		t.Fatalf("expected kind validation context, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "unsupported requirement kind") {
		t.Fatalf("expected unsupported kind message, got %q", err.Error())
	}
}

func TestExecuteContinueRejectsUnsupportedResolutionOutcome(t *testing.T) {
	t.Parallel()

	err := Execute(
		context.Background(),
		[]string{
			"continue",
			"run-000001",
			"--requirement-id", "req-1",
			"--kind", "approval",
			"--outcome", "unknown_outcome",
		},
		io.Discard,
		io.Discard,
	)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !strings.Contains(err.Error(), "continue resolution outcome") {
		t.Fatalf("expected outcome validation context, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "unsupported resolution outcome") {
		t.Fatalf("expected unsupported outcome message, got %q", err.Error())
	}
}
