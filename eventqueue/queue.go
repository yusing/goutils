package eventqueue

import (
	"runtime/debug"
	"time"

	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/task"
)

type (
	EventQueue[Event any] struct {
		task    *task.Task
		queue   []Event
		ticker  *time.Ticker
		onFlush OnFlushFunc[Event]
		onError OnErrorFunc
		debug   bool
	}
	OnFlushFunc[Event any] = func(events []Event)
	OnErrorFunc            = func(err error)

	Options[Event any] struct {
		Capacity      int
		FlushInterval time.Duration
		OnFlush       OnFlushFunc[Event]
		OnError       OnErrorFunc
		Debug         bool
	}
)

const (
	defaultEventQueueCapacity      = 10
	defaultEventQueueFlushInterval = 1 * time.Second
)

// New creates an EventQueue using queueTask and opt. Call Start to process events.
// OnFlush runs for nonempty batches at FlushInterval and must return when the
// batch is finished. OnError receives input errors and recovered OnFlush panics.
// Debug includes a stack trace in recovered panic errors.
func New[Event any](queueTask *task.Task, opt Options[Event]) *EventQueue[Event] {
	capacity := defaultEventQueueCapacity
	if opt.Capacity > 0 {
		capacity = opt.Capacity
	}
	if opt.FlushInterval <= 0 {
		opt.FlushInterval = defaultEventQueueFlushInterval
	}
	return &EventQueue[Event]{
		task:    queueTask,
		queue:   make([]Event, 0, capacity),
		ticker:  time.NewTicker(opt.FlushInterval),
		onFlush: opt.OnFlush,
		onError: opt.OnError,
		debug:   opt.Debug,
	}
}

// Start launches the processing goroutine. Cancellation or closure of either
// input channel discards pending events, waits for an in-flight flush, and
// finishes the queue's task.
func (e *EventQueue[Event]) Start(eventCh <-chan Event, errCh <-chan error) {
	onFlush := e.onFlush
	flush := func(events []Event) (err error) {
		defer func() {
			if errV := recover(); errV != nil {
				var recovered gperr.Error
				switch errV := errV.(type) {
				case error:
					recovered = gperr.PrependSubject(errV, e.task.Name())
				default:
					recovered = gperr.New("recovered panic in onFlush").Withf("%v", errV).Subject(e.task.Name())
				}
				if e.debug {
					recovered = recovered.Withf("%s", debug.Stack())
				}
				err = recovered
			}
		}()
		onFlush(events)
		return nil
	}
	startFlush := func() <-chan error {
		queue := make([]Event, len(e.queue))
		copy(queue, e.queue)
		e.queue = e.queue[:0]

		done := make(chan error, 1)
		go func() {
			done <- flush(queue)
		}()
		return done
	}
	handleError := func(err error) {
		if err != nil && e.onError != nil {
			e.onError(err)
		}
	}

	go func() {
		defer e.ticker.Stop()
		defer e.task.Finish(nil)

		var flushDone <-chan error
		defer func() {
			if flushDone != nil {
				handleError(<-flushDone)
			}
		}()

		for {
			select {
			case <-e.task.Context().Done():
				return
			case <-e.ticker.C:
				if flushDone == nil && len(e.queue) > 0 {
					flushDone = startFlush()
				}
			case err := <-flushDone:
				handleError(err)
				flushDone = nil
				if len(e.queue) > 0 {
					flushDone = startFlush()
				}
			case event, ok := <-eventCh:
				if !ok {
					return
				}
				e.queue = append(e.queue, event)
			case err, ok := <-errCh:
				if !ok {
					return
				}
				handleError(err)
			}
		}
	}()
}
