package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrClosed = errors.New("scheduler is closed, cannot add new tasks")

type Scheduler struct {
	// worker 数量
	workerNum int
	// 任务队列
	//queue *Queue
	// 优先队列
	heap *Heap
	// 读取任务锁
	queueMutex sync.Mutex
	// 任务完成等待组
	workerWaitGroup sync.WaitGroup
	// 是否已经停止接收新任务。
	closed bool
	// 使用原子操作
	closeMutex sync.atomic
	// 任务状态 id: TaskState
	taskSnapshot map[string]*TaskSnapshot
	// task
	taskNotice chan struct{}

	closeNotice chan struct{}
}

func NewScheduler(workerNum int) *Scheduler {
	s := &Scheduler{
		workerNum:    workerNum,
		heap:         NewHeap(),
		taskSnapshot: make(map[string]*TaskSnapshot),
		closed:       false,
		taskNotice:   make(chan struct{}, workerNum),
		closeNotice:  make(chan struct{}, 1),
	}

	s.workerWaitGroup.Add(workerNum)

	i := 0
	for range s.workerNum {
		go s.worker(i)
		i++
	}

	return s
}

func (s *Scheduler) a() {
	s.queueMutex.Lock()

	// 到了这一步通过 s.queueCond 已经重新获得了 q.queueMutex
	task, ok := s.heap.Pop()

	if !ok {
		s.queueMutex.Unlock()
		return
		// continue

	}

	// 修改对应任务的状态
	snapshot := s.taskSnapshot[task.ID]

	if snapshot == nil {
		// 如果任务快照不存在，
		// 出现了异常
		// 这里直接跳过，继续执行下一个任务
		s.queueMutex.Unlock()
		return
		//continue
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
		return
		//continue
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

func (s *Scheduler) worker(workerID int) {
	defer s.workerWaitGroup.Done()

	for {
		if s.closed {
			fmt.Println("关闭通知....", workerID)
			return
		}

		timer := time.NewTimer(time.Second * 1000)

		select {
		case <-s.taskNotice:
			fmt.Println("任务通知....", workerID)
			timer.Stop()
			s.a()
		case <-timer.C:
			fmt.Println("任务超时....", workerID)
			return
		case <-s.closeNotice:
		}
	}
}

func (s *Scheduler) notice() {
	s.taskNotice <- struct{}{}
	// s.queueCond.Signal()
}

func (s *Scheduler) AddTask(task ...Task) error {
	s.queueMutex.Lock()
	defer s.queueMutex.Unlock()

	if s.closed {
		return ErrClosed
	}

	for _, t := range task {
		if _, exists := s.taskSnapshot[t.ID]; exists {
			continue
		}

		s.taskSnapshot[t.ID] = NewTaskSnapshot(t.ID, TaskPedding)
		newT := t
		s.notice()
		s.heap.Push(&newT)
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

	// 任务已执行完毕
	if snapshot.State == TaskFailed || //
		snapshot.State == TaskSucceeded {
		return fmt.Errorf("Task with ID %s has already completed", taskID)
	}

	// 任务已经取消
	if snapshot.State == TaskCanceled || //
		snapshot.State == TaskCanceling {
		return fmt.Errorf("Task with ID %s is already canceled or canceling", taskID)
	}

	switch snapshot.State {
	// 任务还未运行;直接从任务队列中移除，并标记为取消
	case TaskPedding:
		snapshot.State = TaskCanceled
		s.heap.Remove(taskID)
	case TaskRunning:
		// 任务正在运行中;向任务函数发送取消信号，并标记为取消中
		snapshot.State = TaskCanceling
		snapshot.cancel()
	}

	snapshot.Err = context.Canceled

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
	s.closed = true

	for i := 0; i < s.workerNum; i++ {
		s.closeNotice <- struct{}{}
	}
}

func (s *Scheduler) Wait() {
	s.workerWaitGroup.Wait()
}
