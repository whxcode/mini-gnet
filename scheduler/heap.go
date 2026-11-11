package scheduler

import "container/heap"

type heapCore struct {
	data  []*Task
	index map[string]int
}

func (h *heapCore) Len() int {
	return len(h.data)
}

func (h *heapCore) Less(i int, j int) bool {
	// 最大堆，优先级高的任务先执行
	return h.data[i].Priority > h.data[j].Priority
}

func (h *heapCore) Swap(i int, j int) {
	h.data[i], h.data[j] = h.data[j], h.data[i]

	h.index[h.data[i].ID] = i
	h.index[h.data[j].ID] = j
}

func (h *heapCore) Pop() any {
	if len(h.data) == 0 {
		return nil
	}

	lastIndex := len(h.data) - 1

	item := h.data[lastIndex]

	delete(h.index, item.ID)

	h.data[lastIndex] = nil // avoid memory leak
	h.data = h.data[:lastIndex]

	return item
}

func (h *heapCore) Push(x any) {
	item, ok := x.(*Task)
	if !ok {
		panic("invalid type")
	}

	h.index[item.ID] = len(h.data)

	h.data = append(h.data, item)
}

type Heap struct {
	h *heapCore
}

func (h *Heap) Pop() (*Task, bool) {
	if h.h.Len() == 0 {
		return nil, false
	}

	return heap.Pop(h.h).(*Task), true
}

func (h *Heap) Push(x *Task) {
	heap.Push(h.h, x)
}

func (h *Heap) Remove(taskID string) {
	if _, ok := h.h.index[taskID]; ok {
		heap.Remove(h.h, h.h.index[taskID])
		delete(h.h.index, taskID)
	}
}

func (h *Heap) Len() int {
	return len(h.h.data)
}

func NewHeap() *Heap {
	h := &Heap{
		h: &heapCore{
			data:  make([]*Task, 0),
			index: make(map[string]int),
		},
	}

	heap.Init(h.h)

	return h
}

var _ heap.Interface = &heapCore{}
