package logging_test

import (
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"

	"github.com/yusing/goutils/logging"
)

type logEntry struct {
	level   logging.Level
	message string
	fields  []logging.Field
}

type recordingLogger struct {
	mu      sync.Mutex
	entries []logEntry
}

func (l *recordingLogger) Log(level logging.Level, message string, fields ...logging.Field) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, logEntry{level, message, slices.Clone(fields)})
}

func (l *recordingLogger) snapshot() []logEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.entries)
}

func TestLogForwardsLevelMessageAndTypedFields(t *testing.T) {
	t.Cleanup(func() { logging.SetLogger(nil) })
	err := errors.New("original failure")
	fields := []logging.Field{
		{Key: "error", Value: err},
		{Key: "attempt", Value: int64(42)},
		{Key: "ratio", Value: float32(0.5)},
		{Key: "enabled", Value: true},
		{Key: "label", Value: "original"},
		{Key: "optional", Value: nil},
	}
	for _, level := range []logging.Level{logging.Debug, logging.Info, logging.Warn, logging.Error, logging.Level(255)} {
		receiver := new(recordingLogger)
		logging.SetLogger(receiver)
		logging.Log(level, "unchanged %s\nmessage", fields...)
		want := []logEntry{{level, "unchanged %s\nmessage", fields}}
		if got := receiver.snapshot(); !reflect.DeepEqual(got, want) {
			t.Fatalf("level %d: got %#v, want %#v", level, got, want)
		}
		if got := receiver.snapshot()[0].fields[0].Value; got != err {
			t.Fatalf("error identity changed: got %v, want %v", got, err)
		}
	}
}

func TestSetLoggerSwitchesAndDisables(t *testing.T) {
	t.Cleanup(func() { logging.SetLogger(nil) })
	logging.SetLogger(nil)
	logging.Log(logging.Info, "disabled before installation")

	first, second := new(recordingLogger), new(recordingLogger)
	logging.SetLogger(first)
	logging.Log(logging.Info, "first")
	logging.SetLogger(second)
	logging.Log(logging.Warn, "second")
	logging.SetLogger(nil)
	logging.Log(logging.Error, "disabled after installation")
	logging.SetLogger(first)
	logging.Log(logging.Debug, "reinstalled")

	if got, want := first.snapshot(), []logEntry{{logging.Info, "first", nil}, {logging.Debug, "reinstalled", nil}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("first receiver: got %#v, want %#v", got, want)
	}
	if got, want := second.snapshot(), []logEntry{{logging.Warn, "second", nil}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("second receiver: got %#v, want %#v", got, want)
	}
}

// A distinct receiver type also exercises replacement across implementations.
type alternateLogger struct{ *recordingLogger }

func TestSetLoggerAndLogConcurrent(t *testing.T) {
	t.Cleanup(func() { logging.SetLogger(nil) })
	first := new(recordingLogger)
	second := &alternateLogger{new(recordingLogger)}
	fields := []logging.Field{{Key: "attempt", Value: int64(7)}}
	want := logEntry{logging.Info, "concurrent", fields}
	for _, receiver := range []logging.Logger{first, second} {
		logging.SetLogger(receiver)
		logging.Log(want.level, want.message, fields...)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			<-start
			for range 300 {
				logging.SetLogger(first)
				logging.SetLogger(second)
				logging.SetLogger(nil)
			}
		})
	}
	for range 4 {
		wg.Go(func() {
			<-start
			for range 300 {
				logging.Log(want.level, want.message, fields...)
			}
		})
	}
	close(start)
	wg.Wait()

	for _, receiver := range []*recordingLogger{first, second.recordingLogger} {
		entries := receiver.snapshot()
		if len(entries) == 0 {
			t.Fatal("receiver lost its synchronous initial call")
		}
		for _, got := range entries {
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("corrupted concurrent entry: got %#v, want %#v", got, want)
			}
		}
	}
	logging.SetLogger(first)
	before := len(first.snapshot())
	logging.Log(want.level, want.message, fields...)
	if got := len(first.snapshot()); got != before+1 {
		t.Fatalf("final receiver got %d entries, want %d", got, before+1)
	}
	logging.SetLogger(nil)
	logging.Log(want.level, want.message, fields...)
	if got := len(first.snapshot()); got != before+1 {
		t.Fatalf("disabled receiver got %d entries, want %d", got, before+1)
	}
}
