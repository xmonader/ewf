// Package ewf implements an Extensible Workflow Framework.
//
// This file contains the custom backoff implementation that supports:
// - Constant backoff: fixed delay between retries
// - Exponential backoff: delay increases exponentially with each retry
// - Exponential backoff with jitter: adds randomness to prevent thundering herd
// - Zero backoff: no delay between retries
// - Stop backoff: signals that no more retries should be attempted
//
// All backoff implementations are serializable to JSON, allowing workflows
// with retry policies to be persisted and resumed.
package ewf

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"
)

// BackOff interface defines the operations required for backoff implementations.
// This is the core interface that all backoff strategies must implement.
// It provides methods to calculate the next backoff duration and reset the backoff state.
type BackOff interface {
	// NextBackOff returns the duration to wait before retrying the operation.
	// If the implementation returns Stop, the operation should not be retried.
	NextBackOff() time.Duration

	// Reset resets the backoff to its initial state.
	// This is typically called before starting a new sequence of backoff operations.
	Reset()
}

// Stop is a special duration value that indicates that no more retries should be made.
const Stop time.Duration = -1

// BackOffContext wraps a BackOff with a context to support cancellation.
type BackOffContext interface {
	BackOff
	Context() context.Context
}

// Permanent wraps an error to indicate that it should not be retried.
type Permanent struct {
	Err error
}

// Error returns the wrapped error's message.
func (p *Permanent) Error() string {
	if p.Err == nil {
		return "permanent error"
	}
	return p.Err.Error()
}

// Unwrap returns the wrapped error.
func (p *Permanent) Unwrap() error {
	return p.Err
}

// IsPermanent returns true if the given error is a Permanent error.
func IsPermanent(err error) bool {
	var p *Permanent
	return err != nil && errors.As(err, &p)
}

// PermanentError wraps an error to indicate that it should not be retried.
func PermanentError(err error) *Permanent {
	return &Permanent{Err: err}
}

// ConstantBackOffImpl implements a constant backoff strategy.
type ConstantBackOffImpl struct {
	Interval time.Duration `json:"interval"` // Serializable field
}

// NextBackOff returns the constant interval.
func (b *ConstantBackOffImpl) NextBackOff() time.Duration {
	return b.Interval
}

// Reset is a no-op for constant backoff.
func (b *ConstantBackOffImpl) Reset() {}

// ExponentialBackOffImpl implements an exponential backoff strategy.
type ExponentialBackOffImpl struct {
	InitialInterval     time.Duration `json:"initial_interval"`
	CurrentInterval     time.Duration `json:"-"` // Not serialized
	Multiplier          float64       `json:"multiplier"`
	MaxInterval         time.Duration `json:"max_interval"`
	RandomizationFactor float64       `json:"randomization_factor"` // For jitter
	MaxElapsedTime      time.Duration `json:"max_elapsed_time"`
	Clock               Clock         `json:"-"` // Not serialized
	StartTime           time.Time     `json:"-"` // Not serialized
	stopped             bool          `json:"-"` // Not serialized
}

// Clock interface abstracts time operations for easier testing.
type Clock interface {
	Now() time.Time
}

// SystemClock implements the Clock interface using the system clock.
type SystemClock struct{}

// BackOffType identifies the type of backoff implementation
type BackOffType string

const (
	ConstantBackOffType    BackOffType = "constant"
	ExponentialBackOffType BackOffType = "exponential"
	ZeroBackOffType        BackOffType = "zero"
	StopBackOffType        BackOffType = "stop"
)

// BackOffData is used for serializing/deserializing BackOff implementations
type BackOffData struct {
	Type                BackOffType   `json:"type"`
	Interval            time.Duration `json:"interval,omitempty"`             // For ConstantBackOffImpl
	InitialInterval     time.Duration `json:"initial_interval,omitempty"`     // For ExponentialBackOffImpl
	CurrentInterval     time.Duration `json:"current_interval,omitempty"`     // For ExponentialBackOffImpl
	MaxInterval         time.Duration `json:"max_interval,omitempty"`         // For ExponentialBackOffImpl
	Multiplier          float64       `json:"multiplier,omitempty"`           // For ExponentialBackOffImpl
	RandomizationFactor float64       `json:"randomization_factor,omitempty"` // For ExponentialBackOffImpl with jitter
	MaxElapsedTime      time.Duration `json:"max_elapsed_time,omitempty"`     // For ExponentialBackOffImpl
}

