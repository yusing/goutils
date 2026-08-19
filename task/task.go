package task

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/yusing/goutils/intern"
)

type (
	TaskStarter interface {
		// Start starts the object that implements TaskStarter,
		// and returns an error if it fails to start.
		//
		// callerSubtask.Finish must be called when start fails or the object is finished.
		Start(parent Parent) error
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
	if t == nil {
		panic("task is nil")
	}
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
	if t == nil {
		panic("task is nil")
	}
	if values := t.values.Load(); values != nil {
		v, ok := values.Load(key)
		if ok {
			return v
		}
	}
	if !t.parent.isRoot() {
		return t.parent.GetValue(key)
	}
	return nil
}

// isRoot reports whether the task is a root task, which is its own parent.
// Comparing against the root variable alone is not enough: tests replace it, and
// a task whose parent is a superseded root must still terminate the walk.
func (t *Task) isRoot() bool {
	return t == root || t.parent == t
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
		_ = t.waitFinish(waitTimeout())
		return
	}

	t.finishCalled = true
	t.mu.Unlock()

	t.cancel(fmtCause(reason))

	if t.needFinish() {
		// close t.done so onFinish callbacks can be executed
		close(t.done)
	}

	if wait {
		err := t.waitFinish(waitTimeout())
		if err != nil {
			t.reportStucked(err)
		}
		t.detachFromParent(err)
		logFinished(t)
		return
	}

	// Without waiting, detaching now would abandon the callbacks and children this
	// task still owns: the parent's wait would stop covering them and no report
	// could name them. Stay attached until they are done.
	if t.hasPending() {
		go func() {
			t.detachFromParent(t.waitFinish(waitTimeout()))
			logFinished(t)
		}()
		return
	}

	t.detachFromParent(nil)
	logFinished(t)
}

// detachFromParent removes the task from its parent's children set now that the
// work it owns is done.
//
// A task that could not finish while the program is shutting down stays attached
// on purpose: the root report runs last and can only name what is still in the
// tree. Outside shutdown it always detaches, so one stuck callback cannot pin a
// stale task to its parent for the rest of the process lifetime.
func (t *Task) detachFromParent(waitErr error) {
	if t.parent == t { // a root task, which is its own parent
		return
	}
	if waitErr != nil && shutdownDeadline.Load() != 0 {
		return
	}
	t.parent.children.Delete(t)
}

// hasPending reports whether the task still owns unfinished callbacks or children.
func (t *Task) hasPending() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.children.Len() > 0 || t.callbacks.Len() > 0
}

// waitFinish waits up to timeout for the task's children and callbacks to
// finish. The returned error says why the wait ended early: a deadline, or an
// accounting bug inside Dependencies. Both belong in the stucked report, so
// callers must not discard it silently.
func (t *Task) waitFinish(timeout time.Duration) error {
	if t.children == nil && t.callbacks == nil {
		return nil
	}

	// NOTE: do not carry t.ctx's cancellation here
	// when we reached here, t.ctx is already done
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.ctx), timeout)
	defer cancel()

	if t.children != nil {
		if err := t.children.Wait(ctx); err != nil {
			return fmt.Errorf("waiting for children: %w", err)
		}
	}
	if t.callbacks != nil {
		if err := t.callbacks.Wait(ctx); err != nil {
			return fmt.Errorf("waiting for callbacks: %w", err)
		}
	}

	return nil
}

// waitTimeout returns how long a single FinishAndWait may wait. While the
// program is shutting down it never outlasts the program-wide budget, so the
// root wait is the last one to give up and its report describes what is still
// stuck rather than what is mid-teardown.
func waitTimeout() time.Duration {
	if deadline := shutdownDeadline.Load(); deadline != 0 {
		return min(taskTimeout, time.Until(time.Unix(0, deadline)))
	}
	return taskTimeout
}

func (t *Task) fullName() string {
	if t.parent.isRoot() {
		return t.name.Value()
	}
	return t.parent.fullName() + "." + t.name.Value()
}

func (t *Task) needFinish() bool {
	return t.done != closedCh
}
