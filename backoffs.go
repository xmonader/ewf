package ewf

import (
	"time"

	"github.com/cenkalti/backoff/v4"
)

// ConstantBackoff returns a BackOffConfig for constant delays.
func ConstantBackoff(delay time.Duration) BackOffConfig {
	return BackOffConfig{
		Type:     "constant",
		Interval: delay,
	}
}

// ExponentialBackoff returns a BackOffConfig for exponential delays.
// initialInterval: first delay; maxInterval: cap for delay; multiplier: growth factor (e.g. 2.0).
func ExponentialBackoff(initialInterval, maxInterval time.Duration, multiplier float64) BackOffConfig {
	return BackOffConfig{
		Type:        "exponential",
		Interval:    initialInterval,
		MaxInterval: maxInterval,
		Multiplier:  multiplier,
	}
}

// ZeroBackoff returns a backoff.BackOff that always returns zero delay (for immediate retries).
func ZeroBackoff() backoff.BackOff {
	return backoff.NewConstantBackOff(0)
}
