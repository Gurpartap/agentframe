package app

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

func requestLoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			logWriter := &statusCapturingWriter{ResponseWriter: w}

			next.ServeHTTP(logWriter, r)

			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", logWriter.statusCode()),
				slog.Int("bytes", logWriter.bytes),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			}
			if runID := runIDFromPath(r.URL.Path); runID != "" {
				attrs = append(attrs, slog.String("run_id", runID))
			}

			logger.LogAttrs(r.Context(), slog.LevelInfo, "http request", attrs...)
		})
	}
}

type statusCapturingWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusCapturingWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusCapturingWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

func (w *statusCapturingWriter) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *statusCapturingWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func runIDFromPath(path string) string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return ""
	}

	parts := strings.Split(trimmed, "/")
	if len(parts) < 3 || parts[0] != "v1" || parts[1] != "runs" {
		return ""
	}
	if parts[2] == "" || parts[2] == "start" {
		return ""
	}
	return parts[2]
}
