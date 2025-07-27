package ewf

import (
	"time"
)

// ConstantBackoff returns a BackOff that always waits for the specified delay.
func ConstantBackoff(delay time.Duration) BackOff {
	return NewConstantBackOff(delay)
}

// ExponentialBackoff returns a BackOff with exponential delays.
// initialInterval: first delay; maxInterval: cap for delay; multiplier: growth factor (e.g. 2.0).
func ExponentialBackoff(initialInterval, maxInterval time.Duration, multiplier float64) BackOff {
	return NewExponentialBackOff(initialInterval, maxInterval, multiplier)
}

// ExponentialBackoffWithJitter returns a BackOff with exponential delays and jitter.
// initialInterval: first delay; maxInterval: cap for delay; multiplier: growth factor (e.g. 2.0).
// jitter: randomization factor between 0 and 1 that determines the amount of randomness.
func ExponentialBackoffWithJitter(initialInterval, maxInterval time.Duration, multiplier float64, jitter float64) BackOff {
	return NewExponentialBackOffWithJitter(initialInterval, maxInterval, multiplier, jitter)
}

// ZeroBackoff returns a BackOff that always returns zero delay (for immediate retries).
func ZeroBackoff() BackOff {
	return NewZeroBackOff()
}
