package libroutine_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/contenox/runtime/libroutine"
)

func TestPoolSingleton(t *testing.T) {
	defer quiet()
	t.Run("should return singleton instance", func(t *testing.T) {
		pool1 := libroutine.GetPool()
		pool2 := libroutine.GetPool()
		if pool1 != pool2 {
			t.Error("Expected pool to be singleton, got different instances")
		}
	})
}

func TestPoolStartLoop(t *testing.T) {
	pool := libroutine.GetPool()
	ctx := t.Context()

	t.Run("should create new manager and start loop", func(t *testing.T) {
		key := "test-service"
		var callCount int
		var mu sync.Mutex

		pool.StartLoop(ctx, &libroutine.LoopConfig{
			Key:          key,
			Threshold:    2,
			ResetTimeout: 100 * time.Millisecond,
			Interval:     10 * time.Millisecond,
			Operation: func(ctx context.Context) error {
				mu.Lock()
				callCount++
				mu.Unlock()
				return nil
			},
		})

		// Allow some time for the loop to execute.
		time.Sleep(25 * time.Millisecond)

		mu.Lock()
		if callCount < 1 {
			t.Errorf("Expected at least 1 call, got %d", callCount)
		}
		mu.Unlock()

		// Verify loop tracking using the public accessor.
		if !pool.IsLoopActive(key) {
			t.Error("Loop should be tracked as active")
		}
	})

	t.Run("should prevent duplicate loops for same key", func(t *testing.T) {
		key := "duplicate-test"
		var callCount int
		var mu sync.Mutex

		// Start first loop.
		pool.StartLoop(ctx, &libroutine.LoopConfig{
			Key:          key,
			Threshold:    1,
			ResetTimeout: time.Second,
			Interval:     10 * time.Millisecond,
			Operation: func(ctx context.Context) error {
				mu.Lock()
				callCount++
				mu.Unlock()
				return nil
			},
		})

		// Try to start duplicate loop.
		pool.StartLoop(ctx, &libroutine.LoopConfig{
			Key:          key,
			Threshold:    1,
			ResetTimeout: time.Second,
			Interval:     10 * time.Millisecond,
			Operation: func(ctx context.Context) error {
				mu.Lock()
				callCount++
				mu.Unlock()
				return nil
			},
		})

		time.Sleep(25 * time.Millisecond)

		mu.Lock()
		if callCount < 1 {
			t.Errorf("Expected at least 1 call, got %d", callCount)
		}
		// We expect only 1 instance running, so call count should be reasonable
		if callCount > 3 { // Allow some margin for timing variations
			t.Errorf("Expected approximately 2-3 calls, got %d (too many, duplicate loop might be running)", callCount)
		}
		mu.Unlock()
	})

	t.Run("should clean up after context cancellation", func(t *testing.T) {
		key := "cleanup-test"
		localCtx, localCancel := context.WithCancel(ctx)

		pool.StartLoop(localCtx, &libroutine.LoopConfig{
			Key:          key,
			Threshold:    1,
			ResetTimeout: time.Second,
			Interval:     10 * time.Millisecond,
			Operation: func(ctx context.Context) error {
				return nil
			},
		})

		time.Sleep(10 * time.Millisecond)
		localCancel()

		// Wait for cleanup.
		time.Sleep(50 * time.Millisecond) // Increased to ensure cleanup completes

		if pool.IsLoopActive(key) {
			t.Error("Loop should be removed from active tracking")
		}
	})

	t.Run("should handle concurrent StartLoop calls", func(t *testing.T) {
		key := "concurrency-test"
		var wg sync.WaitGroup
		var callCount int
		var mu sync.Mutex

		for range 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				pool.StartLoop(ctx, &libroutine.LoopConfig{
					Key:          key,
					Threshold:    1,
					ResetTimeout: time.Second,
					Interval:     10 * time.Millisecond,
					Operation: func(ctx context.Context) error {
						mu.Lock()
						callCount++
						mu.Unlock()
						return nil
					},
				})
			}()
		}

		wg.Wait()
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		if callCount < 1 {
			t.Errorf("Expected at least 1 call, got %d", callCount)
		}
		// We expect only one instance running, so call count should be reasonable
		if callCount > 6 { // Allow some margin but not excessive
			t.Errorf("Expected approximately 5-6 calls, got %d (too many, concurrency issue)", callCount)
		}
		mu.Unlock()
	})
}

