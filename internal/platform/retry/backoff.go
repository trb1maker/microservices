package retry

import (
	"math/rand/v2"
	"time"
)

func BackoffWithJitter(attempt int, base, maxDelay time.Duration, jitterFraction float64) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := base
	for i := 1; i < attempt; i++ {
		if delay > maxDelay/2 {
			delay = maxDelay
			break
		}
		delay *= 2
	}
	if delay > maxDelay {
		delay = maxDelay
	}
	if jitterFraction <= 0 {
		return delay
	}
	jitter := time.Duration(float64(delay) * jitterFraction)
	if jitter <= 0 {
		return delay
	}
	shift := rand.Int64N(int64(jitter)*2+1) - int64(jitter) //nolint:gosec // jitter does not require crypto strength
	result := delay + time.Duration(shift)
	if result < 0 {
		return 0
	}
	return result
}
