package scheduler

import (
	"container/heap"
	"testing"
)

// TestHeap 验证任务按优先级从高到低出队。
func TestHeap(t *testing.T) {
	h := NewHeap()
	heap.Init(h)

	heap.Push(h, Task{ID: "1", Priority: 1})
	heap.Push(h, Task{ID: "2", Priority: 2})
	heap.Push(h, Task{ID: "3", Priority: 30})
	heap.Push(h, Task{ID: "4", Priority: 4})

	for _, want := range []int{30, 4, 2, 1} {
		task := heap.Pop(h).(Task)

		if task.Priority != want {
			t.Fatalf("优先级：期望 %d，实际 %d", want, task.Priority)
		}

		t.Logf("task id: %s, priority: %d", task.ID, task.Priority)
	}
}
