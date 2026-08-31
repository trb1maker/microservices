package breaker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trb1maker/microservices/internal/platform/breaker"
)

var errTest = errors.New("transport down")

func TestBreaker_opensAfterConsecutiveFailures(t *testing.T) {
	t.Parallel()

	cb := breaker.New(breaker.Config{
		Name:        "test",
		MaxFailures: 3,
		OpenTimeout: time.Second,
	})

	fail := func(context.Context) error { return errTest }
	for range 3 {
		require.Error(t, cb.Execute(context.Background(), fail))
	}

	start := time.Now()
	err := cb.Execute(context.Background(), fail)
	elapsed := time.Since(start)

	require.Error(t, err)
	require.ErrorIs(t, err, breaker.ErrOpen)
	assert.Less(t, elapsed, 50*time.Millisecond)
}

func TestBreaker_halfOpenAllowsProbeAfterTimeout(t *testing.T) {
	t.Parallel()

	cb := breaker.New(breaker.Config{
		Name:        "half-open",
		MaxFailures: 1,
		OpenTimeout: 50 * time.Millisecond,
	})

	require.Error(t, cb.Execute(context.Background(), func(context.Context) error { return errTest }))

	require.Eventually(t, func() bool {
		return cb.Execute(context.Background(), func(context.Context) error { return nil }) == nil
	}, time.Second, 10*time.Millisecond)
}