func TestPoolCircuitBreaking(t *testing.T) {
	defer quiet()

	pool := libroutine.GetPool()
	ctx := context.Background()

	t.Run("should enforce circuit breaker parameters", func(t *testing.T) {
		key := "circuit-params-test"
		failureThreshold := 3
		resetTimeout := 50 * time.Millisecond

		var failures int

		// Use a very long interval so that Execute only runs when triggered.
		pool.StartLoop(ctx, &libroutine.LoopConfig{
			Key:          key,
			Threshold:    failureThreshold,
			ResetTimeout: resetTimeout,
			Interval:     1000 * time.Second,
			Operation: func(ctx context.Context) error {
				failures++
				return fmt.Errorf("simulated failure")
			},
		})

		// Fire triggers to simulate failureThreshold number of calls.
		for range failureThreshold {
			pool.ForceUpdate(key)
			// Give time for the execution to complete.
			time.Sleep(5 * time.Millisecond)
		}

		manager := pool.GetManager(key)
		if manager == nil {
			t.Fatal("Manager not created")
		}

		state := manager.GetState()
		if state != libroutine.Open {
			t.Errorf("Expected circuit to be open after %d failures, got state %v", failureThreshold, state)
		}

		// Wait for reset timeout to elapse.
		time.Sleep(resetTimeout + 10*time.Millisecond)

		// Do not send a trigger here; instead, call Allow() manually to simulate the test call.
		if allowed := manager.Allow(); !allowed {
			t.Error("Expected Allow() to return true in half-open state")
		}
		// Check if circuit breaker transitioned to half-open
		state = manager.GetState()
		if state != libroutine.HalfOpen {
			t.Error("Circuit should transition to half-open after reset timeout")
		}
	})
}

func TestPoolParameterPersistence(t *testing.T) {
	defer quiet()
	pool := libroutine.GetPool()
	ctx := context.Background() // Using Background instead of t.Context() for compatibility

	t.Run("should persist initial parameters", func(t *testing.T) {
		key := "param-persistence-test"
		initialThreshold := 2
		initialTimeout := 100 * time.Millisecond

		// First call with initial parameters.
		pool.StartLoop(ctx, &libroutine.LoopConfig{
			Key:          key,
			Threshold:    initialThreshold,
			ResetTimeout: initialTimeout,
			Interval:     10 * time.Millisecond,
			Operation: func(ctx context.Context) error {
				return nil
			},
		})

		// Subsequent call with different parameters.
		pool.StartLoop(ctx, &libroutine.LoopConfig{
			Key:          key,
			Threshold:    5,
			ResetTimeout: 200 * time.Millisecond,
			Interval:     20 * time.Millisecond,
			Operation: func(ctx context.Context) error {
				return nil
			},
		})

		manager := pool.GetManager(key)
		if manager == nil {
			t.Fatal("Manager not created")
		}

		if manager.GetThreshold() != initialThreshold {
			t.Errorf("Expected threshold %d, got %d", initialThreshold, manager.GetThreshold())
		}
		if manager.GetResetTimeout() != initialTimeout {
			t.Errorf("Expected timeout %v, got %v", initialTimeout, manager.GetResetTimeout())
		}
	})
}

// TestPoolResetRoutine verifies we can reset the circuit breaker state
func TestPoolResetRoutine(t *testing.T) {
	defer quiet()
	pool := libroutine.GetPool()
	ctx := t.Context()

	key := "reset-routine-test"
	var runCount int
	var runCountMu sync.Mutex

	// Start a loop that fails once then succeeds
	pool.StartLoop(ctx, &libroutine.LoopConfig{
		Key:          key,
		Threshold:    1,
		ResetTimeout: 10 * time.Millisecond,
		Interval:     10 * time.Millisecond,
		Operation: func(ctx context.Context) error {
			runCountMu.Lock()
			runCount++
			currentCount := runCount
			runCountMu.Unlock()

			// Fail only on first execution
			if currentCount == 1 {
				return errors.New("fail once")
			}
			return nil
		},
	})

	// Allow the loop to run and fail once
	time.Sleep(21 * time.Millisecond)

	// Get the manager
	manager := pool.GetManager(key)
	if manager == nil {
		t.Fatalf("Manager for key %s not found", key)
	}

	// Verify circuit is open after failure
	if manager.GetState() != libroutine.Open {
		t.Fatalf("Expected circuit to be open after failure, got %v", manager.GetState())
	}

	// Wait for reset timeout to transition to half-open
	time.Sleep(20 * time.Millisecond)

	// Verify circuit is in half-open state
	if manager.GetState() != libroutine.HalfOpen {
		t.Fatalf("Expected circuit to be half-open after reset timeout, got %v", manager.GetState())
	}

	// Force a call to transition to closed state
	pool.ForceUpdate(key)
	time.Sleep(15 * time.Millisecond) // Allow time for execution

	// Verify circuit is now closed
	if manager.GetState() != libroutine.Closed {
		t.Errorf("Expected manager state to be Closed after successful call, got %v", manager.GetState())
	}
}
