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
	// 任务完成等待组
	workerWaitGroup sync.WaitGroup
	// 是否已经停止接收新任务。
	closed bool
	// 使用原子操作
	// closeMutex sync.atomic
	// 任务状态 id: TaskState
	// taskSnapshot map[string]*TaskSnapshot
	taskSnapshot sync.Map //map[string]*TaskSnapshot
	// task
	taskNotice chan struct{}
	// 关闭通知
	closeNotice chan struct{}
	// 关闭worker通知
	closeWorkerMtx sync.Mutex
	// 关闭锁
	closeMtx sync.RWMutex
	// 最大 worker 数量
	maxWorkerNum int
}

func NewScheduler(workerNum int) *Scheduler {
	s := &Scheduler{
		workerNum:    workerNum,
		heap:         NewHeap(),
		taskSnapshot: sync.Map{}, // make(map[string]*TaskSnapshot),
		closed:       false,
		taskNotice:   make(chan struct{}, workerNum),
		closeNotice:  make(chan struct{}, 1),
		maxWorkerNum: workerNum,
	}

	s.workerWaitGroup.Add(workerNum)

	i := 0
	for range s.workerNum {
		go s.worker(i)
		i++
	}

	go func() {
		for {
			select {
			case <-time.NewTimer(time.Second * 2).C:
				s.closeWorkerMtx.Lock()
				fmt.Println("---------")
				fmt.Printf("worker_num[%d]\njobs__num[%d]\n", s.workerNum, s.heap.Len())
				fmt.Println("---------")
				s.closeWorkerMtx.Unlock()
			}
		}
	}()

	return s
}

func (s *Scheduler) a() {
	task, ok := s.heap.Pop()

	if !ok {
		return
	}

	// 修改对应任务的状态
	snapshotPtr, _ := s.taskSnapshot.Load(task.ID)
	snapshot, ok := snapshotPtr.(*TaskSnapshot)

	if snapshot == nil {
		// 如果任务快照不存在，
		// 出现了异常
		// 这里直接跳过，继续执行下一个任务
		return
		//continue
	}

	if task.Timeout == 0 {
		snapshot.ctx, snapshot.cancel = context.WithCancel(context.Background())
	} else {
		snapshot.ctx, snapshot.cancel = context.WithTimeout(context.Background(), task.Timeout)
	}

	snapshot.State = TaskRunning

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
}

func (s *Scheduler) worker(workerID int) {
	defer s.workerWaitGroup.Done()

	for {

		s.closeMtx.RLock()
		if s.closed {
			s.closeMtx.RUnlock()
			fmt.Println("关闭通知....", workerID)
			return
		}

		s.closeMtx.RUnlock()

		timer := time.NewTimer(time.Second * 2)

		select {
		case <-s.taskNotice:
			timer.Stop()
			s.a()
		case <-timer.C:
			s.closeWorkerMtx.Lock()
			if s.workerNum == 1 {
				s.closeWorkerMtx.Unlock()
				continue
			}

			s.workerNum--
			s.closeWorkerMtx.Unlock()
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
	if s.closed {
		return ErrClosed
	}

	for _, t := range task {
		if _, exists := s.taskSnapshot.Load(t.ID); exists {
			continue
		}

		s.taskSnapshot.Store(t.ID, NewTaskSnapshot(t.ID, TaskPedding))

		newT := t
		s.heap.Push(&newT)
		s.notice()
	}

	//  扩容
	s.closeWorkerMtx.Lock()
	defer s.closeWorkerMtx.Unlock()
	if s.heap.Len() > 5 && s.workerNum < s.maxWorkerNum {
		shirk := s.maxWorkerNum - s.workerNum
		// 固定扩容 6 worker
		s.workerNum = s.workerNum + shirk
		s.workerWaitGroup.Add(shirk)

		for i := 0; i < shirk; i++ {
			go s.worker(s.workerNum + i)
		}

		fmt.Println("扩容")
	}

	return nil
}

func (s *Scheduler) Cancel(taskID string) error {
	snapshotPtr, ok := s.taskSnapshot.Load(taskID)
	snapshot, ok := snapshotPtr.(*TaskSnapshot)

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
	if snapshot, ok := s.taskSnapshot.Load(taskID); ok {

		sc := *snapshot.(*TaskSnapshot)
		return &sc
	}

	return nil
}

func (s *Scheduler) Close() {
	// 只有外部一个调用协程调用;
	// 起内部 worker 只是只读

	s.closeMtx.Lock()
	s.closed = true
	s.closeMtx.Unlock()

	s.closeWorkerMtx.Lock()
	defer s.closeWorkerMtx.Unlock()

	for i := 0; i < s.workerNum; i++ {
		s.closeNotice <- struct{}{}
	}
}

func (s *Scheduler) Wait() {
	s.workerWaitGroup.Wait()
}
