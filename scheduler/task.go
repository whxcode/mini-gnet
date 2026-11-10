package scheduler

import (
	"context"
	"time"
)

type TaskState uint8

const (
	// 未执行
	TaskPedding TaskState = iota
	// 任务还未直接;直接取消
	TaskCanceled
	// 执行中
	TaskRunning
	// 任务执行成功
	TaskSucceeded
	// 任务执行失败
	TaskFailed
	// 已请求取消，函数尚未退出。
	TaskCanceling
)

func (s TaskState) String() string {
	switch s {
	case TaskPedding:
		return "Pedding"
	case TaskRunning:
		return "Running"
	case TaskSucceeded:
		return "Succeeded"
	case TaskFailed:
		return "Failed"
	case TaskCanceled:
		return "Canceled"
	case TaskCanceling:
		return "Canceling"
	default:
		return "Unknown"
	}
}

type TaskFunc func(ctx context.Context) (string, error)

type Task struct {
	// 任务 ID
	ID string
	// 任务函数
	Func TaskFunc
	// 任务超时间
	Timeout time.Duration
}

func NewTask(id string, f TaskFunc) Task {
	return Task{
		ID:      id,
		Func:    f,
		Timeout: 0,
	}
}

func NewTaskWithTimeout(
	id string,
	f TaskFunc,
	timeout time.Duration,
) Task {
	return Task{
		ID:      id,
		Func:    f,
		Timeout: timeout,
	}
}

type TaskSnapshot struct {
	// 任务 ID
	ID string
	// 任务状态
	State TaskState
	// 任务结果
	Result any
	// 错误
	Err    error
	ctx    context.Context
	cancel context.CancelFunc
}

func NewTaskSnapshot(id string, state TaskState) *TaskSnapshot {
	return &TaskSnapshot{
		ID:    id,
		State: state,
	}
}
