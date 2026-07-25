package health

import (
	"encoding/json"
	"net/http"
)

type livenessResponse struct {
	Status string `json:"status"`
}

type readinessResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

func LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, livenessResponse{Status: "ok"})
	}
}

func ReadinessHandler(checker *Checker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if checker == nil {
			writeJSON(w, http.StatusOK, readinessResponse{Status: "ready", Checks: map[string]string{}})
			return
		}

		ready, checks := checker.Check(r.Context())
		status := http.StatusOK
		statusText := "ready"
		if !ready {
			status = http.StatusServiceUnavailable
			statusText = "not_ready"
		}

		writeJSON(w, status, readinessResponse{Status: statusText, Checks: checks})
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
