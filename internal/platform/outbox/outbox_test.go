package outbox_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/trb1maker/microservices/internal/platform/outbox"
)

func TestBackoff_growsUntilMax(t *testing.T) {
	t.Parallel()

	base := time.Second
	maxDelay := 8 * time.Second

	assert.Equal(t, time.Second, outbox.Backoff(1, base, maxDelay))
	assert.Equal(t, 2*time.Second, outbox.Backoff(2, base, maxDelay))
	assert.Equal(t, 4*time.Second, outbox.Backoff(3, base, maxDelay))
	assert.Equal(t, 8*time.Second, outbox.Backoff(4, base, maxDelay))
	assert.Equal(t, 8*time.Second, outbox.Backoff(10, base, maxDelay))
}
