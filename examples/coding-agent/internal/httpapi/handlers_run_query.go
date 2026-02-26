package httpapi

import "net/http"

func (h *handlers) handleRunQuery(w http.ResponseWriter, r *http.Request) {
	if !h.ensureRuntime(w) {
		return
	}

	runID, err := pathRunID(r)
	if err != nil {
		writeMappedError(w, err)
		return
	}

	state, err := h.runtime.RunStore.Load(r.Context(), runID)
	if err != nil {
		writeMappedError(w, err)
		return
	}

	writeRunState(w, http.StatusOK, state)
}
