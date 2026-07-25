package httpx

// ShouldSkipObservability reports whether request instrumentation should be skipped.
func ShouldSkipObservability(path string) bool {
	return path == "/metrics"
}