// Now returns the current time.
func (c SystemClock) Now() time.Time {
	return time.Now()
}

// NextBackOff calculates the next backoff interval using the exponential formula.
func (b *ExponentialBackOffImpl) NextBackOff() time.Duration {
	// Check if we've exceeded max elapsed time
	if b.MaxElapsedTime != 0 && b.Clock.Now().Sub(b.StartTime) > b.MaxElapsedTime {
		return Stop
	}

	// First time, use initial interval
	if b.CurrentInterval == 0 {
		b.Reset()
	}

	// Calculate next interval
	next := b.CurrentInterval

	// Apply randomization factor (jitter) if set
	if b.RandomizationFactor > 0 {
		delta := b.RandomizationFactor * float64(next)
		minInterval := float64(next) - delta
		maxInterval := float64(next) + delta

		// Get a random value between min and max
		next = time.Duration(minInterval + (rand.Float64() * (maxInterval - minInterval + 1)))
	}

	// Prepare for the next call
	b.CurrentInterval = time.Duration(float64(b.CurrentInterval) * b.Multiplier)
	if b.CurrentInterval > b.MaxInterval {
		b.CurrentInterval = b.MaxInterval
	}

	return next
}

// Reset resets the backoff to its initial state.
func (b *ExponentialBackOffImpl) Reset() {
	b.CurrentInterval = b.InitialInterval
	b.StartTime = b.Clock.Now()
	b.stopped = false
}

// StopBackOffImpl implements a backoff that always returns Stop.
type StopBackOffImpl struct{}

// NextBackOff always returns Stop.
func (b *StopBackOffImpl) NextBackOff() time.Duration {
	return Stop
}

// Reset is a no-op for stop backoff.
func (b *StopBackOffImpl) Reset() {}

// ZeroBackOffImpl implements a backoff that always returns zero.
type ZeroBackOffImpl struct{}

// NextBackOff always returns zero.
func (b *ZeroBackOffImpl) NextBackOff() time.Duration {
	return 0
}

// Reset is a no-op for zero backoff.
func (b *ZeroBackOffImpl) Reset() {}

// withContextBackOff wraps a BackOff with a context.
type withContextBackOff struct {
	b   BackOff
	ctx context.Context
}

// NextBackOff returns Stop if context is done, otherwise returns the wrapped NextBackOff.
func (b *withContextBackOff) NextBackOff() time.Duration {
	if b.ctx.Err() != nil {
		return Stop
	}
	return b.b.NextBackOff()
}

// Reset resets the wrapped BackOff.
func (b *withContextBackOff) Reset() {
	b.b.Reset()
}

// Context returns the wrapped context.
func (b *withContextBackOff) Context() context.Context {
	return b.ctx
}

// WithContext returns a BackOffContext with the given context and backoff.
func WithContext(b BackOff, ctx context.Context) BackOffContext {
	if b == nil {
		return &withContextBackOff{
			b:   &StopBackOffImpl{},
			ctx: ctx,
		}
	}

	return &withContextBackOff{
		b:   b,
		ctx: ctx,
	}
}

// NewConstantBackOff creates a backoff policy that always returns the same backoff delay.
func NewConstantBackOff(interval time.Duration) BackOff {
	return &ConstantBackOffImpl{
		Interval: interval,
	}
}

// NewExponentialBackOff creates a new exponential backoff policy.
// The formula used is: currentInterval = initialInterval * multiplier^(n-1) with jitter.
func NewExponentialBackOff(initialInterval, maxInterval time.Duration, multiplier float64) BackOff {
	return &ExponentialBackOffImpl{
		InitialInterval:     initialInterval,
		MaxInterval:         maxInterval,
		Multiplier:          multiplier,
		RandomizationFactor: 0, // No jitter by default
		Clock:               &SystemClock{},
	}
}

// NewExponentialBackOffWithJitter creates a new exponential backoff policy with jitter.
func NewExponentialBackOffWithJitter(initialInterval, maxInterval time.Duration, multiplier float64, jitter float64) BackOff {
	return &ExponentialBackOffImpl{
		InitialInterval:     initialInterval,
		MaxInterval:         maxInterval,
		Multiplier:          multiplier,
		RandomizationFactor: jitter,
		Clock:               &SystemClock{},
	}
}

