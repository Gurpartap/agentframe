package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestExecuteEventsReconnectFromCursor(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()

	var (
		mu      sync.Mutex
		cursors []string
		hits    int
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/runs/run-1/events" {
			http.NotFound(w, r)
			return
		}

		mu.Lock()
		cursors = append(cursors, r.URL.Query().Get("cursor"))
		hits++
		currentHit := hits
		mu.Unlock()

		w.Header().Set("Content-Type", "application/x-ndjson")
		if currentHit == 1 {
			_, _ = io.WriteString(w, `{"id":1,"event":{"run_id":"run-1","step":0,"type":"run_started"}}`+"\n")
			_, _ = io.WriteString(w, `{"id":2,"event":{"run_id":"run-1","step":1,"type":"assistant_message"}}`+"\n")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}

		_, _ = io.WriteString(w, `{"id":3,"event":{"run_id":"run-1","step":2,"type":"run_checkpoint"}}`+"\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		if currentHit == 2 {
			time.AfterFunc(30*time.Millisecond, cancel)
		}
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Execute(
		ctx,
		[]string{"--base-url", server.URL, "events", "run-1", "--cursor", "0"},
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("execute events: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(cursors) < 2 {
		t.Fatalf("expected reconnect request, got cursors=%v", cursors)
	}
	if cursors[0] != "0" {
		t.Fatalf("first cursor mismatch: got=%q want=%q", cursors[0], "0")
	}
	if cursors[1] != "2" {
		t.Fatalf("second cursor mismatch: got=%q want=%q", cursors[1], "2")
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 output lines, got=%d output=%q", len(lines), stdout.String())
	}
	if !strings.Contains(lines[0], "id=1") || !strings.Contains(lines[1], "id=2") || !strings.Contains(lines[2], "id=3") {
		t.Fatalf("unexpected event order output: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr output: %q", stderr.String())
	}
}

func TestExecuteEventsJSONPassthrough(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const payload = `{"id":4,"event":{"run_id":"run-2","step":1,"type":"run_started"}}` + "\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/runs/run-2/events" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, payload)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.AfterFunc(30*time.Millisecond, cancel)
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Execute(
		ctx,
		[]string{"--base-url", server.URL, "--json", "events", "run-2"},
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("execute events json: %v", err)
	}
	if stdout.String() != payload {
		t.Fatalf("json passthrough mismatch: got=%q want=%q", stdout.String(), payload)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr output: %q", stderr.String())
	}
}

func TestExecuteEventsInvalidCursorError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/runs/run-3/events" {
			http.NotFound(w, r)
			return
		}
		cursor := r.URL.Query().Get("cursor")
		if _, err := strconv.ParseInt(cursor, 10, 64); err != nil {
			http.Error(w, "bad cursor", http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "conflict",
				"message": "cursor expired",
			},
		})
	}))
	defer server.Close()

	err := Execute(
		context.Background(),
		[]string{"--base-url", server.URL, "events", "run-3", "--cursor", "1"},
		io.Discard,
		io.Discard,
	)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "code=conflict") {
		t.Fatalf("expected conflict code in error, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "cursor expired") {
		t.Fatalf("expected cursor message in error, got %q", err.Error())
	}
}

func TestExecuteEventsCursorValidation(t *testing.T) {
	t.Parallel()

	err := Execute(
		context.Background(),
		[]string{"events", "run-4", "--cursor", "-1"},
		io.Discard,
		io.Discard,
	)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "cursor must be >= 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteEventsNonMonotonicIDFails(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/runs/run-5/events" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"id":1,"event":{"run_id":"run-5","step":0,"type":"run_started"}}`+"\n")
		_, _ = io.WriteString(w, `{"id":1,"event":{"run_id":"run-5","step":1,"type":"assistant_message"}}`+"\n")
	}))
	defer server.Close()

	err := Execute(
		ctx,
		[]string{"--base-url", server.URL, "events", "run-5"},
		io.Discard,
		io.Discard,
	)
	if err == nil {
		t.Fatalf("expected non-monotonic id error")
	}
	if !strings.Contains(err.Error(), "non-monotonic") {
		t.Fatalf("unexpected error: %v", err)
	}
}
