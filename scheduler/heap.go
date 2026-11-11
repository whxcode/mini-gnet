package scheduler

import "container/heap"

type Heap []Task

func (h *Heap) Len() int {
	return len(*h)
}

func (h *Heap) Less(i int, j int) bool {
	// 最大堆，优先级高的任务先执行
	return (*h)[i].Priority > (*h)[j].Priority
}

func (h *Heap) Pop() any {
	heap := *h
	lastIndex := len(heap) - 1
	item := heap[lastIndex]
	heap[lastIndex] = Task{} // avoid memory leak
	*h = heap[:lastIndex]

	return item
}

func (h *Heap) Push(x any) {
	item, ok := x.(Task)
	if !ok {
		panic("invalid type")
	}

	*h = append(*h, item)
}

// Swap implements [heap.Interface].
func (h *Heap) Swap(i int, j int) {
	(*h)[i], (*h)[j] = (*h)[j], (*h)[i]
}

func NewHeap() *Heap {
	return &Heap{}
}

var _ heap.Interface = &Heap{}
