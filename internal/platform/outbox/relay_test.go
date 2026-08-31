package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type stubStore struct {
	messages    []Message
	rescheduled []time.Duration
}

func (s *stubStore) ClaimDue(context.Context, int, time.Duration) ([]Message, error) {
	return s.messages, nil
}

func (s *stubStore) MarkPublished(context.Context, []int64) error {
	return nil
}

func (s *stubStore) Reschedule(_ context.Context, _ int64, nextAttempt time.Time, _ string) error {
	s.rescheduled = append(s.rescheduled, time.Until(nextAttempt))
	return nil
}

var errNATSDown = errors.New("nats down")

type failingPublisher struct{}

func (failingPublisher) Publish(context.Context, Message) error {
	return errNATSDown
}

func TestRelay_reschedulesWhenPublishFails(t *testing.T) {
	t.Parallel()

	store := &stubStore{messages: []Message{{ID: 1, Attempts: 1}}}
	relay := NewRelay(store, failingPublisher{}, RelayConfig{
		BaseBackoff: time.Second,
		MaxBackoff:  time.Minute,
	})

	require.NoError(t, relay.processBatch(context.Background()))
	require.Len(t, store.rescheduled, 1)
	require.InDelta(t, time.Second.Seconds(), store.rescheduled[0].Seconds(), 0.2)
}
