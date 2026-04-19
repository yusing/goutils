package events

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *syncBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *syncBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]byte, len(b.buf))
	copy(out, b.buf)
	return out
}

func decodeEvents(t *testing.T, data []byte) []Event {
	t.Helper()
	var out []Event
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var event Event
		err := dec.Decode(&event)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		out = append(out, event)
	}
	return out
}

func newTimedEvent(ts int64, category, action string, data any) Event {
	return Event{
		ID:        strconv.FormatInt(ts, 10),
		Timestamp: time.Unix(0, ts),
		Level:     LevelInfo,
		Category:  category,
		Action:    action,
		Data:      data,
	}
}

func TestListenCancelIsIdempotent(t *testing.T) {
	t.Parallel()

	h := NewHistory()
	current, ch, cancel := h.SnapshotAndListen()
	require.Len(t, current, 0)
	require.NotPanics(t, cancel)
	require.NotPanics(t, cancel)

	select {
	case _, ok := <-ch:
		require.False(t, ok)
	default:
		t.Fatalf("listener channel should be closed after cancel")
	}
}

func TestConcurrentAddAndCancelDoesNotPanic(t *testing.T) {
	t.Parallel()

	h := NewHistory()
	var wg sync.WaitGroup

	for i := range 8 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := range 2_000 {
				h.Add(NewEvent(LevelInfo, "test", "add", worker*10_000+j))
			}
		}(i)
	}

	for range 8 {
		wg.Go(func() {
			for range 2_000 {
				_, _, cancel := h.SnapshotAndListen()
				cancel()
			}
		})
	}

	wg.Wait()
}

func TestListenJSONNoDuplicateAtBoundary(t *testing.T) {
	t.Parallel()

	h := NewHistory()
	h.Add(NewEvent(LevelInfo, "test", "init-1", nil))
	h.Add(NewEvent(LevelInfo, "test", "init-2", nil))

	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	var w syncBuffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- h.ListenJSON(ctx, &w)
	}()

	h.Add(NewEvent(LevelInfo, "test", "live-1", nil))

	require.Eventually(t, func() bool {
		events := decodeEvents(t, w.Bytes())
		return len(events) >= 3
	}, 2*time.Second, 10*time.Millisecond)

	stop()
	err := <-errCh
	require.ErrorIs(t, err, context.Canceled)

	events := decodeEvents(t, w.Bytes())
	actionCount := make(map[string]int, len(events))
	for _, event := range events {
		actionCount[event.Action]++
	}
	require.Equal(t, 1, actionCount["init-1"])
	require.Equal(t, 1, actionCount["init-2"])
	require.Equal(t, 1, actionCount["live-1"])
}

func TestSnapshotAndListenBoundaryDeliveredOnceUnderContention(t *testing.T) {
	t.Parallel()

	for i := range 200 {
		h := NewHistory()
		h.listenersMu.Lock()

		start := make(chan struct{})
		var (
			current []Event
			ch      <-chan Event
			cancel  func()
		)
		var wg sync.WaitGroup
		wg.Add(2)

		go func(iter int) {
			defer wg.Done()
			<-start
			h.Add(NewEvent(LevelInfo, "test", "boundary", iter))
		}(i)

		go func() {
			defer wg.Done()
			<-start
			current, ch, cancel = h.SnapshotAndListen()
		}()

		close(start)
		time.Sleep(time.Millisecond)
		h.listenersMu.Unlock()
		wg.Wait()

		boundaryCount := 0
		for _, event := range current {
			if event.Action == "boundary" {
				boundaryCount++
			}
		}
		select {
		case event := <-ch:
			if event.Action == "boundary" {
				boundaryCount++
			}
		default:
		}
		cancel()

		require.Equal(t, 1, boundaryCount, "iteration=%d", i)
	}
}

func TestGetReturnsNewestWindowInOrder(t *testing.T) {
	t.Parallel()

	h := NewHistory()
	for i := range maxHistorySize + 20 {
		h.Add(NewEvent(LevelInfo, "test", "order", i))
	}

	events := h.Get()
	require.Len(t, events, maxHistorySize)
	for i := range maxHistorySize {
		require.Equal(t, i+20, events[i].Data)
	}
}

func TestGetKeepsGlobalHistoryBoundAcrossCategories(t *testing.T) {
	t.Parallel()

	h := NewHistory()
	for i := range maxHistorySize + 20 {
		category := "cat-a"
		if i%2 == 1 {
			category = "cat-b"
		}
		h.Add(newTimedEvent(int64(i), category, "global-window", i))
	}

	events := h.Get()
	require.Len(t, events, maxHistorySize)
	for i := range maxHistorySize {
		require.Equal(t, i+20, events[i].Data)
	}
}

func TestGetDoesNotObservePartialAddAll(t *testing.T) {
	t.Parallel()

	for attempt := range 200 {
		h := NewHistory()

		batch := make([]Event, maxHistorySize)
		for i := range batch {
			batch[i] = newTimedEvent(int64(i+1), "category-"+strconv.Itoa(i), "batch", i)
		}

		done := make(chan struct{})
		go func() {
			h.AddAll(batch)
			close(done)
		}()

		for {
			select {
			case <-done:
				goto nextAttempt
			default:
			}

			snapshot := h.Get()
			require.Truef(
				t,
				len(snapshot) == 0 || len(snapshot) == len(batch),
				"attempt=%d observed torn AddAll snapshot len=%d",
				attempt,
				len(snapshot),
			)
			runtime.Gosched()
		}

	nextAttempt:
		require.Len(t, h.Get(), len(batch))
	}
}

func BenchmarkHistoryAdd(b *testing.B) {
	for _, concurrency := range []int{2, 4, 8, 16, 32, 64} {
		b.Run("concurrency="+strconv.Itoa(concurrency), func(b *testing.B) {
			h := NewHistory()
			event := NewEvent(LevelInfo, "bench-add", "add", nil)

			stopProducer := make(chan struct{})

			var producerWG sync.WaitGroup
			for range concurrency {
				producerWG.Go(func() {
					for {
						select {
						case <-stopProducer:
							return
						default:
							h.Add(event)
						}
					}
				})
			}

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				h.Add(event)
			}
			b.StopTimer()

			close(stopProducer)
			producerWG.Wait()
		})
	}
}

func BenchmarkHistoryListenReceive(b *testing.B) {
	for _, concurrency := range []int{2, 4, 8, 16, 32, 64} {
		b.Run("concurrency="+strconv.Itoa(concurrency), func(b *testing.B) {
			h := NewHistory()

			listenerChs := make([]<-chan Event, concurrency)
			for i := range concurrency {
				_, ch, cancel := h.SnapshotAndListen()
				listenerChs[i] = ch
				defer cancel()
			}

			stopProducer := make(chan struct{})
			var producerWG sync.WaitGroup

			event := NewEvent(LevelInfo, "bench-listen", "listen", nil)
			for range concurrency {
				producerWG.Go(func() {
					for {
						select {
						case <-stopProducer:
							return
						default:
							h.Add(event)
						}
					}
				})
			}

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				for _, ch := range listenerChs {
					<-ch
				}
			}
			b.StopTimer()

			close(stopProducer)
			producerWG.Wait()
		})
	}
}
