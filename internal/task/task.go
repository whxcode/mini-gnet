// Package task 定义单次任务的输入、取消和完成通知，不依赖会话或 Agent。
package task

import (
	"context"
	"slices"
	"time"

	"github.com/whxcode/mini-gnet/internal/query"
)

// Task 只携带本项的数据，不复制配置或工具。Err 只能在 Done 关闭后读取。
type Task struct {
	Input   query.Input        // 提交时的独立片段副本，任务开始时才加载引用正文。
	Context context.Context    // 网络请求和工具读取的取消信号。
	cancel  context.CancelFunc // 本项独立的取消入口。
	Done    chan struct{}      // 完成通知，仅由本项的执行者关闭。
	Err     error              // 完成后的错误结果。
}

// New 创建纯文本任务；已解析引用的输入使用 NewInput。超时从提交开始计时。
func New(parent context.Context, text string) *Task {
	return NewInput(parent, query.Input{{Kind: query.Text, Value: text}})
}

// NewInput 复制提交内容并设置一小时超时，后续编辑草稿不会改变本任务。
func NewInput(parent context.Context, input query.Input) *Task {
	ctx, cancel := context.WithTimeout(parent, time.Hour)

	return &Task{
		Input:   slices.Clone(input),
		Context: ctx,
		cancel:  cancel,
		Done:    make(chan struct{}),
	}
}

// Cancel 请求停止本项，不等待网络或工具退出。
func (t *Task) Cancel() {
	t.cancel()
}

// Complete 由取得本项处理权的队列消费者调用一次，保存结果并通知所有等待者。
func (t *Task) Complete(err error) {
	t.Err = err
	t.cancel()
	close(t.Done)
}
