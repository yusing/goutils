package task

import (
	"context"
	"sync"
	"time"

	gperr "github.com/yusing/goutils/errs"
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
		name         string
		ctx          context.Context
		cancel       context.CancelCauseFunc
		done         chan struct{}
		finishCalled bool
		callbacks    *withWg[*Callback]
		children     *withWg[*Task]

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
	}
)

const taskTimeout = 3 * time.Second

func (t *Task) Context() context.Context {
	return t.ctx
}

func (t *Task) Name() string {
	return t.name
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
		t.callbacks = newWithWg[*Callback]()
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
		t.children = newWithWg[*Task]()
		t.mu.Unlock()
	} else {
		t.mu.Unlock()
	}

	child := &Task{
		name:   name,
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

	if t.needFinish() {
		close(t.done)
	}

	t.cancel(fmtCause(reason))
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
	done := make(chan struct{})
	go func() {
		if t.children != nil {
			t.children.Wait()
		}
		if t.callbacks != nil {
			t.callbacks.Wait()
		}
		<-t.done
		close(done)
	}()
	timeoutCh := time.After(timeout)
	select {
	case <-done:
		return true
	case <-timeoutCh:
		return false
	}
}

func (t *Task) fullName() string {
	if t.parent == root {
		return t.name
	}
	return t.parent.fullName() + "." + t.name
}

func (t *Task) needFinish() bool {
	return t.done != closedCh
}
