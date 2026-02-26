package app

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestLoggingMiddleware_LogsRequestAndRunID(t *testing.T) {
	t.Parallel()

	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, nil))

	handler := requestLoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	}))

	request := httptest.NewRequest(http.MethodPost, "/v1/runs/run-123/continue", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status mismatch: got=%d want=%d", recorder.Code, http.StatusCreated)
	}

	logLine := logBuffer.String()
	assertLogContains(t, logLine, "msg=\"http request\"")
	assertLogContains(t, logLine, "method=POST")
	assertLogContains(t, logLine, "path=/v1/runs/run-123/continue")
	assertLogContains(t, logLine, "status=201")
	assertLogContains(t, logLine, "bytes=2")
	assertLogContains(t, logLine, "run_id=run-123")
	assertLogContains(t, logLine, "duration_ms=")
}

func TestRequestLoggingMiddleware_StartRouteOmitsRunID(t *testing.T) {
	t.Parallel()

	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, nil))

	handler := requestLoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	request := httptest.NewRequest(http.MethodPost, "/v1/runs/start", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d", recorder.Code, http.StatusOK)
	}

	logLine := logBuffer.String()
	assertLogContains(t, logLine, "method=POST")
	assertLogContains(t, logLine, "path=/v1/runs/start")
	assertLogContains(t, logLine, "status=200")
	assertLogContains(t, logLine, "bytes=2")
	if strings.Contains(logLine, "run_id=") {
		t.Fatalf("expected no run_id attribute for start route log line: %s", logLine)
	}
}

func TestStatusCapturingWriter_FlushDelegates(t *testing.T) {
	t.Parallel()

	base := &flushWriter{header: make(http.Header)}
	writer := &statusCapturingWriter{ResponseWriter: base}

	flusher, ok := any(writer).(http.Flusher)
	if !ok {
		t.Fatalf("statusCapturingWriter must implement http.Flusher")
	}

	flusher.Flush()
	if base.flushCount != 1 {
		t.Fatalf("flush count mismatch: got=%d want=%d", base.flushCount, 1)
	}
}

func assertLogContains(t *testing.T, line, want string) {
	t.Helper()
	if !strings.Contains(line, want) {
		t.Fatalf("log line missing %q: %s", want, line)
	}
}

type flushWriter struct {
	header     http.Header
	statusCode int
	flushCount int
}

func (w *flushWriter) Header() http.Header {
	return w.header
}

func (w *flushWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

func (w *flushWriter) Write(p []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	return len(p), nil
}

func (w *flushWriter) Flush() {
	w.flushCount++
}
