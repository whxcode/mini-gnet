// Package scheduler
package scheduler

type Queue []Task

func NewQueue() *Queue {
	q := make(Queue, 0)
	return &q
}

func (q *Queue) Enqueue(task ...Task) {
	*q = append(*q, task...)
}

func (q *Queue) Dequeue() (Task, bool) {
	if len(*q) == 0 {
		return Task{}, false
	}

	task := (*q)[0]
	*q = (*q)[1:]

	return task, true
}

func (q *Queue) DequeueWithTaskID(taskID string) {
	index := -1

	for i, task := range *q {
		if task.ID == taskID {
			index = i
			break
		}
	}

	if index != -1 {
		*q = append((*q)[:index], (*q)[index+1:]...)
	}
}
