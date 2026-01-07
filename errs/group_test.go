package gperr_test

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	. "github.com/yusing/goutils/errs"
	expect "github.com/yusing/goutils/testing"
)

func TestBuilderConcurrency(t *testing.T) {
	const numGoroutines = 100
	const errorsPerGoroutine = 10

	errs := NewGroup("concurrent test")

	// Launch multiple goroutines adding errors concurrently
	for i := range numGoroutines {
		errs.Go(func() error {
			for j := range errorsPerGoroutine {
				errs.Addf("goroutine %d, error %d", i, j)
			}
			return nil
		})
	}

	errs.Wait()
}

func TestBuilderConcurrentMixedOperations(t *testing.T) {
	const numGoroutines = 50
	errs := NewGroup("mixed operations test")
	// Test different types of operations concurrently
	for i := range numGoroutines {
		errs.Go(func() error {
			switch i % 4 {
			case 0:
				return fmt.Errorf("error %d", i)
			case 1:
				return New("string error")
			case 2:
				return fmt.Errorf("formatted error %d", i)
			case 3:
				// Add multiple errors at once
				errs.Add(Join(
					fmt.Errorf("range error 1-%d", i),
					fmt.Errorf("range error 2-%d", i),
				))
			}
			return nil
		})
	}

	result := errs.Wait()
	err := result.Error()
	expect.NotNil(t, err)

	// Verify we can iterate without race conditions
	errorCount := 0
	result.ForEach(func(_ error) {
		errorCount++
	})

	expect.True(t, errorCount > 0)
}

func TestBuilderNoRaceWithGoroutineDetector(t *testing.T) {
	// This test is designed to be run with the race detector
	// go test -race -run TestBuilderNoRaceWithGoroutineDetector

	errs := NewGroup("race test")
	done := make(chan bool)

	// Writer goroutines
	for i := range 10 {
		errs.Go(func() error {
			for j := range 100 {
				errs.Add(fmt.Errorf("writer %d, error %d", i, j))
				runtime.Gosched() // Yield to increase chance of race detection
			}
			return nil
		})
	}

	// Wait for all goroutines to finish
	go func() {
		errs.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Test completed successfully
	case <-time.After(10 * time.Second):
		t.Fatal("Test timed out - possible deadlock")
	}
}
