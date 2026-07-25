package sse

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trb1maker/microservices/scripts/ui/internal/orderwatch"
)

func TestBridgeStreamsUpdatesAndCloses(t *testing.T) {
	bridge := NewBridge(
		func(_ context.Context, orderID string) (<-chan orderwatch.StatusUpdate, error) {
			ch := make(chan orderwatch.StatusUpdate, 2)
			ch <- orderwatch.StatusUpdate{OrderID: orderID, Status: "RESERVED", Timestamp: "2026-07-19T00:00:00Z"}
			ch <- orderwatch.StatusUpdate{OrderID: orderID, Status: "CONFIRMED", Timestamp: "2026-07-19T00:00:01Z"}
			close(ch)
			return ch, nil
		},
		func(update orderwatch.StatusUpdate) (string, error) {
			return `<p>` + update.Status + `</p>`, nil
		},
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/orders/o1/events", nil)
	bridge.ServeHTTP(rec, req, "o1")

	body, err := io.ReadAll(rec.Result().Body)
	require.NoError(t, err)
	output := string(body)
	require.Contains(t, output, "event: order_status")
	require.Contains(t, output, "data: <p>RESERVED</p>")
	require.Contains(t, output, "data: <p>CONFIRMED</p>")
	require.Contains(t, output, "event: close")
}

func TestBridgeClosesWhenContextCancelled(t *testing.T) {
	bridge := NewBridge(
		func(ctx context.Context, _ string) (<-chan orderwatch.StatusUpdate, error) {
			ch := make(chan orderwatch.StatusUpdate)
			go func() {
				<-ctx.Done()
				close(ch)
			}()
			return ch, nil
		},
		func(update orderwatch.StatusUpdate) (string, error) {
			return update.Status, nil
		},
	)

	rec := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/orders/o1/events", nil).WithContext(ctx)

	done := make(chan struct{})
	go func() {
		bridge.ServeHTTP(rec, req, "o1")
		close(done)
	}()

	cancel()
	<-done
	require.Equal(t, http.StatusOK, rec.Code)
}
