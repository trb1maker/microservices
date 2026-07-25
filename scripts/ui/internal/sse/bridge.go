package sse

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/trb1maker/microservices/scripts/ui/internal/orderwatch"
)

const (
	eventOrderStatus = "order_status"
	eventClose       = "close"
)

type Bridge struct {
	watcher func(ctx context.Context, orderID string) (<-chan orderwatch.StatusUpdate, error)
	render  func(update orderwatch.StatusUpdate) (string, error)
}

func NewBridge(
	watch func(ctx context.Context, orderID string) (<-chan orderwatch.StatusUpdate, error),
	render func(update orderwatch.StatusUpdate) (string, error),
) *Bridge {
	return &Bridge{watcher: watch, render: render}
}

func (b *Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request, orderID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	updates, err := b.watcher(r.Context(), orderID)
	if err != nil {
		writeEvent(w, flusher, eventOrderStatus, fmt.Sprintf(`<p class="text-red-600">stream error: %s</p>`, escapeHTML(err.Error())))
		writeEvent(w, flusher, eventClose, "")
		return
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case update, open := <-updates:
			if !open {
				writeEvent(w, flusher, eventClose, "")
				return
			}
			html, renderErr := b.render(update)
			if renderErr != nil {
				writeEvent(w, flusher, eventOrderStatus, `<p class="text-red-600">render error</p>`)
				writeEvent(w, flusher, eventClose, "")
				return
			}
			writeEvent(w, flusher, eventOrderStatus, html)
			if update.Status == "CONFIRMED" || update.Status == "CANCELLED" || update.Status == "ERROR" {
				writeEvent(w, flusher, eventClose, "")
				return
			}
		}
	}
}

func writeEvent(w http.ResponseWriter, flusher http.Flusher, event, data string) {
	fmt.Fprintf(w, "event: %s\n", event)
	for _, line := range strings.Split(data, "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
	flusher.Flush()
}

func escapeHTML(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(value)
}
