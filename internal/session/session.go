// Package session 管理会话历史、任务队列和执行状态，不依赖应用入口。
package session

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/whxcode/mini-gnet/internal/agent"
	"github.com/whxcode/mini-gnet/internal/query"
	tasks "github.com/whxcode/mini-gnet/internal/task"
	"github.com/whxcode/mini-gnet/views/conversation"
	"github.com/whxcode/mini-gnet/views/input"
	"github.com/whxcode/mini-gnet/views/message"
)

const (
	maxPendingTasks = 2
)

// Session 是一个打开的聊天窗口：保存聊天记录、输入草稿和这个窗口的任务状态。
// App.CurrentSession 决定看哪个窗口；切换窗口不会移动或重启任务。
type Session struct {
	runner  *agent.Agent         // 应用共用的执行服务，不依赖 App。
	Output  *agent.Output        // 本会话的输出记录，Agent 写入，界面读取。
	History *message.TempMessage // 模型上下文及其 JSONL 文件句柄，负责聊天记录的持久化。

	// 输入框只由界面操作，后台任务不修改用户正在编辑的文字。
	Draft        input.Composer // 未发送的输入，切换会话时保留文字和光标。
	ScrollOffset int            // 向上查看历史的行数；0 表示查看最新消息。

	// 以下字段会被后台任务写入、界面读取，读写时都要持有 mu。
	mu              sync.Mutex       // 防止界面读取到后台更新一半的状态；不锁项目文件。
	taskQueue       chan *tasks.Task // 本会话的任务队列，只由一个 Run 循环消费。
	workerDone      chan struct{}    // Run 退出后关闭，退出程序时等待它。
	closed          bool             // 关闭后拒绝入队，不再启动新任务。
	active          *tasks.Task      // 当前任务；nil 表示没有正在执行的任务。
	pending         []*tasks.Task    // 尚未开始的任务，用于显示和取消，包含已出队但尚未取得锁的任务。
	status          string           // 界面标签，例如“空闲”“执行中”“已完成”“失败”。
	title           string           // 会话列表中的标题，默认取第一条用户提问。
	hasUnreadResult bool             // 本轮已有结果但还没查看，用于后台完成提醒。
	updatedAt       time.Time        // 最近一次状态或文字更新的时间，用于会话列表。
}

// New 根据聊天记录创建一个窗口；加载历史只恢复显示，不执行历史任务。
func New(history *message.TempMessage, runner *agent.Agent) *Session {
	session := &Session{
		runner:     runner,
		History:    history,
		taskQueue:  make(chan *tasks.Task, maxPendingTasks),
		workerDone: make(chan struct{}),
		Draft:      input.NewComposer(),
		status:     "空闲",
		title:      "新会话",
		updatedAt:  time.Now(),
		Output:     agent.NewOutput(historyEvents(history)),
	}

	for _, record := range history.Records() {
		if title := record.SessionTitle(); title != "" {
			session.title = title
			break
		}
	}

	return session
}

// Enqueue 是管道等纯文本入口：解析显式引用后，交给结构化入队方法。
func (s *Session) Enqueue(parent context.Context, text string) (*tasks.Task, error) {
	input, err := query.Parse(text)
	if err != nil {
		return nil, err
	}
	return s.EnqueueInput(parent, input)
}

// EnqueueInput 校验并复制输入，只提交任务；引用内容由 Agent 在开始执行时读取。
func (s *Session) EnqueueInput(parent context.Context, input query.Input) (*tasks.Task, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	if err := parent.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, fmt.Errorf("当前会话已关闭")
	}

	if len(s.pending) >= cap(s.taskQueue) {
		return nil, fmt.Errorf("当前会话的等待队列已满（%d 项），请稍后再发送", cap(s.taskQueue))
	}

	job := tasks.NewInput(parent, input)

	// 使用非阻塞发送：队列满时保留输入，不能让主循环卡在这里。
	select {
	case s.taskQueue <- job:
		s.pending = append(s.pending, job)
		s.updatedAt = time.Now()

		if s.title == "新会话" {
			s.title = strings.Join(strings.Fields(input.Label()), " ")
		}

		return job, nil
	default:
		job.Cancel()

		return nil, fmt.Errorf("当前会话的等待队列已满（%d 项），请稍后再发送", maxPendingTasks)
	}
}

