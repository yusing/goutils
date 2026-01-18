package task_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yusing/goutils/task"
)

type contextKey struct{}

func TestWithValues(t *testing.T) {
	t.Run("test with values", func(t *testing.T) {
		task := task.RootTask("test", false)
		task.SetValue(contextKey{}, "value")
		assert.Equal(t, "value", task.Context().Value(contextKey{}))
		assert.Equal(t, "value", task.GetValue(contextKey{}))
	})
}

func TestChildTaskWithValues(t *testing.T) {
	t.Run("inherit from parent", func(t *testing.T) {
		task := task.RootTask("test", false)
		task.SetValue(contextKey{}, "value")
		child := task.Subtask("child", false)
		assert.Equal(t, "value", child.Context().Value(contextKey{}))
		assert.Equal(t, "value", child.GetValue(contextKey{}))
	})
	t.Run("child only", func(t *testing.T) {
		task := task.RootTask("test", false)
		child := task.Subtask("child", false)
		child.SetValue(contextKey{}, "value")
		assert.Equal(t, "value", child.Context().Value(contextKey{}))
		assert.Equal(t, "value", child.GetValue(contextKey{}))
		assert.Nil(t, task.Context().Value(contextKey{}))
		assert.Nil(t, task.GetValue(contextKey{}))
	})
}

func TestTaskSetValueAfterContextRetrieved(t *testing.T) {
	// set value after context is retrieved
	task := task.RootTask("test", false)
	ctx := task.Context()
	task.SetValue(contextKey{}, "value")
	assert.Equal(t, "value", ctx.Value(contextKey{}))
	assert.Equal(t, "value", task.GetValue(contextKey{}))
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
		assert.Equal(t, i, task.GetValue(i))
	}
}
