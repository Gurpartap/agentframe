package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/config"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/policyauth"
)

func TestShutdownClosesActiveEventStream(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.HTTPAddr = pickLocalAddr(t)
	cfg.ModelMode = config.ModelModeMock
	cfg.ToolMode = config.ToolModeMock
	cfg.ShutdownTimeout = 2 * time.Second

	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, nil))
	application, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- application.Start()
	}()

	baseURL := "http://" + cfg.HTTPAddr
	waitForHealthz(t, baseURL)

	client := &http.Client{}
	runID := createRunForStreamTest(t, client, baseURL)

	streamReq, err := http.NewRequest(http.MethodGet, baseURL+"/v1/runs/"+runID+"/events?cursor=0", nil)
	if err != nil {
		t.Fatalf("new stream request: %v", err)
	}
	streamResp, err := client.Do(streamReq)
	if err != nil {
		t.Fatalf("open stream request: %v", err)
	}
	defer streamResp.Body.Close()

	if streamResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(streamResp.Body)
		t.Fatalf("stream status mismatch: got=%d want=%d body=%s", streamResp.StatusCode, http.StatusOK, string(body))
	}

	scanner := bufio.NewScanner(streamResp.Body)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			t.Fatalf("read initial stream line: %v", err)
		}
		t.Fatalf("expected initial stream line")
	}
	if strings.TrimSpace(scanner.Text()) == "" {
		t.Fatalf("expected non-empty initial stream line")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := application.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown app: %v", err)
	}

	select {
	case err := <-serverErrCh:
		if err != nil {
			t.Fatalf("server exited with error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for server exit")
	}

	if !strings.Contains(logBuffer.String(), "graceful shutdown timed out; forcing connection close") {
		t.Fatalf("expected forced-close shutdown warning log, got: %s", logBuffer.String())
	}
}

func TestShutdownWithoutActiveStreamIsGraceful(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.HTTPAddr = pickLocalAddr(t)
	cfg.ModelMode = config.ModelModeMock
	cfg.ToolMode = config.ToolModeMock
	cfg.ShutdownTimeout = 2 * time.Second

	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, nil))
	application, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- application.Start()
	}()

	baseURL := "http://" + cfg.HTTPAddr
	waitForHealthz(t, baseURL)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := application.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown app: %v", err)
	}

	select {
	case err := <-serverErrCh:
		if err != nil {
			t.Fatalf("server exited with error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for server exit")
	}

	if strings.Contains(logBuffer.String(), "graceful shutdown timed out; forcing connection close") {
		t.Fatalf("expected graceful shutdown path without forced close warning, got: %s", logBuffer.String())
	}
}

func waitForHealthz(t *testing.T, baseURL string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for {
		req, err := http.NewRequest(http.MethodGet, baseURL+"/healthz", nil)
		if err != nil {
			t.Fatalf("new healthz request: %v", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}

		if time.Now().After(deadline) {
			t.Fatalf("healthz did not become ready before deadline")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func createRunForStreamTest(t *testing.T, client *http.Client, baseURL string) string {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"user_prompt": "[loop] keep stream open",
		"max_steps":   1,
	})
	if err != nil {
		t.Fatalf("marshal start payload: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/runs/start", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new start request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(policyauth.HeaderAuthorization, policyauth.BearerPrefix+policyauth.DefaultToken)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("start request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read start response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("start status mismatch: got=%d want=%d body=%s", resp.StatusCode, http.StatusOK, string(body))
	}

	var run struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(body, &run); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	if run.RunID == "" {
		t.Fatalf("start response missing run_id: %s", string(body))
	}
	return run.RunID
}

func pickLocalAddr(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for local addr: %v", err)
	}
	defer listener.Close()

	return listener.Addr().String()
}
