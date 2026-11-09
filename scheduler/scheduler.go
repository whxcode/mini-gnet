package scheduler

import (
	"context"
	"fmt"
	"sync"
)

type Scheduler struct {
	// worker 数量
	workerNum int
	// 任务队列
	queue *Queue
	// 读取任务锁
	queueMutex sync.Mutex
	// 任务执行通知
	queueCond *sync.Cond
	// 任务完成等待组
	workerWaitGroup sync.WaitGroup
	// 是否已经停止接收新任务。
	closed bool
	// 任务状态 id: TaskState
	taskSnapshot map[string]*TaskSnapshot
}

func NewScheduler(workerNum int) *Scheduler {
	s := &Scheduler{
		workerNum:    workerNum,
		queue:        NewQueue(),
		taskSnapshot: make(map[string]*TaskSnapshot),
		closed:       false,
	}

	s.queueCond = sync.NewCond(&s.queueMutex)
	s.workerWaitGroup.Add(workerNum)

	fmt.Println("Starting scheduler with", workerNum, "workers")

	for range s.workerNum {
		go s.worker()
	}

	return s
}

func (s *Scheduler) worker() {
	defer s.workerWaitGroup.Done()

	for {
		s.queueMutex.Lock()

		// 没任务且还会接收任务：等待。
		for len(*s.queue) == 0 && !s.closed {
			s.queueCond.Wait()
		}

		// 已停止接收，剩余任务也处理完了：退出。
		if len(*s.queue) == 0 && s.closed {
			s.queueMutex.Unlock()
			return
		}

		task, ok := s.queue.Dequeue()
		if !ok {
			s.queueMutex.Unlock()
			continue
		}

		// 修改对应任务的状态
		snapshot := s.taskSnapshot[task.ID]

		if snapshot != nil {
			snapshot.State = TaskRunning
			snapshot.ctx, snapshot.cancel = context.WithCancel(context.Background())
		}

		s.queueMutex.Unlock()
		var result any
		var err error

		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("task %s panicked: %v", task.ID, r)
				}
			}()

			result, err = task.Func(snapshot.ctx)
		}()

		if snapshot != nil {
			s.queueMutex.Lock()

			if err != nil {
				snapshot.State = TaskFailed
			} else {
				snapshot.State = TaskSucceeded
			}

			snapshot.Result = result
			snapshot.Err = err
			s.queueMutex.Unlock()
		}

	}
}

func (s *Scheduler) notice() {
	s.queueCond.Signal()
}

func (s *Scheduler) AddTask(task ...Task) error {
	s.queueMutex.Lock()
	defer s.queueMutex.Unlock()

	if s.closed {
		return fmt.Errorf("Scheduler is closed, cannot add new tasks")
	}

	for _, t := range task {
		if _, exists := s.taskSnapshot[t.ID]; exists {
			return fmt.Errorf("Task with ID %s already exists", t.ID)
		}

		s.taskSnapshot[t.ID] = NewTaskSnapshot(t.ID, TaskPedding)
	}

	s.queue.Enqueue(task...)

	for range task {
		s.notice()
	}

	return nil
}

func (s *Scheduler) Cancel(taskID string) error {
	s.queueMutex.Lock()
	defer s.queueMutex.Unlock()
	snapshot, ok := s.taskSnapshot[taskID]

	if !ok {
		return fmt.Errorf("Task with ID %s does not exist", taskID)
	}

	// 任务还未运行
	switch snapshot.State {
	case TaskPedding:
		snapshot.State = TaskCanceled
		s.queue.DequeueWithTaskID(taskID)
	case TaskRunning:
		// 任务正在运行中
		snapshot.State = TaskCanceling
		snapshot.cancel()
	}

	return nil
}

func (s *Scheduler) GetTaskSnapshot(taskID string) *TaskSnapshot {
	s.queueMutex.Lock()
	defer s.queueMutex.Unlock()

	if snapshot, ok := s.taskSnapshot[taskID]; ok {

		sc := *snapshot
		return &sc
	}

	return nil
}

func (s *Scheduler) Close() {
	s.queueMutex.Lock()
	s.closed = true
	s.queueMutex.Unlock()

	s.queueCond.Broadcast()
}

func (s *Scheduler) Wait() {
	s.workerWaitGroup.Wait()
}
