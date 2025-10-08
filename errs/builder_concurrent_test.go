package gperr_test

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	. "github.com/yusing/goutils/errs"
	expect "github.com/yusing/goutils/testing"
)

func TestBuilderConcurrentAdd(t *testing.T) {
	const numGoroutines = 100
	const errorsPerGoroutine = 10

	builder := NewBuilderWithConcurrency("concurrent test")
	var wg sync.WaitGroup

	// Launch multiple goroutines adding errors concurrently
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < errorsPerGoroutine; j++ {
				builder.Add(fmt.Errorf("goroutine %d, error %d", id, j))
			}
		}(i)
	}

	wg.Wait()

	// Verify all errors were collected
	expect.True(t, builder.HasError())
	err := builder.Error()
	expect.NotNil(t, err)

	// Count the total number of errors
	errorCount := 0
	builder.ForEach(func(_ error) {
		errorCount++
	})

	expectedCount := numGoroutines * errorsPerGoroutine
	expect.Equal(t, errorCount, expectedCount)
}

func TestBuilderConcurrentMixedOperations(t *testing.T) {
	const numGoroutines = 50
	builder := NewBuilderWithConcurrency("mixed operations test")
	var wg sync.WaitGroup

	// Test different types of operations concurrently
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			switch id % 4 {
			case 0:
				builder.Add(fmt.Errorf("error %d", id))
			case 1:
				builder.Adds("string error")
			case 2:
				builder.Addf("formatted error %d", id)
			case 3:
				// Add multiple errors at once
				builder.AddRange(
					fmt.Errorf("range error 1-%d", id),
					fmt.Errorf("range error 2-%d", id),
				)
			}
		}(i)
	}

	wg.Wait()

	expect.True(t, builder.HasError())

	// Verify we can iterate without race conditions
	errorCount := 0
	builder.ForEach(func(_ error) {
		errorCount++
	})

	expect.True(t, errorCount > 0)
}

func TestBuilderConcurrentReads(t *testing.T) {
	const numGoroutines = 20
	const numReaders = 100

	builder := NewBuilderWithConcurrency("read test")

	// Add some initial errors
	for i := range 10 {
		builder.Add(fmt.Errorf("initial error %d", i))
	}

	var wg sync.WaitGroup

	// Launch many reader goroutines
	for i := range numReaders {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Perform various read operations
			_ = builder.HasError()
			_ = builder.Error()
			_ = builder.String()
			_ = builder.About()

			// Iterate over errors
			count := 0
			builder.ForEach(func(_ error) {
				count++
			})
			expect.True(t, count >= 10) // Should have at least the initial errors
		}(i)
	}

	// Also add some errors while reading
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			builder.Add(fmt.Errorf("concurrent error %d", id))
		}(i)
	}

	wg.Wait()

	expect.True(t, builder.HasError())
}

func TestBuilderConcurrentAddFrom(t *testing.T) {
	const numGoroutines = 20

	mainBuilder := NewBuilderWithConcurrency("main")
	var wg sync.WaitGroup

	// Create multiple builders and add them to the main builder concurrently
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			subBuilder := NewBuilder("sub")
			subBuilder.Add(fmt.Errorf("sub error %d.1", id))
			subBuilder.Add(fmt.Errorf("sub error %d.2", id))

			mainBuilder.AddFrom(&subBuilder, true)
		}(i)
	}

	wg.Wait()

	expect.True(t, mainBuilder.HasError())

	// Count total errors
	errorCount := 0
	mainBuilder.ForEach(func(_ error) {
		errorCount++
	})

	expectedCount := numGoroutines * 2
	expect.Equal(t, errorCount, expectedCount)
}

func TestBuilderNoRaceWithGoroutineDetector(t *testing.T) {
	// This test is designed to be run with the race detector
	// go test -race -run TestBuilderNoRaceWithGoroutineDetector

	builder := NewBuilderWithConcurrency("race test")
	var wg sync.WaitGroup
	done := make(chan bool)

	// Writer goroutines
	for i := range 10 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				builder.Add(fmt.Errorf("writer %d, error %d", id, j))
				runtime.Gosched() // Yield to increase chance of race detection
			}
		}(i)
	}

	// Reader goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = builder.HasError()
				_ = builder.String()
				builder.ForEach(func(_ error) {
					// Just iterate
				})
				runtime.Gosched() // Yield to increase chance of race detection
			}
		}(i)
	}

	// Wait for all goroutines to finish
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Test completed successfully
	case <-time.After(10 * time.Second):
		t.Fatal("Test timed out - possible deadlock")
	}
}