// Run 是会话唯一的消费循环：取一个任务、标记开始、执行到底，再取下一个。
// App.AttachSession 只启动一次；切换会话不启动新的消费协程。
func (s *Session) Run() {
	defer close(s.workerDone)

	for job := range s.taskQueue {
		// 与入队和停止共用一把锁，避免 /stop 漏掉刚出队、尚未开始的任务。
		s.mu.Lock()

		if index := slices.Index(s.pending, job); index >= 0 {
			s.pending = slices.Delete(s.pending, index, index+1)
		}

		if s.closed {
			job.Cancel()
		}

		if err := job.Context.Err(); err != nil {
			job.Complete(err)
			s.mu.Unlock()
			continue
		}

		s.active = job
		s.status = "执行中"
		s.Output.Reset()
		s.hasUnreadResult = false
		s.updatedAt = time.Now()
		s.mu.Unlock()

		// 不加 go：上一项真正结束后才开始下一项，同一会话的上下文不会交叉。
		err := s.runner.Run(job, s.History, s.Output)
		s.finishTask(job, err)
	}
}

// finishTask 在 Agent.Run 返回后顺序调用：保存最终状态，保留中断的文字，并通知等待者任务已结束。
// 所有内存状态和历史写入都结束后才关闭 done，避免程序退出时提前关闭文件。
func (s *Session) finishTask(job *tasks.Task, runErr error) {
	// 1. 写入错误已由 Agent.Run 直接返回；这里只补充任务取消的状态。
	if job.Context.Err() != nil {
		runErr = job.Context.Err()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 2. 保存未完成文字并清除流式预览。
	s.Output.Finish()
	s.active = nil

	// 3. 设置最终状态。此时后台任务已不再请求模型或执行工具。
	switch {
	case errors.Is(runErr, context.Canceled):
		s.status = "已停止"
		s.Output.Update(conversation.Event{
			Role: conversation.System,
			Text: "任务已停止。",
		})

	case runErr != nil:
		s.status = "失败"
		s.Output.Update(conversation.Event{
			Role: conversation.Error,
			Text: runErr.Error(),
		})

	default:
		s.status = "已完成"
	}

	// 4. 提醒有新结果并通知本项等待者；退出时还要等整个 Session.Run 结束。
	s.hasUnreadResult = true
	s.updatedAt = time.Now()
	job.Complete(runErr)
}

// Stop 取消当前任务和所有待执行任务；只影响本会话，之后仍可提交新任务。
func (s *Session) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.active != nil {
		s.status = "正在停止"
		s.active.Cancel()
	} else if len(s.pending) > 0 {
		s.status = "已停止"
	}

	count := len(s.pending)

	for _, job := range s.pending {
		job.Cancel()
	}

	// 立即腾空缓冲区。已被 Run 取走的任务由 Run 检查取消信号并结束，避免重复关闭 done。
drain:
	for {
		select {
		case job, ok := <-s.taskQueue:
			if !ok {
				break drain
			}

			job.Complete(context.Canceled)
		default:
			break drain
		}
	}
	s.pending = nil

	if count > 0 {
		s.Output.Update(conversation.Event{
			Role: conversation.System,
			Text: "已取消本会话的待执行任务。",
		})
	}

	s.updatedAt = time.Now()
}

// Shutdown 禁止继续提交，取消执行和等待项；不阻塞其他会话的关闭。
func (s *Session) Shutdown() {
	s.mu.Lock()

	if !s.closed {
		s.closed = true
		close(s.taskQueue)
	}

	s.mu.Unlock()
	s.Stop()
}

// Close 等待消费循环退出后关闭历史文件；调用前先 Shutdown。
func (s *Session) Close() error {
	<-s.workerDone

	return s.History.Close()
}
