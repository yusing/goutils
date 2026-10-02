package task_test

import (
	"sync"
	"testing"

	"github.com/yusing/goutils/task"
	expect "github.com/yusing/goutils/testing"
)

type contextKey struct{}

func TestWithValues(t *testing.T) {
	t.Run("test with values", func(t *testing.T) {
		task := task.RootTask("test", false)
		task.SetValue(contextKey{}, "value")
		expect.Equal(t, expect.Type[string](t, task.Context().Value(contextKey{})), "value")
		expect.Equal(t, expect.Type[string](t, task.GetValue(contextKey{})), "value")
	})
}

func TestChildTaskWithValues(t *testing.T) {
	t.Run("inherit from parent", func(t *testing.T) {
		task := task.RootTask("test", false)
		task.SetValue(contextKey{}, "value")
		child := task.Subtask("child", false)
		expect.Equal(t, expect.Type[string](t, child.Context().Value(contextKey{})), "value")
		expect.Equal(t, expect.Type[string](t, child.GetValue(contextKey{})), "value")
	})
	t.Run("child only", func(t *testing.T) {
		task := task.RootTask("test", false)
		child := task.Subtask("child", false)
		child.SetValue(contextKey{}, "value")
		expect.Equal(t, expect.Type[string](t, child.Context().Value(contextKey{})), "value")
		expect.Equal(t, expect.Type[string](t, child.GetValue(contextKey{})), "value")
		expect.Nil(t, task.Context().Value(contextKey{}))
		expect.Nil(t, task.GetValue(contextKey{}))
	})
}

func TestTaskSetValueAfterContextRetrieved(t *testing.T) {
	// set value after context is retrieved
	task := task.RootTask("test", false)
	ctx := task.Context()
	task.SetValue(contextKey{}, "value")
	expect.Equal(t, expect.Type[string](t, ctx.Value(contextKey{})), "value")
	expect.Equal(t, expect.Type[string](t, task.GetValue(contextKey{})), "value")
}

func TestTaskConcurrentSetValue(t *testing.T) {
	task := task.RootTask("test", false)
	wg := sync.WaitGroup{}
	for i := range 100 {
		wg.Go(func() {
			task.SetValue(i, i)
		})
	}
	wg.Wait()

	for i := range 10 {
		expect.Equal(t, expect.Type[int](t, task.GetValue(i)), i)
	}
}