// NewZeroBackOff creates a backoff policy that always returns 0 (no waiting).
func NewZeroBackOff() BackOff {
	return &ZeroBackOffImpl{}
}

// NewStopBackOff creates a backoff policy that always returns Stop.
func NewStopBackOff() BackOff {
	return &StopBackOffImpl{}
}

// MarshalBackOff serializes a BackOff implementation to JSON
func MarshalBackOff(b BackOff) ([]byte, error) {
	if b == nil {
		return json.Marshal(nil)
	}

	var data BackOffData

	switch impl := b.(type) {
	case *ConstantBackOffImpl:
		data = BackOffData{
			Type:     ConstantBackOffType,
			Interval: impl.Interval,
		}
	case *ExponentialBackOffImpl:
		data = BackOffData{
			Type:                ExponentialBackOffType,
			InitialInterval:     impl.InitialInterval,
			CurrentInterval:     impl.CurrentInterval,
			MaxInterval:         impl.MaxInterval,
			Multiplier:          impl.Multiplier,
			RandomizationFactor: impl.RandomizationFactor,
			MaxElapsedTime:      impl.MaxElapsedTime,
		}
	case *ZeroBackOffImpl:
		data = BackOffData{
			Type: ZeroBackOffType,
		}
	case *StopBackOffImpl:
		data = BackOffData{
			Type: StopBackOffType,
		}
	default:
		return nil, fmt.Errorf("unsupported BackOff implementation: %T", b)
	}

	return json.Marshal(data)
}

// UnmarshalBackOff deserializes a JSON representation into a BackOff implementation
func UnmarshalBackOff(data []byte) (BackOff, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}

	var backoffData BackOffData
	if err := json.Unmarshal(data, &backoffData); err != nil {
		return nil, err
	}

	switch backoffData.Type {
	case ConstantBackOffType:
		return &ConstantBackOffImpl{
			Interval: backoffData.Interval,
		}, nil
	case ExponentialBackOffType:
		bo := &ExponentialBackOffImpl{
			InitialInterval:     backoffData.InitialInterval,
			MaxInterval:         backoffData.MaxInterval,
			Multiplier:          backoffData.Multiplier,
			RandomizationFactor: backoffData.RandomizationFactor,
			MaxElapsedTime:      backoffData.MaxElapsedTime,
			Clock:               &SystemClock{},
		}
		// Only reset if CurrentInterval is not set
		if backoffData.CurrentInterval > 0 {
			bo.CurrentInterval = backoffData.CurrentInterval
		} else {
			bo.Reset()
		}
		return bo, nil
	case ZeroBackOffType:
		return &ZeroBackOffImpl{}, nil
	case StopBackOffType:
		return &StopBackOffImpl{}, nil
	default:
		return nil, fmt.Errorf("unknown BackOff type: %s", backoffData.Type)
	}
}

// Retry retries the operation function using the specified BackOff policy.
// If the operation returns nil, Retry returns nil.
// If the operation returns an error, Retry retries as specified by the BackOff policy.
// If the BackOff policy indicates no more retries, the most recent error is returned.
func Retry(operation func() error, b BackOff) error {
	b.Reset()
	var err error
	var next time.Duration

	for {
		if err = operation(); err == nil {
			return nil
		}

		// Check if it's a permanent error
		if IsPermanent(err) {
			return err
		}

		// Get next backoff duration
		next = b.NextBackOff()
		if next == Stop {
			return err
		}

		// Sleep for the backoff duration
		time.Sleep(next)
	}
}

// RetryNotify is the same as Retry but calls notify after each failed attempt.
func RetryNotify(operation func() error, b BackOff, notify func(error, time.Duration)) error {
	var err error
	var next time.Duration

	b.Reset()
	for {
		err = operation()
		if err == nil {
			return nil
		}

		if IsPermanent(err) {
			return err
		}

		next = b.NextBackOff()
		if next == Stop {
			return err
		}

		if notify != nil {
			notify(err, next)
		}

		t := time.NewTimer(next)
		<-t.C
		t.Stop()
	}
}
