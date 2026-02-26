package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/runstream"
)

const streamPollInterval = 25 * time.Millisecond

func (h *handlers) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	if !h.ensureRuntime(w) {
		return
	}

	runID, err := pathRunID(r)
	if err != nil {
		writeMappedError(w, err)
		return
	}

	if _, err := h.runtime.RunStore.Load(r.Context(), runID); err != nil {
		writeMappedError(w, err)
		return
	}

	cursor, err := parseCursor(r)
	if err != nil {
		writeMappedError(w, err)
		return
	}

	buffered, err := h.runtime.StreamBroker.EventsAfter(runID, cursor)
	if err != nil {
		writeMappedError(w, err)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errorCodeRuntime, "streaming is unsupported by response writer")
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	encoder := json.NewEncoder(w)

	for _, streamEvent := range buffered {
		if err := writeNDJSONEvent(encoder, flusher, streamEvent); err != nil {
			return
		}
		cursor = streamEvent.ID
	}

	ticker := time.NewTicker(streamPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			next, err := h.runtime.StreamBroker.EventsAfter(runID, cursor)
			if err != nil {
				return
			}
			for _, streamEvent := range next {
				if err := writeNDJSONEvent(encoder, flusher, streamEvent); err != nil {
					return
				}
				cursor = streamEvent.ID
			}
		}
	}
}

func parseCursor(r *http.Request) (int64, error) {
	raw := r.URL.Query().Get("cursor")
	if raw == "" {
		return 0, nil
	}

	cursor, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || cursor < 0 {
		return 0, fmt.Errorf("%w: cursor must be a non-negative integer", runstream.ErrCursorInvalid)
	}
	return cursor, nil
}

func writeNDJSONEvent(encoder *json.Encoder, flusher http.Flusher, streamEvent runstream.StreamEvent) error {
	if err := encoder.Encode(streamEvent); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
