package scheduler

import (
	"testing"
)

// TestHeap 验证任务按优先级从高到低出队。
func _TestHeap(t *testing.T) {
	h := NewHeap()

	h.Push(&Task{ID: "1", Priority: 1})
	h.Push(&Task{ID: "2", Priority: 2})
	h.Push(&Task{ID: "3", Priority: 30})
	h.Push(&Task{ID: "4", Priority: 4})

	for _, want := range []int{30, 4, 2, 1} {
		task, _ := h.Pop()

		if task.Priority != want {
			t.Fatalf("优先级：期望 %d，实际 %d", want, task.Priority)
		}

		t.Logf("task id: %s, priority: %d", task.ID, task.Priority)
	}
}

func _TestHeapRemove(t *testing.T) {
	h := NewHeap()

	h.Push(&Task{ID: "1", Priority: 1})
	h.Push(&Task{ID: "2", Priority: 2})
	h.Push(&Task{ID: "3", Priority: 30})
	h.Push(&Task{ID: "4", Priority: 4})

	h.Remove("3")
	h.Remove("2")
	h.Remove("4")
	h.Remove("1")
	h.Remove("20")
	h.Remove("1")

	if len(h.h.data) != 0 {
		t.Fatalf("期望堆为空，实际长度为 %d", len(h.h.data))
	}
}
