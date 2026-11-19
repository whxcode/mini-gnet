package scheduler

import (
	"sync"
	"time"
)

type Empty struct{}

var seedWorkerId uint64 = 0

type Worker struct {
	// 任务 id
	id uint64
	// 最长等待时间
	maxWaitTime time.Duration
	// 任务函数
	taskFunc func(task *Task) error
	// 条件变量
	cond *sync.Cond
}

func NewWorker(maxWaitTime time.Duration, taskFunc func(task *Task) error) *Worker {
	w := &Worker{
		id:          seedWorkerId,
		maxWaitTime: maxWaitTime,
		taskFunc:    taskFunc,
	}

	seedWorkerId++

	return w
}

func (w *Worker) worker(s *Scheduler) {
	// --
}
