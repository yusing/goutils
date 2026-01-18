package task

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/intern"
)

type (
	TaskStarter interface {
		// Start starts the object that implements TaskStarter,
		// and returns an error if it fails to start.
		//
		// callerSubtask.Finish must be called when start fails or the object is finished.
		Start(parent Parent) gperr.Error
		Task() *Task
	}
	TaskFinisher interface {
		Finish(reason any)
	}
	Callback struct {
		fn    func()
		about string
		wait  bool // true for onFinish callbacks, false for onCancel callbacks
	}
	// Task controls objects' lifetime.
	//
	// Objects that uses a Task should implement the TaskStarter and the TaskFinisher interface.
	//
	// Use Task.Finish to stop all subtasks of the Task.
	Task struct {
		parent       *Task
		name         intern.Handle[string]
		ctx          context.Context
		cancel       context.CancelCauseFunc
		done         chan struct{}
		finishCalled bool
		callbacks    *Dependencies[*Callback]
		children     *Dependencies[*Task]

		values atomic.Pointer[xsync.Map[any, any]]

		mu sync.Mutex
	}
	Parent interface {
		Context() context.Context
		// Subtask returns a new subtask with the given name, derived from the parent's context.
		//
		// This should not be called after Finish is called on the task or its parent task.
		Subtask(name string, needFinish bool) *Task
		Name() string
		Finish(reason any)
		OnCancel(name string, f func())
		// SetValue sets a value in the task's context.
		//
		// This value will be available to all subtasks of the task.
		//
		// This method is thread-safe.
		SetValue(key any, value any)
		// GetValue gets a value from the task's context.
		//
		// It will search the value in the task's context, and then in the parent's context.
		//
		// This method is thread-safe.
		GetValue(key any) any
	}
)

const taskTimeout = 3 * time.Second

func (t *Task) Context() context.Context {
	return ctxWithValues{task: t}
}

func (t *Task) Name() string {
	return t.name.Value()
}

func (t *Task) SetValue(key any, value any) {
	values := t.values.Load()
	if values == nil {
		// only initialize once
		t.values.CompareAndSwap(nil, xsync.NewMap[any, any](xsync.WithGrowOnly()))
		values = t.values.Load()
	}
	values.Store(key, value)
}

func (t *Task) GetValue(key any) any {
	if values := t.values.Load(); values != nil {
		v, ok := values.Load(key)
		if ok {
			return v
		}
	}
	if t.parent != root {
		return t.parent.GetValue(key)
	}
	return nil
}

// String returns the full name of the task.
func (t *Task) String() string {
	return t.fullName()
}

// MarshalText implements encoding.TextMarshaler.
func (t *Task) MarshalText() ([]byte, error) {
	return []byte(t.fullName()), nil
}

// Finish marks the task as finished, with the given reason (if any).
func (t *Task) Finish(reason any) {
	t.finish(reason, false)
}

// FinishCause returns the reason / error that caused the task to be finished.
func (t *Task) FinishCause() error {
	return context.Cause(t.ctx)
}

// FinishAndWait cancel all subtasks and wait for them to finish,
// then marks the task as finished, with the given reason (if any).
func (t *Task) FinishAndWait(reason any) {
	t.finish(reason, true)
}

// OnFinished calls fn when the task is canceled and all subtasks are finished.
//
// It should not be called after Finish is called.
func (t *Task) OnFinished(about string, fn func()) {
	t.addCallback(about, fn, t.needFinish()) // when needFinish() is false, it's OnCancel
}

// OnCancel calls fn when the task is canceled.
//
// It should not be called after Finish is called.
func (t *Task) OnCancel(about string, fn func()) {
	t.addCallback(about, fn, false)
}

// addCallback adds a callback with the specified wait parameter.
// It initializes the callbacks goroutine if needed.
func (t *Task) addCallback(about string, fn func(), wait bool) {
	t.mu.Lock()
	if t.callbacks != nil {
		t.mu.Unlock()
	} else {
		t.callbacks = NewDependencies[*Callback]()
		t.mu.Unlock()

		context.AfterFunc(t.ctx, func() {
			// Execute non-waiting callbacks immediately when context is done
			for cb := range t.callbacks.Range {
				if !cb.wait { // Execute non-waiting callbacks (OnCancel)
					go func(cb *Callback) {
						invokeWithRecover(cb)
						t.callbacks.Delete(cb)
					}(cb)
				}
			}

			// Wait for all subtasks to finish, then execute waiting callbacks
			<-t.done
			for cb := range t.callbacks.Range {
				if cb.wait { // Execute waiting callbacks (OnFinished)
					go func(cb *Callback) {
						invokeWithRecover(cb)
						t.callbacks.Delete(cb)
					}(cb)
				}
			}
		})
	}

	t.callbacks.Add(&Callback{fn: fn, about: about, wait: wait})
}

// Subtask returns a new subtask with the given name, derived from the parent's context.
//
// This should not be called after Finish is called on the task or its parent task.
func (t *Task) Subtask(name string, needFinish bool) *Task {
	t.mu.Lock()
	if t.children == nil {
		t.children = NewDependencies[*Task]()
		t.mu.Unlock()
	} else {
		t.mu.Unlock()
	}

	child := &Task{
		name:   intern.Make(name),
		parent: t,
	}

	t.children.Add(child)

	child.ctx, child.cancel = context.WithCancelCause(t.ctx)

	if needFinish {
		child.done = make(chan struct{})
	} else {
		child.done = closedCh
		context.AfterFunc(child.ctx, func() {
			child.Finish(child.FinishCause())
		})
	}

	logStarted(child)
	return child
}

func (t *Task) finish(reason any, wait bool) {
	t.mu.Lock()
	if t.finishCalled {
		t.mu.Unlock()
		// wait but not report stucked (again)
		t.waitFinish(taskTimeout)
		return
	}

	t.finishCalled = true
	t.mu.Unlock()

	t.cancel(fmtCause(reason))

	if t.needFinish() {
		// close t.done so onFinish callbacks can be executed
		close(t.done)
	}

	if wait && !t.waitFinish(taskTimeout) {
		t.reportStucked()
	}

	if t != root {
		t.parent.children.Delete(t)
	}
	logFinished(t)
}

func (t *Task) waitFinish(timeout time.Duration) bool {
	if t.children == nil && t.callbacks == nil {
		return true
	}

	// NOTE: do not use t.ctx here
	// when we reached here, t.ctx is already done
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if t.children != nil {
		if err := t.children.Wait(ctx); err != nil {
			return false
		}
	}
	if t.callbacks != nil {
		if err := t.callbacks.Wait(ctx); err != nil {
			return false
		}
	}

	return true
}

func (t *Task) fullName() string {
	if t.parent == root {
		return t.name.Value()
	}
	return t.parent.fullName() + "." + t.name.Value()
}

func (t *Task) needFinish() bool {
	return t.done != closedCh
}
