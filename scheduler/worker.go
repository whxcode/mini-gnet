package scheduler

import (
	"context"
	"errors"
	"fmt"
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
	defer s.workerWaitGroup.Done()

	if w.cond == nil {
		w.cond = sync.NewCond(&s.queueMutex)
	}

	for {

		// 保护任务队列的锁
		s.queueMutex.Lock()
		// defer s.queueMutex.Unlock()

		// 没任务且还会接收任务：等待。
		for s.heap.Len() == 0 && !s.closed {
			timeOut := false

			timer := time.AfterFunc(w.maxWaitTime, func() {
				s.queueMutex.Lock()
				defer s.queueMutex.Unlock()
				timeOut = true
				w.cond.Signal()
			})

			for s.heap.Len() == 0 && !s.closed && !timeOut {
				fmt.Println("继续等待任务....")
				w.cond.Wait()
			}

			timer.Stop()

			if timeOut {
				fmt.Println("等待任务超时....")
				s.queueMutex.Unlock()
				return
			}

		}

		// 已停止接收，剩余任务也处理完了：退出。
		if s.heap.Len() == 0 && s.closed {
			s.queueMutex.Unlock()
			return
		}

		// 到了这一步通过 s.queueCond 已经重新获得了 q.queueMutex
		task, ok := s.heap.Pop()

		if !ok {
			s.queueMutex.Unlock()
			continue

		}

		// 修改对应任务的状态
		snapshot := s.taskSnapshot[task.ID]

		if snapshot == nil {
			// 如果任务快照不存在，
			// 出现了异常
			// 这里直接跳过，继续执行下一个任务
			s.queueMutex.Unlock()
			continue
		}

		if task.Timeout == 0 {
			snapshot.ctx, snapshot.cancel = context.WithCancel(context.Background())
		} else {
			snapshot.ctx, snapshot.cancel = context.WithTimeout(context.Background(), task.Timeout)
		}

		snapshot.State = TaskRunning

		s.queueMutex.Unlock()

		result, err := func() (result any, err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("task %s panicked: %v", task.ID, r)
				}
			}()

			defer snapshot.cancel()

			return task.Func(snapshot.ctx)
		}()

		if snapshot == nil {
			continue
		}

		s.queueMutex.Lock()

		if err != nil {
			// 如果任务在运行中被取消.
			// 不在重复设置状态值
			if errors.Is(err, context.Canceled) {
				snapshot.State = TaskFailed
			} else {
				snapshot.State = TaskFailed
				snapshot.Err = err
			}
		} else {
			snapshot.State = TaskSucceeded
			snapshot.Result = result
		}

		s.queueMutex.Unlock()
	}
}
