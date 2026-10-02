package httputils

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"

	"github.com/yusing/goutils/logging"
)

type requestLogEntry struct {
	level   logging.Level
	message string
	fields  []logging.Field
}

type requestRecordingLogger struct{ entries []requestLogEntry }

func (l *requestRecordingLogger) Log(level logging.Level, message string, fields ...logging.Field) {
	l.entries = append(l.entries, requestLogEntry{level, message, slices.Clone(fields)})
}

func TestRequestLoggingLevelsAndFields(t *testing.T) {
	t.Cleanup(func() { logging.SetLogger(nil) })
	err := errors.New("original request failure")
	request := httptest.NewRequest(http.MethodPatch, "http://example.test:8080/items?limit=2", nil)
	request.RemoteAddr = "[2001:db8::1]:1234"
	custom := []logging.Field{
		{Key: "error", Value: err},
		{Key: "attempt", Value: int64(42)},
		{Key: "ratio", Value: float32(0.5)},
	}
	for _, tc := range []struct {
		name  string
		level logging.Level
		log   func(*http.Request, string, ...logging.Field)
	}{
		{"error", logging.Error, LogError},
		{"warn", logging.Warn, LogWarn},
		{"info", logging.Info, LogInfo},
		{"debug", logging.Debug, LogDebug},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() { logging.SetLogger(nil) })
			for _, fields := range [][]logging.Field{nil, custom} {
				receiver := new(requestRecordingLogger)
				logging.SetLogger(receiver)
				tc.log(request, "unchanged %s\nmessage", fields...)
				wantFields := []logging.Field{
					{Key: "remote", Value: "[2001:db8::1]:1234"},
					{Key: "host", Value: "example.test:8080"},
					{Key: "uri", Value: "PATCH http://example.test:8080/items?limit=2"},
				}
				want := []requestLogEntry{{tc.level, "unchanged %s\nmessage", append(wantFields, fields...)}}
				if got := receiver.entries; !reflect.DeepEqual(got, want) {
					t.Fatalf("got %#v, want %#v", got, want)
				}
				if fields != nil && receiver.entries[0].fields[3].Value != err {
					t.Fatal("custom error identity changed")
				}
			}
		})
	}
	if got := custom; !reflect.DeepEqual(got, []logging.Field{{Key: "error", Value: err}, {Key: "attempt", Value: int64(42)}, {Key: "ratio", Value: float32(0.5)}}) {
		t.Fatalf("caller fields changed: %#v", got)
	}
}
