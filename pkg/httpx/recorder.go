package httpx

import "net/http"

// StatusRecorder wraps http.ResponseWriter and captures the HTTP status code.
type StatusRecorder struct {
	http.ResponseWriter
	Status int
}

func (r *StatusRecorder) WriteHeader(status int) {
	r.Status = status
	r.ResponseWriter.WriteHeader(status)
}
