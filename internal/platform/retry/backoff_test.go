package retry_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/trb1maker/microservices/internal/platform/retry"
)

func TestBackoffWithJitter_growsUntilMax(t *testing.T) {
	t.Parallel()

	base := 2 * time.Second
	maxDelay := 30 * time.Second

	for attempt, wantMin := range map[int]time.Duration{
		1: 2 * time.Second,
		2: 4 * time.Second,
		3: 8 * time.Second,
		5: 30 * time.Second,
	} {
		got := retry.BackoffWithJitter(attempt, base, maxDelay, 0)
		assert.Equal(t, wantMin, got)
	}
}

func TestBackoffWithJitter_jitterWithinBounds(t *testing.T) {
	t.Parallel()

	base := time.Second
	maxDelay := 10 * time.Second
	for range 50 {
		got := retry.BackoffWithJitter(2, base, maxDelay, 0.2)
		assert.GreaterOrEqual(t, got, 1600*time.Millisecond)
		assert.LessOrEqual(t, got, 2400*time.Millisecond)
	}
}
