package ewf

import (
	"time"
	"github.com/cenkalti/backoff/v4"
)

// ConstantBackoff returns a backoff.BackOff that always waits for the specified delay.
func ConstantBackoff(delay time.Duration) backoff.BackOff {
	return backoff.NewConstantBackOff(delay)
}

// ExponentialBackoff returns a backoff.BackOff with exponential delays.
// initialInterval: first delay; maxInterval: cap for delay; multiplier: growth factor (e.g. 2.0).
func ExponentialBackoff(initialInterval, maxInterval time.Duration, multiplier float64) backoff.BackOff {
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = initialInterval
	bo.MaxInterval = maxInterval
	bo.Multiplier = multiplier
	return bo
}

// ZeroBackoff returns a backoff.BackOff that always returns zero delay (for immediate retries).
func ZeroBackoff() backoff.BackOff {
	return backoff.NewConstantBackOff(0)
}
